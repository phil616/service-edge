package service

import (
	"errors"

	"gorm.io/gorm"

	"github.com/dreamreflex/service-edge/internal/config"
	"github.com/dreamreflex/service-edge/internal/pki"
	"github.com/dreamreflex/service-edge/internal/store"
)

// ErrNotFound is returned when a requested resource does not exist.
var ErrNotFound = errors.New("not found")

// ErrConflict signals a uniqueness/occupancy violation (e.g. port in use).
var ErrConflict = errors.New("conflict")

// Service holds the business logic dependencies.
type Service struct {
	Store    *store.Store
	CA       *pki.CA
	Cfg      *config.Config
	Notifier *Notifier
}

func New(st *store.Store, ca *pki.CA, cfg *config.Config) *Service {
	return &Service{Store: st, CA: ca, Cfg: cfg, Notifier: NewNotifier()}
}

func isNotFound(err error) bool { return errors.Is(err, gorm.ErrRecordNotFound) }
