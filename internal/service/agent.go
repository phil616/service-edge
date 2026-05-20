package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dreamreflex/service-edge/internal/model"
	"github.com/dreamreflex/service-edge/internal/pki"
	"github.com/dreamreflex/service-edge/internal/protocol"
)

// CurrentConfigVersion returns the agent's current target config_version.
func (s *Service) CurrentConfigVersion(agentType, uuid string) (int, error) {
	switch agentType {
	case "frps":
		node, err := s.GetFRPS(uuid)
		if err != nil {
			return 0, err
		}
		return node.ConfigVersion, nil
	case "frpc":
		var c model.FRPCClient
		if err := s.Store.DB.Where("uuid = ?", uuid).First(&c).Error; err != nil {
			if isNotFound(err) {
				return 0, ErrNotFound
			}
			return 0, err
		}
		return c.ConfigVersion, nil
	}
	return 0, fmt.Errorf("unknown agent type %q", agentType)
}

// MaybeRenewCert reissues and persists a leaf cert nearing expiry, bumping the
// config_version so the renewal is delivered on the next poll.
func (s *Service) MaybeRenewCert(agentType, uuid string) error {
	switch agentType {
	case "frps":
		node, err := s.GetFRPS(uuid)
		if err != nil {
			return err
		}
		if !pki.NeedsRenewal(node.TLSCert) {
			return nil
		}
		sans := []string{"frps-" + uuid}
		if node.PublicIP != "" {
			sans = append(sans, node.PublicIP)
		}
		cert, err := s.CA.IssueServerCert(uuid, sans)
		if err != nil {
			return err
		}
		node.TLSCert = cert.CertPEM
		node.TLSKey = cert.KeyPEM
		node.ConfigVersion++
		node.UpdatedAt = time.Now()
		if err := s.Store.DB.Save(node).Error; err != nil {
			return err
		}
		s.Notifier.Publish(uuid)
	case "frpc":
		var c model.FRPCClient
		if err := s.Store.DB.Where("uuid = ?", uuid).First(&c).Error; err != nil {
			return err
		}
		if !pki.NeedsRenewal(c.TLSCert) {
			return nil
		}
		cert, err := s.CA.IssueClientCert(uuid)
		if err != nil {
			return err
		}
		c.TLSCert = cert.CertPEM
		c.TLSKey = cert.KeyPEM
		c.ConfigVersion++
		c.UpdatedAt = time.Now()
		if err := s.Store.DB.Save(&c).Error; err != nil {
			return err
		}
		s.Notifier.Publish(uuid)
	}
	return nil
}

// BuildConfigResponse assembles the full config bundle for an agent.
func (s *Service) BuildConfigResponse(agentType, uuid, osName, arch string) (*protocol.ConfigResponse, error) {
	if osName == "" {
		osName = "linux"
	}
	if arch == "" {
		arch = "amd64"
	}
	switch agentType {
	case "frps":
		node, err := s.GetFRPS(uuid)
		if err != nil {
			return nil, err
		}
		return &protocol.ConfigResponse{
			ConfigVersion: node.ConfigVersion,
			FrpBinary:     s.frpBinary(node.FrpVersion, osName, arch),
			FrpConfig:     RenderFRPSConfig(node),
			TLSCert:       node.TLSCert,
			TLSKey:        node.TLSKey,
			CACert:        s.CA.CertPEM(),
		}, nil
	case "frpc":
		client, err := s.GetFRPC(uuid)
		if err != nil {
			return nil, err
		}
		node, err := s.GetFRPS(client.FRPSUUID)
		if err != nil {
			return nil, err
		}
		serverAddr := node.PublicIP
		if serverAddr == "" {
			serverAddr = "frps-" + node.UUID // placeholder until public_ip is set
		}
		return &protocol.ConfigResponse{
			ConfigVersion: client.ConfigVersion,
			FrpBinary:     s.frpBinary(client.FrpVersion, osName, arch),
			FrpConfig:     RenderFRPCConfig(client, node, serverAddr, client.Proxies),
			TLSCert:       client.TLSCert,
			TLSKey:        client.TLSKey,
			CACert:        s.CA.CertPEM(),
		}, nil
	}
	return nil, fmt.Errorf("unknown agent type %q", agentType)
}

// normalizeFrpVersion ensures the version string has a "v" prefix (the frp binary
// outputs "0.61.1", but GitHub release tags use "v0.61.1").
func normalizeFrpVersion(version string) string {
	if version != "" && !strings.HasPrefix(version, "v") {
		return "v" + version
	}
	return version
}

// frpBinary builds the release download descriptor for a version/os/arch.
func (s *Service) frpBinary(version, osName, arch string) protocol.FrpBinary {
	tag := normalizeFrpVersion(version) // always v-prefixed for the URL path
	v := strings.TrimPrefix(tag, "v")
	file := fmt.Sprintf("frp_%s_%s_%s.tar.gz", v, osName, arch)
	url := fmt.Sprintf("%s/%s/%s", strings.TrimRight(s.Cfg.FrpRelease.BaseURL, "/"), tag, file)
	return protocol.FrpBinary{Version: version, DownloadURL: url}
}

// RecordHeartbeat updates last_heartbeat/status from a heartbeat ping.
func (s *Service) RecordHeartbeat(agentType, uuid string, alive bool) error {
	now := time.Now()
	status := "online"
	if !alive {
		status = "offline"
	}
	return s.updateAgentLiveness(agentType, uuid, now, status)
}

// RecordStatus persists a detailed status report (currently liveness + version).
func (s *Service) RecordStatus(agentType, uuid string, req protocol.StatusRequest) error {
	now := time.Now()
	status := "online"
	if !req.ProcessAlive {
		status = "offline"
	}
	if err := s.updateAgentLiveness(agentType, uuid, now, status); err != nil {
		return err
	}
	updates := map[string]any{
		"rt_os":           req.SystemInfo.OS,
		"rt_arch":         req.SystemInfo.Arch,
		"rt_kernel":       req.SystemInfo.Kernel,
		"rt_memory_mb":    req.SystemInfo.MemoryMB,
		"rt_uptime_sec":   req.SystemInfo.UptimeS,
		"rt_process_pid":  req.ProcessPID,
		"rt_active_conns": req.FRPStatus.ActiveConnections,
		"rt_last_error":   req.FRPStatus.LastError,
		"rt_reported_at":  now,
	}
	if req.ListeningPorts != nil {
		if b, err := json.Marshal(req.ListeningPorts); err == nil {
			updates["rt_listen_ports"] = string(b)
		}
	}
	if req.FrpVersion != "" {
		updates["frp_version"] = normalizeFrpVersion(req.FrpVersion)
	}
	s.Store.DB.Model(modelFor(agentType)).Where("uuid = ?", uuid).UpdateColumns(updates)
	return nil
}

// LivenessTimeout is how long without a heartbeat before an agent that was
// "online" is considered "offline". Heartbeats arrive every ~20s, so 60s allows
// three misses before flipping. This is what makes a node go offline when its
// agent is stopped/uninstalled or the host disappears — without it, status would
// stay "online" forever.
const LivenessTimeout = 60 * time.Second

// ReapStaleAgents flips any frps node / frpc client whose last heartbeat is older
// than LivenessTimeout from "online" to "offline". Pending (never-enrolled) rows
// are untouched since they are not "online". Returns the number of rows updated.
func (s *Service) ReapStaleAgents() int64 {
	cutoff := time.Now().Add(-LivenessTimeout)
	now := time.Now()
	var total int64
	for _, m := range []any{&model.FRPSNode{}, &model.FRPCClient{}} {
		res := s.Store.DB.Model(m).
			Where("status = ? AND (last_heartbeat IS NULL OR last_heartbeat < ?)", "online", cutoff).
			UpdateColumns(map[string]any{"status": "offline", "updated_at": now})
		if res.Error == nil {
			total += res.RowsAffected
		}
	}
	return total
}

func (s *Service) updateAgentLiveness(agentType, uuid string, t time.Time, status string) error {
	res := s.Store.DB.Model(modelFor(agentType)).Where("uuid = ?", uuid).
		UpdateColumns(map[string]any{"last_heartbeat": t, "status": status, "updated_at": t})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func modelFor(agentType string) any {
	if agentType == "frps" {
		return &model.FRPSNode{}
	}
	return &model.FRPCClient{}
}
