package store

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"mybuilds/internal/protocol"
)

func completeReportsTerminal(t *testing.T, s *Store) (NodeActor, protocol.ExecutionEvent) {
	t.Helper()
	a, g, e, in := finalReportAPI(t, s)
	if _, err := s.CommitArtifact(testContext, a, in); err != nil {
		t.Fatal(err)
	}
	e.Sealed = true
	p := stepProgress("reports_sealed", "", "", "", 0)
	p.StepKind = ""
	p.Reports = &e
	accept(t, s, a, event(g.Ref, 6, p))
	post := protocol.ExecutionProgress{Kind: "post_selected", PostPhase: "success", RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}
	accept(t, s, a, event(g.Ref, 7, post))
	skip := stepProgress("skipped", "skipped", "always", "cleanup", 1)
	skip.Reason = "condition"
	skip.StopConfirmed = true
	accept(t, s, a, event(g.Ref, 8, skip))
	data, _ := encode(e)
	terminal := protocol.ExecutionProgress{Kind: "build_finished", Status: "succeeded", Started: true, StopConfirmed: true, RemainingPostBudgetNS: int64(2 * time.Minute), LastArtifactSeq: 1, ArtifactSteps: []protocol.ArtifactExpectation{}, ReportManifest: &protocol.ReportManifest{SealDigest: reportKey(data), IDs: []string{e.Files[0].ArtifactID}}}
	last := event(g.Ref, 9, terminal)
	accept(t, s, a, last)
	return a, last
}
func TestReportsRetryStartsFreshAndRecoveryRetainsFrozenSeal(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, last := completeReportsTerminal(t, s)
		req := protocol.TerminalReceiptRequest{Ref: last.Ref, Seq: last.Seq, Digest: last.Digest}
		receipt, err := s.TerminalReceipt(testContext, a, req)
		if err != nil || !receipt.StopKnown || receipt.Digest != last.Digest {
			t.Fatal("complete report stop receipt", err)
		}
		changed := last.Progress
		changed.ReportManifest = &protocol.ReportManifest{SealDigest: last.Progress.ReportManifest.SealDigest, IDs: []string{uuid.NewString()}}
		changedEvent := event(last.Ref, last.Seq, changed)
		wrong := req
		wrong.Digest = changedEvent.Digest
		if _, err = s.TerminalReceipt(testContext, a, wrong); err != ErrConflict {
			t.Fatal("different report manifest digest trusted", err)
		}
		var original buildRecord
		if err = s.db.First(&original, "id = ?", last.Ref.BuildID).Error; err != nil {
			t.Fatal(err)
		}
		before := recoveryRows(t, s)
		if err = s.Recover(testContext); err != nil {
			t.Fatal("recover actual sealed terminal", err)
		}
		if !reflect.DeepEqual(before, recoveryRows(t, s)) {
			t.Fatal("Recover changed original evidence or budget")
		}
		retry, err := s.Retry(testContext, localAdmin, RetryInput{BuildID: last.Ref.BuildID, Key: "report-retry"})
		if err != nil || len(retry.Builds) != 1 {
			t.Fatal("retry sealed execution", err)
		}
		var fresh buildRecord
		if err = s.db.First(&fresh, "id = ?", retry.Builds[0].ID).Error; err != nil {
			t.Fatal(err)
		}
		var oldSnap, newSnap BuildSnapshot
		json.Unmarshal([]byte(original.SnapshotJSON), &oldSnap)
		json.Unmarshal([]byte(fresh.SnapshotJSON), &newSnap)
		if !reflect.DeepEqual(oldSnap.Definition.Reports, newSnap.Definition.Reports) || fresh.ReportRevision != 0 || fresh.ReportFinal || fresh.ReportsJSON != "" || fresh.ReportSealDigest != "" || fresh.ReportCheckedIndex != 0 || fresh.LastArtifactSeq != 0 || fresh.LastEventSeq != 0 || fresh.LastLogSeq != 0 || fresh.AttemptID != nil || retry.Builds[0].Reports != nil {
			t.Fatal("retry inherited evidence or lost reports definition")
		}
		var files int64
		if err = s.db.Model(&artifactRecord{}).Where("build_id = ?", fresh.ID).Count(&files).Error; err != nil || files != 0 {
			t.Fatal("retry copied old files", err)
		}
		before = recoveryRows(t, s)
		if err = s.Recover(testContext); err != nil {
			t.Fatal("recover fresh queued reports", err)
		}
		if !reflect.DeepEqual(before, recoveryRows(t, s)) {
			t.Fatal("Recover rewrote queued facts/budget")
		}
		var after buildRecord
		if err = s.db.First(&after, "id = ?", original.ID).Error; err != nil || after.ReportsJSON != original.ReportsJSON || after.ReportSealDigest != original.ReportSealDigest {
			t.Fatal("retry mutated original sealed evidence", err)
		}
	})
}
