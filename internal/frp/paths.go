// Package frp holds frp deployment path conventions plus the agent-side binary
// installer, systemd integration and process control. The path helpers are
// shared between the control plane (which bakes absolute paths into rendered
// frps.toml/frpc.toml) and the agent (which writes certs/configs to those exact
// paths), keeping the two sides in sync.
package frp

import "path/filepath"

const (
	FRPSBaseDir = "/opt/service-edge/frps-agent"
	FRPCBaseDir = "/opt/service-edge/frpc-agent"

	FRPSSystemdUnit  = "service-edge-frps"
	FRPCSystemdUnit  = "service-edge-frpc" // template: service-edge-frpc@<uuid>.service
	AgentFRPSUnit    = "service-edge-frps-agent"
	AgentFRPCUnit    = "service-edge-frpc-agent"
)

// FRPSPaths returns the on-disk paths for an frps agent deployment.
type DeployPaths struct {
	ConfigDir  string
	DataDir    string
	LogDir     string
	ConfigFile string
	NewConfig  string
	BackupFile string
	CertFile   string
	KeyFile    string
	CAFile     string
	LogFile    string
}

// FRPSPaths returns the standard layout for the (single) frps deployment.
func FRPSPaths() DeployPaths {
	cfg := filepath.Join(FRPSBaseDir, "config")
	return DeployPaths{
		ConfigDir:  cfg,
		DataDir:    filepath.Join(FRPSBaseDir, "data"),
		LogDir:     filepath.Join(FRPSBaseDir, "logs"),
		ConfigFile: filepath.Join(cfg, "frps.toml"),
		NewConfig:  filepath.Join(cfg, "frps.toml.new"),
		BackupFile: filepath.Join(cfg, "frps.toml.backup"),
		CertFile:   filepath.Join(cfg, "server.crt"),
		KeyFile:    filepath.Join(cfg, "server.key"),
		CAFile:     filepath.Join(cfg, "ca.crt"),
		LogFile:    filepath.Join(FRPSBaseDir, "logs", "frps.log"),
	}
}

// FRPCPaths returns the standard layout for one frpc instance identified by uuid.
func FRPCPaths(uuid string) DeployPaths {
	inst := filepath.Join(FRPCBaseDir, "instances", uuid)
	cfg := filepath.Join(inst, "config")
	return DeployPaths{
		ConfigDir:  cfg,
		DataDir:    filepath.Join(inst, "data"),
		LogDir:     filepath.Join(inst, "logs"),
		ConfigFile: filepath.Join(cfg, "frpc.toml"),
		NewConfig:  filepath.Join(cfg, "frpc.toml.new"),
		BackupFile: filepath.Join(cfg, "frpc.toml.backup"),
		CertFile:   filepath.Join(cfg, "client.crt"),
		KeyFile:    filepath.Join(cfg, "client.key"),
		CAFile:     filepath.Join(cfg, "ca.crt"),
		LogFile:    filepath.Join(inst, "logs", "frpc.log"),
	}
}
