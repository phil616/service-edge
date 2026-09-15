package agent

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/dreamreflex/service-edge/internal/frp"
	"github.com/dreamreflex/service-edge/internal/protocol"
)

const connectionReportInterval = 10 * time.Second

func (r *Runner) connectionStatusLoop(ctx context.Context) {
	if r.cfg.AgentType != "frpc" {
		return
	}
	ticker := time.NewTicker(connectionReportInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.reportConnections(ctx, true)
		}
	}
}

func (r *Runner) reportHostStatus(ctx context.Context) { r.reportConnections(ctx, false) }

func (r *Runner) reportConnections(ctx context.Context, connectionsOnly bool) {
	// Serialize periodic and apply-triggered snapshots; never send older samples
	// after a newer report from this agent process.
	r.statusMu.Lock()
	defer r.statusMu.Unlock()
	if ctx.Err() != nil {
		return
	}
	sampleCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	conns := r.collectConnections(sampleCtx)
	cancel()
	req := protocol.StatusRequest{
		ConfigVersion: r.state.Version(), ProcessAlive: true, Connections: conns,
		ConnectionsOnly: connectionsOnly, ConnectionReportIntervalSeconds: int(connectionReportInterval / time.Second),
	}
	if !connectionsOnly {
		req.FrpVersion = frp.FrpVersion(r.cfg.FrpBinaryPath)
		req.SystemInfo = collectSystemInfo()
		req.ListeningPorts = collectListeningPorts()
	}
	// Collection and delivery have separate budgets: slow local admin APIs must
	// not prevent us from reporting their failures to the control plane.
	uploadCtx, done := context.WithTimeout(ctx, 15*time.Second)
	defer done()
	if err := r.client.ReportStatus(uploadCtx, req); err != nil {
		slog.Warn("connection status report failed", "err", err)
	}
}

func (r *Runner) collectConnections(ctx context.Context) []protocol.ConnectionStatus {
	entries := r.state.ConnEntries()
	out := make([]protocol.ConnectionStatus, len(entries))
	type job struct {
		index int
		uuid  string
		state ConnState
	}
	jobs := make(chan job, len(entries))
	i := 0
	for uuid, st := range entries {
		jobs <- job{i, uuid, st}
		i++
	}
	close(jobs)
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				cs := protocol.ConnectionStatus{UUID: j.uuid, ConfigVersion: j.state.Version}
				cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
				alive, pid, err := r.systemd.ProcessStatus(cctx, frpcUnit(j.uuid))
				cs.ProcessAlive, cs.ProcessPID = alive, pid
				if err != nil {
					cs.StatusError = "process observation: " + err.Error()
				} else {
					cs.ProcessStatusAvailable = true
					if alive {
						cs.ProxyStatuses, err = r.queryProxyStatusesFor(cctx, j.uuid, j.state.AdminPort)
						if err != nil {
							cs.StatusError = "admin observation: " + err.Error()
						} else {
							cs.ProxyStatusAvailable = true
						}
					}
				}
				cancel()
				out[j.index] = cs
			}
		}()
	}
	wg.Wait()
	return out
}
