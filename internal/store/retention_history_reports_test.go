package store

import (
	"reflect"
	"testing"
	"time"

	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
)

func retentionHistoryNewerSkipped(t *testing.T, s *Store, id string) string {
	t.Helper()
	pid := retentionProjectForBuild(t, s, id)
	var project projectRecord
	if err := s.db.First(&project, "id = ?", pid).Error; err != nil {
		t.Fatal(err)
	}
	p, err := projectView(project)
	if err != nil {
		t.Fatal(err)
	}
	retentionSkipped(t, s, p, 1)
	return pid
}

func retentionHistoryCompleteRegistered(t *testing.T, s *Store, a NodeActor, pid, id string) {
	t.Helper()
	if _, err := s.ScheduleRetention(testContext, localAdmin, pid, 100); err != nil {
		t.Fatal(err)
	}
	retentionHistoryDeleteCentral(t, s, pid)
	claims, err := s.ClaimNodeDeletions(testContext, a, 10)
	if err != nil || len(claims) != 1 || claims[0].BuildID != id {
		t.Fatal("实际登记事项", err, len(claims))
	}
	retentionHistoryDeleteNode(t, s, a, claims[0])
	if _, err = s.FinalizeRetention(testContext, pid, 100); err != nil {
		t.Fatal(err)
	}
}

// 使用019实际解析、上传、seal、post及终态消费者；墓碑不可改写包含报告清单的末尾摘要。
func TestRetentionHistoryReportSealAndPostRetainsExactTerminalReceipt(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 1, Days: 30}); err != nil {
			t.Fatal(err)
		}
		a, g, evidence, in := finalReportAPI(t, s)
		registration := resourceRegistration(g.Ref)
		registration.HasResults = true
		if err := s.RegisterNodeResource(testContext, a, registration); err != nil {
			t.Fatal(err)
		}
		if _, err := s.CommitArtifact(testContext, a, in); err != nil {
			t.Fatal(err)
		}
		evidence.Sealed = true
		progress := stepProgress("reports_sealed", "", "", "", 0)
		progress.StepKind = ""
		progress.Reports = &evidence
		accept(t, s, a, event(g.Ref, 6, progress))
		accept(t, s, a, event(g.Ref, 7, protocol.ExecutionProgress{Kind: "post_selected", PostPhase: "success", RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}))
		skip := stepProgress("skipped", "skipped", "always", "cleanup", 1)
		skip.Reason, skip.StopConfirmed = "condition", true
		accept(t, s, a, event(g.Ref, 8, skip))
		data, err := encode(evidence)
		if err != nil {
			t.Fatal(err)
		}
		last := event(g.Ref, 9, protocol.ExecutionProgress{Kind: "build_finished", Status: "succeeded", Started: true, StopConfirmed: true, RemainingPostBudgetNS: int64(2 * time.Minute), LastArtifactSeq: 1, ArtifactSteps: []protocol.ArtifactExpectation{}, ReportManifest: &protocol.ReportManifest{SealDigest: reportKey(data), IDs: []string{evidence.Files[0].ArtifactID}}})
		accept(t, s, a, last)
		request := protocol.TerminalReceiptRequest{Ref: last.Ref, Seq: last.Seq, Digest: last.Digest}
		before, err := s.TerminalReceipt(testContext, a, request)
		if err != nil || !before.StopKnown {
			t.Fatal(err)
		}
		var original buildRecord
		var receipt executionReceiptRecord
		var file artifactRecord
		if err = s.db.First(&original, "id = ?", g.Ref.BuildID).Error; err != nil {
			t.Fatal(err)
		}
		if err = s.db.First(&receipt, "build_id = ? AND seq = ?", original.ID, original.LastEventSeq).Error; err != nil {
			t.Fatal(err)
		}
		if err = s.db.First(&file, "id = ?", evidence.Files[0].ArtifactID).Error; err != nil {
			t.Fatal(err)
		}
		if original.ReportSealDigest == "" || original.ReportRevision != 2 || !original.ReportFinal || file.Purpose != "junit" || file.VerifiedJUnitJSON == "" || receipt.Kind != "build_finished" || !receipt.StopKnown {
			t.Fatal("没有真实019封存/终态证据")
		}
		pid := retentionHistoryNewerSkipped(t, s, original.ID)
		retentionHistoryCompleteRegistered(t, s, a, pid, original.ID)
		var cleaned buildRecord
		if err = s.db.First(&cleaned, "id = ?", original.ID).Error; err != nil {
			t.Fatal(err)
		}
		if cleaned.HistoryState != "cleaned" || cleaned.ReportsJSON != "" || cleaned.ReportRevision != 0 || cleaned.ReportFinal || cleaned.ReportSealDigest != "" || cleaned.ReportCheckedIndex != 0 || cleaned.LastArtifactSeq != original.LastArtifactSeq || cleaned.LastEventSeq != original.LastEventSeq || !cleaned.TerminalAt.Equal(*original.TerminalAt) || !reflect.DeepEqual(buildRef(cleaned), last.Ref) {
			t.Fatal("清理改写原报告身份或未清正文")
		}
		after, err := s.TerminalReceipt(testContext, a, request)
		if err != nil || !reflect.DeepEqual(before, after) {
			t.Fatal("报告清单摘要终态ACK不再精确", err)
		}
		var kept executionReceiptRecord
		if err = s.db.First(&kept, "id = ?", receipt.ID).Error; err != nil || !reflect.DeepEqual(receipt, kept) {
			t.Fatal("原末尾receipt变更", err)
		}
		for _, model := range []any{&artifactRecord{}, &stepRecord{}} {
			var count int64
			if err = s.db.Model(model).Where("build_id = ?", original.ID).Count(&count).Error; err != nil || count != 0 {
				t.Fatal("正文/VerifiedJUnit未清", err)
			}
		}
		if err = s.Recover(testContext); err != nil {
			t.Fatal("019墓碑恢复", err)
		}
	})
}

// 独立停止只保留其真实原回执，不把started末尾伪造为build_finished。
func TestRetentionHistoryInterruptedCompletionRetainsIndependentStop(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 1, Days: 30}); err != nil {
			t.Fatal(err)
		}
		a, g, _ := claimed(t, s)
		registration := resourceRegistration(g.Ref)
		registration.HasResults = true
		if err := s.RegisterNodeResource(testContext, a, registration); err != nil {
			t.Fatal(err)
		}
		accept(t, s, a, event(g.Ref, 1, stepProgress("intent", "", "ordinary", "compile", 1)))
		started := stepProgress("started", "", "ordinary", "compile", 1)
		started.Started = true
		accept(t, s, a, event(g.Ref, 2, started))
		for _, state := range []string{"disabled", "enabled"} {
			if err := s.SetNodeState(testContext, localAdmin, "linux", state); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.ConfirmNodeStopped(testContext, a, protocol.StopConfirmation{Ref: g.Ref, EvidenceCode: "process_group_reaped", Note: "本夹具独立停止证据"}); err != nil {
			t.Fatal(err)
		}
		var original buildRecord
		var receipt executionReceiptRecord
		var stop stopConfirmationRecord
		if err := s.db.First(&original, "id = ?", g.Ref.BuildID).Error; err != nil {
			t.Fatal(err)
		}
		if err := s.db.First(&receipt, "build_id = ? AND seq = 2", original.ID).Error; err != nil {
			t.Fatal(err)
		}
		if err := s.db.First(&stop, "attempt_id = ?", g.Ref.AttemptID).Error; err != nil {
			t.Fatal(err)
		}
		registration.Completion = &protocol.NodeResourceCompletion{LastEventSeq: original.LastEventSeq, LastLogSeq: original.LastLogSeq, LastLogOffset: original.LastLogOffset, LastArtifactSeq: original.LastArtifactSeq, StopCode: "process_group_reaped"}
		if err := s.RegisterNodeResource(testContext, a, registration); err != nil {
			t.Fatal(err)
		}
		pid := retentionHistoryNewerSkipped(t, s, original.ID)
		retentionHistoryCompleteRegistered(t, s, a, pid, original.ID)
		var cleaned buildRecord
		var kept executionReceiptRecord
		var keptStop stopConfirmationRecord
		if err := s.db.First(&cleaned, "id = ?", original.ID).Error; err != nil {
			t.Fatal(err)
		}
		if err := s.db.First(&kept, "id = ?", receipt.ID).Error; err != nil {
			t.Fatal(err)
		}
		if err := s.db.First(&keptStop, "id = ?", stop.ID).Error; err != nil {
			t.Fatal(err)
		}
		if cleaned.HistoryState != "cleaned" || cleaned.Status != "interrupted" || cleaned.Reason != original.Reason || cleaned.StopUnconfirmed || cleaned.LastEventSeq != 2 || !reflect.DeepEqual(buildRef(cleaned), g.Ref) || !reflect.DeepEqual(receipt, kept) || !reflect.DeepEqual(stop, keptStop) || kept.Kind != "started" || kept.StopKnown {
			t.Fatal("独立Stop改写成假终态或丢失原证据")
		}
		var count int64
		if err := s.db.Model(&executionReceiptRecord{}).Where("build_id = ?", original.ID).Count(&count).Error; err != nil || count != 1 {
			t.Fatal("末尾started未精确保留", err)
		}
		if err := s.Recover(testContext); err != nil {
			t.Fatal("真实interrupted墓碑恢复", err)
		}
	})
}
