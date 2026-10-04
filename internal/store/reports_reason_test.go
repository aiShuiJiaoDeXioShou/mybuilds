package store

import (
	"testing"
	"time"

	"mybuilds/internal/protocol"
)

func TestReportsAPIKeepsCommandFailureAndCancellation(t *testing.T) {
	for _, reason := range []string{"exit", "cancelled"} {
		t.Run(reason, func(t *testing.T) {
			stores(t, func(t *testing.T, s *Store, opt Options) {
				a, g := queuedReports(t, s, true, []string{"results/*.xml"}, false)
				p := stepProgress("intent", "", "ordinary", "compile", 1)
				accept(t, s, a, event(g.Ref, 1, p))
				p.Kind = "started"
				p.Started = true
				accept(t, s, a, event(g.Ref, 2, p))
				p.Kind = "finished"
				p.Status = "failed"
				p.Reason = reason
				p.ExitCode = 3
				p.StopConfirmed = true
				if reason == "cancelled" {
					p.Status = "cancelled"
					p.ExitCode = -1
				}
				accept(t, s, a, event(g.Ref, 3, p))
				e, parsed := actualReport(t, `<testsuite><testcase><failure>actual failure</failure></testcase></testsuite>`)
				accept(t, s, a, checkedEvent(g, 4, &e, false))
				e.Revision = 2
				accept(t, s, a, checkedEvent(g, 5, &e, true))
				in := reportCommit(g, e, &parsed)
				if _, err := s.CommitArtifact(testContext, a, in); err != nil {
					t.Fatal(err)
				}
				sealed := e
				sealed.Sealed = true
				seal := stepProgress("reports_sealed", "", "", "", 0)
				seal.StepKind = ""
				seal.Reports = &sealed
				accept(t, s, a, event(g.Ref, 6, seal))
				post := protocol.ExecutionProgress{Kind: "post_selected", PostPhase: "failure", RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}
				if reason == "cancelled" {
					post.PostPhase = "none"
				}
				accept(t, s, a, event(g.Ref, 7, post))
				skip := stepProgress("skipped", "skipped", "always", "cleanup", 1)
				skip.Reason = "condition"
				skip.StopConfirmed = true
				accept(t, s, a, event(g.Ref, 8, skip))
				data, _ := encode(sealed)
				terminal := protocol.ExecutionProgress{Kind: "build_finished", Status: p.Status, Reason: reason, Started: true, StopConfirmed: true, RemainingPostBudgetNS: int64(2 * time.Minute), LastArtifactSeq: 1, ArtifactSteps: []protocol.ArtifactExpectation{}, ReportManifest: &protocol.ReportManifest{SealDigest: reportKey(data), IDs: []string{e.Files[0].ArtifactID}}}
				accept(t, s, a, event(g.Ref, 9, terminal))
				view, err := s.GetBuild(testContext, g.Ref.BuildID)
				if err != nil || view.Reason != reason || view.Status != p.Status || view.Steps[0].ExitCode != p.ExitCode || view.Reports == nil || view.Reports.Reason != "report_failed" {
					t.Fatal("ordinary reason was rewritten", err)
				}
			})
		})
	}
}
