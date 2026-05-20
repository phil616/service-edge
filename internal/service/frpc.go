package service

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/dreamreflex/service-edge/internal/model"
	"github.com/dreamreflex/service-edge/internal/util"
)

// CreateFRPCInput is the payload for creating an frpc client with its proxies.
type CreateFRPCInput struct {
	Name       string             `json:"name" binding:"required"`
	FRPSUUID   string             `json:"frps_uuid" binding:"required"`
	FrpVersion string             `json:"frp_version"`
	Proxies    []ProxyMappingInput `json:"proxies"`
}

// UpdateFRPCInput updates mutable frpc fields.
type UpdateFRPCInput struct {
	Name       *string `json:"name"`
	FrpVersion *string `json:"frp_version"`
}

func (s *Service) ListFRPC() ([]model.FRPCClient, error) {
	var clients []model.FRPCClient
	if err := s.Store.DB.Order("id desc").Find(&clients).Error; err != nil {
		return nil, err
	}
	return clients, nil
}

func (s *Service) GetFRPC(uuid string) (*model.FRPCClient, error) {
	var client model.FRPCClient
	if err := s.Store.DB.Where("uuid = ?", uuid).First(&client).Error; err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	proxies, err := s.ListProxies(uuid)
	if err != nil {
		return nil, err
	}
	client.Proxies = proxies
	return &client, nil
}

func (s *Service) CreateFRPC(in CreateFRPCInput) (*model.FRPCClient, error) {
	version := in.FrpVersion

	var client *model.FRPCClient
	err := s.Store.DB.Transaction(func(tx *gorm.DB) error {
		// Target frps must exist.
		var node model.FRPSNode
		if err := tx.Where("uuid = ?", in.FRPSUUID).First(&node).Error; err != nil {
			if isNotFound(err) {
				return fmt.Errorf("%w: target frps %s not found", ErrNotFound, in.FRPSUUID)
			}
			return err
		}
		if version == "" {
			version = node.FrpVersion
		}

		uuid := util.NewUUID()
		cert, err := s.CA.IssueClientCert(uuid)
		if err != nil {
			return fmt.Errorf("issue client cert: %w", err)
		}
		c := &model.FRPCClient{
			UUID:          uuid,
			Name:          in.Name,
			FRPSUUID:      in.FRPSUUID,
			TLSCert:       cert.CertPEM,
			TLSKey:        cert.KeyPEM,
			FrpVersion:    version,
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
			// Mark inactive if the host already has this remote_port bound.
			setHostOccupancy(&row, externalPorts(node, used))
			if pin.RemotePort != nil {
				used[*pin.RemotePort] = true
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		client = c
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetFRPC(client.UUID)
}

func (s *Service) UpdateFRPC(uuid string, in UpdateFRPCInput) (*model.FRPCClient, error) {
	err := s.Store.DB.Transaction(func(tx *gorm.DB) error {
		var c model.FRPCClient
		if err := tx.Where("uuid = ?", uuid).First(&c).Error; err != nil {
			if isNotFound(err) {
				return ErrNotFound
			}
			return err
		}
		if in.Name != nil {
			c.Name = *in.Name
		}
		if in.FrpVersion != nil {
			c.FrpVersion = *in.FrpVersion
		}
		c.ConfigVersion++
		c.UpdatedAt = time.Now()
		return tx.Save(&c).Error
	})
	if err != nil {
		return nil, err
	}
	s.Notifier.Publish(uuid)
	return s.GetFRPC(uuid)
}

func (s *Service) DeleteFRPC(uuid string) error {
	return s.Store.DB.Transaction(func(tx *gorm.DB) error {
		res := tx.Where("uuid = ?", uuid).Delete(&model.FRPCClient{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return tx.Where("frpc_uuid = ?", uuid).Delete(&model.ProxyMapping{}).Error
	})
}

// bumpFRPC increments an frpc's config_version and wakes its long-poll.
func (s *Service) bumpFRPC(uuid string) {
	s.Store.DB.Model(&model.FRPCClient{}).Where("uuid = ?", uuid).
		UpdateColumns(map[string]any{"config_version": gorm.Expr("config_version + 1"), "updated_at": time.Now()})
	s.Notifier.Publish(uuid)
}
