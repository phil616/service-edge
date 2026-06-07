package service

import (
	"encoding/json"
	"fmt"

	"gorm.io/gorm"

	"github.com/dreamreflex/service-edge/internal/model"
)

// ProxyMappingInput is the create/update payload for one proxy.
type ProxyMappingInput struct {
	Name          string   `json:"name" binding:"required"`
	ProxyType     string   `json:"proxy_type" binding:"required"`
	LocalIP       string   `json:"local_ip"`
	LocalPort     int      `json:"local_port" binding:"required"`
	RemotePort    *int     `json:"remote_port"`
	CustomDomains []string `json:"custom_domains"`
	Subdomain     string   `json:"subdomain"`
}

func (p ProxyMappingInput) toModel(frpcUUID string) model.ProxyMapping {
	localIP := p.LocalIP
	if localIP == "" {
		localIP = "127.0.0.1"
	}
	domains := ""
	if len(p.CustomDomains) > 0 {
		if b, err := json.Marshal(p.CustomDomains); err == nil {
			domains = string(b)
		}
	}
	return model.ProxyMapping{
		FRPCUUID:      frpcUUID,
		Name:          p.Name,
		ProxyType:     p.ProxyType,
		LocalIP:       localIP,
		LocalPort:     p.LocalPort,
		RemotePort:    p.RemotePort,
		CustomDomains: domains,
		Subdomain:     p.Subdomain,
	}
}

func validateProxy(p ProxyMappingInput, usedPorts map[int]bool) error {
	switch p.ProxyType {
	case "tcp", "udp":
		if p.RemotePort == nil || *p.RemotePort <= 0 {
			return fmt.Errorf("%w: %s proxy %q requires remote_port", ErrConflict, p.ProxyType, p.Name)
		}
		if usedPorts[*p.RemotePort] {
			return fmt.Errorf("%w: remote_port %d already in use", ErrConflict, *p.RemotePort)
		}
	case "http", "https":
		if len(p.CustomDomains) == 0 && p.Subdomain == "" {
			return fmt.Errorf("%w: %s proxy %q requires custom_domains or subdomain", ErrConflict, p.ProxyType, p.Name)
		}
	default:
		return fmt.Errorf("%w: unknown proxy_type %q", ErrConflict, p.ProxyType)
	}
	return nil
}

func (s *Service) ListProxies(frpcUUID string) ([]model.ProxyMapping, error) {
	var rows []model.ProxyMapping
	if err := s.Store.DB.Where("frpc_uuid = ?", frpcUUID).Order("id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *Service) AddProxy(connUUID string, in ProxyMappingInput) (*model.ProxyMapping, error) {
	var row model.ProxyMapping
	err := s.Store.DB.Transaction(func(tx *gorm.DB) error {
		var c model.FRPCConnection
		if err := tx.Where("uuid = ?", connUUID).First(&c).Error; err != nil {
			if isNotFound(err) {
				return ErrNotFound
			}
			return err
		}
		var node model.FRPSNode
		if err := tx.Where("uuid = ?", c.FRPSUUID).First(&node).Error; err != nil {
			return err
		}
		used, err := usedPortsTx(tx, c.FRPSUUID, node)
		if err != nil {
			return err
		}
		if err := validateProxy(in, used); err != nil {
			return err
		}
		row = in.toModel(connUUID)
		// Host-occupancy check: a remote_port held by a non-service-edge process
		// on the frps host cannot bind, so the mapping is created inactive.
		setHostOccupancy(&row, externalPorts(node, used))
		return tx.Create(&row).Error
	})
	if err != nil {
		return nil, err
	}
	s.bumpConnection(connUUID)
	return &row, nil
}

func (s *Service) UpdateProxy(id uint, in ProxyMappingInput) (*model.ProxyMapping, error) {
	var row model.ProxyMapping
	var connUUID string
	err := s.Store.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&row, id).Error; err != nil {
			if isNotFound(err) {
				return ErrNotFound
			}
			return err
		}
		connUUID = row.FRPCUUID
		var c model.FRPCConnection
		if err := tx.Where("uuid = ?", connUUID).First(&c).Error; err != nil {
			return err
		}
		var node model.FRPSNode
		if err := tx.Where("uuid = ?", c.FRPSUUID).First(&node).Error; err != nil {
			return err
		}
		used, err := usedPortsTx(tx, c.FRPSUUID, node)
		if err != nil {
			return err
		}
		// Exclude this proxy's current port from the occupancy check.
		if row.RemotePort != nil {
			delete(used, *row.RemotePort)
		}
		if err := validateProxy(in, used); err != nil {
			return err
		}
		// Host-occupancy: the proxy's own current port may appear in the host's
		// reported ports (frp binds it), so exclude it from the external set.
		external := externalPorts(node, used)
		if row.RemotePort != nil {
			delete(external, *row.RemotePort)
		}
		updated := in.toModel(connUUID)
		updated.ID = row.ID
		updated.CreatedAt = row.CreatedAt
		setHostOccupancy(&updated, external)
		row = updated
		return tx.Save(&row).Error
	})
	if err != nil {
		return nil, err
	}
	s.bumpConnection(connUUID)
	return &row, nil
}

func (s *Service) DeleteProxy(id uint) error {
	var row model.ProxyMapping
	if err := s.Store.DB.First(&row, id).Error; err != nil {
		if isNotFound(err) {
			return ErrNotFound
		}
		return err
	}
	if err := s.Store.DB.Delete(&model.ProxyMapping{}, id).Error; err != nil {
		return err
	}
	s.bumpConnection(row.FRPCUUID)
	return nil
}

// ReevaluateOccupancy reactivates tcp/udp mappings on the given frps whose
// remote_port is no longer bound on the frps host. It runs after an frps status
// report refreshes the host's listening ports.
//
// setHostOccupancy only runs at create/update time against a possibly-stale port
// snapshot, so a mapping marked inactive while its port was held — by a foreign
// process, or by a just-deleted proxy the data plane hadn't torn down yet — would
// otherwise stay unrendered forever once the port frees. To stay flap-free this
// only ever CLEARS inactive: a mapping is healed only when its port is absent from
// the host's live listen set (so nothing holds it). Deactivation remains a
// create/update-time decision.
func (s *Service) ReevaluateOccupancy(frpsUUID string, listenPorts []int) {
	bound := make(map[int]bool, len(listenPorts))
	for _, p := range listenPorts {
		bound[p] = true
	}
	var connUUIDs []string
	if err := s.Store.DB.Model(&model.FRPCConnection{}).Where("frps_uuid = ?", frpsUUID).
		Pluck("uuid", &connUUIDs).Error; err != nil || len(connUUIDs) == 0 {
		return
	}
	var rows []model.ProxyMapping
	if err := s.Store.DB.
		Where("frpc_uuid IN ? AND inactive = ? AND remote_port IS NOT NULL", connUUIDs, true).
		Find(&rows).Error; err != nil {
		return
	}
	reactivated := map[string]bool{}
	for _, row := range rows {
		if row.ProxyType != "tcp" && row.ProxyType != "udp" {
			continue
		}
		if row.RemotePort == nil || bound[*row.RemotePort] {
			continue // port still held by something — leave inactive
		}
		if err := s.Store.DB.Model(&model.ProxyMapping{}).Where("id = ?", row.ID).
			UpdateColumns(map[string]any{"inactive": false, "inactive_reason": ""}).Error; err != nil {
			continue
		}
		reactivated[row.FRPCUUID] = true
	}
	// Bump each affected connection so the now-active mapping is rendered & delivered.
	for connUUID := range reactivated {
		s.bumpConnection(connUUID)
	}
}

// parseListenPorts decodes the JSON port array an agent reported for its host.
func parseListenPorts(raw string) []int {
	if raw == "" {
		return nil
	}
	var ports []int
	_ = json.Unmarshal([]byte(raw), &ports)
	return ports
}

// externalPorts returns the set of ports the frps host reports as bound that are
// NOT assigned by service-edge (serviceEdge) — i.e. held by other processes.
func externalPorts(node model.FRPSNode, serviceEdge map[int]bool) map[int]bool {
	ext := map[int]bool{}
	for _, p := range parseListenPorts(node.Runtime.ListenPorts) {
		if !serviceEdge[p] {
			ext[p] = true
		}
	}
	return ext
}

// setHostOccupancy marks a tcp/udp mapping inactive when its remote_port is held
// by another process on the frps host (so frp would fail to bind it).
func setHostOccupancy(row *model.ProxyMapping, external map[int]bool) {
	row.Inactive = false
	row.InactiveReason = ""
	if (row.ProxyType == "tcp" || row.ProxyType == "udp") && row.RemotePort != nil && external[*row.RemotePort] {
		row.Inactive = true
		row.InactiveReason = fmt.Sprintf("远程端口 %d 已被目标节点主机上的其他进程占用，映射未激活；请更换端口或删除该映射", *row.RemotePort)
	}
}

// usedPortsTx computes the occupied remote ports within an existing tx: the
// node's reserved ports plus every proxy remote_port across all connections
// targeting this frps (connections may belong to many different hosts).
func usedPortsTx(tx *gorm.DB, frpsUUID string, node model.FRPSNode) (map[int]bool, error) {
	used := nodeReservedPorts(node)
	var connUUIDs []string
	if err := tx.Model(&model.FRPCConnection{}).Where("frps_uuid = ?", frpsUUID).Pluck("uuid", &connUUIDs).Error; err != nil {
		return nil, err
	}
	if len(connUUIDs) > 0 {
		var ports []int
		if err := tx.Model(&model.ProxyMapping{}).
			Where("frpc_uuid IN ? AND remote_port IS NOT NULL", connUUIDs).
			Pluck("remote_port", &ports).Error; err != nil {
			return nil, err
		}
		for _, p := range ports {
			used[p] = true
		}
	}
	return used, nil
}
