package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/dreamreflex/service-edge/internal/protocol"
)

// Client talks to the control-plane agent API.
type Client struct {
	endpoint  string
	token     string
	uuid      string
	agentType string

	http     *http.Client
	pollHTTP *http.Client
}

func NewClient(cfg *Config) *Client {
	return &Client{
		endpoint:  cfg.APIEndpoint,
		token:     cfg.APIToken,
		uuid:      cfg.UUID,
		agentType: cfg.AgentType,
		http:      &http.Client{Timeout: 15 * time.Second},
		// Long-poll client must outlast the server's 30s hang.
		pollHTTP: &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("X-Agent-Token", c.token)
	req.Header.Set("X-Agent-UUID", c.uuid)
	req.Header.Set("X-Agent-Type", c.agentType)
	req.Header.Set("Content-Type", "application/json")
}

func (c *Client) postJSON(ctx context.Context, path string, body any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+path, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	c.setHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s: status %d: %s", path, resp.StatusCode, string(b))
	}
	return nil
}

func (c *Client) Heartbeat(ctx context.Context, configVersion int, alive bool) error {
	return c.postJSON(ctx, "/api/v1/agent/heartbeat", protocol.HeartbeatRequest{
		ConfigVersion: configVersion,
		ProcessAlive:  alive,
	})
}

func (c *Client) ReportStatus(ctx context.Context, req protocol.StatusRequest) error {
	return c.postJSON(ctx, "/api/v1/agent/status", req)
}

func (c *Client) Ack(ctx context.Context, req protocol.AckRequest) error {
	return c.postJSON(ctx, "/api/v1/agent/config/ack", req)
}

func (c *Client) Enroll(ctx context.Context, token string, req protocol.EnrollRequest) error {
	buf, _ := json.Marshal(req)
	u := fmt.Sprintf("%s/api/v1/agent/enroll?token=%s", c.endpoint, url.QueryEscape(token))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	httpReq.Header.Set("X-Agent-Token", c.token)
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("enroll: status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// PollConfig issues the long-poll. notModified is true on a 304 timeout.
func (c *Client) PollConfig(ctx context.Context, currentVersion int, osName, arch string) (cfg *protocol.ConfigResponse, notModified bool, err error) {
	q := url.Values{}
	q.Set("current_version", strconv.Itoa(currentVersion))
	q.Set("os", osName)
	q.Set("arch", arch)
	u := c.endpoint + "/api/v1/agent/config?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, false, err
	}
	c.setHeaders(req)
	resp, err := c.pollHTTP.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotModified:
		return nil, true, nil
	case http.StatusOK:
		var out protocol.ConfigResponse
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return nil, false, err
		}
		return &out, false, nil
	default:
		b, _ := io.ReadAll(resp.Body)
		return nil, false, fmt.Errorf("config poll: status %d: %s", resp.StatusCode, string(b))
	}
}
