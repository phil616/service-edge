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
	"github.com/dreamreflex/service-edge/internal/store"
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
	err := s.Store.DB.Transaction(func(tx *gorm.DB) error {
		scoped := *s
		scoped.Store = &store.Store{DB: tx}
		now := time.Now()
		status := "online"
		if !req.ProcessAlive {
			status = "offline"
		}
		if err := scoped.updateAgentLiveness(agentType, uuid, now, status); err != nil {
			return err
		}
		if !req.ConnectionsOnly {
			updates := map[string]any{
				"rt_os": req.SystemInfo.OS, "rt_arch": req.SystemInfo.Arch, "rt_kernel": req.SystemInfo.Kernel,
				"rt_memory_mb": req.SystemInfo.MemoryMB, "rt_uptime_sec": req.SystemInfo.UptimeS,
				"rt_last_error": req.FRPStatus.LastError, "rt_reported_at": now,
			}
			if req.FrpVersion != "" && req.FrpVersion != "unknown" {
				updates["rt_binary_version"] = normalizeFrpVersion(req.FrpVersion)
			}
			if agentType == "frps" {
				updates["rt_process_pid"] = req.ProcessPID
				updates["rt_active_conns"] = req.FRPStatus.ActiveConnections
			}
			if req.ListeningPorts != nil {
				b, err := json.Marshal(req.ListeningPorts)
				if err != nil {
					return err
				}
				updates["rt_listen_ports"] = string(b)
			}
			if err := tx.Model(modelFor(agentType)).Where("uuid = ?", uuid).UpdateColumns(updates).Error; err != nil {
				return err
			}
		}
		if err := scoped.RecordAppliedVersion(agentType, uuid, req.ConfigVersion); err != nil {
			return err
		}
		if agentType == "frpc" {
			return scoped.recordConnectionStatuses(uuid, now, req)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if agentType == "frps" && req.ListeningPorts != nil {
		s.ReevaluateOccupancy(uuid, req.ListeningPorts)
	}
	return nil
}

// Reports repair lost success ACKs, without letting an older report lower the
// applied revision or overwrite the desired binary/configuration.
func (s *Service) RecordAppliedVersion(agentType, uuid string, version int) error {
	if version <= 0 {
		return nil
	}
	return s.Store.DB.Model(modelFor(agentType)).
		Where("uuid = ? AND config_version >= ? AND rt_applied_config_version < ?", uuid, version, version).
		UpdateColumns(map[string]any{
			"rt_applied_config_version": version,
			"rt_last_apply_error":       gorm.Expr("CASE WHEN rt_last_apply_version <= ? THEN '' ELSE rt_last_apply_error END", version),
			"rt_last_apply_version":     gorm.Expr("CASE WHEN rt_last_apply_version < ? THEN ? ELSE rt_last_apply_version END", version, version),
		}).Error
}

func (s *Service) RecordConfigAck(agentType, uuid string, req protocol.AckRequest) error {
	return s.Store.DB.Transaction(func(tx *gorm.DB) error {
		scoped := *s
		scoped.Store = &store.Store{DB: tx}
		if req.Success {
			if err := scoped.RecordAppliedVersion(agentType, uuid, req.ConfigVersion); err != nil {
				return err
			}
		}
		errText := req.Error
		if req.Success {
			errText = ""
		}
		query := tx.Model(modelFor(agentType)).Where("uuid = ? AND config_version >= ? AND rt_last_apply_version <= ?", uuid, req.ConfigVersion, req.ConfigVersion)
		if req.Success {
			query = query.Where("rt_applied_config_version <= ?", req.ConfigVersion)
		} else {
			query = query.Where("rt_applied_config_version < ?", req.ConfigVersion)
		}
		return query.
			UpdateColumns(map[string]any{"rt_last_apply_version": req.ConfigVersion, "rt_last_apply_error": errText}).Error
	})
}

func connectionStatusTTL(req protocol.StatusRequest) time.Duration {
	// Old agents report every 180s; the new lightweight channel reports every 10s.
	if req.ConnectionReportIntervalSeconds <= 0 {
		return 240 * time.Second
	}
	seconds := req.ConnectionReportIntervalSeconds
	if seconds > 180 {
		seconds = 180
	}
	ttl := time.Duration(seconds) * 3 * time.Second
	if ttl < LivenessTimeout {
		ttl = LivenessTimeout
	}
	return ttl
}

func (s *Service) recordConnectionStatuses(hostUUID string, now time.Time, req protocol.StatusRequest) error {
	for _, cs := range req.Connections {
		var conn model.FRPCConnection
		err := s.Store.DB.Where("uuid = ? AND host_uuid = ?", cs.UUID, hostUUID).First(&conn).Error
		if isNotFound(err) {
			continue
		}
		if err != nil {
			return err
		}
		if cs.ConfigVersion > 0 && cs.ConfigVersion < conn.AppliedConfigVersion {
			continue
		}
		available := cs.ProxyStatusAvailable || cs.ProxyStatuses != nil // Legacy agents omit the availability flag.
		st := "unknown"
		statusError := cs.StatusError
		if cs.ProcessStatusAvailable && !cs.ProcessAlive {
			st = "offline"
		} else if cs.ProcessAlive && available {
			st = "idle"
			if len(cs.ProxyStatuses) > 0 {
				st = "online"
			}
			for _, ps := range cs.ProxyStatuses {
				if ps.Status != "running" {
					st = "degraded"
					break
				}
			}
		}
		// A report for an older config must not be attached to today's proxy definitions.
		current := cs.ConfigVersion == 0 || cs.ConfigVersion == conn.ConfigVersion
		if !current {
			st = "unknown"
			statusError = "配置尚未同步"
		}
		expires := now.Add(connectionStatusTTL(req))
		updates := map[string]any{"status": st, "last_heartbeat": now, "updated_at": now, "status_expires_at": expires,
			"process_alive": cs.ProcessAlive, "process_pid": cs.ProcessPID, "status_error": statusError}
		if cs.ConfigVersion > 0 && cs.ConfigVersion <= conn.ConfigVersion {
			updates["applied_config_version"] = cs.ConfigVersion
		}
		if err := s.Store.DB.Model(&conn).UpdateColumns(updates).Error; err != nil {
			return err
		}
		// Full snapshot: absent proxies and unavailable observations become unknown.
		if err := s.Store.DB.Model(&model.ProxyMapping{}).Where("frpc_uuid = ?", cs.UUID).
			UpdateColumns(map[string]any{"observed_status": "unknown", "observed_error": statusError, "observed_at": now}).Error; err != nil {
			return err
		}
		if current && available {
			if err := s.applyProxyStatuses(cs.UUID, cs.ProxyStatuses); err != nil {
				return err
			}
			var missing int64
			if err := s.Store.DB.Model(&model.ProxyMapping{}).Where("frpc_uuid = ? AND inactive = ? AND observed_status = ?", cs.UUID, false, "unknown").Count(&missing).Error; err != nil {
				return err
			}
			if missing > 0 && (st == "online" || st == "idle") {
				if err := s.Store.DB.Model(&conn).UpdateColumns(map[string]any{"status": "unknown", "status_error": "代理状态不完整"}).Error; err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// Observations never mutate desired enablement or trigger configuration restarts.
func (s *Service) applyProxyStatuses(connUUID string, statuses []protocol.ProxyStatus) error {
	for _, st := range statuses {
		if err := s.Store.DB.Model(&model.ProxyMapping{}).Where("frpc_uuid = ? AND name = ?", connUUID, st.Name).
			UpdateColumns(map[string]any{"observed_status": st.Status, "observed_error": st.Err, "observed_at": time.Now()}).Error; err != nil {
			return err
		}
	}
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
	for _, m := range []any{&model.FRPSNode{}, &model.FRPCHost{}} {
		res := s.Store.DB.Model(m).
			Where("status = ? AND (last_heartbeat IS NULL OR last_heartbeat < ?)", "online", cutoff).
			UpdateColumns(map[string]any{"status": "offline", "updated_at": now})
		if res.Error == nil {
			total += res.RowsAffected
		}
	}
	// Connection observation expiry is independent of the host heartbeat lease.
	res := s.Store.DB.Model(&model.FRPCConnection{}).
		Where("status NOT IN ?", []string{"pending", "unknown"}).
		Where("(status_expires_at IS NOT NULL AND status_expires_at < ?) OR (status_expires_at IS NULL AND (last_heartbeat IS NULL OR last_heartbeat < ?)) OR host_uuid IN (?)", now, now.Add(-240*time.Second), s.Store.DB.Model(&model.FRPCHost{}).Select("uuid").Where("status = ?", "offline")).
		UpdateColumns(map[string]any{"status": "unknown", "status_error": "状态已过期或 Agent 不可达", "updated_at": now})
	if res.Error == nil {
		total += res.RowsAffected
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
