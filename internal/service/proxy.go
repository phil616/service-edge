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

func (s *Service) AddProxy(frpcUUID string, in ProxyMappingInput) (*model.ProxyMapping, error) {
	var row model.ProxyMapping
	err := s.Store.DB.Transaction(func(tx *gorm.DB) error {
		var c model.FRPCClient
		if err := tx.Where("uuid = ?", frpcUUID).First(&c).Error; err != nil {
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
		row = in.toModel(frpcUUID)
		return tx.Create(&row).Error
	})
	if err != nil {
		return nil, err
	}
	s.bumpFRPC(frpcUUID)
	return &row, nil
}

func (s *Service) UpdateProxy(id uint, in ProxyMappingInput) (*model.ProxyMapping, error) {
	var row model.ProxyMapping
	var frpcUUID string
	err := s.Store.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&row, id).Error; err != nil {
			if isNotFound(err) {
				return ErrNotFound
			}
			return err
		}
		frpcUUID = row.FRPCUUID
		var c model.FRPCClient
		if err := tx.Where("uuid = ?", frpcUUID).First(&c).Error; err != nil {
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
		updated := in.toModel(frpcUUID)
		updated.ID = row.ID
		updated.CreatedAt = row.CreatedAt
		row = updated
		return tx.Save(&row).Error
	})
	if err != nil {
		return nil, err
	}
	s.bumpFRPC(frpcUUID)
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
	s.bumpFRPC(row.FRPCUUID)
	return nil
}

// usedPortsTx computes the occupied remote ports within an existing tx.
func usedPortsTx(tx *gorm.DB, frpsUUID string, node model.FRPSNode) (map[int]bool, error) {
	used := map[int]bool{node.BindPort: true}
	if node.DashboardPort != nil {
		used[*node.DashboardPort] = true
	}
	var clientUUIDs []string
	if err := tx.Model(&model.FRPCClient{}).Where("frps_uuid = ?", frpsUUID).Pluck("uuid", &clientUUIDs).Error; err != nil {
		return nil, err
	}
	if len(clientUUIDs) > 0 {
		var ports []int
		if err := tx.Model(&model.ProxyMapping{}).
			Where("frpc_uuid IN ? AND remote_port IS NOT NULL", clientUUIDs).
			Pluck("remote_port", &ports).Error; err != nil {
			return nil, err
		}
		for _, p := range ports {
			used[p] = true
		}
	}
	return used, nil
}
