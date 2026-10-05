package store

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type Store struct {
	db, writer                *gorm.DB
	conn                      *sql.Conn
	lockFile                  *os.File
	lockPath, dbPath          string
	lockInfo, dbInfo          os.FileInfo
	driver                    string
	mu                        sync.Mutex
	lost, closed              bool
	transactionExpiry         *time.Time
	transactionReportDeadline *time.Time
	evidenceReadOwner         string
}

func Open(ctx context.Context, opt Options) (*Store, error) {
	if !supportedPlatform() {
		return nil, ErrInvalid
	}
	if opt.Driver != "sqlite" && opt.Driver != "postgres" {
		return nil, ErrInvalid
	}
	if ctx.Err() != nil {
		return nil, errDatabase
	}
	s := &Store{driver: opt.Driver, evidenceReadOwner: uuid.NewString()}
	cfg := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), NowFunc: func() time.Time { return time.Now().UTC() }, DisableAutomaticPing: true}
	var err error
	if opt.Driver == "sqlite" {
		if err = s.openSQLiteLock(opt.DSN); err != nil {
			return nil, err
		}
		dsn := (&url.URL{Scheme: "file", Path: s.dbPath}).String() + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)"
		s.db, err = gorm.Open(sqlite.Open(dsn), cfg)
		s.writer = s.db
	} else {
		if strings.TrimSpace(opt.DSN) == "" {
			return nil, ErrInvalid
		}
		parsed, parseErr := postgresConfig(ctx, opt.DSN)
		if parseErr != nil {
			return nil, parseErr
		}
		pgPool := stdlib.OpenDB(*parsed)
		s.db, err = gorm.Open(postgres.New(postgres.Config{Conn: pgPool}), cfg)
		if err != nil {
			pgPool.Close()
		}
		if err == nil {
			var pool *sql.DB
			pool, err = s.db.DB()
			if err == nil {
				s.conn, err = pool.Conn(ctx)
			}
			if err == nil {
				var acquired bool
				err = s.conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(1973481521, 6)").Scan(&acquired)
				if err == nil && !acquired {
					err = ErrLocked
				}
			}
			if err == nil {
				s.writer, err = gorm.Open(postgres.New(postgres.Config{Conn: s.conn}), cfg)
			}
		}
	}
	if err != nil {
		s.Close()
		return nil, safeError(err)
	}
	pool, err := s.db.DB()
	if err != nil {
		s.Close()
		return nil, safeError(err)
	}
	pool.SetMaxOpenConns(8)
	pool.SetMaxIdleConns(8)
	if err = pool.PingContext(ctx); err != nil {
		s.Close()
		return nil, safeError(err)
	}
	return s, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	var failed bool
	if s.conn != nil {
		// 同一 session 释放；失效时不尝试从连接池恢复运行权。
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		if !s.lost {
			if _, err := s.conn.ExecContext(ctx, "SELECT pg_advisory_unlock(1973481521, 6)"); err != nil {
				failed = true
			}
		}
		cancel()
		if err := s.conn.Close(); err != nil && !s.lost {
			failed = true
		}
	}
	if s.db != nil {
		if pool, err := s.db.DB(); err != nil {
			failed = true
		} else if err = pool.Close(); err != nil {
			failed = true
		}
	}
	if s.lockFile != nil {
		if err := releaseFileLock(s.lockFile); err != nil {
			failed = true
		}
	}
	if failed {
		return errDatabase
	}
	return nil
}

func (s *Store) CheckLock(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.checkLock(ctx)
}
func (s *Store) checkLock(ctx context.Context) error {
	if s.lost || s.closed {
		return ErrLockLost
	}
	if ctx.Err() != nil {
		return errDatabase
	}
	valid := false
	if s.driver == "sqlite" {
		lock, err := os.Lstat(s.lockPath)
		if err == nil && lock.Mode().IsRegular() && os.SameFile(s.lockInfo, lock) && singleLink(lock) {
			db, err := os.Lstat(s.dbPath)
			valid = err == nil && db.Mode().IsRegular() && os.SameFile(s.dbInfo, db) && singleLink(db)
		}
	} else {
		err := s.conn.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM pg_locks WHERE locktype = 'advisory' AND pid = pg_backend_pid() AND classid = 1973481521 AND objid = 6 AND objsubid = 2 AND granted)").Scan(&valid)
		if err != nil {
			valid = false
		}
	}
	if !valid {
		s.lost = true
		return ErrLockLost
	}
	return nil
}

// ponytail: 单控制端写事务串行；需要吞吐扩展时再评估更细的事务竞争。
func (s *Store) write(ctx context.Context, fn func(*gorm.DB) error) error {
	// 排队也沿调用方原期限；不能先无限等mutex再检查已取消的context。
	for !s.mu.TryLock() {
		wait := time.NewTimer(time.Millisecond)
		select {
		case <-ctx.Done():
			wait.Stop()
			return errDatabase
		case <-wait.C:
		}
	}
	defer s.mu.Unlock()
	s.transactionExpiry = nil
	s.transactionReportDeadline = nil
	defer func() {
		s.transactionExpiry = nil
		s.transactionReportDeadline = nil
	}()
	if err := s.checkLock(ctx); err != nil {
		return err
	}
	err := s.writer.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := fn(tx); err != nil {
			return err
		}
		if err := s.checkLock(ctx); err != nil {
			return err
		}
		// 最后一次锁查询也可能耗尽原执行权；在COMMIT前再比较捕获值。
		if s.transactionExpiry != nil && !time.Now().UTC().Before(*s.transactionExpiry) {
			return ErrLeaseExpired
		}
		if s.transactionReportDeadline != nil && !time.Now().UTC().Before(*s.transactionReportDeadline) {
			return ErrBudgetInvalid
		}
		return nil
	})
	if err != nil && s.driver == "postgres" && ctx.Err() == nil {
		// SQL失败后的独占复核不能借新期限延长原请求。
		checkCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		if s.checkLock(checkCtx) == ErrLockLost {
			return ErrLockLost
		}
	}
	return safeError(err)
}

func (s *Store) Migrate(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkLock(ctx); err != nil {
		return err
	}
	db := s.writer.WithContext(ctx)
	// SQLite 可直接添加默认NULL的自关联列，避免重建已有attempt/receipt引用的父表。
	if s.driver == "sqlite" && db.Migrator().HasTable(&buildRecord{}) && !db.Migrator().HasColumn(&buildRecord{}, "RetryOf") {
		if err := db.Exec("ALTER TABLE builds ADD COLUMN retry_of TEXT CONSTRAINT fk_builds_retry_original REFERENCES builds(id) ON DELETE RESTRICT").Error; err != nil {
			return safeError(err)
		}
	}
	if err := db.AutoMigrate(&groupRecord{}, &projectRecord{}, &identityRecord{}, &metadataRecord{}, &auditRecord{}, &batchRecord{}, &buildRecord{}, &stepRecord{}, &requestRecord{}, &nodeRecord{}, &nodeCredentialRecord{}, &nodeSessionRecord{}, &attemptRecord{}, &executionReceiptRecord{}, &stopConfirmationRecord{}, &logChunkRecord{}, &artifactRecord{}, &retentionPolicyRecord{}, &retentionJobRecord{}, &retentionObjectRecord{}, &evidenceReadRecord{}, &nodeResourceRecord{}, &nodeDeletionRecord{}, &nodeDeletionReceiptRecord{}, &applicationRecord{}, &publishIntentRecord{}, &applicationGuardRecord{}, &publishQueryRecord{}, &publishDecisionRecord{}); err != nil {
		return safeError(err)
	}
	if err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS build_active_name ON builds(project_id,name) WHERE status = 'running' OR stop_unconfirmed = true").Error; err != nil {
		return safeError(err)
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := backfillTerminalTimes(tx); err != nil {
			return err
		}
		group := groupRecord{ID: "default", Name: "default"}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&group).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&metadataRecord{ID: 1}).Error; err != nil {
			return err
		}
		return s.checkLock(ctx)
	})
	return safeError(err)
}

func normalizeSQLite(path string) (string, error) {
	if path == "" || path == ":memory:" || strings.HasPrefix(path, "file:") || strings.ContainsAny(path, "?\x00") {
		return "", ErrInvalid
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", ErrInvalid
	}
	if err = os.MkdirAll(filepath.Dir(abs), 0700); err != nil {
		return "", errDatabase
	}
	if resolved, e := filepath.EvalSymlinks(abs); e == nil {
		abs = resolved
	} else if !os.IsNotExist(e) {
		return "", ErrInvalid
	} else {
		parent, e := filepath.EvalSymlinks(filepath.Dir(abs))
		if e != nil {
			return "", ErrInvalid
		}
		abs = filepath.Join(parent, filepath.Base(abs))
	}
	return abs, nil
}

// 仅从已有精确终态事实回填；未知中断不借更新时间或租约过期猜测。
func backfillTerminalTimes(tx *gorm.DB) error {
	receipt := `SELECT r.created_at FROM execution_receipts r JOIN attempts a ON a.id = r.attempt_id
 WHERE r.build_id = builds.id AND r.attempt_id = builds.attempt_id
 AND r.seq = builds.last_event_seq AND r.seq > 0 AND r.kind = 'build_finished'
 AND a.build_id = builds.id AND a.node_id = builds.node_id AND a.session_id = builds.session_id
 AND a.lease_id = builds.lease_id AND a.epoch = builds.lease_epoch`
	if err := tx.Exec(`UPDATE builds SET terminal_at = (` + receipt + `)
 WHERE terminal_at IS NULL AND status IN ('succeeded','failed','cancelled','skipped','interrupted')
 AND EXISTS (` + receipt + `)`).Error; err != nil {
		return err
	}
	if err := tx.Exec(`UPDATE builds SET terminal_at = created_at
 WHERE terminal_at IS NULL AND status = 'skipped' AND attempt_id IS NULL
 AND NOT EXISTS (SELECT 1 FROM attempts a WHERE a.build_id = builds.id)`).Error; err != nil {
		return err
	}
	cancel := `SELECT MIN(a.created_at) FROM audits a
 WHERE a.action = 'build_cancel' AND a.object_id = builds.id AND a."to" = 'cancelled'`
	return tx.Exec(`UPDATE builds SET terminal_at = (` + cancel + `)
 WHERE terminal_at IS NULL AND status = 'cancelled' AND attempt_id IS NULL
 AND NOT EXISTS (SELECT 1 FROM attempts a WHERE a.build_id = builds.id)`).Error
}
