package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dreamreflex/service-edge/internal/frp"
	"github.com/dreamreflex/service-edge/internal/protocol"
)

// frpcUnit returns the templated systemd unit for one frpc connection.
func frpcUnit(connUUID string) string {
	return fmt.Sprintf("%s@%s", frp.FRPCSystemdUnit, connUUID)
}

// reconcile brings the host's running frpc processes in line with the desired set
// of connections from the control plane: it (re)applies new/changed connections,
// stops connections that were removed, and reports the result. It returns true
// only when every connection applied cleanly; the caller backs off and retries on
// false.
func (r *Runner) reconcile(ctx context.Context, bundle *protocol.HostConfigResponse) bool {
	slog.Info("host config received", "version", bundle.ConfigVersion, "connections", len(bundle.Connections))

	// The frpc binary is shared by all connections on the host; install once.
	if bundle.FrpBinary.DownloadURL != "" {
		if err := frp.EnsureBinary(r.cfg.FrpBinaryPath, bundle.FrpBinary.DownloadURL, bundle.FrpBinary.Version, bundle.FrpBinary.SHA256); err != nil {
			slog.Error("frpc binary install failed", "err", err)
			r.ack(ctx, bundle.ConfigVersion, false, "binary install: "+err.Error())
			return false
		}
	}

	desired := map[string]bool{}
	var errs []string
	for _, conn := range bundle.Connections {
		desired[conn.UUID] = true
		if r.state.HasConn(conn.UUID) && r.state.ConnVersion(conn.UUID) >= conn.ConfigVersion {
			continue // already up to date
		}
		if err := r.applyConnection(conn, bundle.CACert); err != nil {
			slog.Error("connection apply failed", "conn", conn.UUID, "err", err)
			errs = append(errs, conn.UUID[:8]+": "+err.Error())
			continue
		}
		r.state.SetConn(conn.UUID, conn.ConfigVersion, conn.AdminPort)
		slog.Info("connection applied", "conn", conn.UUID, "version", conn.ConfigVersion)
	}

	// Stop and clean up connections that are no longer assigned to this host.
	for _, uuid := range r.state.ConnUUIDs() {
		if desired[uuid] {
			continue
		}
		r.stopConnection(uuid)
		r.state.RemoveConn(uuid)
		slog.Info("connection removed", "conn", uuid)
	}

	ok := len(errs) == 0
	// Only advance the host's aggregate version when every connection applied. On
	// partial failure we keep the old version so the next long-poll re-delivers the
	// bundle and the failed connections are retried (succeeded ones are skipped via
	// their per-connection version); the caller backs off between attempts.
	if ok {
		if err := r.state.SaveHost(bundle.ConfigVersion); err != nil {
			slog.Warn("failed to persist host state", "err", err)
		}
	}
	r.ack(ctx, bundle.ConfigVersion, ok, strings.Join(errs, "; "))
	r.scheduleStatusReport(ctx)
	return ok
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
	// Persist the unit across reboots (best effort).
	if err := r.systemd.Enable(frpcUnit(conn.UUID)); err != nil {
		slog.Debug("enable connection unit failed (continuing)", "conn", conn.UUID, "err", err)
	}
	return nil
}

// stopConnection stops, disables and removes one frpc connection's instance.
func (r *Runner) stopConnection(connUUID string) {
	unit := frpcUnit(connUUID)
	_ = r.systemd.Stop(unit)
	_ = r.systemd.Disable(unit)
	// Remove the per-instance directory tree.
	inst := filepath.Dir(filepath.Dir(frp.FRPCPaths(connUUID).ConfigDir)) // .../instances/<uuid>
	if err := os.RemoveAll(inst); err != nil {
		slog.Debug("remove instance dir failed", "conn", connUUID, "err", err)
	}
}

// reportHostStatus reports host facts plus per-connection frp status (queried from
// each connection's localhost admin API).
func (r *Runner) reportHostStatus(ctx context.Context) {
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var conns []protocol.ConnectionStatus
	for uuid, st := range r.state.ConnEntries() {
		unit := frpcUnit(uuid)
		conns = append(conns, protocol.ConnectionStatus{
			UUID:          uuid,
			ProcessAlive:  r.systemd.IsActive(unit),
			ProcessPID:    r.systemd.MainPID(unit),
			ProxyStatuses: r.queryProxyStatusesFor(cctx, uuid, st.AdminPort),
		})
	}

	req := protocol.StatusRequest{
		ConfigVersion:  r.state.Version(),
		ProcessAlive:   true, // the agent itself is alive
		FrpVersion:     frp.FrpVersion(r.cfg.FrpBinaryPath),
		SystemInfo:     collectSystemInfo(),
		ListeningPorts: collectListeningPorts(),
		Connections:    conns,
	}
	if err := r.client.ReportStatus(cctx, req); err != nil {
		slog.Debug("host status report failed", "err", err)
	}
}
