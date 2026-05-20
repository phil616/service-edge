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

// Runner is the agent's top-level coordinator running the four concurrent loops.
type Runner struct {
	cfg     *Config
	client  *Client
	state   *State
	applier *Applier
	systemd frp.Systemd
	unit    string
}

func NewRunner(cfg *Config) *Runner {
	statePath := filepath.Join(cfg.Paths().DataDir, "state.json")
	return &Runner{
		cfg:     cfg,
		client:  NewClient(cfg),
		state:   LoadState(statePath),
		applier: NewApplier(cfg),
		unit:    cfg.ServiceUnit(),
	}
}

// Run starts all loops and blocks until ctx is cancelled.
func (r *Runner) Run(ctx context.Context) {
	slog.Info("agent starting", "type", r.cfg.AgentType, "uuid", r.cfg.UUID, "unit", r.unit)
	// Persist the frp unit across reboots (best effort; no-op without systemd).
	if err := r.systemd.Enable(r.unit); err != nil {
		slog.Debug("enable frp unit failed (continuing)", "unit", r.unit, "err", err)
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

func (r *Runner) processAlive() bool { return r.systemd.IsActive(r.unit) }

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
			if err := r.client.Heartbeat(cctx, r.state.Version(), r.processAlive()); err != nil {
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
	report := func() {
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		req := protocol.StatusRequest{
			ConfigVersion:  r.state.Version(),
			ProcessAlive:   r.processAlive(),
			ProcessPID:     r.systemd.MainPID(r.unit),
			FrpVersion:     frp.FrpVersion(r.cfg.FrpBinaryPath),
			SystemInfo:     collectSystemInfo(),
			ListeningPorts: collectListeningPorts(),
		}
		if err := r.client.ReportStatus(cctx, req); err != nil {
			slog.Debug("status report failed", "err", err)
		}
	}
	report() // initial report on startup
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			report()
		}
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
}

func (r *Runner) ack(ctx context.Context, version int, ok bool, errMsg string) {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := r.client.Ack(cctx, protocol.AckRequest{ConfigVersion: version, Success: ok, Error: errMsg}); err != nil {
		slog.Debug("ack failed", "err", err)
	}
}

// watchdogLoop keeps the frp process alive once a config has been applied,
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
			if r.processAlive() {
				continue
			}
			now := time.Now()
			restarts = pruneOld(restarts, now.Add(-5*time.Minute))
			if len(restarts) >= 3 {
				slog.Error("frp restart threshold reached, backing off", "unit", r.unit)
				continue
			}
			slog.Warn("frp not running, restarting", "unit", r.unit)
			if err := r.systemd.Restart(r.unit); err != nil {
				slog.Error("watchdog restart failed", "err", err)
			}
			restarts = append(restarts, now)
		}
	}
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
