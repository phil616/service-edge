package agent

import (
	"context"
	"log/slog"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/dreamreflex/service-edge/internal/frp"
	"github.com/dreamreflex/service-edge/internal/protocol"
)

// Runner is the agent's top-level coordinator. An frps agent manages a single frp
// process; an frpc agent is a HOST that reconciles many frpc connection processes.
type Runner struct {
	cfg     *Config
	client  *Client
	state   *State
	systemd frp.Systemd

	// frps-only: the single managed unit + its applier.
	applier *Applier
	unit    string
}

func NewRunner(cfg *Config) *Runner {
	r := &Runner{cfg: cfg, client: NewClient(cfg)}
	if cfg.AgentType == "frps" {
		r.state = LoadState(filepath.Join(cfg.Paths().DataDir, "state.json"))
		r.applier = NewApplier(cfg)
		r.unit = cfg.ServiceUnit()
	} else {
		// frpc host: state lives at the host data dir; instances are per-connection.
		r.state = LoadState(filepath.Join(frp.FRPCBaseDir, "data", "state.json"))
	}
	return r
}

// Run starts all loops and blocks until ctx is cancelled.
func (r *Runner) Run(ctx context.Context) {
	slog.Info("agent starting", "type", r.cfg.AgentType, "uuid", r.cfg.UUID)
	if r.cfg.AgentType == "frps" {
		if err := r.systemd.Enable(r.unit); err != nil {
			slog.Debug("enable frp unit failed (continuing)", "unit", r.unit, "err", err)
		}
	}
	var wg sync.WaitGroup
	for _, loop := range []func(context.Context){
		r.heartbeatLoop,
		r.statusLoop,
		r.configSyncLoop,
		r.watchdogLoop,
	} {
		wg.Add(1)
		go func(f func(context.Context)) {
			defer wg.Done()
			f(ctx)
		}(loop)
	}
	wg.Wait()
	slog.Info("agent stopped")
}

// heartbeatAlive reports whether the managed frp is alive. For an frpc host the
// agent process itself being alive is the liveness signal (per-connection state
// is reported in status); for frps it's the single unit's activity.
func (r *Runner) heartbeatAlive() bool {
	if r.cfg.AgentType == "frps" {
		return r.systemd.IsActive(r.unit)
	}
	return true
}

// heartbeatLoop pings liveness frequently; failures are logged, never fatal.
func (r *Runner) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(r.cfg.HeartbeatInterval.Std())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			if err := r.client.Heartbeat(cctx, r.state.Version(), r.heartbeatAlive()); err != nil {
				slog.Debug("heartbeat failed", "err", err)
			}
			cancel()
		}
	}
}

// statusLoop reports detailed status at a lower frequency.
func (r *Runner) statusLoop(ctx context.Context) {
	ticker := time.NewTicker(r.cfg.StatusReportInterval.Std())
	defer ticker.Stop()
	r.reportStatus(ctx) // initial report on startup
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.reportStatus(ctx)
		}
	}
}

// reportStatus dispatches to the frps single-process or frpc host reporter.
func (r *Runner) reportStatus(ctx context.Context) {
	if r.cfg.AgentType == "frpc" {
		r.reportHostStatus(ctx)
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req := protocol.StatusRequest{
		ConfigVersion:  r.state.Version(),
		ProcessAlive:   r.systemd.IsActive(r.unit),
		ProcessPID:     r.systemd.MainPID(r.unit),
		FrpVersion:     frp.FrpVersion(r.cfg.FrpBinaryPath),
		SystemInfo:     collectSystemInfo(),
		ListeningPorts: collectListeningPorts(),
	}
	if err := r.client.ReportStatus(cctx, req); err != nil {
		slog.Debug("status report failed", "err", err)
	}
}

// configSyncLoop long-polls for config and applies updates. Network errors back
// off exponentially up to 60s; 304 timeouts re-poll immediately.
func (r *Runner) configSyncLoop(ctx context.Context) {
	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if r.cfg.AgentType == "frpc" {
			bundle, notModified, err := r.client.PollHostConfig(ctx, r.state.Version(), runtime.GOOS, runtime.GOARCH)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				slog.Warn("config poll error, backing off", "err", err, "backoff", backoff)
				sleepCtx(ctx, backoff)
				backoff = nextBackoff(backoff)
				continue
			}
			backoff = time.Second
			if notModified {
				continue
			}
			r.reconcile(ctx, bundle)
			continue
		}

		bundle, notModified, err := r.client.PollConfig(ctx, r.state.Version(), runtime.GOOS, runtime.GOARCH)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			slog.Warn("config poll error, backing off", "err", err, "backoff", backoff)
			sleepCtx(ctx, backoff)
			backoff = nextBackoff(backoff)
			continue
		}
		backoff = time.Second
		if notModified {
			continue
		}
		r.applyBundle(ctx, bundle)
	}
}

func (r *Runner) applyBundle(ctx context.Context, bundle *protocol.ConfigResponse) {
	slog.Info("new config received", "version", bundle.ConfigVersion)

	// Ensure the right frp binary is installed before applying.
	if bundle.FrpBinary.DownloadURL != "" {
		if err := frp.EnsureBinary(r.cfg.FrpBinaryPath, bundle.FrpBinary.DownloadURL, bundle.FrpBinary.Version, bundle.FrpBinary.SHA256); err != nil {
			slog.Error("frp binary install failed", "err", err)
			r.ack(ctx, bundle.ConfigVersion, false, "binary install: "+err.Error())
			return
		}
	}

	if err := r.applier.Apply(bundle); err != nil {
		slog.Error("config apply failed, keeping previous config", "err", err)
		r.ack(ctx, bundle.ConfigVersion, false, err.Error())
		return
	}

	if err := r.state.Save(bundle.ConfigVersion, bundle.FrpBinary.Version); err != nil {
		slog.Warn("failed to persist state", "err", err)
	}
	r.ack(ctx, bundle.ConfigVersion, true, "")
	slog.Info("config applied", "version", bundle.ConfigVersion)

	r.scheduleStatusReport(ctx)
}

// scheduleStatusReport reports status shortly after a config apply so the control
// plane learns the new frp state without waiting a full status interval.
func (r *Runner) scheduleStatusReport(ctx context.Context) {
	go func() {
		select {
		case <-ctx.Done():
		case <-time.After(processSettleWait):
			r.reportStatus(ctx)
		}
	}()
}

func (r *Runner) ack(ctx context.Context, version int, ok bool, errMsg string) {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := r.client.Ack(cctx, protocol.AckRequest{ConfigVersion: version, Success: ok, Error: errMsg}); err != nil {
		slog.Debug("ack failed", "err", err)
	}
}

// watchdogLoop keeps managed frp processes alive once a config has been applied,
// throttled to at most 3 restarts per 5 minutes.
func (r *Runner) watchdogLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	var restarts []time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if r.state.Version() == 0 {
				continue // nothing applied yet
			}
			units := r.managedUnits()
			for _, unit := range units {
				if r.systemd.IsActive(unit) {
					continue
				}
				now := time.Now()
				restarts = pruneOld(restarts, now.Add(-5*time.Minute))
				if len(restarts) >= 3 {
					slog.Error("frp restart threshold reached, backing off", "unit", unit)
					continue
				}
				slog.Warn("frp not running, restarting", "unit", unit)
				if err := r.systemd.Restart(unit); err != nil {
					slog.Error("watchdog restart failed", "unit", unit, "err", err)
				}
				restarts = append(restarts, now)
			}
		}
	}
}

// managedUnits is the set of frp systemd units this agent keeps alive.
func (r *Runner) managedUnits() []string {
	if r.cfg.AgentType == "frps" {
		return []string{r.unit}
	}
	var units []string
	for _, uuid := range r.state.ConnUUIDs() {
		units = append(units, frpcUnit(uuid))
	}
	return units
}

func pruneOld(times []time.Time, cutoff time.Time) []time.Time {
	out := times[:0]
	for _, t := range times {
		if t.After(cutoff) {
			out = append(out, t)
		}
	}
	return out
}

func nextBackoff(d time.Duration) time.Duration {
	d *= 2
	if d > 60*time.Second {
		return 60 * time.Second
	}
	return d
}

func sleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
