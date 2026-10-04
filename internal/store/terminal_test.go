package store

import (
	"mybuilds/internal/protocol"
	"testing"
	"time"
)

func TestTerminalPostFailureAndCleanupPreservePhysicalProtection(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, grant, _ := claimed(t, s)
		p := stepProgress("intent", "", "ordinary", "compile", 1)
		accept(t, s, a, event(grant.Ref, 1, p))
		p.Kind = "started"
		p.Started = true
		accept(t, s, a, event(grant.Ref, 2, p))
		p.Kind = "finished"
		p.Status = "succeeded"
		p.StopConfirmed = true
		p.ExitCode = 0
		accept(t, s, a, event(grant.Ref, 3, p))
		selectPost := protocol.ExecutionProgress{Kind: "post_selected", PostPhase: "success", RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}
		accept(t, s, a, event(grant.Ref, 4, selectPost))
		p = stepProgress("intent", "", "always", "cleanup", 1)
		accept(t, s, a, event(grant.Ref, 5, p))
		p.Kind = "started"
		p.Started = true
		accept(t, s, a, event(grant.Ref, 6, p))
		p.Kind = "finished"
		p.Status = "failed"
		p.Reason = "exit"
		p.StopConfirmed = true
		p.ExitCode = 7
		accept(t, s, a, event(grant.Ref, 7, p))
		terminal := protocol.ExecutionProgress{Kind: "build_finished", Status: "succeeded", Started: true, StopConfirmed: true, RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}
		if _, err := s.ApplyEvent(testContext, a, event(grant.Ref, 8, terminal)); err != ErrEventConflict {
			t.Fatal("failed post became success", err)
		}
		terminal.Status = "failed"
		terminal.Reason = "post_error"
		accept(t, s, a, event(grant.Ref, 8, terminal))
		view, err := s.GetBuild(testContext, grant.Ref.BuildID)
		if err != nil || view.Reason != "post_error" || view.Post[0].Reason != "exit" || view.Post[0].ExitCode != 7 {
			t.Fatal("post evidence lost", err)
		}
	})
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, grant, _ := claimed(t, s)
		p := stepProgress("intent", "", "ordinary", "compile", 1)
		accept(t, s, a, event(grant.Ref, 1, p))
		p.Kind = "started"
		p.Started = true
		accept(t, s, a, event(grant.Ref, 2, p))
		p.Kind = "finished"
		p.Status = "failed"
		p.Reason = "exit"
		p.CleanupFailed = true
		p.ExitCode = 7
		accept(t, s, a, event(grant.Ref, 3, p))
		selectPost := protocol.ExecutionProgress{Kind: "post_selected", PostPhase: "failure", RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}
		accept(t, s, a, event(grant.Ref, 4, selectPost))
		p = stepProgress("skipped", "skipped", "always", "cleanup", 1)
		p.Reason = "cleanup_error"
		p.StopConfirmed = true
		accept(t, s, a, event(grant.Ref, 5, p))
		terminal := protocol.ExecutionProgress{Kind: "build_finished", Status: "failed", Reason: "exit", Started: true, CleanupFailed: true, RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}
		accept(t, s, a, event(grant.Ref, 6, terminal))
		view, err := s.GetBuild(testContext, grant.Ref.BuildID)
		if err != nil || view.Status != "interrupted" || view.Reason != "exit" || !view.StopUnconfirmed {
			t.Fatal("cleanup falsely stopped", view, err)
		}
		node, err := s.GetNode(testContext, localAdmin, "linux")
		if err != nil || !node.Quarantined {
			t.Fatal("unknown cleanup not guarded", err)
		}
	})
}
func TestAllOrdinaryConditionsSkippedIsNotSuccess(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, grant, _ := claimed(t, s)
		p := stepProgress("skipped", "skipped", "ordinary", "compile", 1)
		p.Reason = "condition"
		p.StopConfirmed = true
		accept(t, s, a, event(grant.Ref, 1, p))
		selectPost := protocol.ExecutionProgress{Kind: "post_selected", PostPhase: "none", RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}
		accept(t, s, a, event(grant.Ref, 2, selectPost))
		p = stepProgress("skipped", "skipped", "always", "cleanup", 1)
		p.Reason = "not_selected"
		p.StopConfirmed = true
		accept(t, s, a, event(grant.Ref, 3, p))
		terminal := protocol.ExecutionProgress{Kind: "build_finished", Status: "succeeded", StopConfirmed: true, RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}
		if _, err := s.ApplyEvent(testContext, a, event(grant.Ref, 4, terminal)); err != ErrEventConflict {
			t.Fatal("all skipped became success", err)
		}
		terminal.Status = "skipped"
		terminal.Reason = "condition"
		accept(t, s, a, event(grant.Ref, 4, terminal))
	})
}
