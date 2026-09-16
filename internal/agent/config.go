// Package agent implements on-host management of one frps or many frpc
// processes. It reports liveness and observed status, reconciles complete
// configurations, and delegates process supervision to systemd.
package agent

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/dreamreflex/service-edge/internal/config"
	"github.com/dreamreflex/service-edge/internal/frp"
)

// Config is the agent's own configuration (agent.yaml).
type Config struct {
	AgentType            string          `yaml:"agent_type"` // frps|frpc
	UUID                 string          `yaml:"uuid"`
	APIEndpoint          string          `yaml:"api_endpoint"`
	APIToken             string          `yaml:"api_token"`
	HeartbeatInterval    config.Duration `yaml:"heartbeat_interval"`
	StatusReportInterval config.Duration `yaml:"status_report_interval"`
	ConfigPollTimeout    config.Duration `yaml:"config_poll_timeout"`
	FrpBinaryPath        string          `yaml:"frp_binary_path"`
	FrpConfigPath        string          `yaml:"frp_config_path"`
	SystemdUnit          string          `yaml:"systemd_unit"`
}

// LoadConfig reads and validates agent.yaml.
func LoadConfig(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read agent config: %w", err)
	}
	var c Config
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("parse agent config: %w", err)
	}
	if c.UUID == "" || (c.AgentType != "frps" && c.AgentType != "frpc") {
		return nil, fmt.Errorf("agent config missing uuid or invalid agent_type")
	}
	if c.APIEndpoint == "" || c.APIToken == "" {
		return nil, fmt.Errorf("agent config missing api_endpoint or api_token")
	}
	if c.HeartbeatInterval == 0 {
		c.HeartbeatInterval = config.Duration(20 * time.Second)
	}
	if c.StatusReportInterval == 0 {
		c.StatusReportInterval = config.Duration(180 * time.Second)
	}

	if c.HeartbeatInterval < 0 || c.StatusReportInterval < 0 {
		return nil, fmt.Errorf("report intervals must be positive")
	}
	if c.ConfigPollTimeout < 0 {
		return nil, fmt.Errorf("config_poll_timeout must not be negative")
	}
	if c.ConfigPollTimeout.Std() <= 30*time.Second {
		if c.ConfigPollTimeout != 0 {
			slog.Warn("upgrading legacy config_poll_timeout to 60s")
		}
		c.ConfigPollTimeout = config.Duration(60 * time.Second)
	}
	c.APIEndpoint = strings.TrimRight(strings.TrimSpace(c.APIEndpoint), "/")
	u, err := url.Parse(c.APIEndpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return nil, fmt.Errorf("api_endpoint must be an HTTP(S) base URL without credentials, query or fragment")
	}
	if strings.HasSuffix(u.Path, "/api/v1") {
		return nil, fmt.Errorf("api_endpoint must not include /api/v1")
	}
	if c.FrpBinaryPath == "" {
		c.FrpBinaryPath = frp.FRPSBaseDir + "/bin/frps"
		if c.AgentType == "frpc" {
			c.FrpBinaryPath = frp.FRPCBaseDir + "/bin/frpc"
		}
	}

	return &c, nil
}

// Paths returns the deploy path layout for this agent.
func (c *Config) Paths() frp.DeployPaths {
	if c.AgentType == "frps" {
		return frp.FRPSPaths()
	}
	return frp.FRPCPaths(c.UUID)
}

// ServiceUnit returns the systemd unit name for the managed frp process.
// frpc uses a template unit instanced by UUID.
func (c *Config) ServiceUnit() string {
	if c.AgentType == "frpc" {
		return fmt.Sprintf("%s@%s", frp.FRPCSystemdUnit, c.UUID)
	}
	return frp.FRPSSystemdUnit
}
