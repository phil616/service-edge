package service

import (
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/dreamreflex/service-edge/internal/model"
)

// Setting keys persisted in the settings table.
const (
	SettingAgentDownloadFRPS = "agent_download_url_frps"
	SettingAgentDownloadFRPC = "agent_download_url_frpc"
)

// GetSetting returns the stored value for a key ("" if unset).
func (s *Service) GetSetting(key string) string {
	var row model.Setting
	if err := s.Store.DB.Where("key = ?", key).First(&row).Error; err != nil {
		return ""
	}
	return row.Value
}

// SetSetting upserts a setting; an empty value deletes the row so the built-in
// default applies again.
func (s *Service) SetSetting(key, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return s.Store.DB.Where("key = ?", key).Delete(&model.Setting{}).Error
	}
	row := model.Setting{Key: key, Value: value, UpdatedAt: time.Now()}
	return s.Store.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&row).Error
}

// AgentDownloadURL returns the effective agent download base for an agent type:
// the web-configured override if set, otherwise the control-plane default from
// config. The install script appends the per-arch suffix to this base.
func (s *Service) AgentDownloadURL(agentType string) string {
	key := SettingAgentDownloadFRPC
	if agentType == "frps" {
		key = SettingAgentDownloadFRPS
	}
	if v := s.GetSetting(key); v != "" {
		return v
	}
	return s.Cfg.AgentDownloadBase
}

// AgentDownloadSettings is the settings view returned to the web UI.
type AgentDownloadSettings struct {
	ControlPlaneBase    string `json:"control_plane_base"`
	FRPSAgentDownloadURL string `json:"agent_download_url_frps"`
	FRPCAgentDownloadURL string `json:"agent_download_url_frpc"`
}

// GetAgentDownloadSettings returns the control-plane default plus any overrides.
func (s *Service) GetAgentDownloadSettings() AgentDownloadSettings {
	return AgentDownloadSettings{
		ControlPlaneBase:     s.Cfg.AgentDownloadBase,
		FRPSAgentDownloadURL: s.GetSetting(SettingAgentDownloadFRPS),
		FRPCAgentDownloadURL: s.GetSetting(SettingAgentDownloadFRPC),
	}
}

// UpdateAgentDownloadSettings persists the per-type overrides (empty clears).
func (s *Service) UpdateAgentDownloadSettings(frps, frpc string) error {
	return s.Store.DB.Transaction(func(tx *gorm.DB) error {
		// Reuse SetSetting semantics within the tx via the same store DB.
		if err := setSettingTx(tx, SettingAgentDownloadFRPS, frps); err != nil {
			return err
		}
		return setSettingTx(tx, SettingAgentDownloadFRPC, frpc)
	})
}

func setSettingTx(tx *gorm.DB, key, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return tx.Where("key = ?", key).Delete(&model.Setting{}).Error
	}
	row := model.Setting{Key: key, Value: value, UpdatedAt: time.Now()}
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&row).Error
}
