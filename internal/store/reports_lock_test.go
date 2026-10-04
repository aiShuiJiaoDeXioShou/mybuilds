//go:build darwin || linux

package store

import (
	"os"
	"testing"
)

func TestReportsAllConsumersRejectLostControlLock(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, g, e, in := finalReportAPI(t, s)
		if opt.Driver == "sqlite" {
			if err := os.Rename(opt.DSN+".lock", opt.DSN+".lock.replaced"); err != nil {
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
				t.Fatal("own fixture backend termination")
			}
		}
		if _, err := s.ReportUploadBudget(testContext, a, g.Ref); err != ErrLockLost {
			t.Fatal("budget ignored lost lock", err)
		}
		if _, err := s.CommitArtifact(testContext, a, in); err != ErrLockLost {
			t.Fatal("XML confirmation ignored lost lock", err)
		}
		e.Sealed = true
		p := stepProgress("reports_sealed", "", "", "", 0)
		p.StepKind = ""
		p.Reports = &e
		if _, err := s.ApplyEvent(testContext, a, event(g.Ref, 6, p)); err != ErrLockLost {
			t.Fatal("seal ignored lost lock", err)
		}
		var row buildRecord
		if err := s.db.First(&row, "id = ?", g.Ref.BuildID).Error; err != nil {
			t.Fatal(err)
		}
		var count int64
		s.db.Model(&artifactRecord{}).Where("attempt_id = ?", g.Ref.AttemptID).Count(&count)
		if row.LastEventSeq != 5 || row.LastArtifactSeq != 0 || row.ReportSealDigest != "" || count != 0 {
			t.Fatal("lost lock committed trusted evidence")
		}
	})
}
