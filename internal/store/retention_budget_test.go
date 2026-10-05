package store

import (
	"context"
	"testing"
	"time"

	"mybuilds/internal/config"
)

func TestRetentionCandidatesMutexWaitDeadlineAndCancel(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 1, Days: 1}); err != nil {
			t.Fatal(err)
		}
		p, err := s.CreateProject(testContext, localAdmin, projectInput("budget"))
		if err != nil {
			t.Fatal(err)
		}
		for _, deadline := range []bool{false, true} {
			ctx, cancel := context.WithCancel(testContext)
			if deadline {
				ctx, cancel = context.WithTimeout(testContext, 30*time.Millisecond)
			}
			s.mu.Lock()
			done := make(chan error, 1)
			go func() { _, err := s.EvaluateRetention(ctx, localAdmin, p.ID, Page{}); done <- err }()
			if !deadline {
				cancel()
			}
			start := time.Now()
			select {
			case err = <-done:
			case <-time.After(300 * time.Millisecond):
				s.mu.Unlock()
				cancel()
				<-done
				t.Fatal("评估无限等待实际mutex")
			}
			s.mu.Unlock()
			cancel()
			expected := ErrRetentionCancelled
			if deadline {
				expected = ErrRetentionTimeout
			}
			if err != expected || time.Since(start) > 300*time.Millisecond {
				t.Fatal("排队未包括原期限", err)
			}
		}
	})
}
func TestRetentionCandidatesPostgresFinalLockUsesSameDeadline(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		if opt.Driver != "postgres" {
			t.Skip("SQLite无网络末尾锁查询，实际mutex/SQL入口同suite覆盖")
		}
		if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 1, Days: 1}); err != nil {
			t.Fatal(err)
		}
		p, err := s.CreateProject(testContext, localAdmin, projectInput("final-lock"))
		if err != nil {
			t.Fatal(err)
		}
		retentionSkipped(t, s, p, 2)
		var schema string
		if err = s.conn.QueryRowContext(testContext, "SELECT current_schema()").Scan(&schema); err != nil {
			t.Fatal(err)
		}
		for _, query := range []string{
			`CREATE SEQUENCE retention_lock_reads`,
			`CREATE FUNCTION delayed_retention_lock() RETURNS boolean LANGUAGE plpgsql AS $$ BEGIN IF nextval('retention_lock_reads') > 1 THEN PERFORM pg_sleep(1); END IF; RETURN true; END $$`,
			`CREATE VIEW pg_locks AS WITH boundary AS MATERIALIZED (SELECT delayed_retention_lock() AS ok) SELECT locks.* FROM pg_catalog.pg_locks AS locks CROSS JOIN boundary WHERE boundary.ok`,
		} {
			if err = s.writer.Exec(query).Error; err != nil {
				t.Fatal("自有schema有界等待", safeError(err))
			}
		}
		if _, err = s.conn.ExecContext(testContext, "SET search_path TO "+schema+",pg_catalog"); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(testContext, 200*time.Millisecond)
		defer cancel()
		start := time.Now()
		result, err := s.EvaluateRetention(ctx, localAdmin, p.ID, Page{})
		// 最后查询超期无法再次证明独占，原control_lock_lost优先于业务timeout。
		if err != ErrLockLost || result.Items != nil || time.Since(start) > 600*time.Millisecond {
			t.Fatal("末尾控制锁查询延长原SQL期限或泄漏未提交页", err)
		}
	})
}
