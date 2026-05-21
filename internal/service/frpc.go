package service

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/dreamreflex/service-edge/internal/model"
	"github.com/dreamreflex/service-edge/internal/util"
)

// adminPortBase is the first localhost frpc admin-API port assigned to a host's
// connections; each additional connection on the same host gets the next port.
const adminPortBase = 7400

// CreateConnectionInput creates one frpc connection (to one frps) under a host.
type CreateConnectionInput struct {
	Name     string              `json:"name" binding:"required"`
	FRPSUUID string              `json:"frps_uuid" binding:"required"`
	Protocol string              `json:"protocol"`
	Proxies  []ProxyMappingInput `json:"proxies"`
}

// UpdateConnectionInput updates mutable connection fields.
type UpdateConnectionInput struct {
	Name     *string `json:"name"`
	Protocol *string `json:"protocol"`
}

func (s *Service) ListConnectionsOfHost(hostUUID string) ([]model.FRPCConnection, error) {
	var conns []model.FRPCConnection
	if err := s.Store.DB.Where("host_uuid = ?", hostUUID).Order("id asc").Find(&conns).Error; err != nil {
		return nil, err
	}
	return conns, nil
}

func (s *Service) GetConnection(uuid string) (*model.FRPCConnection, error) {
	var conn model.FRPCConnection
	if err := s.Store.DB.Where("uuid = ?", uuid).First(&conn).Error; err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	proxies, err := s.ListProxies(uuid)
	if err != nil {
		return nil, err
	}
	conn.Proxies = proxies
	return &conn, nil
}

func (s *Service) CreateConnection(hostUUID string, in CreateConnectionInput) (*model.FRPCConnection, error) {
	var conn *model.FRPCConnection
	err := s.Store.DB.Transaction(func(tx *gorm.DB) error {
		var host model.FRPCHost
		if err := tx.Where("uuid = ?", hostUUID).First(&host).Error; err != nil {
			if isNotFound(err) {
				return fmt.Errorf("%w: host %s not found", ErrNotFound, hostUUID)
			}
			return err
		}
		var node model.FRPSNode
		if err := tx.Where("uuid = ?", in.FRPSUUID).First(&node).Error; err != nil {
			if isNotFound(err) {
				return fmt.Errorf("%w: target frps %s not found", ErrNotFound, in.FRPSUUID)
			}
			return err
		}
		protocol, err := validateClientProtocol(node, in.Protocol)
		if err != nil {
			return err
		}

		uuid := util.NewUUID()
		cert, err := s.CA.IssueClientCert(uuid)
		if err != nil {
			return fmt.Errorf("issue client cert: %w", err)
		}
		c := &model.FRPCConnection{
			UUID:          uuid,
			HostUUID:      hostUUID,
			Name:          in.Name,
			FRPSUUID:      in.FRPSUUID,
			Protocol:      protocol,
			AdminPort:     nextAdminPort(tx, hostUUID),
			TLSCert:       cert.CertPEM,
			TLSKey:        cert.KeyPEM,
			ConfigVersion: 1,
			Status:        "pending",
		}
		if err := tx.Create(c).Error; err != nil {
			return err
		}

		used, err := usedPortsTx(tx, in.FRPSUUID, node)
		if err != nil {
			return err
		}
		for _, pin := range in.Proxies {
			if err := validateProxy(pin, used); err != nil {
				return err
			}
			row := pin.toModel(uuid)
			setHostOccupancy(&row, externalPorts(node, used))
			if pin.RemotePort != nil {
				used[*pin.RemotePort] = true
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		conn = c
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.bumpHost(hostUUID)
	return s.GetConnection(conn.UUID)
}

func (s *Service) UpdateConnection(uuid string, in UpdateConnectionInput) (*model.FRPCConnection, error) {
	var hostUUID string
	err := s.Store.DB.Transaction(func(tx *gorm.DB) error {
		var c model.FRPCConnection
		if err := tx.Where("uuid = ?", uuid).First(&c).Error; err != nil {
			if isNotFound(err) {
				return ErrNotFound
			}
			return err
		}
		hostUUID = c.HostUUID
		if in.Name != nil {
			c.Name = *in.Name
		}
		if in.Protocol != nil {
			var node model.FRPSNode
			if err := tx.Where("uuid = ?", c.FRPSUUID).First(&node).Error; err != nil {
				return err
			}
			protocol, err := validateClientProtocol(node, *in.Protocol)
			if err != nil {
				return err
			}
			c.Protocol = protocol
		}
		c.ConfigVersion++
		c.UpdatedAt = time.Now()
		return tx.Save(&c).Error
	})
	if err != nil {
		return nil, err
	}
	s.bumpHost(hostUUID)
	return s.GetConnection(uuid)
}

func (s *Service) DeleteConnection(uuid string) error {
	var hostUUID string
	err := s.Store.DB.Transaction(func(tx *gorm.DB) error {
		var c model.FRPCConnection
		if err := tx.Where("uuid = ?", uuid).First(&c).Error; err != nil {
			if isNotFound(err) {
				return ErrNotFound
			}
			return err
		}
		hostUUID = c.HostUUID
		if err := tx.Where("frpc_uuid = ?", uuid).Delete(&model.ProxyMapping{}).Error; err != nil {
			return err
		}
		return tx.Where("uuid = ?", uuid).Delete(&model.FRPCConnection{}).Error
	})
	if err != nil {
		return err
	}
	s.bumpHost(hostUUID)
	return nil
}

// nextAdminPort returns the next free localhost admin port for a host's
// connections (base + count), so multiple frpc processes don't collide on 7400.
func nextAdminPort(tx *gorm.DB, hostUUID string) int {
	var maxPort *int
	tx.Model(&model.FRPCConnection{}).Where("host_uuid = ?", hostUUID).
		Select("MAX(admin_port)").Scan(&maxPort)
	if maxPort == nil || *maxPort < adminPortBase {
		return adminPortBase
	}
	return *maxPort + 1
}

// bumpConnection increments one connection's config_version then bumps its host's
// aggregate version (which wakes the host's long-poll).
func (s *Service) bumpConnection(connUUID string) {
	var conn model.FRPCConnection
	if err := s.Store.DB.Where("uuid = ?", connUUID).First(&conn).Error; err != nil {
		return
	}
	s.Store.DB.Model(&model.FRPCConnection{}).Where("uuid = ?", connUUID).
		UpdateColumns(map[string]any{"config_version": gorm.Expr("config_version + 1"), "updated_at": time.Now()})
	s.bumpHost(conn.HostUUID)
}
