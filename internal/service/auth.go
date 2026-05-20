package service

import (
	"errors"
	"log/slog"

	"golang.org/x/crypto/bcrypt"

	"github.com/dreamreflex/service-edge/internal/model"
)

// ErrInvalidCredentials is returned on a failed login.
var ErrInvalidCredentials = errors.New("invalid username or password")

// BootstrapAdmin creates the initial admin user if no users exist.
func (s *Service) BootstrapAdmin(username, password string) error {
	if username == "" || password == "" {
		return nil
	}
	var count int64
	if err := s.Store.DB.Model(&model.User{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := s.Store.DB.Create(&model.User{Username: username, PasswordHash: string(hash)}).Error; err != nil {
		return err
	}
	slog.Info("bootstrap admin user created", "username", username)
	return nil
}

// Login verifies credentials and returns the user on success.
func (s *Service) Login(username, password string) (*model.User, error) {
	var u model.User
	if err := s.Store.DB.Where("username = ?", username).First(&u).Error; err != nil {
		return nil, ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return nil, ErrInvalidCredentials
	}
	return &u, nil
}

// GetUser fetches a user by id.
func (s *Service) GetUser(id uint) (*model.User, error) {
	var u model.User
	if err := s.Store.DB.First(&u, id).Error; err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}
