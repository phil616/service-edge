package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dreamreflex/service-edge/internal/frp"
	"github.com/dreamreflex/service-edge/internal/protocol"
)

type processManager interface {
	deploymentProcess
	Disable(string) error
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
	if bundle == nil || bundle.ConfigVersion <= 0 || bundle.Connections == nil || bundle.Decommission {
		return false
	}
	desired := map[string]bool{}
	for _, conn := range bundle.Connections {
		if _, err := frp.FRPCInstanceDir(r.connectionBase(), conn.UUID); err != nil {
			r.ack(ctx, bundle.ConfigVersion, false, err.Error())
			return false
		}
		if desired[conn.UUID] {
			r.ack(ctx, bundle.ConfigVersion, false, "duplicate connection identifier")
			return false
		}
		desired[conn.UUID] = true
	}
	var errs []string
	known, err := r.managedConnections()
	if err != nil {
		r.ack(ctx, bundle.ConfigVersion, false, err.Error())
		return false
	}
	for _, uuid := range known {
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
	binary := r.cfg.FrpBinaryPath
	if len(bundle.Connections) > 0 {
		var err error
		binary, err = frp.PrepareBinary(ctx, r.cfg.FrpBinaryPath, bundle.FrpBinary.DownloadURL, bundle.FrpBinary.Version, bundle.FrpBinary.SHA256)
		if err != nil {
			slog.Error("prepare frp binary failed", "version", bundle.FrpBinary.Version, "err", err)
			for _, conn := range bundle.Connections {
				if persistErr := r.state.SetConnFailure(conn.UUID, conn.AdminPort, "prepare binary: "+err.Error()); persistErr != nil {
					slog.Error("persist connection failure", "uuid", conn.UUID, "error", persistErr)
				}
			}
			r.scheduleStatusReport(ctx)
			r.ack(ctx, bundle.ConfigVersion, false, "prepare binary: "+err.Error())
			return false
		}
	}
	applied := r.state.ConnEntries()
	for _, conn := range bundle.Connections {
		if conn.ConfigError != "" {
			errs = append(errs, conn.UUID+": "+conn.ConfigError)
			if err := r.state.SetConnFailure(conn.UUID, conn.AdminPort, conn.ConfigError); err != nil {
				errs = append(errs, err.Error())
			}
			continue
		}
		fingerprint := connectionFingerprint(conn, bundle.CACert, bundle.FrpBinary.Version)
		if st, ok := applied[conn.UUID]; ok && st.Fingerprint == fingerprint && r.connectionFilesExist(conn.UUID) && r.processNotStopped(ctx, frpcUnit(conn.UUID)) {
			if st.Version != conn.ConfigVersion || st.LastApplyError != "" {
				if err := r.state.SetConn(conn.UUID, conn.ConfigVersion, conn.AdminPort, fingerprint, bundle.FrpBinary.Version); err != nil {
					errs = append(errs, conn.UUID+": persist: "+err.Error())
				}
			}
			continue
		}
		if err := r.applyConnection(conn, bundle.CACert, binary); err != nil {
			errs = append(errs, conn.UUID+": "+err.Error())
			if persistErr := r.state.SetConnFailure(conn.UUID, conn.AdminPort, err.Error()); persistErr != nil {
				errs = append(errs, conn.UUID+": persist failure: "+persistErr.Error())
			}
			continue
		}
		if err := r.state.SetConn(conn.UUID, conn.ConfigVersion, conn.AdminPort, fingerprint, bundle.FrpBinary.Version); err != nil {
			errs = append(errs, conn.UUID+": persist: "+err.Error())
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

func (r *Runner) connectionFilesExist(uuid string) bool {
	p := frp.FRPCPathsAt(r.connectionBase(), uuid)
	for _, name := range []string{filepath.Base(p.ConfigFile), filepath.Base(p.CertFile), filepath.Base(p.KeyFile), filepath.Base(p.CAFile), "frpc", "activated"} {
		if info, err := os.Stat(filepath.Join(p.ConfigDir, "current", name)); err != nil || !info.Mode().IsRegular() {
			return false
		}
	}
	return true
}

// applyConnection writes one connection's config/certs and (re)starts its frpc
// process, reusing the per-instance applier (atomic apply with rollback).
func (r *Runner) applyConnection(conn protocol.ConnectionConfig, caCert, binary string) error {
	applier := NewConnectionApplier(conn.UUID, binary)
	applier.paths = frp.FRPCPathsAt(r.connectionBase(), conn.UUID)
	applier.systemd = r.systemd
	cr := &protocol.ConfigResponse{
		ConfigVersion: conn.ConfigVersion,
		FrpConfig:     strings.ReplaceAll(conn.FrpConfig, frp.FRPCBaseDir+"/", r.connectionBase()+"/"),
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
	return removeConnection(r.systemd, r.connectionBase(), uuid)
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

func (r *Runner) processNotStopped(ctx context.Context, unit string) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	active, _, err := r.systemd.ProcessStatus(ctx, unit)
	// An unavailable observation is not a reason to restart a working tunnel.
	return err != nil || active
}

func (r *Runner) connectionBase() string {
	if r.instanceBase != "" {
		return r.instanceBase
	}
	return frp.FRPCBaseDir
}

// Inventory is not state.json: a crash after activating a unit but before saving
// state must not turn that unit into an unmanaged, permanently running tunnel.
func (r *Runner) managedConnections() ([]string, error) {
	known := map[string]bool{}
	for _, uuid := range r.state.ConnUUIDs() {
		known[uuid] = true
	}
	entries, err := os.ReadDir(filepath.Join(r.connectionBase(), "instances"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		} // Never follow a symlink outside our inventory.
		if _, err := frp.FRPCInstanceDir(r.connectionBase(), entry.Name()); err != nil {
			return nil, err
		}
		known[entry.Name()] = true
	}
	out := make([]string, 0, len(known))
	for uuid := range known {
		out = append(out, uuid)
	}
	return out, nil
}

func (r *Runner) decommission(ctx context.Context, version int) bool {
	r.operationMu.Lock()
	defer r.operationMu.Unlock()
	var err error
	if r.cfg.AgentType == "frpc" {
		var known []string
		known, err = r.managedConnections()
		if err == nil {
			for _, uuid := range known {
				if err = r.stopConnection(uuid); err != nil {
					break
				}
				if err = r.state.RemoveConn(uuid); err != nil {
					break
				}
			}
		}
	} else {
		err = r.systemd.Stop(r.unit)
		if err == nil {
			err = r.systemd.Disable(r.unit)
		}
	}
	if err == nil {
		err = r.state.Save(version, "")
	}
	if err != nil {
		r.ack(ctx, version, false, err.Error())
		return false
	}
	r.ack(ctx, version, true, "")
	return true
}
