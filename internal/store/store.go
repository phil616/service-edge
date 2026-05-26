package store

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/dreamreflex/service-edge/internal/model"
)

// Store wraps the GORM database handle.
type Store struct {
	DB *gorm.DB
}

// Open opens (creating if needed) the SQLite database, runs migrations and
// returns a ready-to-use Store.
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}

	// busy_timeout + WAL keep concurrent long-poll readers from blocking writes.
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// SQLite allows only one writer at a time. Serializing all access through a
	// single connection (combined with WAL for read snapshots) removes the
	// SQLITE_BUSY errors that a multi-connection pool produces under concurrent
	// writes from many agents (heartbeats + status + acks) plus UI traffic. All
	// queries here are sub-millisecond, so the throughput cost is negligible.
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql db: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)

	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	slog.Info("database ready", "path", path)
	return &Store{DB: db}, nil
}

// Audit writes an audit log row. Failures are logged but never block the caller.
func (s *Store) Audit(userID *uint, action, targetType, targetUUID, detail, ip string) {
	entry := &model.AuditLog{
		UserID:     userID,
		Action:     action,
		TargetType: targetType,
		TargetUUID: targetUUID,
		Detail:     detail,
		IP:         ip,
		CreatedAt:  time.Now(),
	}
	if err := s.DB.Create(entry).Error; err != nil {
		slog.Warn("failed to write audit log", "action", action, "err", err)
	}
}
