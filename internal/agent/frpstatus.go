package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/dreamreflex/service-edge/internal/protocol"
)

// queryProxyStatuses fetches per-proxy status from frpc's localhost admin API.
// frpc only; returns nil for frps or when the admin API is unreachable (e.g.
// before the first config with the admin server has been applied).
func (r *Runner) queryProxyStatuses(ctx context.Context) []protocol.ProxyStatus {
	if r.cfg.AgentType != "frpc" {
		return nil
	}
	user, pass := protocol.FRPCAdminCreds(r.cfg.UUID, r.cfg.APIToken)
	url := fmt.Sprintf("http://%s:%d/api/status", protocol.FRPCAdminAddr, protocol.FRPCAdminPort)

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
