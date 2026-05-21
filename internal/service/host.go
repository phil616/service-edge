package service

import (
	"time"

	"gorm.io/gorm"

	"github.com/dreamreflex/service-edge/internal/model"
	"github.com/dreamreflex/service-edge/internal/util"
)

// CreateFRPCHostInput creates a host (a machine that will run the frpc agent).
type CreateFRPCHostInput struct {
	Name       string `json:"name" binding:"required"`
	FrpVersion string `json:"frp_version"`
}

// UpdateFRPCHostInput updates mutable host fields.
type UpdateFRPCHostInput struct {
	Name       *string `json:"name"`
	FrpVersion *string `json:"frp_version"`
}

func (s *Service) ListFRPCHosts() ([]model.FRPCHost, error) {
	var hosts []model.FRPCHost
	if err := s.Store.DB.Order("id desc").Find(&hosts).Error; err != nil {
		return nil, err
	}
	// Attach connections (without proxies) so the UI can show counts/targets.
	for i := range hosts {
		conns, err := s.ListConnectionsOfHost(hosts[i].UUID)
		if err != nil {
			return nil, err
		}
		hosts[i].Connections = conns
	}
	return hosts, nil
}

func (s *Service) GetFRPCHost(uuid string) (*model.FRPCHost, error) {
	var host model.FRPCHost
	if err := s.Store.DB.Where("uuid = ?", uuid).First(&host).Error; err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	conns, err := s.ListConnectionsOfHost(uuid)
	if err != nil {
		return nil, err
	}
	for i := range conns {
		proxies, err := s.ListProxies(conns[i].UUID)
		if err != nil {
			return nil, err
		}
		conns[i].Proxies = proxies
	}
	host.Connections = conns
	return &host, nil
}

func (s *Service) CreateFRPCHost(in CreateFRPCHostInput) (*model.FRPCHost, error) {
	version := in.FrpVersion
	if version == "" {
		version = s.Cfg.FrpRelease.DefaultVersion
	}
	host := &model.FRPCHost{
		UUID:          util.NewUUID(),
		Name:          in.Name,
		FrpVersion:    version,
		ConfigVersion: 1,
		Status:        "pending",
	}
	if err := s.Store.DB.Create(host).Error; err != nil {
		return nil, err
	}
	return host, nil
}

func (s *Service) UpdateFRPCHost(uuid string, in UpdateFRPCHostInput) (*model.FRPCHost, error) {
	var host model.FRPCHost
	err := s.Store.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("uuid = ?", uuid).First(&host).Error; err != nil {
			if isNotFound(err) {
				return ErrNotFound
			}
			return err
		}
		if in.Name != nil {
			host.Name = *in.Name
		}
		if in.FrpVersion != nil {
			host.FrpVersion = *in.FrpVersion
		}
		// Changing the host's frp version re-renders every connection's binary ref.
		host.ConfigVersion++
		host.UpdatedAt = time.Now()
		return tx.Save(&host).Error
	})
	if err != nil {
		return nil, err
	}
	s.Notifier.Publish(uuid)
	return s.GetFRPCHost(uuid)
}

func (s *Service) DeleteFRPCHost(uuid string) error {
	return s.Store.DB.Transaction(func(tx *gorm.DB) error {
		var connUUIDs []string
		if err := tx.Model(&model.FRPCConnection{}).Where("host_uuid = ?", uuid).Pluck("uuid", &connUUIDs).Error; err != nil {
			return err
		}
		if len(connUUIDs) > 0 {
			if err := tx.Where("frpc_uuid IN ?", connUUIDs).Delete(&model.ProxyMapping{}).Error; err != nil {
				return err
			}
			if err := tx.Where("host_uuid = ?", uuid).Delete(&model.FRPCConnection{}).Error; err != nil {
				return err
			}
		}
		res := tx.Where("uuid = ?", uuid).Delete(&model.FRPCHost{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// bumpHost increments a host's aggregate config_version and wakes its long-poll.
func (s *Service) bumpHost(hostUUID string) {
	s.Store.DB.Model(&model.FRPCHost{}).Where("uuid = ?", hostUUID).
		UpdateColumns(map[string]any{"config_version": gorm.Expr("config_version + 1"), "updated_at": time.Now()})
	s.Notifier.Publish(hostUUID)
}
