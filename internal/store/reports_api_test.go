package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"mybuilds/internal/reports"
)

func queuedReports(t *testing.T, s *Store, required bool, patterns []string, second bool) (NodeActor, protocol.LeaseGrant) {
	t.Helper()
	a, session, project, policy := leaseFixture(t, s)
	in := enqueueInput(project, "real-reports")
	in.Builds[0].Snapshot.Definition.Reports = &config.Reports{JUnit: &config.JUnitReport{Paths: patterns, Required: &required}}
	if second {
		in.Builds[0].Snapshot.Definition.Steps = append(in.Builds[0].Snapshot.Definition.Steps, config.Step{Kind: "run", Name: "sentinel", Run: "echo sentinel"})
		in.Builds[0].Steps = append(in.Builds[0].Steps, StepProgress{Phase: "ordinary", Index: 2, Name: "sentinel", Kind: "run", Condition: "ready", Status: "pending"})
	}
	out, err := s.Enqueue(testContext, in)
	if err != nil {
		t.Fatal("actual reports enqueue", err)
	}
	grant, err := s.Claim(testContext, a, protocol.ClaimRequest{SessionID: session.SessionID, ClaimKey: uuid.NewString()}, policy)
	if err != nil || grant == nil || grant.Ref.BuildID != out.Builds[0].ID {
		t.Fatal("claim", err)
	}
	return a, *grant
}
func actualReport(t *testing.T, xml string) (protocol.ReportEvidence, protocol.JUnitResult) {
	t.Helper()
	parsed, err := reports.ParseJUnit(context.Background(), strings.NewReader(xml), nil)
	if err != nil {
		t.Fatal(err)
	}
	e := reportEvidence(uuid.NewString())
	e.Counts = parsed.Counts
	e.Files[0].Counts = parsed.Counts
	e.Files[0].Size = int64(len(xml))
	e.Files[0].SHA256 = reportKey(xml)
	for _, d := range parsed.Diagnostics {
		d.PathKey = e.Files[0].Key
		e.Diagnostics = append(e.Diagnostics, d)
	}
	if parsed.Counts.Failures+parsed.Counts.Errors > 0 {
		e.Outcome = "failed"
		e.Reason = "report_failed"
	}
	return e, parsed
}
func checkedEvent(g protocol.LeaseGrant, seq int64, e *protocol.ReportEvidence, final bool) protocol.ExecutionEvent {
	p := stepProgress("reports_checked", "", "ordinary", "compile", 1)
	p.Reports = e
	if final {
		p.Phase = ""
		p.Name = ""
		p.StepKind = ""
		p.Index = 0
	}
	return event(g.Ref, seq, p)
}
func TestReportsAPICompleteFilesSealManifestReceipt(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, g := queuedReports(t, s, true, []string{"results/*.xml"}, false)
		reportFinished(t, s, a, g)
		e, parsed := actualReport(t, `<testsuite><testcase time="0.5"/></testsuite>`)
		first := checkedEvent(g, 4, &e, false)
		accept(t, s, a, first)
		accept(t, s, a, first)
		upload := reportCommit(g, e, &parsed)
		if _, err := s.CommitArtifact(testContext, a, upload); err != ErrArtifactConflict {
			t.Fatal("nonfinal upload", err)
		}
		e.Revision = 2
		e.Outcome = "passed"
		accept(t, s, a, checkedEvent(g, 5, &e, true))
		upload = reportCommit(g, e, &parsed)
		visible, err := s.ListArtifacts(testContext, localAdmin, g.Ref.BuildID, Page{})
		if err != nil || len(visible) != 0 {
			t.Fatal("unsealed publication", err)
		}
		bad := upload
		bad.VerifiedJUnit = nil
		if _, err = s.CommitArtifact(testContext, a, bad); err != ErrInvalid {
			t.Fatal("summary-only upload", err)
		}
		for _, mutate := range []func(*protocol.ArtifactDeclaration){func(d *protocol.ArtifactDeclaration) { d.ReportRevision = 1 }, func(d *protocol.ArtifactDeclaration) { d.ReportKey = strings.Repeat("c", 64) }, func(d *protocol.ArtifactDeclaration) { d.Index = 2 }, func(d *protocol.ArtifactDeclaration) { d.ID = uuid.NewString() }, func(d *protocol.ArtifactDeclaration) { d.SHA256 = strings.Repeat("d", 64) }} {
			bad = upload
			mutate(&bad.Declaration)
			if _, err = s.CommitArtifact(testContext, a, bad); err != ErrArtifactConflict {
				t.Fatal("mismatched final declaration", err)
			}
		}
		confirmed, err := s.CommitArtifact(testContext, a, upload)
		if err != nil || !confirmed.Created || confirmed.View.Purpose != "junit" {
			t.Fatal("actual XML confirmation", err)
		}
		retry := upload
		retry.StorageID = uuid.NewString()
		again, err := s.CommitArtifact(testContext, a, retry)
		if err != nil || again.Created || again.StorageID != upload.StorageID {
			t.Fatal("canonical repeat", err)
		}
		visible, err = s.ListArtifacts(testContext, localAdmin, g.Ref.BuildID, Page{})
		if err != nil || len(visible) != 0 {
			t.Fatal("uploaded unsealed evidence visible", err)
		}
		sealed := e
		sealed.Sealed = true
		p := stepProgress("reports_sealed", "", "", "", 0)
		p.StepKind = ""
		p.Reports = &sealed
		accept(t, s, a, event(g.Ref, 6, p))
		data, _ := encode(sealed)
		digest := reportKey(data)
		post := protocol.ExecutionProgress{Kind: "post_selected", PostPhase: "success", RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}
		accept(t, s, a, event(g.Ref, 7, post))
		skip := stepProgress("skipped", "skipped", "always", "cleanup", 1)
		skip.Reason = "condition"
		skip.StopConfirmed = true
		accept(t, s, a, event(g.Ref, 8, skip))
		terminal := protocol.ExecutionProgress{Kind: "build_finished", Status: "succeeded", Started: true, StopConfirmed: true, RemainingPostBudgetNS: int64(2 * time.Minute), LastArtifactSeq: 1, ArtifactSteps: []protocol.ArtifactExpectation{}, ReportManifest: &protocol.ReportManifest{SealDigest: digest, IDs: []string{e.Files[0].ArtifactID}}}
		wrong := terminal
		wrong.ReportManifest = &protocol.ReportManifest{SealDigest: digest, IDs: nil}
		if _, err = s.ApplyEvent(testContext, a, event(g.Ref, 9, wrong)); err != ErrEventConflict {
			t.Fatal("null manifest accepted", err)
		}
		wrong = terminal
		wrong.ReportManifest = &protocol.ReportManifest{SealDigest: digest, IDs: []string{uuid.NewString()}}
		if _, err = s.ApplyEvent(testContext, a, event(g.Ref, 9, wrong)); err != ErrEventConflict {
			t.Fatal("incomplete manifest accepted", err)
		}
		accept(t, s, a, event(g.Ref, 9, terminal))
		view, err := s.GetBuild(testContext, g.Ref.BuildID)
		if err != nil || view.Status != "succeeded" || view.Reports == nil || !view.Reports.Sealed || view.ReportSealDigest != digest {
			t.Fatal("safe sealed build view", err)
		}
		visible, err = s.ListArtifacts(testContext, localAdmin, g.Ref.BuildID, Page{})
		if err != nil || len(visible) != 1 || visible[0].SHA256 != upload.Declaration.SHA256 {
			t.Fatal("current sealed file publication", err)
		}
		var receipt executionReceiptRecord
		if err = s.db.First(&receipt, "build_id = ? AND seq = ?", g.Ref.BuildID, 9).Error; err != nil || receipt.Kind != "build_finished" || !receipt.StopKnown {
			t.Fatal("incomplete stop receipt", err)
		}
	})
}
func TestReportsAPIFailedBlocksSentinelAndPreservesOriginalFailure(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, g := queuedReports(t, s, true, []string{"results/*.xml"}, true)
		reportFinished(t, s, a, g)
		sentinel := stepProgress("intent", "", "ordinary", "sentinel", 2)
		if _, err := s.ApplyEvent(testContext, a, event(g.Ref, 4, sentinel)); err != ErrEventConflict {
			t.Fatal("unchecked run advanced", err)
		}
		e, parsed := actualReport(t, `<testsuite><testcase><failure>real failed test</failure></testcase></testsuite>`)
		accept(t, s, a, checkedEvent(g, 4, &e, false))
		if _, err := s.ApplyEvent(testContext, a, event(g.Ref, 5, sentinel)); err != ErrEventConflict {
			t.Fatal("failed reports advanced", err)
		}
		skip := stepProgress("skipped", "skipped", "ordinary", "sentinel", 2)
		skip.Reason = "report_failed"
		skip.StopConfirmed = true
		accept(t, s, a, event(g.Ref, 5, skip))
		e.Revision = 2
		accept(t, s, a, checkedEvent(g, 6, &e, true))
		upload := reportCommit(g, e, &parsed)
		if _, err := s.CommitArtifact(testContext, a, upload); err != nil {
			t.Fatal(err)
		}
		sealed := e
		sealed.Sealed = true
		p := stepProgress("reports_sealed", "", "", "", 0)
		p.StepKind = ""
		p.Reports = &sealed
		accept(t, s, a, event(g.Ref, 7, p))
		post := protocol.ExecutionProgress{Kind: "post_selected", PostPhase: "failure", RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}
		accept(t, s, a, event(g.Ref, 8, post))
		skip = stepProgress("skipped", "skipped", "always", "cleanup", 1)
		skip.Reason = "condition"
		skip.StopConfirmed = true
		accept(t, s, a, event(g.Ref, 9, skip))
		data, _ := encode(sealed)
		terminal := protocol.ExecutionProgress{Kind: "build_finished", Status: "failed", Reason: "report_failed", Started: true, StopConfirmed: true, RemainingPostBudgetNS: int64(2 * time.Minute), LastArtifactSeq: 1, ArtifactSteps: []protocol.ArtifactExpectation{}, ReportManifest: &protocol.ReportManifest{SealDigest: reportKey(data), IDs: []string{e.Files[0].ArtifactID}}}
		accept(t, s, a, event(g.Ref, 10, terminal))
		view, err := s.GetBuild(testContext, g.Ref.BuildID)
		if err != nil || view.Status != "failed" || view.Steps[0].Status != "succeeded" || view.Steps[1].Started || view.Reason != "report_failed" {
			t.Fatal("report changed real step facts", err)
		}
	})
}
