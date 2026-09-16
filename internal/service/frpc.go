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
		adminPort, err := nextAdminPort(tx, hostUUID)
		if err != nil {
			return err
		}
		c := &model.FRPCConnection{
			UUID:          uuid,
			HostUUID:      hostUUID,
			Name:          in.Name,
			FRPSUUID:      in.FRPSUUID,
			Protocol:      protocol,
			AdminPort:     adminPort,
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
		names := map[string]bool{}
		for _, pin := range in.Proxies {
			if names[pin.Name] {
				return fmt.Errorf("%w: duplicate proxy name %q", ErrConflict, pin.Name)
			}
			names[pin.Name] = true
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
		return bumpHostTx(tx, hostUUID)
	})
	if err != nil {
		return nil, err
	}
	s.Notifier.Publish(hostUUID)
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
		if err := tx.Save(&c).Error; err != nil {
			return err
		}
		return bumpHostTx(tx, hostUUID)
	})
	if err != nil {
		return nil, err
	}
	s.Notifier.Publish(hostUUID)
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
		if err := tx.Where("uuid = ?", uuid).Delete(&model.FRPCConnection{}).Error; err != nil {
			return err
		}
		return bumpHostTx(tx, hostUUID)
	})
	if err != nil {
		return err
	}
	s.Notifier.Publish(hostUUID)
	return nil
}

// nextAdminPort reuses gaps and fails explicitly when the host exhausts ports.
func nextAdminPort(tx *gorm.DB, hostUUID string) (int, error) {
	var ports []int
	if err := tx.Model(&model.FRPCConnection{}).Where("host_uuid = ?", hostUUID).Pluck("admin_port", &ports).Error; err != nil {
		return 0, err
	}
	used := map[int]bool{}
	for _, port := range ports {
		used[port] = true
	}
	for port := adminPortBase; port <= 65535; port++ {
		if !used[port] {
			return port, nil
		}
	}
	return 0, fmt.Errorf("%w: no available admin port", ErrConflict)
}

func bumpConnectionTx(tx *gorm.DB, connUUID string) (string, error) {
	var conn model.FRPCConnection
	if err := tx.Where("uuid = ?", connUUID).First(&conn).Error; err != nil {
		return "", err
	}
	if err := tx.Model(&conn).UpdateColumns(map[string]any{"config_version": gorm.Expr("config_version + 1"), "updated_at": time.Now()}).Error; err != nil {
		return "", err
	}
	return conn.HostUUID, bumpHostTx(tx, conn.HostUUID)
}
