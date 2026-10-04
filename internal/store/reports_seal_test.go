package store

import (
	"encoding/json"
	"path"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"mybuilds/internal/protocol"
)

func checkedForSeal(t *testing.T, s *Store, a NodeActor, g protocol.LeaseGrant, xml string) (protocol.ReportEvidence, protocol.JUnitResult) {
	t.Helper()
	e, parsed := actualReport(t, xml)
	accept(t, s, a, checkedEvent(g, 4, &e, false))
	e.Revision = 2
	if e.Outcome != "failed" {
		e.Outcome = "passed"
	}
	accept(t, s, a, checkedEvent(g, 5, &e, true))
	return e, parsed
}
func reportCommit(g protocol.LeaseGrant, e protocol.ReportEvidence, parsed *protocol.JUnitResult) ArtifactCommit {
	f := e.Files[0]
	return ArtifactCommit{Declaration: protocol.ArtifactDeclaration{Ref: g.Ref, ID: f.ArtifactID, Seq: 1, Phase: "ordinary", Index: f.SourceIndex, Step: f.SourceStep, Name: path.Base(f.Path), Size: f.Size, SHA256: f.SHA256, Purpose: "junit", ReportRevision: e.Revision, ReportKey: f.Key}, StorageID: uuid.NewString(), VerifiedJUnit: parsed}
}
func verifiedReportRow(t *testing.T, s *Store, a NodeActor, g protocol.LeaseGrant, in ArtifactCommit) {
	t.Helper()
	out, err := s.CommitArtifact(testContext, a, in)
	if err != nil || !out.Created {
		t.Fatal("actual verified XML commit", err)
	}
}
func TestReportsSealRecomputesActualParsedEvidence(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, g := reportFixture(t, s, true)
		reportFinished(t, s, a, g)
		e, parsed := checkedForSeal(t, s, a, g, `<testsuite><testcase name="actual" time="0.123"><failure>real diagnostic</failure></testcase></testsuite>`)
		in := reportCommit(g, e, &parsed)
		p := stepProgress("reports_sealed", "", "", "", 0)
		p.StepKind = ""
		sealed := e
		sealed.Sealed = true
		p.Reports = &sealed
		if err := reportTx(s, a, g, func(tx *gorm.DB, row *buildRecord) error { return applyReportsSealed(tx, row, p) }); err != ErrEventConflict {
			t.Fatal("partial upload sealed", err)
		}
		bad := parsed
		bad.Counts.Tests++
		wrong := in
		wrong.VerifiedJUnit = &bad
		if err := reportTx(s, a, g, func(tx *gorm.DB, row *buildRecord) error {
			_, err := validateJUnitArtifact(tx, *row, wrong)
			return err
		}); err != ErrArtifactConflict {
			t.Fatal("untrusted counts", err)
		}
		nilDiags := parsed
		nilDiags.Diagnostics = nil
		wrong.VerifiedJUnit = &nilDiags
		if err := reportTx(s, a, g, func(tx *gorm.DB, row *buildRecord) error {
			_, err := validateJUnitArtifact(tx, *row, wrong)
			return err
		}); err != ErrArtifactConflict {
			t.Fatal("null diagnostic", err)
		}
		verifiedReportRow(t, s, a, g, in)
		// 即使Node声明已有原诊断，中央stage真正解析的结果仍必须逐字相同。
		wrongParsed := parsed
		wrongParsed.Diagnostics = append([]protocol.JUnitDiagnostic{}, parsed.Diagnostics...)
		wrongParsed.Diagnostics[0].Message = "forged diagnostic"
		wrongJSON, _ := encode(wrongParsed)
		if err := s.writer.Model(&artifactRecord{}).Where("id = ?", in.Declaration.ID).Update("verified_junit_json", wrongJSON).Error; err != nil {
			t.Fatal(err)
		}
		if err := reportTx(s, a, g, func(tx *gorm.DB, row *buildRecord) error { return applyReportsSealed(tx, row, p) }); err != ErrEventConflict {
			t.Fatal("forged diagnostics sealed", err)
		}
		correctJSON, _ := encode(parsed)
		if err := s.writer.Model(&artifactRecord{}).Where("id = ?", in.Declaration.ID).Update("verified_junit_json", correctJSON).Error; err != nil {
			t.Fatal(err)
		}
		if err := reportTx(s, a, g, func(tx *gorm.DB, row *buildRecord) error { return applyReportsSealed(tx, row, p) }); err != nil {
			t.Fatal(err)
		}
		canonical, _ := encode(sealed)
		var row buildRecord
		s.db.First(&row, "id = ?", g.Ref.BuildID)
		if row.ReportSealDigest != reportKey(canonical) {
			t.Fatal("noncanonical seal")
		}
		var stored protocol.ReportEvidence
		json.Unmarshal([]byte(row.ReportsJSON), &stored)
		if !stored.Sealed || stored.Counts.Tests != 1 || stored.Counts.Failures != 1 {
			t.Fatal("verified evidence lost")
		}
		post := protocol.ExecutionProgress{Kind: "post_selected", PostPhase: "failure"}
		if err := reportTx(s, a, g, func(tx *gorm.DB, row *buildRecord) error {
			if err := validateReportPost(tx, row, post); err != nil {
				return err
			}
			if row.Reason != "report_failed" {
				t.Fatal("report did not fail success")
			}
			row.Reason = "exit"
			if err := validateReportPost(tx, row, post); err != nil {
				return err
			}
			if row.Reason != "exit" {
				t.Fatal("command reason overwritten")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		terminal := protocol.ExecutionProgress{Kind: "build_finished", ReportManifest: &protocol.ReportManifest{SealDigest: row.ReportSealDigest, IDs: []string{in.Declaration.ID}}}
		if err := reportTx(s, a, g, func(tx *gorm.DB, row *buildRecord) error { return validateReportManifest(tx, *row, terminal) }); err != nil {
			t.Fatal(err)
		}
		terminal.ReportManifest.IDs = nil
		if err := reportTx(s, a, g, func(tx *gorm.DB, row *buildRecord) error { return validateReportManifest(tx, *row, terminal) }); err != ErrEventConflict {
			t.Fatal("null IDs", err)
		}
		if _, err := s.ReportUploadBudget(testContext, a, g.Ref); err != ErrEventConflict {
			t.Fatal("sealed upload allowed", err)
		}
	})
}
func TestReportsFinalOptionalAndRequiredMissing(t *testing.T) {
	for _, required := range []bool{true, false} {
		stores(t, func(t *testing.T, s *Store, opt Options) {
			a, g := reportFixture(t, s, required)
			reportFinished(t, s, a, g)
			e := protocol.ReportEvidence{Revision: 1, Required: required, Outcome: "pending", Diagnostics: []protocol.JUnitDiagnostic{}, Files: []protocol.ReportFile{}}
			accept(t, s, a, checkedEvent(g, 4, &e, false))
			e.Revision = 2
			e.Outcome = "missing"
			if required {
				e.Outcome = "failed"
				e.Reason = "report_missing"
			}
			accept(t, s, a, checkedEvent(g, 5, &e, true))
			e.Sealed = true
			p := stepProgress("reports_sealed", "", "", "", 0)
			p.StepKind = ""
			p.Reports = &e
			accept(t, s, a, event(g.Ref, 6, p))
		})
	}
}

func TestReportsSealCannotSpendExhaustedBudget(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, g := reportFixture(t, s, true)
		reportFinished(t, s, a, g)
		e, parsed := checkedForSeal(t, s, a, g, `<testsuite><testcase/></testsuite>`)
		in := reportCommit(g, e, &parsed)
		verifiedReportRow(t, s, a, g, in)
		if err := s.writer.Model(&buildRecord{}).Where("id = ?", g.Ref.BuildID).Update("remaining_budget_ns", 0).Error; err != nil {
			t.Fatal(err)
		}
		e.Sealed = true
		p := stepProgress("reports_sealed", "", "", "", 0)
		p.StepKind = ""
		p.Reports = &e
		if err := reportTx(s, a, g, func(tx *gorm.DB, row *buildRecord) error { return applyReportsSealed(tx, row, p) }); err != ErrBudgetInvalid {
			t.Fatal("seal bypassed cumulative budget", err)
		}
		var row buildRecord
		if err := s.db.First(&row, "id = ?", g.Ref.BuildID).Error; err != nil {
			t.Fatal(err)
		}
		if row.ReportSealDigest != "" {
			t.Fatal("invalid seal committed")
		}
	})
}
func TestReportsPostCannotResumeUnfinishedCollection(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, g := reportFixture(t, s, true)
		reportFinished(t, s, a, g)
		for _, reason := range []string{"report_error", "timeout"} {
			e := protocol.ReportEvidence{Revision: 1, Required: true, Sealed: true, Outcome: "failed", Reason: reason, Files: []protocol.ReportFile{}, Diagnostics: []protocol.JUnitDiagnostic{}}
			data, _ := encode(e)
			if err := s.writer.Model(&buildRecord{}).Where("id = ?", g.Ref.BuildID).Updates(map[string]any{"report_revision": 1, "report_final": true, "reports_json": data, "report_seal_digest": reportKey(data)}).Error; err != nil {
				t.Fatal(err)
			}
			if err := reportTx(s, a, g, func(tx *gorm.DB, row *buildRecord) error {
				return validateReportPost(tx, row, protocol.ExecutionProgress{Kind: "post_selected", PostPhase: "failure"})
			}); err != ErrEventConflict {
				t.Fatal("unfinished collection allowed post", err)
			}
		}
	})
}
