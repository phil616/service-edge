package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/dreamreflex/service-edge/internal/frp"
	"github.com/dreamreflex/service-edge/internal/protocol"
)

type processManager interface {
	Enable(string) error
	Disable(string) error
	Stop(string) error
	Restart(string) error
	IsActive(string) bool
	MainPID(string) int
	ProcessStatus(context.Context, string) (bool, int, error)
}

func frpcUnit(uuid string) string { return fmt.Sprintf("%s@%s", frp.FRPCSystemdUnit, uuid) }

// The binary and CA participate in every instance's applied identity, so shared
// binary upgrades and interrupted partial upgrades cannot skip old processes.
func connectionFingerprint(conn protocol.ConnectionConfig, ca, binaryVersion string) string {
	// A revision-only change (such as a display name) does not alter the process.
	conn.ConfigVersion = 0
	data, _ := json.Marshal(struct {
		Connection protocol.ConnectionConfig
		CA, Binary string
	}{conn, ca, strings.TrimPrefix(binaryVersion, "v")})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (r *Runner) reconcile(ctx context.Context, bundle *protocol.HostConfigResponse) bool {
	r.operationMu.Lock()
	defer r.operationMu.Unlock()
	// Validate the entire desired set before touching files or processes.
	desired := map[string]bool{}
	for _, conn := range bundle.Connections {
		if _, err := frp.FRPCInstanceDir(frp.FRPCBaseDir, conn.UUID); err != nil {
			r.ack(ctx, bundle.ConfigVersion, false, err.Error())
			return false
		}
		if desired[conn.UUID] {
			r.ack(ctx, bundle.ConfigVersion, false, "duplicate connection identifier")
			return false
		}
		desired[conn.UUID] = true
	}
	if bundle.FrpBinary.DownloadURL != "" {
		if err := frp.EnsureBinary(r.cfg.FrpBinaryPath, bundle.FrpBinary.DownloadURL, bundle.FrpBinary.Version, bundle.FrpBinary.SHA256); err != nil {
			r.ack(ctx, bundle.ConfigVersion, false, "binary install: "+err.Error())
			return false
		}
	}
	var errs []string
	applied := r.state.ConnEntries()
	for _, conn := range bundle.Connections {
		fingerprint := connectionFingerprint(conn, bundle.CACert, bundle.FrpBinary.Version)
		if st, ok := applied[conn.UUID]; ok && st.Fingerprint == fingerprint && connectionFilesExist(conn.UUID) {
			if st.Version != conn.ConfigVersion {
				if err := r.state.SetConn(conn.UUID, conn.ConfigVersion, conn.AdminPort, fingerprint); err != nil {
					errs = append(errs, conn.UUID+": persist: "+err.Error())
				}
			}
			continue
		}
		if err := r.applyConnection(conn, bundle.CACert); err != nil {
			errs = append(errs, conn.UUID+": "+err.Error())
			continue
		}
		if err := r.state.SetConn(conn.UUID, conn.ConfigVersion, conn.AdminPort, fingerprint); err != nil {
			errs = append(errs, conn.UUID+": persist: "+err.Error())
		}
	}
	for _, uuid := range r.state.ConnUUIDs() {
		if desired[uuid] {
			continue
		}
		if err := r.stopConnection(uuid); err != nil {
			errs = append(errs, uuid+": remove: "+err.Error())
			continue
		}
		if err := r.state.RemoveConn(uuid); err != nil {
			errs = append(errs, uuid+": persist removal: "+err.Error())
		}
	}
	if len(errs) == 0 {
		if err := r.state.SaveHost(bundle.ConfigVersion); err != nil {
			errs = append(errs, "persist host: "+err.Error())
		}
	}
	ok := len(errs) == 0
	if !ok {
		slog.Error("host config incomplete", "errors", errs)
	}
	r.ack(ctx, bundle.ConfigVersion, ok, strings.Join(errs, "; "))
	r.scheduleStatusReport(ctx)
	return ok
}

func connectionFilesExist(uuid string) bool {
	p := frp.FRPCPaths(uuid)
	for _, path := range []string{p.ConfigFile, p.CertFile, p.KeyFile, p.CAFile} {
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
			return false
		}
	}
	return true
}

// applyConnection writes one connection's config/certs and (re)starts its frpc
// process, reusing the per-instance applier (atomic apply with rollback).
func (r *Runner) applyConnection(conn protocol.ConnectionConfig, caCert string) error {
	applier := NewConnectionApplier(conn.UUID, r.cfg.FrpBinaryPath)
	cr := &protocol.ConfigResponse{
		ConfigVersion: conn.ConfigVersion,
		FrpConfig:     conn.FrpConfig,
		TLSCert:       conn.TLSCert,
		TLSKey:        conn.TLSKey,
		CACert:        caCert,
	}
	if err := applier.Apply(cr); err != nil {
		return err
	}
	// Treat failure to persist across reboots as an incomplete apply.
	if err := r.systemd.Enable(frpcUnit(conn.UUID)); err != nil {
		return fmt.Errorf("enable connection unit: %w", err)
	}
	return nil
}

func (r *Runner) stopConnection(uuid string) error {
	return removeConnection(r.systemd, frp.FRPCBaseDir, uuid)
}

// Only remove this instance after systemd confirms stop and disable succeeded.
// baseDir is explicit so tests exercise the actual deletion in a temporary tree.
func removeConnection(manager processManager, baseDir, uuid string) error {
	dir, err := frp.FRPCInstanceDir(baseDir, uuid)
	if err != nil {
		return err
	}
	unit := frpcUnit(uuid)
	if err := manager.Stop(unit); err != nil {
		return err
	}
	if err := manager.Disable(unit); err != nil {
		return err
	}
	return os.RemoveAll(dir)
}
