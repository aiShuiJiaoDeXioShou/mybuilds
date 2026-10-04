package store

import (
	"testing"
	"time"

	"mybuilds/internal/protocol"
)

func TestReportsAPIZeroActionPrecheckHasNoFakeSeal(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, g := queuedReports(t, s, true, []string{"results/*.xml"}, false)
		e := protocol.ReportEvidence{Revision: 1, Required: true, Outcome: "failed", Reason: "report_missing", Files: []protocol.ReportFile{}, Diagnostics: []protocol.JUnitDiagnostic{}}
		if _, err := s.ApplyEvent(testContext, a, checkedEvent(g, 1, &e, true)); err != ErrEventConflict {
			t.Fatal("required check fabricated before action", err)
		}
		p := protocol.ExecutionProgress{Kind: "build_finished", Status: "failed", Reason: "precheck_error", PostPhase: "none", StopConfirmed: true, RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}
		accept(t, s, a, event(g.Ref, 1, p))
		view, err := s.GetBuild(testContext, g.Ref.BuildID)
		if err != nil || view.Reports != nil || view.ReportSealDigest != "" || view.Steps[0].Started {
			t.Fatal("zero-action evidence fabricated", err)
		}
	})
}
