package store

import (
	"testing"
	"time"

	"mybuilds/internal/protocol"
)

func TestTerminalReceiptIndependentStopCannotInventTerminalEvidence(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, g, _ := claimed(t, s)
		p := stepProgress("intent", "", "ordinary", "compile", 1)
		accept(t, s, a, event(g.Ref, 1, p))
		p.Kind = "started"
		p.Started = true
		accept(t, s, a, event(g.Ref, 2, p))
		expired := time.Now().UTC()
		s.writer.Model(&buildRecord{}).Where("id = ?", g.Ref.BuildID).Update("lease_expires_at", expired)
		s.writer.Model(&attemptRecord{}).Where("id = ?", g.Ref.AttemptID).Update("lease_expires_at", expired)
		if err := s.ExpireLeases(testContext); err != nil {
			t.Fatal(err)
		}
		if err := s.ConfirmStopped(testContext, localAdmin, protocol.StopConfirmation{Ref: g.Ref, EvidenceCode: "admin_observed_stopped", Note: "测试只确认独立物理停止"}); err != nil {
			t.Fatal(err)
		}
		e := event(g.Ref, 2, p)
		if _, err := s.TerminalReceipt(testContext, a, protocol.TerminalReceiptRequest{Ref: g.Ref, Seq: 2, Digest: e.Digest}); err != ErrConflict {
			t.Fatal("独立停止冒充完整终态", err)
		}
		var receipts []executionReceiptRecord
		if err := s.db.Where("build_id = ?", g.Ref.BuildID).Find(&receipts).Error; err != nil {
			t.Fatal(err)
		}
		for _, receipt := range receipts {
			if receipt.Kind == "build_finished" || receipt.StopKnown {
				t.Fatal("停止确认改写历史回执")
			}
		}
		if _, err := s.Retry(testContext, localAdmin, RetryInput{BuildID: g.Ref.BuildID, Key: "confirmed"}); err != nil {
			t.Fatal("明确停止后应可新retry", err)
		}
	})
}
func TestTerminalReceiptCleanupFailedNeverStopKnown(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, g, _ := claimed(t, s)
		p := stepProgress("intent", "", "ordinary", "compile", 1)
		accept(t, s, a, event(g.Ref, 1, p))
		p.Kind = "started"
		p.Started = true
		accept(t, s, a, event(g.Ref, 2, p))
		p.Kind = "finished"
		p.Status = "failed"
		p.Reason = "cleanup_error"
		p.CleanupFailed = true
		accept(t, s, a, event(g.Ref, 3, p))
		selectPost := protocol.ExecutionProgress{Kind: "post_selected", PostPhase: "failure", RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}
		accept(t, s, a, event(g.Ref, 4, selectPost))
		skip := stepProgress("skipped", "skipped", "always", "cleanup", 1)
		skip.Reason = "not_started"
		skip.StopConfirmed = true
		accept(t, s, a, event(g.Ref, 5, skip))
		terminal := protocol.ExecutionProgress{Kind: "build_finished", Status: "failed", Reason: "cleanup_error", Started: true, CleanupFailed: true, RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}
		e := event(g.Ref, 6, terminal)
		accept(t, s, a, e)
		var receipt executionReceiptRecord
		s.db.First(&receipt, "build_id = ? AND seq = ?", g.Ref.BuildID, e.Seq)
		if receipt.Kind != "build_finished" || receipt.StopKnown {
			t.Fatal("cleanup失败给停止已知")
		}
		if _, err := s.TerminalReceipt(testContext, a, protocol.TerminalReceiptRequest{Ref: g.Ref, Seq: e.Seq, Digest: e.Digest}); err != ErrConflict {
			t.Fatal("清理失败终态仍确认", err)
		}
	})
}
