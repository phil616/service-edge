package service

import (
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm"

	"github.com/dreamreflex/service-edge/internal/model"
	"github.com/dreamreflex/service-edge/internal/util"
)

// CreateFRPSInput is the payload for creating an frps node.
type CreateFRPSInput struct {
	Name          string `json:"name" binding:"required"`
	BindPort      int    `json:"bind_port" binding:"required"`
	DashboardPort *int   `json:"dashboard_port"`
	DashboardUser string `json:"dashboard_user"`
	DashboardPwd  string `json:"dashboard_pwd"`
	FrpVersion    string `json:"frp_version"`
	PublicIP      string `json:"public_ip"`
}

// UpdateFRPSInput is the payload for updating an frps node.
type UpdateFRPSInput struct {
	Name          *string `json:"name"`
	BindPort      *int    `json:"bind_port"`
	DashboardPort *int    `json:"dashboard_port"`
	DashboardUser *string `json:"dashboard_user"`
	DashboardPwd  *string `json:"dashboard_pwd"`
	FrpVersion    *string `json:"frp_version"`
	PublicIP      *string `json:"public_ip"`
}

func (s *Service) ListFRPS() ([]model.FRPSNode, error) {
	var nodes []model.FRPSNode
	if err := s.Store.DB.Order("id desc").Find(&nodes).Error; err != nil {
		return nil, err
	}
	return nodes, nil
}

func (s *Service) GetFRPS(uuid string) (*model.FRPSNode, error) {
	var node model.FRPSNode
	if err := s.Store.DB.Where("uuid = ?", uuid).First(&node).Error; err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &node, nil
}

func (s *Service) CreateFRPS(in CreateFRPSInput) (*model.FRPSNode, error) {
	version := in.FrpVersion
	if version == "" {
		version = s.Cfg.FrpRelease.DefaultVersion
	}
	uuid := util.NewUUID()

	// Sign the server cert (CN frps-<uuid>) including any known public IP as SAN.
	sans := []string{"frps-" + uuid}
	if in.PublicIP != "" {
		sans = append(sans, in.PublicIP)
	}
	cert, err := s.CA.IssueServerCert(uuid, sans)
	if err != nil {
		return nil, fmt.Errorf("issue server cert: %w", err)
	}

	node := &model.FRPSNode{
		UUID:          uuid,
		Name:          in.Name,
		BindPort:      in.BindPort,
		DashboardPort: in.DashboardPort,
		DashboardUser: in.DashboardUser,
		DashboardPwd:  in.DashboardPwd,
		FrpToken:      util.RandomToken(32), // 64 hex chars
		TLSCert:       cert.CertPEM,
		TLSKey:        cert.KeyPEM,
		FrpVersion:    version,
		ConfigVersion: 1,
		Status:        "pending",
		PublicIP:      in.PublicIP,
	}
	if err := s.Store.DB.Create(node).Error; err != nil {
		return nil, err
	}
	return node, nil
}

func (s *Service) UpdateFRPS(uuid string, in UpdateFRPSInput) (*model.FRPSNode, error) {
	var node *model.FRPSNode
	err := s.Store.DB.Transaction(func(tx *gorm.DB) error {
		var n model.FRPSNode
		if err := tx.Where("uuid = ?", uuid).First(&n).Error; err != nil {
			if isNotFound(err) {
				return ErrNotFound
			}
			return err
		}
		if in.Name != nil {
			n.Name = *in.Name
		}
		if in.BindPort != nil {
			n.BindPort = *in.BindPort
		}
		if in.DashboardPort != nil {
			n.DashboardPort = in.DashboardPort
		}
		if in.DashboardUser != nil {
			n.DashboardUser = *in.DashboardUser
		}
		if in.DashboardPwd != nil {
			n.DashboardPwd = *in.DashboardPwd
		}
		if in.FrpVersion != nil {
			n.FrpVersion = *in.FrpVersion
		}
		if in.PublicIP != nil {
			n.PublicIP = *in.PublicIP
		}
		n.ConfigVersion++
		n.UpdatedAt = time.Now()
		if err := tx.Save(&n).Error; err != nil {
			return err
		}
		node = &n
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Changing the frps config also affects every frpc connected to it.
	s.bumpClientsOf(uuid)
	s.Notifier.Publish(uuid)
	return node, nil
}

func (s *Service) DeleteFRPS(uuid string) error {
	return s.Store.DB.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&model.FRPCClient{}).Where("frps_uuid = ?", uuid).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("%w: %d frpc client(s) still reference this node", ErrConflict, count)
		}
		res := tx.Where("uuid = ?", uuid).Delete(&model.FRPSNode{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// UsedRemotePorts returns the set of remote_port values already taken on the
// given frps (across all connected frpc clients), plus the bind/dashboard port.
func (s *Service) UsedRemotePorts(frpsUUID string) (map[int]bool, error) {
	node, err := s.GetFRPS(frpsUUID)
	if err != nil {
		return nil, err
	}
	used := map[int]bool{node.BindPort: true}
	if node.DashboardPort != nil {
		used[*node.DashboardPort] = true
	}

	var clientUUIDs []string
	if err := s.Store.DB.Model(&model.FRPCClient{}).Where("frps_uuid = ?", frpsUUID).Pluck("uuid", &clientUUIDs).Error; err != nil {
		return nil, err
	}
	if len(clientUUIDs) > 0 {
		var ports []int
		if err := s.Store.DB.Model(&model.ProxyMapping{}).
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

// HostOccupiedPorts returns ports the frps host reports as bound that are NOT
// assigned by service-edge — i.e. occupied by external processes. These cannot
// be used as remote_port; the control plane learns them only from agent reports.
func (s *Service) HostOccupiedPorts(frpsUUID string) ([]int, error) {
	node, err := s.GetFRPS(frpsUUID)
	if err != nil {
		return nil, err
	}
	used, err := s.UsedRemotePorts(frpsUUID)
	if err != nil {
		return nil, err
	}
	ext := externalPorts(*node, used)
	out := make([]int, 0, len(ext))
	for p := range ext {
		out = append(out, p)
	}
	sort.Ints(out)
	return out, nil
}

// bumpClientsOf increments config_version for all frpc clients of an frps and
// wakes their long-polls.
func (s *Service) bumpClientsOf(frpsUUID string) {
	var uuids []string
	if err := s.Store.DB.Model(&model.FRPCClient{}).Where("frps_uuid = ?", frpsUUID).Pluck("uuid", &uuids).Error; err != nil {
		return
	}
	if len(uuids) == 0 {
		return
	}
	s.Store.DB.Model(&model.FRPCClient{}).Where("frps_uuid = ?", frpsUUID).
		UpdateColumns(map[string]any{"config_version": gorm.Expr("config_version + 1"), "updated_at": time.Now()})
	for _, u := range uuids {
		s.Notifier.Publish(u)
	}
}
