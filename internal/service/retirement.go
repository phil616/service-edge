package service

import (
	"github.com/dreamreflex/service-edge/internal/model"
	"github.com/dreamreflex/service-edge/internal/protocol"
	"gorm.io/gorm"
	"time"
)

func (s *Service) AgentRetirement(kind, uuid string) (*model.AgentRetirement, error) {
	var row model.AgentRetirement
	result := s.Store.DB.Where("agent_type = ? AND uuid = ?", kind, uuid).Limit(1).Find(&row)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &row, nil
}

func retireAgentTx(tx *gorm.DB, kind, uuid string) error {
	var revision int
	res := tx.Model(modelFor(kind)).Where("uuid = ?", uuid).Select("config_version").Scan(&revision)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return tx.Create(&model.AgentRetirement{AgentType: kind, UUID: uuid, ConfigVersion: revision + 1}).Error
}

func (s *Service) RecordRetirementAck(kind, uuid string, req protocol.AckRequest) (bool, error) {
	row, err := s.AgentRetirement(kind, uuid)
	if err != nil || row == nil {
		return false, err
	}
	if req.ConfigVersion != row.ConfigVersion {
		return true, nil
	}
	updates := map[string]any{"last_error": req.Error}
	if req.Success {
		updates["completed_at"] = time.Now()
		updates["last_error"] = ""
	}
	// A delayed failure cannot erase a successful shutdown.
	err = s.Store.DB.Model(row).Where("completed_at IS NULL").Updates(updates).Error
	return true, err
}
