package store

import (
	"errors"
	sqliteDriver "github.com/glebarez/go-sqlite"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

var (
	ErrLocked                    = errors.New("control_locked")
	ErrLockLost                  = errors.New("control_lock_lost")
	ErrInvalid                   = errors.New("invalid_request")
	ErrNotFound                  = errors.New("not_found")
	ErrConflict                  = errors.New("conflict")
	ErrForbidden                 = errors.New("forbidden")
	ErrUnauthorized              = errors.New("unauthorized")
	errDatabase                  = errors.New("database_error")
	ErrNodeUnauthorized          = errors.New("node_unauthorized")
	ErrSessionConflict           = errors.New("session_conflict")
	ErrSessionExpired            = errors.New("session_expired")
	ErrLeaseInvalid              = errors.New("lease_invalid")
	ErrLeaseExpired              = errors.New("lease_expired")
	ErrEventConflict             = errors.New("event_conflict")
	ErrSequenceInvalid           = errors.New("sequence_invalid")
	ErrBudgetInvalid             = errors.New("budget_invalid")
	ErrStopUnconfirmed           = errors.New("stop_unconfirmed")
	ErrArtifactConflict          = errors.New("artifact_conflict")
	ErrLogConflict               = errors.New("log_conflict")
	ErrRetentionTimeout          = errors.New("retention_timeout")
	ErrRetentionCancelled        = errors.New("retention_cancelled")
	ErrRetentionInvalid          = errors.New("retention_invalid")
	ErrRetentionReadersActive    = errors.New("retention_readers_active")
	ErrRetentionOwnershipUnknown = errors.New("retention_ownership_unknown")
	ErrRetentionIO               = errors.New("retention_io_error")
	ErrRetentionRetired          = errors.New("retention_retired")
	ErrRetentionProtected        = errors.New("retention_protected")
	ErrRetentionObjectInvalid    = errors.New("retention_object_invalid")
	ErrRetentionLimit            = errors.New("retention_limit")
	ErrRetentionReceiptConflict  = errors.New("retention_receipt_conflict")
)

// 不保留原始驱动错误；SQLite RESTRICT 的扩展码不总被 GORM 翻译。
func safeError(err error) error {
	if err == nil {
		return nil
	}
	for _, known := range []error{ErrLocked, ErrLockLost, ErrInvalid, ErrNotFound, ErrConflict, ErrForbidden, ErrUnauthorized, errDatabase, ErrNodeUnauthorized, ErrSessionConflict, ErrSessionExpired, ErrLeaseInvalid, ErrLeaseExpired, ErrEventConflict, ErrSequenceInvalid, ErrStopUnconfirmed, ErrBudgetInvalid, ErrArtifactConflict, ErrLogConflict, ErrRetentionTimeout, ErrRetentionCancelled, ErrRetentionInvalid, ErrRetentionReadersActive, ErrRetentionOwnershipUnknown, ErrRetentionIO, ErrRetentionRetired, ErrRetentionProtected, ErrRetentionObjectInvalid, ErrRetentionLimit, ErrRetentionReceiptConflict} {
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
