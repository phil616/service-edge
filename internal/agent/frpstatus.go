package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/dreamreflex/service-edge/internal/protocol"
)

// queryProxyStatusesFor fetches per-proxy status from one frpc connection's
// localhost admin API (its dedicated admin port). Returns nil when unreachable
// (e.g. before the connection's first config has been applied).
func (r *Runner) queryProxyStatusesFor(ctx context.Context, connUUID string, adminPort int) []protocol.ProxyStatus {
	if adminPort == 0 {
		return nil
	}
	user, pass := protocol.FRPCAdminCreds(connUUID, r.cfg.APIToken)
	url := fmt.Sprintf("http://%s:%d/api/status", protocol.FRPCAdminAddr, adminPort)

	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	req.SetBasicAuth(user, pass)
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}

	// frpc /api/status returns a map of proxy type -> array of proxy statuses.
	var byType map[string][]struct {
		Name       string `json:"name"`
		Type       string `json:"type"`
		Status     string `json:"status"`
		Err        string `json:"err"`
		RemoteAddr string `json:"remote_addr"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&byType); err != nil {
		return nil
	}
	var out []protocol.ProxyStatus
	for _, group := range byType {
		for _, p := range group {
			out = append(out, protocol.ProxyStatus{
				Name:       p.Name,
				Type:       p.Type,
				Status:     p.Status,
				Err:        p.Err,
				RemoteAddr: p.RemoteAddr,
			})
		}
	}
	return out
}
