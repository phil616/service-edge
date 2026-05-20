package agent

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/dreamreflex/service-edge/internal/frp"
	"github.com/dreamreflex/service-edge/internal/protocol"
)

const processSettleWait = 5 * time.Second

// Applier applies config bundles to disk and reloads the frp process, rolling
// back on any failure.
type Applier struct {
	paths   frp.DeployPaths
	binary  string
	unit    string
	systemd frp.Systemd
}

func NewApplier(cfg *Config) *Applier {
	return &Applier{
		paths:  cfg.Paths(),
		binary: cfg.FrpBinaryPath,
		unit:   cfg.ServiceUnit(),
	}
}

// Apply implements the config-application flow from the design: stage to .new,
// verify, back up, atomic swap, reload, settle-check, rollback on failure.
func (a *Applier) Apply(bundle *protocol.ConfigResponse) error {
	p := a.paths

	if err := ensureDirs(p); err != nil {
		return err
	}

	// 1-2. Stage config + certs to .new files (don't touch live files yet).
	stage := map[string]string{
		p.NewConfig:        bundle.FrpConfig,
		p.CertFile + ".new": bundle.TLSCert,
		p.KeyFile + ".new":  bundle.TLSKey,
		p.CAFile + ".new":   bundle.CACert,
	}
	for path, content := range stage {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			a.cleanupStaged()
			return fmt.Errorf("stage %s: %w", path, err)
		}
	}

	// 3. Verify staged config syntax.
	if err := frp.VerifyConfig(a.binary, p.NewConfig); err != nil {
		a.cleanupStaged()
		return fmt.Errorf("verify: %w", err)
	}

	// 4. Back up current live files (best effort; absent on first apply).
	backups := map[string]string{
		p.ConfigFile: p.ConfigFile + ".backup",
		p.CertFile:   p.CertFile + ".backup",
		p.KeyFile:    p.KeyFile + ".backup",
		p.CAFile:     p.CAFile + ".backup",
	}
	for live, bak := range backups {
		copyIfExists(live, bak)
	}

	// 5. Atomic swap .new -> live.
	swaps := [][2]string{
		{p.NewConfig, p.ConfigFile},
		{p.CertFile + ".new", p.CertFile},
		{p.KeyFile + ".new", p.KeyFile},
		{p.CAFile + ".new", p.CAFile},
	}
	for _, sw := range swaps {
		if err := os.Rename(sw[0], sw[1]); err != nil {
			a.rollback(backups)
			return fmt.Errorf("swap %s: %w", sw[0], err)
		}
	}

	// 6. Reload (SIGHUP) or restart.
	restarted, err := a.systemd.ReloadOrRestart(a.unit)
	if err != nil {
		a.rollback(backups)
		a.systemd.Restart(a.unit)
		return fmt.Errorf("reload/restart: %w", err)
	}

	// 7. Settle check.
	if !a.systemd.WaitActive(a.unit, processSettleWait) {
		slog.Warn("frp not active after apply, rolling back", "unit", a.unit, "restarted", restarted)
		a.rollback(backups)
		a.systemd.Restart(a.unit)
		return fmt.Errorf("frp process not active after apply")
	}

	return nil
}

func (a *Applier) rollback(backups map[string]string) {
	for live, bak := range backups {
		if _, err := os.Stat(bak); err == nil {
			_ = os.Rename(bak, live)
		}
	}
}

func (a *Applier) cleanupStaged() {
	p := a.paths
	for _, f := range []string{p.NewConfig, p.CertFile + ".new", p.KeyFile + ".new", p.CAFile + ".new"} {
		_ = os.Remove(f)
	}
}

func ensureDirs(p frp.DeployPaths) error {
	for _, d := range []string{p.ConfigDir, p.DataDir, p.LogDir} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			return err
		}
	}
	return nil
}

func copyIfExists(src, dst string) {
	data, err := os.ReadFile(src)
	if err != nil {
		return
	}
	_ = os.WriteFile(dst, data, 0o600)
}
