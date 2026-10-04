package store

import (
	"errors"
	sqliteDriver "github.com/glebarez/go-sqlite"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

var (
	ErrLocked       = errors.New("control_locked")
	ErrLockLost     = errors.New("control_lock_lost")
	ErrInvalid      = errors.New("invalid_request")
	ErrNotFound     = errors.New("not_found")
	ErrConflict     = errors.New("conflict")
	ErrForbidden    = errors.New("forbidden")
	ErrUnauthorized = errors.New("unauthorized")
	errDatabase     = errors.New("database_error")
)

// 不保留原始驱动错误；SQLite RESTRICT 的扩展码不总被 GORM 翻译。
func safeError(err error) error {
	if err == nil {
		return nil
	}
	for _, known := range []error{ErrLocked, ErrLockLost, ErrInvalid, ErrNotFound, ErrConflict, ErrForbidden, ErrUnauthorized, errDatabase} {
		if errors.Is(err, known) {
			return known
		}
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) || errors.Is(err, gorm.ErrForeignKeyViolated) {
		return ErrConflict
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) && (pg.Code == "23505" || pg.Code == "23503" || pg.Code == "23514") {
		return ErrConflict
	}
	var sq *sqliteDriver.Error
	if errors.As(err, &sq) && sq.Code()&255 == 19 {
		return ErrConflict
	}
	return errDatabase
}
