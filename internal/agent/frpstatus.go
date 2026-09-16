package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dreamreflex/service-edge/internal/protocol"
)

// queryProxyStatusesFor fetches per-proxy status from one frpc connection's
// localhost admin API. Observation failures are explicit, never an empty success.
func (r *Runner) queryProxyStatusesFor(ctx context.Context, connUUID string, adminPort int) ([]protocol.ProxyStatus, error) {
	if adminPort <= 0 || adminPort > 65535 {
		return nil, fmt.Errorf("invalid admin port %d", adminPort)
	}
	user, pass := protocol.FRPCAdminCreds(connUUID, r.cfg.APIToken)
	url := fmt.Sprintf("http://%s:%d/api/status", protocol.FRPCAdminAddr, adminPort)

	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(user, pass)
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("admin API status %d", resp.StatusCode)
	}

	// frpc /api/status returns a map of proxy type -> array of proxy statuses.
	var byType map[string][]struct {
		Name       string `json:"name"`
		Type       string `json:"type"`
		Status     string `json:"status"`
		Err        string `json:"err"`
		RemoteAddr string `json:"remote_addr"`
		LocalAddr  string `json:"local_addr"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&byType); err != nil {
		return nil, err
	}
	var out []protocol.ProxyStatus
	for _, group := range byType {
		for _, p := range group {
			out = append(out, protocol.ProxyStatus{
				Name:       strings.TrimPrefix(p.Name, connUUID+"."),
				Type:       p.Type,
				Status:     p.Status,
				Err:        p.Err,
				RemoteAddr: p.RemoteAddr,
				LocalAddr:  p.LocalAddr,
			})
		}
	}
	probeLocalTargets(ctx, out)
	return out, nil
}

// A running FRP proxy only proves registration at frps. Probe the configured
// local TCP endpoint separately; UDP requires application-specific checks.
func probeLocalTargets(ctx context.Context, proxies []protocol.ProxyStatus) {
	jobs := make(chan int, len(proxies))
	for i := range proxies {
		jobs <- i
	}
	close(jobs)
	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				p := &proxies[i]
				if p.Type == "udp" || p.LocalAddr == "" {
					continue
				}
				if ctx.Err() != nil {
					p.LocalError = "probe budget exhausted"
					continue
				}
				probeCtx, cancel := context.WithTimeout(ctx, 700*time.Millisecond)
				conn, err := (&net.Dialer{}).DialContext(probeCtx, "tcp", p.LocalAddr)
				cancel()
				if ctx.Err() != nil {
					if conn != nil {
						conn.Close()
					}
					p.LocalError = "probe budget exhausted"
					continue
				}
				reachable := err == nil
				p.LocalReachable = &reachable
				if err != nil {
					p.LocalError = err.Error()
				} else {
					conn.Close()
				}
			}
		}()
	}
	wg.Wait()
}
