package service

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"gorm.io/gorm"

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
		var h model.FRPCHost
		if err := s.Store.DB.Where("uuid = ?", uuid).First(&h).Error; err != nil {
			if isNotFound(err) {
				return 0, ErrNotFound
			}
			return 0, err
		}
		return h.ConfigVersion, nil
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
		// uuid is the host; renew any of its connections' certs near expiry.
		conns, err := s.ListConnectionsOfHost(uuid)
		if err != nil {
			return err
		}
		bumped := false
		for _, conn := range conns {
			if !pki.NeedsRenewal(conn.TLSCert) {
				continue
			}
			cert, err := s.CA.IssueClientCert(conn.UUID)
			if err != nil {
				return err
			}
			if err := s.Store.DB.Model(&model.FRPCConnection{}).Where("uuid = ?", conn.UUID).
				UpdateColumns(map[string]any{
					"tls_cert":       cert.CertPEM,
					"tls_key":        cert.KeyPEM,
					"config_version": gorm.Expr("config_version + 1"),
					"updated_at":     time.Now(),
				}).Error; err != nil {
				return err
			}
			bumped = true
		}
		if bumped {
			s.bumpHost(uuid)
		}
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
	}
	// frpc agents are hosts; they use BuildHostConfig (a bundle of connections).
	return nil, fmt.Errorf("unsupported agent type %q for single config", agentType)
}

// BuildHostConfig assembles the multi-connection config bundle an frpc host's
// agent reconciles: one ConnectionConfig per frpc process, each with its rendered
// frpc.toml, certs and localhost admin port.
func (s *Service) BuildHostConfig(hostUUID, osName, arch string) (*protocol.HostConfigResponse, error) {
	if osName == "" {
		osName = "linux"
	}
	if arch == "" {
		arch = "amd64"
	}
	host, err := s.GetFRPCHost(hostUUID)
	if err != nil {
		return nil, err
	}
	resp := &protocol.HostConfigResponse{
		ConfigVersion: host.ConfigVersion,
		FrpBinary:     s.frpBinary(host.FrpVersion, osName, arch),
		CACert:        s.CA.CertPEM(),
	}
	for i := range host.Connections {
		conn := host.Connections[i]
		node, err := s.GetFRPS(conn.FRPSUUID)
		if err != nil {
			return nil, err
		}
		serverAddr := node.PublicIP
		if serverAddr == "" {
			serverAddr = "frps-" + node.UUID // placeholder until public_ip is set
		}
		adminUser, adminPass := protocol.FRPCAdminCreds(conn.UUID, s.Cfg.AgentAPIToken)
		resp.Connections = append(resp.Connections, protocol.ConnectionConfig{
			UUID:          conn.UUID,
			ConfigVersion: conn.ConfigVersion,
			FrpConfig:     RenderFRPCConfig(&conn, node, serverAddr, conn.Proxies, adminUser, adminPass),
			TLSCert:       conn.TLSCert,
			TLSKey:        conn.TLSKey,
			AdminPort:     conn.AdminPort,
		})
	}
	return resp, nil
}

// normalizeFrpVersion ensures the version string has a "v" prefix (the frp binary
// outputs "0.61.1", but GitHub release tags use "v0.61.1").
func normalizeFrpVersion(version string) string {
	if version != "" && !strings.HasPrefix(version, "v") {
		return "v" + version
	}
	return version
}

// frpBinary builds the release download descriptor for a version/os/arch. If a
// matching release tarball has been uploaded to the control plane, the agent is
// pointed at the local dist endpoint instead of GitHub — so binary installs
// triggered after enrollment (version change, missing binary) also work when the
// host can't reach GitHub. Falls back to the configured GitHub base otherwise.
func (s *Service) frpBinary(version, osName, arch string) protocol.FrpBinary {
	tag := normalizeFrpVersion(version) // always v-prefixed for the URL path
	v := strings.TrimPrefix(tag, "v")
	file := fmt.Sprintf("frp_%s_%s_%s.tar.gz", v, osName, arch)
	if s.hasFRPDist(file) {
		url := strings.TrimRight(s.Cfg.Server.ExternalURL, "/") + "/frp-dist/" + file
		return protocol.FrpBinary{Version: version, DownloadURL: url}
	}
	url := fmt.Sprintf("%s/%s/%s", strings.TrimRight(s.Cfg.FrpRelease.BaseURL, "/"), tag, file)
	return protocol.FrpBinary{Version: version, DownloadURL: url}
}

// hasFRPDist reports whether a release tarball with the exact filename has been
// uploaded (and is therefore served by the local /frp-dist endpoint).
func (s *Service) hasFRPDist(filename string) bool {
	var count int64
	s.Store.DB.Model(&model.FRPDistFile{}).Where("filename = ?", filename).Count(&count)
	return count > 0
}

// NoteFRPSPublicIP auto-fills an frps node's public IP from the source address
// the agent connects from, but only when it is not already set. frpc clients dial
// this address (serverAddr); without it they get a non-resolvable placeholder
// ("frps-<uuid>") and cannot connect. Setting it bumps connected frpc clients so
// their config is re-rendered with the real address. A manually set IP is never
// overwritten; loopback sources are ignored (never a usable remote dial address).
func (s *Service) NoteFRPSPublicIP(agentType, uuid, ip string) {
	if agentType != "frps" || ip == "" {
		return
	}
	if parsed := net.ParseIP(ip); parsed == nil || parsed.IsLoopback() || parsed.IsUnspecified() {
		return
	}
	res := s.Store.DB.Model(&model.FRPSNode{}).
		Where("uuid = ? AND (public_ip IS NULL OR public_ip = '')", uuid).
		Update("public_ip", ip)
	if res.Error == nil && res.RowsAffected > 0 {
		s.bumpClientsOf(uuid)
	}
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

// RecordStatus persists a detailed status report: host runtime for both agent
// types, plus per-connection frp status for frpc hosts.
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
		"rt_os":          req.SystemInfo.OS,
		"rt_arch":        req.SystemInfo.Arch,
		"rt_kernel":      req.SystemInfo.Kernel,
		"rt_memory_mb":   req.SystemInfo.MemoryMB,
		"rt_uptime_sec":  req.SystemInfo.UptimeS,
		"rt_last_error":  req.FRPStatus.LastError,
		"rt_reported_at": now,
	}
	if agentType == "frps" {
		updates["rt_process_pid"] = req.ProcessPID
		updates["rt_active_conns"] = req.FRPStatus.ActiveConnections
		if req.FrpVersion != "" {
			updates["frp_version"] = normalizeFrpVersion(req.FrpVersion)
		}
	}
	if req.ListeningPorts != nil {
		if b, err := json.Marshal(req.ListeningPorts); err == nil {
			updates["rt_listen_ports"] = string(b)
		}
	}
	s.Store.DB.Model(modelFor(agentType)).Where("uuid = ?", uuid).UpdateColumns(updates)

	if agentType == "frpc" {
		s.recordConnectionStatuses(uuid, now, req)
	}
	// An frps report refreshes the host's bound ports — heal any mappings that were
	// stuck inactive while their remote_port was occupied but is now free.
	if agentType == "frps" && req.ListeningPorts != nil {
		s.ReevaluateOccupancy(uuid, req.ListeningPorts)
	}
	return nil
}

// recordConnectionStatuses updates each connection's liveness from the host's
// per-connection report and reconciles proxy frp status.
func (s *Service) recordConnectionStatuses(hostUUID string, now time.Time, req protocol.StatusRequest) {
	for _, cs := range req.Connections {
		st := "online"
		if !cs.ProcessAlive {
			st = "offline"
		}
		s.Store.DB.Model(&model.FRPCConnection{}).Where("uuid = ? AND host_uuid = ?", cs.UUID, hostUUID).
			UpdateColumns(map[string]any{"status": st, "last_heartbeat": now, "updated_at": now})
		s.applyProxyStatuses(cs.UUID, cs.ProxyStatuses)
	}
}

// applyProxyStatuses reconciles ProxyMappings with the live frp status the frpc
// agent reported. A proxy whose remote_port failed to bind on the frps host (frp
// status "start error"/"check failed") is deactivated so it stops being pushed,
// with the real frp error recorded for the user.
func (s *Service) applyProxyStatuses(connUUID string, statuses []protocol.ProxyStatus) {
	bumped := false
	for _, st := range statuses {
		if st.Status != "start error" && st.Status != "check failed" {
			continue
		}
		reason := fmt.Sprintf("frp 启动失败（%s）", st.Status)
		if st.Err != "" {
			reason += "：" + st.Err
		}
		reason += "；请更换远程端口或删除该映射"
		// Only flip currently-active rows so we don't repeatedly bump the version.
		res := s.Store.DB.Model(&model.ProxyMapping{}).
			Where("frpc_uuid = ? AND name = ? AND inactive = ?", connUUID, st.Name, false).
			UpdateColumns(map[string]any{"inactive": true, "inactive_reason": reason})
		if res.Error == nil && res.RowsAffected > 0 {
			bumped = true
		}
	}
	if bumped {
		s.bumpConnection(connUUID)
	}
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
	for _, m := range []any{&model.FRPSNode{}, &model.FRPCHost{}, &model.FRPCConnection{}} {
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
	return &model.FRPCHost{}
}
