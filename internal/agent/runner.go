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
	cfg           *Config
	client        *Client
	state         *State
	systemd       processManager
	operationMu   sync.Mutex
	statusMu      sync.Mutex
	statusTrigger chan struct{}

	// frps-only: the single managed unit + its applier.
	applier *Applier
	unit    string
}

func NewRunner(cfg *Config) *Runner {
	r := &Runner{cfg: cfg, client: NewClient(cfg), systemd: frp.Systemd{}, statusTrigger: make(chan struct{}, 1)}
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
	var wg sync.WaitGroup
	for _, loop := range []func(context.Context){
		r.heartbeatLoop,
		r.statusLoop,
		r.connectionStatusLoop,
		r.configSyncLoop,
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

// heartbeatAlive reports Agent liveness; FRP is observed independently.
func (r *Runner) heartbeatAlive() bool { return true }

// heartbeatLoop reports immediately and independently of FRP process observation.
func (r *Runner) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(r.cfg.HeartbeatInterval.Std())
	defer ticker.Stop()
	for {
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		if err := r.client.Heartbeat(cctx, r.state.Version(), r.heartbeatAlive()); err != nil {
			slog.Warn("heartbeat failed", "err", err)
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
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
		case <-r.statusTrigger:
			r.reportStatus(ctx)
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
	r.reportFRPSStatus(ctx, false)
}

func (r *Runner) reportFRPSStatus(ctx context.Context, lightweight bool) {
	r.statusMu.Lock()
	defer r.statusMu.Unlock()
	sampleCtx, sampleCancel := context.WithTimeout(ctx, 2*time.Second)
	active, pid, observeErr := r.systemd.ProcessStatus(sampleCtx, r.unit)
	sampleCancel()
	req := protocol.StatusRequest{ConfigVersion: r.state.Version(), ProcessAlive: active, ProcessPID: pid, ProcessStatusAvailable: observeErr == nil,
		ConnectionsOnly: lightweight}
	if !lightweight {
		req.FrpVersion = frp.FrpVersion(filepath.Join(r.cfg.Paths().ConfigDir, "current", "frps"))
		req.SystemInfo = collectSystemInfo()
		req.ListeningPorts = collectListeningPorts()
	}
	if observeErr != nil {
		req.StatusError = observeErr.Error()
	}
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := r.client.ReportStatus(cctx, req); err != nil {
		slog.Warn("status report failed", "err", err)
	}
}

// configSyncLoop long-polls for config and applies updates. Network errors back
// off exponentially up to 60s; 304 timeouts re-poll immediately.
func (r *Runner) configSyncLoop(ctx context.Context) {
	backoff := time.Second
	var nextFullSync time.Time
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if r.cfg.AgentType == "frpc" {
			// Re-read the desired set on startup and periodically, even at the same
			// version, to repair missing files and migrate old applied-state records.
			pollVersion := r.state.Version()
			if time.Now().After(nextFullSync) {
				pollVersion = 0
			}
			bundle, notModified, err := r.client.PollHostConfig(ctx, pollVersion, runtime.GOOS, runtime.GOARCH)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				slog.Warn("config poll error, backing off", "err", err, "backoff", backoff)
				sleepCtx(ctx, backoff)
				backoff = nextBackoff(backoff)
				continue
			}
			if notModified {
				backoff = time.Second
				continue
			}
			if !r.reconcile(ctx, bundle) {
				// Apply incomplete: the host version was not advanced, so the next
				// poll re-delivers the bundle. Back off so we don't churn-restart frp.
				slog.Warn("host config apply incomplete, backing off", "backoff", backoff)
				sleepCtx(ctx, backoff)
				backoff = nextBackoff(backoff)
				continue
			}
			backoff = time.Second
			nextFullSync = time.Now().Add(5 * time.Minute)
			continue
		}

		pollVersion := r.state.Version()
		if time.Now().After(nextFullSync) {
			pollVersion = 0
		}
		bundle, notModified, err := r.client.PollConfig(ctx, pollVersion, runtime.GOOS, runtime.GOARCH)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			slog.Warn("config poll error, backing off", "err", err, "backoff", backoff)
			sleepCtx(ctx, backoff)
			backoff = nextBackoff(backoff)
			continue
		}
		if notModified {
			backoff = time.Second
			continue
		}
		if !r.applyBundle(ctx, bundle) {
			slog.Warn("config apply failed, backing off", "backoff", backoff)
			sleepCtx(ctx, backoff)
			backoff = nextBackoff(backoff)
			continue
		}
		backoff = time.Second
		nextFullSync = time.Now().Add(5 * time.Minute)
	}
}

// applyBundle stages and applies one frps config bundle. It returns true on a
// clean apply; on failure it leaves the applied version untouched so the caller
// re-polls (the same bundle is re-delivered) and retries with backoff.
func (r *Runner) applyBundle(ctx context.Context, bundle *protocol.ConfigResponse) bool {
	r.operationMu.Lock()
	defer r.operationMu.Unlock()
	slog.Info("new config received", "version", bundle.ConfigVersion)

	binary, err := frp.PrepareBinary(ctx, r.cfg.FrpBinaryPath, bundle.FrpBinary.DownloadURL, bundle.FrpBinary.Version, bundle.FrpBinary.SHA256)
	if err != nil {
		slog.Error("prepare frp binary failed", "version", bundle.FrpBinary.Version, "err", err)
		r.ack(ctx, bundle.ConfigVersion, false, "prepare binary: "+err.Error())
		return false
	}
	r.applier.binary = binary

	if err := r.applier.Apply(bundle); err != nil {
		slog.Error("config apply failed, keeping previous config", "err", err)
		r.ack(ctx, bundle.ConfigVersion, false, err.Error())
		return false
	}

	if err := r.state.Save(bundle.ConfigVersion, bundle.FrpBinary.Version); err != nil {
		slog.Error("failed to persist state", "err", err)
		r.ack(ctx, bundle.ConfigVersion, false, "persist state: "+err.Error())
		return false
	}
	r.ack(ctx, bundle.ConfigVersion, true, "")
	slog.Info("config applied", "version", bundle.ConfigVersion)

	r.scheduleStatusReport(ctx)
	return true
}

// scheduleStatusReport reports status shortly after a config apply so the control
// plane learns the new frp state without waiting a full status interval.
func (r *Runner) scheduleStatusReport(ctx context.Context) {
	select {
	case r.statusTrigger <- struct{}{}:
	default:
	}
}

func (r *Runner) ack(ctx context.Context, version int, ok bool, errMsg string) {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := r.client.Ack(cctx, protocol.AckRequest{ConfigVersion: version, Success: ok, Error: errMsg}); err != nil {
		slog.Warn("ack failed", "err", err)
	}
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
