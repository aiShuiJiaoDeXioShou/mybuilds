package store

import (
	"encoding/json"
	"testing"
	"time"

	"gorm.io/gorm"
	"mybuilds/internal/protocol"
)

func reportTx(s *Store, a NodeActor, g protocol.LeaseGrant, fn func(*gorm.DB, *buildRecord) error) error {
	return s.write(testContext, func(tx *gorm.DB) error {
		row, err := s.currentExecution(tx, a, g.Ref)
		if err != nil {
			return err
		}
		if err = fn(tx, &row); err != nil {
			return err
		}
		return checkBoundary(tx, a, g.Ref, *row.LeaseExpiresAt)
	})
}
func TestReportsCheckedSemanticTransaction(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, g := reportFixture(t, s, true)
		reportFinished(t, s, a, g)
		good := reportEvidence("99b7bc37-a72e-4cb9-bbf2-7794f45c9b54")
		p := stepProgress("reports_checked", "", "ordinary", "compile", 1)
		mutate := []func(*protocol.ReportEvidence){
			func(e *protocol.ReportEvidence) { e.Files = nil }, func(e *protocol.ReportEvidence) { e.Diagnostics = nil }, func(e *protocol.ReportEvidence) { e.Revision = 2 }, func(e *protocol.ReportEvidence) { e.Sealed = true }, func(e *protocol.ReportEvidence) { e.Required = false }, func(e *protocol.ReportEvidence) { e.Outcome = "passed" },
			func(e *protocol.ReportEvidence) { e.Files[0].Path = "../unsafe.xml" }, func(e *protocol.ReportEvidence) { e.Files[0].Path = "results\\unsafe.xml" }, func(e *protocol.ReportEvidence) { e.Files[0].Key = "invalid" }, func(e *protocol.ReportEvidence) {
				e.Files[0].Path = "other/test.xml"
				e.Files[0].Key = reportKey(e.Files[0].Path)
			}, func(e *protocol.ReportEvidence) { e.Files[0].SourceStep = "cleanup" }, func(e *protocol.ReportEvidence) { e.Files[0].SourceIndex = 2 }, func(e *protocol.ReportEvidence) { e.Counts.Tests = 2 }, func(e *protocol.ReportEvidence) { e.Files[0].Size = (8 << 20) + 1 }, func(e *protocol.ReportEvidence) { e.Files = append(e.Files, e.Files[0]) },
		}
		for i, change := range mutate {
			b, _ := json.Marshal(good)
			var e protocol.ReportEvidence
			json.Unmarshal(b, &e)
			change(&e)
			p.Reports = &e
			if err := reportTx(s, a, g, func(tx *gorm.DB, row *buildRecord) error { return applyReportsChecked(tx, row, p) }); err != ErrEventConflict {
				t.Fatalf("fixture %d accepted: %v", i, err)
			}
		}
		p.Reports = &good
		if err := reportTx(s, a, g, func(tx *gorm.DB, row *buildRecord) error { return applyReportsChecked(tx, row, p) }); err != nil {
			t.Fatal(err)
		}
		if err := reportTx(s, a, g, func(tx *gorm.DB, row *buildRecord) error { return applyReportsChecked(tx, row, p) }); err != ErrEventConflict {
			t.Fatal("revision replay bypass", err)
		}
		good.Revision = 2
		good.Outcome = "passed"
		p = stepProgress("reports_checked", "", "", "", 0)
		p.StepKind = ""
		p.Reports = &good
		if err := reportTx(s, a, g, func(tx *gorm.DB, row *buildRecord) error { return applyReportsChecked(tx, row, p) }); err != nil {
			t.Fatal("final checked", err)
		}
		// 普通成功不能先于完整XML确认选择post。
		post := protocol.ExecutionProgress{Kind: "post_selected", PostPhase: "success"}
		if err := reportTx(s, a, g, func(tx *gorm.DB, row *buildRecord) error { return validateReportPost(tx, row, post) }); err != ErrEventConflict {
			t.Fatal("unsealed post", err)
		}
	})
}
func TestReportsCheckedRequiresTrueStoppedSource(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, g := reportFixture(t, s, true)
		e := reportEvidence("83e8aa04-9385-471f-b529-9b412c3e4d40")
		p := stepProgress("reports_checked", "", "ordinary", "compile", 1)
		p.Reports = &e
		if err := reportTx(s, a, g, func(tx *gorm.DB, row *buildRecord) error { return applyReportsChecked(tx, row, p) }); err != ErrEventConflict {
			t.Fatal("pending source", err)
		}
		reportFinished(t, s, a, g)
		if err := s.writer.Model(&stepRecord{}).Where("build_id = ? AND phase = ?", g.Ref.BuildID, "ordinary").Updates(map[string]any{"stop_confirmed": false, "cleanup_failed": true}).Error; err != nil {
			t.Fatal(err)
		}
		if err := reportTx(s, a, g, func(tx *gorm.DB, row *buildRecord) error { return applyReportsChecked(tx, row, p) }); err != ErrEventConflict {
			t.Fatal("unknown source", err)
		}
	})
}
func TestReportUploadBudgetTrustedSharedAnchor(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, g := reportFixture(t, s, false)
		reportFinished(t, s, a, g)
		e := protocol.ReportEvidence{Revision: 1, Outcome: "missing", Diagnostics: []protocol.JUnitDiagnostic{}, Files: []protocol.ReportFile{}}
		p := stepProgress("reports_checked", "", "", "", 0)
		p.StepKind = ""
		p.Reports = &e
		if err := reportTx(s, a, g, func(tx *gorm.DB, row *buildRecord) error { return applyReportsChecked(tx, row, p) }); err != ErrEventConflict {
			t.Fatal("unchecked run final", err)
		}
		// 先确认真实run检查，再 final；receipt锚属于实际控制端而不是Node At。
		e.Outcome = "pending"
		p = stepProgress("reports_checked", "", "ordinary", "compile", 1)
		p.Reports = &e
		accept(t, s, a, event(g.Ref, 4, p))
		e.Revision = 2
		e.Outcome = "missing"
		p = stepProgress("reports_checked", "", "", "", 0)
		p.StepKind = ""
		p.Reports = &e
		accept(t, s, a, event(g.Ref, 5, p))
		// 破坏最后真实receipt.Kind，必须拒绝，不可用普通finished作报告预算锚。
		if err := s.writer.Model(&executionReceiptRecord{}).Where("build_id = ? AND seq = ?", g.Ref.BuildID, 5).Update("kind", "finished").Error; err != nil {
			t.Fatal(err)
		}
		if _, err := s.ReportUploadBudget(testContext, a, g.Ref); err != ErrEventConflict {
			t.Fatal("finished receipt used", err)
		}
		if err := s.writer.Model(&executionReceiptRecord{}).Where("build_id = ? AND seq = ?", g.Ref.BuildID, 5).Updates(map[string]any{"kind": "reports_checked", "created_at": time.Now().Add(-time.Second).UTC()}).Error; err != nil {
			t.Fatal(err)
		}
		remaining := int64(5 * time.Second)
		if err := s.writer.Model(&buildRecord{}).Where("id = ?", g.Ref.BuildID).Update("remaining_budget_ns", remaining).Error; err != nil {
			t.Fatal(err)
		}
		first, err := s.ReportUploadBudget(testContext, a, g.Ref)
		if err != nil || first == nil || *first >= remaining || *first <= int64(3*time.Second) {
			t.Fatal("trusted elapsed not charged", err)
		}
		time.Sleep(15 * time.Millisecond)
		second, err := s.ReportUploadBudget(testContext, a, g.Ref)
		if err != nil || second == nil || *second >= *first {
			t.Fatal("per-file budget reset", err)
		}
		if err = s.writer.Model(&executionReceiptRecord{}).Where("build_id = ? AND seq = ?", g.Ref.BuildID, 5).Update("created_at", time.Now().Add(time.Hour).UTC()).Error; err != nil {
			t.Fatal(err)
		}
		if _, err = s.ReportUploadBudget(testContext, a, g.Ref); err != ErrEventConflict {
			t.Fatal("future clock", err)
		}
		if err = s.writer.Model(&executionReceiptRecord{}).Where("build_id = ? AND seq = ?", g.Ref.BuildID, 5).Update("created_at", time.Now().Add(-time.Hour).UTC()).Error; err != nil {
			t.Fatal(err)
		}
		if _, err = s.ReportUploadBudget(testContext, a, g.Ref); err != ErrBudgetInvalid {
			t.Fatal("exhausted budget", err)
		}
	})
}

func TestReportsCheckedRenderedPatternsUseTrustedFacts(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, g := reportFixture(t, s, true)
		reportFinished(t, s, a, g)
		var row buildRecord
		s.writer.First(&row, "id = ?", g.Ref.BuildID)
		var snap BuildSnapshot
		json.Unmarshal([]byte(row.SnapshotJSON), &snap)
		snap.Definition.Reports.JUnit.Paths = []string{"results/{{version}}/{{node.name}}/{{build.number}}/*.xml"}
		snap.Params["version"] = "v1"
		snap.Facts["node.name"] = "forged"
		data, _ := encode(snap)
		if err := s.writer.Model(&row).Update("snapshot_json", data).Error; err != nil {
			t.Fatal(err)
		}
		var patterns []string
		if err := reportTx(s, a, g, func(tx *gorm.DB, row *buildRecord) error {
			cfg, err := configuredReport(*row)
			if err != nil {
				return err
			}
			patterns, err = reportsPatterns(tx, *row, cfg)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if len(patterns) != 1 || patterns[0] != "results/v1/linux/1/*.xml" {
			t.Fatal("node facts forged or params reinterpreted")
		}
		snap.Definition.Reports.JUnit.Paths = []string{"results/{{workspace}}/*.xml"}
		data, _ = encode(snap)
		s.writer.Model(&row).Update("snapshot_json", data)
		if err := reportTx(s, a, g, func(tx *gorm.DB, row *buildRecord) error {
			cfg, err := configuredReport(*row)
			if err != nil {
				return err
			}
			_, err = reportsPatterns(tx, *row, cfg)
			return err
		}); err != ErrEventConflict {
			t.Fatal("private fact guessed", err)
		}
	})
}
