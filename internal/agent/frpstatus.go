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
	}
	if err := json.NewDecoder(resp.Body).Decode(&byType); err != nil {
		return nil, err
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
	return out, nil
}
