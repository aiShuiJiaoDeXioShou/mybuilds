package store

import (
	"testing"
	"time"

	"mybuilds/internal/protocol"
)

func finalReportAPI(t *testing.T, s *Store) (NodeActor, protocol.LeaseGrant, protocol.ReportEvidence, ArtifactCommit) {
	t.Helper()
	a, g := queuedReports(t, s, true, []string{"results/*.xml"}, false)
	reportFinished(t, s, a, g)
	e, parsed := actualReport(t, `<testsuite><testcase/></testsuite>`)
	accept(t, s, a, checkedEvent(g, 4, &e, false))
	e.Revision = 2
	e.Outcome = "passed"
	accept(t, s, a, checkedEvent(g, 5, &e, true))
	return a, g, e, reportCommit(g, e, &parsed)
}
func TestReportsCommitRealSQLFailureIsInvisibleAndCanonicalRetry(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, g, _, in := finalReportAPI(t, s)
		var sql string
		if opt.Driver == "postgres" {
			sql = `CREATE FUNCTION reject_junit_insert() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'private database diagnostic'; END $$`
			if err := s.writer.Exec(sql).Error; err != nil {
				t.Fatal("private fixture function")
			}
			sql = `CREATE TRIGGER reject_junit_insert BEFORE INSERT ON artifacts FOR EACH ROW EXECUTE FUNCTION reject_junit_insert()`
		} else {
			sql = `CREATE TRIGGER reject_junit_insert BEFORE INSERT ON artifacts BEGIN SELECT RAISE(ABORT,'private database diagnostic'); END`
		}
		if err := s.writer.Exec(sql).Error; err != nil {
			t.Fatal("private fixture trigger")
		}
		_, err := s.CommitArtifact(testContext, a, in)
		if err != errDatabase && err != ErrConflict {
			t.Fatal("SQL diagnostic not mapped safely", err)
		}
		var row buildRecord
		s.db.First(&row, "id = ?", g.Ref.BuildID)
		var count int64
		s.db.Model(&artifactRecord{}).Where("attempt_id = ?", g.Ref.AttemptID).Count(&count)
		if row.LastArtifactSeq != 0 || count != 0 || row.ReportSealDigest != "" {
			t.Fatal("failed publication became visible")
		}
		if _, err = s.FindNodeArtifact(testContext, a, g.Ref, in.Declaration.ID); err != ErrNotFound {
			t.Fatal("uncommitted XML visible to node", err)
		}
		if opt.Driver == "postgres" {
			sql = `DROP TRIGGER reject_junit_insert ON artifacts`
		} else {
			sql = `DROP TRIGGER reject_junit_insert`
		}
		if err = s.writer.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
		out, err := s.CommitArtifact(testContext, a, in)
		if err != nil || !out.Created {
			t.Fatal("same declaration retry", err)
		}
		out, err = s.CommitArtifact(testContext, a, in)
		if err != nil || out.Created {
			t.Fatal("repeat duplicated file", err)
		}
	})
}
func TestReportsCommitRechecksExpiryAfterRealDatabaseDelay(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, g, _, in := finalReportAPI(t, s)
		if opt.Driver == "postgres" {
			if err := s.writer.Exec(`CREATE FUNCTION slow_junit_insert() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_sleep(0.2); RETURN NEW; END $$`).Error; err != nil {
				t.Fatal(err)
			}
			if err := s.writer.Exec(`CREATE TRIGGER slow_junit_insert BEFORE INSERT ON artifacts FOR EACH ROW EXECUTE FUNCTION slow_junit_insert()`).Error; err != nil {
				t.Fatal(err)
			}
		} else {
			if err := s.writer.Exec(`CREATE TRIGGER slow_junit_insert AFTER INSERT ON artifacts BEGIN SELECT sum(x) FROM (WITH RECURSIVE delay(x) AS (VALUES(0) UNION ALL SELECT x+1 FROM delay WHERE x<1000000) SELECT x FROM delay); END`).Error; err != nil {
				t.Fatal(err)
			}
		}
		deadline := time.Now().UTC().Add(100 * time.Millisecond)
		if err := s.writer.Model(&buildRecord{}).Where("id = ?", g.Ref.BuildID).Update("lease_expires_at", deadline).Error; err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		if _, err := s.CommitArtifact(testContext, a, in); err != ErrLeaseExpired {
			t.Fatal("late artifact crossed old expiry", err)
		}
		if time.Since(start) < 100*time.Millisecond {
			t.Fatal("fixture did not cross deadline")
		}
		var row buildRecord
		s.db.First(&row, "id = ?", g.Ref.BuildID)
		var count int64
		s.db.Model(&artifactRecord{}).Where("attempt_id = ?", g.Ref.AttemptID).Count(&count)
		if row.LastArtifactSeq != 0 || count != 0 || row.ReportSealDigest != "" {
			t.Fatal("expired file committed")
		}
	})
}

func TestReportsCommitRechecksCumulativeBudgetAfterRealSQLDelay(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, g, _, in := finalReportAPI(t, s)
		if opt.Driver == "postgres" {
			if err := s.writer.Exec(`CREATE FUNCTION budget_junit_insert() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_sleep(0.2); RETURN NEW; END $$`).Error; err != nil {
				t.Fatal(err)
			}
			if err := s.writer.Exec(`CREATE TRIGGER budget_junit_insert BEFORE INSERT ON artifacts FOR EACH ROW EXECUTE FUNCTION budget_junit_insert()`).Error; err != nil {
				t.Fatal(err)
			}
		} else {
			if err := s.writer.Exec(`CREATE TRIGGER budget_junit_insert AFTER INSERT ON artifacts BEGIN SELECT sum(x) FROM (WITH RECURSIVE delay(x) AS (VALUES(0) UNION ALL SELECT x+1 FROM delay WHERE x<1000000) SELECT x FROM delay); END`).Error; err != nil {
				t.Fatal(err)
			}
		}
		if err := s.writer.Model(&executionReceiptRecord{}).Where("build_id = ? AND seq = ?", g.Ref.BuildID, 5).Update("created_at", time.Now().UTC()).Error; err != nil {
			t.Fatal(err)
		}
		if err := s.writer.Model(&buildRecord{}).Where("id = ?", g.Ref.BuildID).Update("remaining_budget_ns", int64(100*time.Millisecond)).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := s.CommitArtifact(testContext, a, in); err != ErrBudgetInvalid {
			t.Fatal("SQL consumed report budget but commit succeeded", err)
		}
		var row buildRecord
		s.db.First(&row, "id = ?", g.Ref.BuildID)
		var count int64
		s.db.Model(&artifactRecord{}).Where("attempt_id = ?", g.Ref.AttemptID).Count(&count)
		if row.LastArtifactSeq != 0 || count != 0 {
			t.Fatal("budget-exhausted confirmation became visible")
		}
	})
}

func TestReportsPostgresFinalLockQueryCannotExtendReportBudget(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		if opt.Driver != "postgres" {
			t.Skip("只有PG末尾控制锁查询有网络等待；SQLite SQL延迟由同suite覆盖")
		}
		a, g, _, in := finalReportAPI(t, s)
		var schema string
		if err := s.conn.QueryRowContext(testContext, "SELECT current_schema()").Scan(&schema); err != nil {
			t.Fatal(err)
		}
		// 只在自有schema透传pg_catalog；首次读锁无候选，事务末尾确有候选才发生等待。
		if err := s.writer.Exec(`CREATE FUNCTION delayed_report_lock() RETURNS boolean LANGUAGE plpgsql AS $$ BEGIN IF EXISTS (SELECT 1 FROM artifacts WHERE purpose = 'junit') THEN PERFORM pg_sleep(0.3); END IF; RETURN true; END $$`).Error; err != nil {
			t.Fatal(err)
		}
		if err := s.writer.Exec(`CREATE VIEW pg_locks AS WITH boundary AS MATERIALIZED (SELECT delayed_report_lock() AS ok) SELECT locks.* FROM pg_catalog.pg_locks AS locks CROSS JOIN boundary WHERE boundary.ok`).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := s.conn.ExecContext(testContext, "SET search_path TO "+schema+",pg_catalog"); err != nil {
			t.Fatal(err)
		}
		if err := s.writer.Model(&executionReceiptRecord{}).Where("build_id = ? AND seq = ?", g.Ref.BuildID, 5).Update("created_at", time.Now().UTC()).Error; err != nil {
			t.Fatal(err)
		}
		if err := s.writer.Model(&buildRecord{}).Where("id = ?", g.Ref.BuildID).Update("remaining_budget_ns", int64(100*time.Millisecond)).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := s.CommitArtifact(testContext, a, in); err != ErrBudgetInvalid {
			t.Fatal("final lock query crossed trusted report budget", err)
		}
		var count int64
		if err := s.db.Model(&artifactRecord{}).Where("attempt_id = ?", g.Ref.AttemptID).Count(&count).Error; err != nil || count != 0 {
			t.Fatal("late candidate committed", err)
		}
	})
}
