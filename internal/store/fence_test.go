//go:build darwin || linux

package store

import (
	"github.com/google/uuid"
	"mybuilds/internal/protocol"
	"os"
	"strings"
	"testing"
	"time"
)

func TestAllNodeWritesRejectLostControlLock(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		actor, grant, policy := claimed(t, s)
		if opt.Driver == "sqlite" {
			if err := os.Rename(opt.DSN+".lock", opt.DSN+".lock.old"); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(opt.DSN+".lock", nil, 0600); err != nil {
				t.Fatal(err)
			}
		} else {
			var pid int
			if err := s.conn.QueryRowContext(testContext, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
				t.Fatal(err)
			}
			pool, _ := s.db.DB()
			if _, err := pool.ExecContext(testContext, "SELECT pg_terminate_backend($1)", pid); err != nil {
				t.Fatal(err)
			}
		}
		request := protocol.ClaimRequest{SessionID: grant.Ref.SessionID, ClaimKey: uuid.NewString()}
		declaration := protocol.ArtifactDeclaration{Ref: grant.Ref, ID: uuid.NewString(), Seq: 1, Phase: "ordinary", Step: "compile", Index: 1, Name: "app.apk", Size: 0, SHA256: strings.Repeat("a", 64)}
		operations := []func() error{
			func() error { _, err := s.Claim(testContext, actor, request, policy); return err },
			func() error { _, err := s.Renew(testContext, actor, grant.Ref, policy); return err },
			func() error {
				_, err := s.ApplyEvent(testContext, actor, event(grant.Ref, 1, stepProgress("intent", "", "ordinary", "compile", 1)))
				return err
			},
			func() error {
				_, err := s.CommitLogChunk(testContext, actor, LogCommit{Ref: grant.Ref, Seq: 1, Size: 10, RecordCount: 1, Digest: strings.Repeat("a", 64), StorageID: uuid.NewString()})
				return err
			},
			func() error {
				_, err := s.CommitArtifact(testContext, actor, ArtifactCommit{Declaration: declaration, StorageID: uuid.NewString()})
				return err
			},
			func() error { return s.ExpireLeases(testContext) },
			func() error { _, err := s.Cancel(testContext, localAdmin, grant.Ref.BuildID); return err },
			func() error {
				_, err := s.Heartbeat(testContext, actor, protocol.HeartbeatRequest{SessionID: grant.Ref.SessionID, Report: healthyReport()}, policy)
				return err
			},
			func() error {
				return s.ConfirmNodeStopped(testContext, actor, protocol.StopConfirmation{Ref: grant.Ref, EvidenceCode: "process_group_reaped", Note: "实际停止证据"})
			},
			func() error { return s.CheckExecution(testContext, actor, grant.Ref) },
		}
		for i, operation := range operations {
			if err := operation(); err != ErrLockLost {
				t.Fatalf("operation %d after lock loss: %v", i, err)
			}
		}
		var row buildRecord
		if err := s.db.First(&row, "id = ?", grant.Ref.BuildID).Error; err != nil || row.Status != "running" || row.LastEventSeq != 0 || row.LastLogSeq != 0 || row.LastArtifactSeq != 0 || row.CancelRequested {
			t.Fatal("mutation committed after loss", err)
		}
		var count int64
		if err := s.db.Model(&attemptRecord{}).Count(&count).Error; err != nil || count != 1 {
			t.Fatal("claim after loss", count, err)
		}
	})
}
func TestFiniteBudgetCannotGrowOrBecomeUnlimited(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		actor, session, p, policy := leaseFixture(t, s)
		in := enqueueInput(p, "finite")
		in.Builds[0].Snapshot.Definition.Timeout = "1s"
		initial := int64(time.Second)
		in.Builds[0].InitialBudgetNS = &initial
		if _, err := s.Enqueue(testContext, in); err != nil {
			t.Fatal(err)
		}
		grant, err := s.Claim(testContext, actor, protocol.ClaimRequest{SessionID: session.SessionID, ClaimKey: uuid.NewString()}, policy)
		if err != nil || grant == nil {
			t.Fatal(err)
		}
		progress := stepProgress("intent", "", "ordinary", "compile", 1)
		if _, err = s.ApplyEvent(testContext, actor, event(grant.Ref, 1, progress)); err != ErrBudgetInvalid {
			t.Fatal("finite became unlimited", err)
		}
		grow := initial + 1
		progress.RemainingBudgetNS = &grow
		if _, err = s.ApplyEvent(testContext, actor, event(grant.Ref, 1, progress)); err != ErrBudgetInvalid {
			t.Fatal("budget grew", err)
		}
		zero := int64(0)
		progress.RemainingBudgetNS = &zero
		accept(t, s, actor, event(grant.Ref, 1, progress))
		progress.Kind = "started"
		progress.Started = true
		progress.RemainingBudgetNS = &initial
		if _, err = s.ApplyEvent(testContext, actor, event(grant.Ref, 2, progress)); err != ErrBudgetInvalid {
			t.Fatal("exhausted budget reset", err)
		}
		view, err := s.GetBuild(testContext, grant.Ref.BuildID)
		if err != nil || view.RemainingBudgetNS == nil || *view.RemainingBudgetNS != 0 {
			t.Fatal("zero budget not persisted", err)
		}
	})
}

func TestPostgresFinalLockQueryCannotExtendExpiredOriginalLease(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		if opt.Driver != "postgres" {
			t.Skip("最终网络锁查询只存在PostgreSQL；SQLite另用真实SQL触发器覆盖")
		}
		actor, grant, policy := claimed(t, s)
		var schema string
		if err := s.conn.QueryRowContext(testContext, "SELECT current_schema()").Scan(&schema); err != nil {
			t.Fatal(err)
		}
		// 只在本用例私有schema创建透传真实pg_catalog.pg_locks的view。
		// 入口读取原短租约不延迟；Renew写入新租约后的最终锁查询才延迟一次。
		if err := s.writer.Exec(`CREATE FUNCTION delayed_lock_boundary() RETURNS boolean LANGUAGE plpgsql AS $$ BEGIN IF EXISTS (SELECT 1 FROM builds WHERE lease_expires_at > clock_timestamp() + interval '5 seconds') THEN PERFORM pg_sleep(0.3); END IF; RETURN true; END $$`).Error; err != nil {
			t.Fatal(err)
		}
		if err := s.writer.Exec(`CREATE VIEW pg_locks AS WITH boundary AS MATERIALIZED (SELECT delayed_lock_boundary() AS ok) SELECT locks.* FROM pg_catalog.pg_locks AS locks CROSS JOIN boundary WHERE boundary.ok`).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := s.conn.ExecContext(testContext, "SET search_path TO "+schema+",pg_catalog"); err != nil {
			t.Fatal(err)
		}
		original := time.Now().UTC().Add(200 * time.Millisecond)
		if err := s.writer.Model(&buildRecord{}).Where("id = ?", grant.Ref.BuildID).Update("lease_expires_at", original).Error; err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		if _, err := s.Renew(testContext, actor, grant.Ref, policy); err != ErrLeaseExpired {
			t.Fatal("final lock query revived old expiry", err)
		}
		if time.Since(start) < 300*time.Millisecond {
			t.Fatal("fixture did not delay final check")
		}
		var row buildRecord
		if err := s.db.First(&row, "id = ?", grant.Ref.BuildID).Error; err != nil || row.LeaseExpiresAt == nil || !row.LeaseExpiresAt.Equal(original) {
			t.Fatal("late lock query renewal committed", err)
		}
		if err := s.CheckLock(testContext); err != nil {
			t.Fatal("expiry incorrectly marked lock lost", err)
		}
		if s.transactionExpiry != nil {
			t.Fatal("deadline leaked to next transaction")
		}
		if _, err := s.CreateGroup(testContext, localAdmin, "after-expired-transaction"); err != nil {
			t.Fatal("expired deadline blocked unrelated management", err)
		}
	})
}
