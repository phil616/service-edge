// Package protocol defines the JSON wire types shared by the control-plane
// agent API handlers and the agent client, so both sides stay in sync.
package protocol

import (
	"crypto/sha256"
	"encoding/hex"
)

// frpc admin (webServer) API: rendered into frpc.toml and queried by the agent
// to learn each proxy's real frp status (e.g. a remote_port that failed to bind).
const (
	FRPCAdminAddr = "127.0.0.1"
	FRPCAdminPort = 7400
	FRPCAdminUser = "service-edge-agent"
)

// FRPCAdminCreds derives the frpc admin-API credentials deterministically from
// the agent UUID and the shared agent API token, so the control plane (renderer)
// and the agent compute identical values without extra plumbing.
func FRPCAdminCreds(uuid, apiToken string) (user, password string) {
	sum := sha256.Sum256([]byte(uuid + "|" + apiToken))
	return FRPCAdminUser, hex.EncodeToString(sum[:])[:32]
}

// ProxyStatus is one proxy's live status as reported by frpc's admin API.
type ProxyStatus struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Status     string `json:"status"` // running | start error | check failed | ...
	Err        string `json:"err,omitempty"`
	RemoteAddr string `json:"remote_addr,omitempty"`
}

// SystemInfo describes the host an agent runs on.
type SystemInfo struct {
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Kernel   string `json:"kernel"`
	MemoryMB uint64 `json:"memory_mb"`
	UptimeS  uint64 `json:"uptime_sec"`
}

// HeartbeatRequest is the high-frequency liveness ping.
type HeartbeatRequest struct {
	ConfigVersion int  `json:"config_version"`
	ProcessAlive  bool `json:"process_alive"`
}

// FRPStatus is a coarse view of the managed frp process.
type FRPStatus struct {
	ActiveConnections int    `json:"active_connections"`
	LastError         string `json:"last_error"`
}

// ConfigSummary summarizes the applied config.
type ConfigSummary struct {
	ProxyCount int `json:"proxy_count"`
}

// StatusRequest is the low-frequency detailed report.
type StatusRequest struct {
	ConfigVersion int           `json:"config_version"`
	ProcessAlive  bool          `json:"process_alive"`
	ProcessPID    int           `json:"process_pid"`
	FrpVersion    string        `json:"frp_version"`
	SystemInfo    SystemInfo    `json:"system_info"`
	FRPStatus     FRPStatus     `json:"frp_status"`
	ConfigSummary ConfigSummary `json:"config_summary"`
	// ListeningPorts are the TCP/UDP ports currently bound on the agent's host.
	// The control plane uses these to detect remote_port conflicts caused by
	// processes outside service-edge (it cannot probe hosts itself).
	ListeningPorts []int `json:"listening_ports,omitempty"`
	// ProxyStatuses is the live per-proxy status from frpc's admin API (frpc
	// agents only). The control plane uses it to flag proxies whose remote_port
	// failed to bind on the frps host.
	ProxyStatuses []ProxyStatus `json:"proxy_statuses,omitempty"`
}

// FrpBinary tells the agent which frp release to install.
type FrpBinary struct {
	Version     string `json:"version"`
	DownloadURL string `json:"download_url"`
	SHA256      string `json:"sha256,omitempty"`
}

// ConfigResponse is the long-poll payload delivered when a newer config exists.
type ConfigResponse struct {
	ConfigVersion int       `json:"config_version"`
	FrpBinary     FrpBinary `json:"frp_binary"`
	FrpConfig     string    `json:"frp_config"`
	TLSCert       string    `json:"tls_cert"`
	TLSKey        string    `json:"tls_key"`
	CACert        string    `json:"ca_cert"`
}

// AckRequest reports the result of applying a config version.
type AckRequest struct {
	ConfigVersion int    `json:"config_version"`
	Success       bool   `json:"success"`
	Error         string `json:"error"`
}

// EnrollRequest is sent once at install time alongside the enrollment token.
type EnrollRequest struct {
	UUID       string     `json:"uuid"`
	AgentType  string     `json:"agent_type"`
	SystemInfo SystemInfo `json:"system_info"`
}
