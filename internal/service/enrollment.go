package service

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/dreamreflex/service-edge/internal/model"
	"github.com/dreamreflex/service-edge/internal/util"
)

// ErrEnrollmentInvalid is returned when a token is missing, expired or already used.
var ErrEnrollmentInvalid = errors.New("enrollment token invalid, expired or already used")

// CreateEnrollment issues a one-time install token for a target agent.
func (s *Service) CreateEnrollment(targetType, targetUUID string) (*model.EnrollmentToken, error) {
	tok := &model.EnrollmentToken{
		Token:      util.RandomToken(24),
		TargetType: targetType,
		TargetUUID: targetUUID,
		ExpiresAt:  time.Now().Add(s.Cfg.EnrollmentTokenTTL.Std()),
	}
	if err := s.Store.DB.Create(tok).Error; err != nil {
		return nil, err
	}
	return tok, nil
}

// PeekEnrollment validates a token without consuming it (for script download).
func (s *Service) PeekEnrollment(token string) (*model.EnrollmentToken, error) {
	var t model.EnrollmentToken
	if err := s.Store.DB.Where("token = ?", token).First(&t).Error; err != nil {
		if isNotFound(err) {
			return nil, ErrEnrollmentInvalid
		}
		return nil, err
	}
	if t.UsedAt != nil || time.Now().After(t.ExpiresAt) {
		return nil, ErrEnrollmentInvalid
	}
	return &t, nil
}

// ConsumeEnrollment atomically marks a token used. The UPDATE ... WHERE
// used_at IS NULL guarantees single consumption even under concurrency.
func (s *Service) ConsumeEnrollment(token, agentUUID, agentType string) (*model.EnrollmentToken, error) {
	var t model.EnrollmentToken
	err := s.Store.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("token = ?", token).First(&t).Error; err != nil {
			if isNotFound(err) {
				return ErrEnrollmentInvalid
			}
			return err
		}
		if time.Now().After(t.ExpiresAt) {
			return ErrEnrollmentInvalid
		}
		if t.TargetUUID != agentUUID || t.TargetType != agentType {
			return ErrEnrollmentInvalid
		}
		now := time.Now()
		res := tx.Model(&model.EnrollmentToken{}).
			Where("token = ? AND used_at IS NULL", token).
			Update("used_at", now)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrEnrollmentInvalid
		}
		t.UsedAt = &now
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &t, nil
}
