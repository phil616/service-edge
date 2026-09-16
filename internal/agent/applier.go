package agent

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dreamreflex/service-edge/internal/frp"
	"github.com/dreamreflex/service-edge/internal/protocol"
)

const processSettleWait = 5 * time.Second

type deploymentProcess interface {
	Configure(unit, binary, config string) error
	Restart(string) error
	Enable(string) error
	Stop(string) error
	WaitActive(string, time.Duration) bool
}

// Applier switches one immutable configuration/certificate/binary generation.
// The Agent config stays outside current, so applying FRP cannot replace it.
type Applier struct {
	paths        frp.DeployPaths
	binary, unit string
	systemd      deploymentProcess
}

func NewApplier(cfg *Config) *Applier {
	return &Applier{paths: cfg.Paths(), binary: cfg.FrpBinaryPath, unit: cfg.ServiceUnit(), systemd: frp.Systemd{}}
}
func NewConnectionApplier(uuid, binary string) *Applier {
	return &Applier{paths: frp.FRPCPaths(uuid), binary: binary, unit: frpcUnit(uuid), systemd: frp.Systemd{}}
}

func (a *Applier) Apply(bundle *protocol.ConfigResponse) error {
	if err := validateBundleTLS(bundle); err != nil {
		return fmt.Errorf("certificate validation: %w", err)
	}
	p := a.paths
	for _, dir := range []string{p.ConfigDir, p.DataDir, p.LogDir} {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return err
		}
	}
	current := filepath.Join(p.ConfigDir, "current")
	if err := a.recoverPending(current); err != nil {
		return fmt.Errorf("recover interrupted deployment: %w", err)
	}
	binaryVersion := frp.FrpVersion(a.binary)
	if binaryVersion == "unknown" {
		return fmt.Errorf("candidate frp binary cannot run")
	}
	identity, _ := json.Marshal([]string{bundle.FrpConfig, bundle.TLSCert, bundle.TLSKey, bundle.CACert, binaryVersion})
	sum := sha256.Sum256(identity)
	fingerprint := hex.EncodeToString(sum[:])
	manifest := filepath.Join(current, "fingerprint")
	if b, err := os.ReadFile(manifest); err == nil && string(b) == fingerprint && a.generationComplete(current) {
		binaryName := strings.TrimSuffix(filepath.Base(p.ConfigFile), ".toml")
		if err := a.systemd.Configure(a.unit, filepath.Join(current, binaryName), filepath.Join(current, filepath.Base(p.ConfigFile))); err != nil {
			return err
		}
		if err := a.systemd.Enable(a.unit); err != nil {
			return err
		}
		if !a.systemd.WaitActive(a.unit, 2*time.Second) {
			if err := a.systemd.Restart(a.unit); err != nil {
				return err
			}
			if !a.systemd.WaitActive(a.unit, processSettleWait) {
				return fmt.Errorf("existing deployment failed to restart")
			}
		}
		return nil
	}
	revisions := filepath.Join(p.DataDir, "revisions")
	if err := os.MkdirAll(revisions, 0700); err != nil {
		return err
	}
	generation, err := os.MkdirTemp(revisions, "revision-")
	if err != nil {
		return err
	}
	retain := false
	defer func() {
		if !retain {
			_ = os.RemoveAll(generation)
		}
	}()
	configName := filepath.Base(p.ConfigFile)
	binaryName := strings.TrimSuffix(configName, ".toml")
	config := rewriteCertificatePaths(bundle.FrpConfig, p, generation)
	for name, content := range map[string]string{configName: config, filepath.Base(p.CertFile): bundle.TLSCert, filepath.Base(p.KeyFile): bundle.TLSKey, filepath.Base(p.CAFile): bundle.CACert, "fingerprint": fingerprint} {
		if err := os.WriteFile(filepath.Join(generation, name), []byte(content), 0600); err != nil {
			return err
		}
	}
	if err := copyFile(a.binary, filepath.Join(generation, binaryName), 0755); err != nil {
		return err
	}
	if err := frp.VerifyConfig(filepath.Join(generation, binaryName), filepath.Join(generation, configName)); err != nil {
		return fmt.Errorf("verify candidate: %w", err)
	}
	// Preserve a legacy deployment on first migration. Shared binaries are never
	// overwritten by PrepareBinary, so this snapshot keeps the original executable.
	previous, err := os.Readlink(current)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if previous == "" {
		previous, err = a.snapshotLegacy(revisions, binaryName)
		if err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(generation, "previous"), []byte(previous), 0600); err != nil {
		return err
	}
	if err := a.systemd.Configure(a.unit, filepath.Join(current, binaryName), filepath.Join(current, configName)); err != nil {
		return fmt.Errorf("configure unit: %w", err)
	}
	if err := switchGeneration(current, generation); err != nil {
		return err
	}
	rollback := func(cause error) error {
		var restoreErr error
		if previous != "" {
			restoreErr = switchGeneration(current, previous)
		} else {
			restoreErr = os.Remove(current)
		}
		if restoreErr != nil {
			retain = true
			return errors.Join(cause, fmt.Errorf("restore generation: %w", restoreErr))
		}
		if previous != "" {
			restoreErr = a.systemd.Restart(a.unit)
		} else {
			restoreErr = a.systemd.Stop(a.unit)
		}
		if restoreErr != nil {
			return errors.Join(cause, fmt.Errorf("restore process: %w", restoreErr))
		}
		return cause
	}
	if err := a.systemd.Enable(a.unit); err != nil {
		return rollback(fmt.Errorf("enable unit: %w", err))
	}
	if err := a.systemd.Restart(a.unit); err != nil {
		return rollback(fmt.Errorf("restart: %w", err))
	}
	if !a.systemd.WaitActive(a.unit, processSettleWait) {
		return rollback(fmt.Errorf("frp process did not remain active; inspect %s", p.LogFile))
	}
	if err := os.WriteFile(filepath.Join(generation, "activated"), []byte("applied\n"), 0600); err != nil {
		return rollback(err)
	}
	retain = true
	pruneGenerations(revisions, generation, previous)
	return nil
}

func (a *Applier) generationComplete(dir string) bool {
	names := []string{filepath.Base(a.paths.ConfigFile), filepath.Base(a.paths.CertFile), filepath.Base(a.paths.KeyFile), filepath.Base(a.paths.CAFile), strings.TrimSuffix(filepath.Base(a.paths.ConfigFile), ".toml"), "activated"}
	for _, n := range names {
		info, err := os.Stat(filepath.Join(dir, n))
		if err != nil || !info.Mode().IsRegular() {
			return false
		}
	}
	return true
}
func rewriteCertificatePaths(config string, p frp.DeployPaths, dir string) string {
	return strings.NewReplacer(p.CertFile, filepath.Join(dir, filepath.Base(p.CertFile)), p.KeyFile, filepath.Join(dir, filepath.Base(p.KeyFile)), p.CAFile, filepath.Join(dir, filepath.Base(p.CAFile))).Replace(config)
}
func validateBundleTLS(b *protocol.ConfigResponse) error {
	pair, err := tls.X509KeyPair([]byte(b.TLSCert), []byte(b.TLSKey))
	if err != nil {
		return err
	}
	cert, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(b.CACert)) {
		return fmt.Errorf("invalid CA certificate")
	}
	intermediates := x509.NewCertPool()
	for _, raw := range pair.Certificate[1:] {
		c, e := x509.ParseCertificate(raw)
		if e != nil {
			return e
		}
		intermediates.AddCert(c)
	}
	_, err = cert.Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}})
	return err
}
func switchGeneration(current, target string) error {
	tmp := current + ".next"
	if err := os.Remove(tmp); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, current); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	syncErr := out.Sync()
	closeErr := out.Close()
	return errors.Join(copyErr, syncErr, closeErr)
}
func (a *Applier) snapshotLegacy(revisions, binaryName string) (string, error) {
	p := a.paths
	if _, err := os.Stat(p.ConfigFile); os.IsNotExist(err) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	legacy, err := os.MkdirTemp(revisions, "legacy-")
	if err != nil {
		return "", err
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.RemoveAll(legacy)
		}
	}()
	for _, src := range []string{p.ConfigFile, p.CertFile, p.KeyFile, p.CAFile} {
		if err := copyFile(src, filepath.Join(legacy, filepath.Base(src)), 0600); err != nil {
			if os.IsNotExist(err) {
				slog.Warn("legacy deployment incomplete; no rollback snapshot", "file", src)
				return "", nil
			}
			return "", fmt.Errorf("backup legacy configuration: %w", err)
		}
	}
	// The legacy executable is at the stable installation path, not the candidate cache.
	oldBinary := filepath.Join(filepath.Dir(p.ConfigDir), "bin", binaryName)
	if binaryName == "frpc" {
		oldBinary = filepath.Join(frp.FRPCBaseDir, "bin", binaryName)
	}
	if err := copyFile(oldBinary, filepath.Join(legacy, binaryName), 0755); err != nil {
		if os.IsNotExist(err) {
			slog.Warn("legacy binary missing; no rollback snapshot", "file", oldBinary)
			return "", nil
		}
		return "", fmt.Errorf("backup legacy binary: %w", err)
	}
	cfg, err := os.ReadFile(p.ConfigFile)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(legacy, filepath.Base(p.ConfigFile)), []byte(rewriteCertificatePaths(string(cfg), p, legacy)), 0600); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(legacy, "activated"), []byte("legacy\n"), 0600); err != nil {
		return "", err
	}
	ok = true
	return legacy, nil
}

// An activation record is written only after the replacement process settles.
// A crash between pointer swap and activation rolls back before another attempt.
func (a *Applier) recoverPending(current string) error {
	target, err := os.Readlink(current)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(target, "activated")); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	previous, err := os.ReadFile(filepath.Join(target, "previous"))
	if err != nil {
		return err
	}
	if len(previous) > 0 {
		if err := switchGeneration(current, string(previous)); err != nil {
			return err
		}
		return a.systemd.Restart(a.unit)
	}
	if err := a.systemd.Stop(a.unit); err != nil {
		return err
	}
	return os.Remove(current)
}

func pruneGenerations(root, current, previous string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		if entry.IsDir() && path != current && path != previous && (strings.HasPrefix(entry.Name(), "revision-") || strings.HasPrefix(entry.Name(), "legacy-")) {
			_ = os.RemoveAll(path)
		}
	}
}
