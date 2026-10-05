package store

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
)

func retentionHistoryDeleteCentral(t *testing.T, s *Store, pid string) {
	t.Helper()
	objects, err := s.AdvanceRetention(testContext, pid, 100)
	if err != nil {
		t.Fatal(err)
	}
	for i, o := range objects {
		identity := fmt.Sprintf("v1:linux:0000000000000001:%016x:1700000000:123456789", i+2)
		if _, err = s.AuthorizeRetentionObject(testContext, o.ID, RetentionObjectObservation{Identity: identity, Size: o.Size, SHA256: o.SHA256}); err != nil {
			t.Fatal(err)
		}
		for _, state := range []string{"quarantined", "deleted"} {
			if err = s.ConfirmRetentionObject(testContext, o.ID, RetentionObjectResult{State: state, Identity: identity}); err != nil {
				t.Fatal(err)
			}
		}
	}
}
func retentionHistoryDeleteNode(t *testing.T, s *Store, a NodeActor, d protocol.NodeDeletion) {
	t.Helper()
	authority, err := s.AuthorizeNodeDeletion(testContext, a, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	workspace, results := "not_applicable", "not_applicable"
	if d.HasWorkspace {
		workspace = "deleted"
	}
	if d.HasResults {
		results = "deleted"
	}
	in := retentionDeletionConfirmation(t, d, authority, 1, workspace, results, "")
	if err = s.ConfirmNodeDeletion(testContext, a, in); err != nil {
		t.Fatal(err)
	}
}
func TestRetentionHistoryNoNodeRealCleanupAndStableReplay(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		p, ids := retentionJobProject(t, s)
		var before buildRecord
		s.db.First(&before, "id = ?", ids[0])
		var projectBefore projectRecord
		s.db.First(&projectBefore, "id = ?", p.ID)
		if _, err := s.ScheduleRetention(testContext, localAdmin, p.ID, 100); err != nil {
			t.Fatal(err)
		}
		page, err := s.FinalizeRetention(testContext, p.ID, 100)
		if err != nil {
			t.Fatal(err)
		}
		e := retentionEntry(t, page, ids[0])
		if e.HistoryState != "cleaned" || e.CleanedAt == nil || e.CentralState != "completed" || e.NodeState != "not_applicable" || e.NodeCompletedAt != nil {
			t.Fatal("合法无节点不能伪确认", e)
		}
		var cleaned buildRecord
		s.db.First(&cleaned, "id = ?", ids[0])
		if cleaned.SnapshotJSON != "" || cleaned.ParameterKeysJSON != "" || cleaned.ReasonsJSON != "" || cleaned.ReportsJSON != "" || cleaned.ReportSealDigest != "" {
			t.Fatal("墓碑只打标签未删正文")
		}
		if cleaned.ID != before.ID || cleaned.BatchID != before.BatchID || cleaned.ProjectID != before.ProjectID || cleaned.Name != before.Name || cleaned.Number != nil || cleaned.Status != before.Status || !cleaned.TerminalAt.Equal(*before.TerminalAt) {
			t.Fatal("改写原身份/结果/终态时间")
		}
		var steps int64
		s.db.Model(&stepRecord{}).Where("build_id = ?", cleaned.ID).Count(&steps)
		if steps != 0 {
			t.Fatal("步骤正文仍在")
		}
		var projectAfter projectRecord
		s.db.First(&projectAfter, "id = ?", p.ID)
		var lastJob retentionJobRecord
		if err = s.db.Where("project_id = ?", p.ID).Order("created_at DESC, id DESC").First(&lastJob).Error; err != nil {
			t.Fatal(err)
		}
		if projectAfter.RetentionCandidateCursor != ids[1] || projectAfter.RetentionFinalizeCursor != lastJob.ID || projectAfter.RetentionObjectCursor != "" {
			t.Fatal("私有位置不对应实际扫描范围")
		}
		// 只允许本轮实际扫描的两个私有位置变化，所有原业务字段仍完整比较。
		projectBefore.RetentionCandidateCursor = ids[1]
		projectBefore.RetentionFinalizeCursor = lastJob.ID
		if !reflect.DeepEqual(projectBefore, projectAfter) {
			t.Fatal("清理动了项目计数器或策略")
		}
		if _, err = s.FinalizeRetention(testContext, p.ID, 100); err != nil {
			t.Fatal(err)
		}
		var same buildRecord
		s.db.First(&same, "id = ?", cleaned.ID)
		if !reflect.DeepEqual(cleaned, same) {
			t.Fatal("完成重放刷新墓碑")
		}
		if err = s.Recover(testContext); err != nil {
			t.Fatal("原Recover兼容墓碑", err)
		}
	})
}
func TestRetentionHistoryCentralAndNodeMustBothComplete(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		actor, pid, d := retentionDeletionFixture(t, s)
		var original buildRecord
		s.db.First(&original, "id = ?", d.BuildID)
		var terminal executionReceiptRecord
		s.db.First(&terminal, "build_id = ? AND seq = ?", original.ID, original.LastEventSeq)
		req := protocol.TerminalReceiptRequest{Ref: buildRef(original), Seq: terminal.Seq, Digest: terminal.Digest}
		before, err := s.TerminalReceipt(testContext, actor, req)
		if err != nil {
			t.Fatal(err)
		}
		retentionHistoryDeleteNode(t, s, actor, d)
		if _, err = s.FinalizeRetention(testContext, pid, 100); err != nil {
			t.Fatal(err)
		}
		var held buildRecord
		s.db.First(&held, "id = ?", original.ID)
		if held.HistoryState == "cleaned" || held.SnapshotJSON == "" {
			t.Fatal("Node完成冒充中央完成")
		}
		retentionHistoryDeleteCentral(t, s, pid)
		if _, err = s.FinalizeRetention(testContext, pid, 100); err != nil {
			t.Fatal(err)
		}
		var cleaned buildRecord
		s.db.First(&cleaned, "id = ?", original.ID)
		if cleaned.HistoryState != "cleaned" || cleaned.CleanedAt == nil || cleaned.SnapshotJSON != "" {
			t.Fatal("两侧完成未真清正文")
		}
		after, err := s.TerminalReceipt(testContext, actor, req)
		if err != nil || !reflect.DeepEqual(before, after) {
			t.Fatal("原精确TerminalReceipt被改写", err)
		}
		var kept executionReceiptRecord
		s.db.First(&kept, "id = ?", terminal.ID)
		if !reflect.DeepEqual(kept, terminal) {
			t.Fatal("末尾wire摘要/StopKnown变了")
		}
		var count int64
		s.db.Model(&executionReceiptRecord{}).Where("build_id = ?", original.ID).Count(&count)
		if count != 1 {
			t.Fatal("中间事件未删或删了末尾")
		}
		for _, model := range []any{&stepRecord{}, &artifactRecord{}, &logChunkRecord{}, &evidenceReadRecord{}} {
			s.db.Model(model).Where("build_id = ?", original.ID).Count(&count)
			if count != 0 {
				t.Fatal("大记录仍在")
			}
		}
		if err = s.Recover(testContext); err != nil {
			t.Fatal("保持原attempt/Ref恢复", err)
		}
	})
}
func TestRetentionHistoryProtectedNodeFactsReconcileLater(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		actor, pid, d := retentionDeletionFixture(t, s)
		retentionHistoryDeleteCentral(t, s, pid)
		authority, err := s.AuthorizeNodeDeletion(testContext, actor, d.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.SyncGlobalRetention(testContext, config.Retention{Builds: 100, Days: 30}); err != nil {
			t.Fatal(err)
		}
		in := retentionDeletionConfirmation(t, d, authority, 1, "deleted", "deleted", "")
		if err = s.ConfirmNodeDeletion(testContext, actor, in); err != nil {
			t.Fatal(err)
		}
		var deletion nodeDeletionRecord
		s.db.First(&deletion, "id = ?", d.ID)
		var job retentionJobRecord
		s.db.First(&job, "id = ?", deletion.JobID)
		if deletion.CompletedAt == nil || job.NodeCompletedAt != nil {
			t.Fatal("事实/资格未区分")
		}
		var receipt nodeDeletionReceiptRecord
		s.db.First(&receipt, "delete_id = ? AND seq = 1", d.ID)
		if _, err = s.FinalizeRetention(testContext, pid, 100); err != nil {
			t.Fatal(err)
		}
		var protected buildRecord
		s.db.First(&protected, "id = ?", d.BuildID)
		if protected.HistoryState == "cleaned" {
			t.Fatal("新policy保护仍清body")
		}
		if err = s.SyncGlobalRetention(testContext, config.Retention{Builds: 1, Days: 30}); err != nil {
			t.Fatal(err)
		}
		if _, err = s.FinalizeRetention(testContext, pid, 100); err != nil {
			t.Fatal(err)
		}
		var done buildRecord
		s.db.First(&done, "id = ?", d.BuildID)
		s.db.First(&job, "id = ?", deletion.JobID)
		if done.HistoryState != "cleaned" || job.NodeCompletedAt == nil {
			t.Fatal("已删资源要求假重删才完成")
		}
		var unchanged nodeDeletionReceiptRecord
		s.db.First(&unchanged, "id = ?", receipt.ID)
		if !reflect.DeepEqual(receipt, unchanged) {
			t.Fatal("reconcile改写已有事实或时间")
		}
	})
}
func TestRetentionHistoryRetryChildBeforeParent(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 1, Days: 30}); err != nil {
			t.Fatal(err)
		}
		p, parent := cancelledQueued(t, s)
		out, err := s.Retry(testContext, localAdmin, RetryInput{BuildID: parent, Key: "history-child"})
		if err != nil {
			t.Fatal(err)
		}
		child := out.Builds[0].ID
		if _, err = s.Cancel(testContext, localAdmin, child); err != nil {
			t.Fatal(err)
		}
		retentionSkipped(t, s, p, 1)
		if _, err = s.ScheduleRetention(testContext, localAdmin, p.ID, 100); err != nil {
			t.Fatal(err)
		}
		if _, err = s.FinalizeRetention(testContext, p.ID, 100); err != nil {
			t.Fatal(err)
		}
		var original, next buildRecord
		s.db.First(&original, "id = ?", parent)
		s.db.First(&next, "id = ?", child)
		if original.HistoryState != "live" || next.HistoryState != "cleaned" {
			t.Fatal("祖先未等实际child内容完成")
		}
		if _, err = s.ScheduleRetention(testContext, localAdmin, p.ID, 100); err != nil {
			t.Fatal(err)
		}
		if _, err = s.FinalizeRetention(testContext, p.ID, 100); err != nil {
			t.Fatal(err)
		}
		s.db.First(&original, "id = ?", parent)
		if original.HistoryState != "cleaned" {
			t.Fatal("已cleaned child永久保护祖先")
		}
		if err = s.writer.Delete(&buildRecord{}, "id = ?", parent).Error; err == nil {
			t.Fatal("关闭/丢失原RESTRICT")
		}
		if err = s.Recover(testContext); err != nil {
			t.Fatal(err)
		}
	})
}

func TestRetentionHistoryCleanedBuildUsesMinimalView(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		p, ids := retentionJobProject(t, s)
		if _, err := s.ScheduleRetention(testContext, localAdmin, p.ID, 100); err != nil {
			t.Fatal(err)
		}
		if _, err := s.FinalizeRetention(testContext, p.ID, 100); err != nil {
			t.Fatal(err)
		}
		view, err := s.GetBuild(testContext, ids[0])
		if err != nil {
			t.Fatal("真正cleaned投影不能解已清JSON", err)
		}
		if view.ID != ids[0] || view.HistoryState != "cleaned" || view.CleanedAt == nil || view.Status != "skipped" || view.Reports != nil || view.Steps == nil || view.Post == nil || view.ParameterKeys == nil || view.Reasons == nil || len(view.Steps) != 0 || len(view.Post) != 0 || len(view.ParameterKeys) != 0 {
			t.Fatal("最小墓碑投影", view.HistoryState)
		}
	})
}
func TestRetentionHistoryOriginalTriggerReplayAndNextNumber(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 1, Days: 30}); err != nil {
			t.Fatal(err)
		}
		input := projectInput("history-number")
		input.BuildNumberStart = 101
		p, err := s.CreateProject(testContext, localAdmin, input)
		if err != nil {
			t.Fatal(err)
		}
		trigger := enqueueInput(p, "original-trigger-101")
		original, err := s.Enqueue(testContext, trigger)
		if err != nil || *original.Builds[0].Number != 101 {
			t.Fatal("真实分配101", err)
		}
		id := original.Builds[0].ID
		if _, err = s.Cancel(testContext, localAdmin, id); err != nil {
			t.Fatal(err)
		}
		retentionSkipped(t, s, p, 1)
		if _, err = s.ScheduleRetention(testContext, localAdmin, p.ID, 100); err != nil {
			t.Fatal(err)
		}
		if _, err = s.FinalizeRetention(testContext, p.ID, 100); err != nil {
			t.Fatal(err)
		}
		if _, err = s.CreateGroup(testContext, localAdmin, "history-moved"); err != nil {
			t.Fatal(err)
		}
		moved, err := s.MoveProject(testContext, localAdmin, p.Name, "history-moved")
		if err != nil || moved.ID != p.ID {
			t.Fatal(err)
		}
		replay, err := s.Enqueue(testContext, trigger)
		if err != nil {
			t.Fatal("原已确认键只读cleaned关系", err)
		}
		if !replay.Replayed || replay.ID != original.ID || len(replay.Builds) != 1 || replay.Builds[0].ID != id || *replay.Builds[0].Number != 101 || replay.Builds[0].HistoryState != "cleaned" || replay.Builds[0].Group != "history-moved" {
			t.Fatal("原键重放改变身份/编号")
		}
		changed := trigger
		changed.RequestDigest = strings.Repeat("d", 64)
		if _, err = s.Enqueue(testContext, changed); err != ErrConflict {
			t.Fatal("同键不同请求未冲突", err)
		}
		next, err := s.Enqueue(testContext, enqueueInput(moved, "new-trigger-102"))
		if err != nil || *next.Builds[0].Number != 102 {
			t.Fatal("真正新请求继续102", err)
		}
	})
}
func TestRetentionHistoryConfirmedRetryReplayAndNewRetryRejected(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 1, Days: 30}); err != nil {
			t.Fatal(err)
		}
		p, parent := cancelledQueued(t, s)
		input := RetryInput{BuildID: parent, Key: "old-confirmed-retry"}
		original, err := s.Retry(testContext, localAdmin, input)
		if err != nil {
			t.Fatal(err)
		}
		child := original.Builds[0].ID
		if _, err = s.Cancel(testContext, localAdmin, child); err != nil {
			t.Fatal(err)
		}
		retentionSkipped(t, s, p, 1)
		for range 2 {
			if _, err = s.ScheduleRetention(testContext, localAdmin, p.ID, 100); err != nil {
				t.Fatal(err)
			}
			if _, err = s.FinalizeRetention(testContext, p.ID, 100); err != nil {
				t.Fatal(err)
			}
		}
		var parentRow, childRow buildRecord
		s.db.First(&parentRow, "id = ?", parent)
		s.db.First(&childRow, "id = ?", child)
		if parentRow.HistoryState != "cleaned" || childRow.HistoryState != "cleaned" {
			t.Fatal("真实child-parent清理未成立")
		}
		replay, err := s.Retry(testContext, localAdmin, input)
		if err != nil {
			t.Fatal("已确认retry键从最小batch重放", err)
		}
		if !replay.Replayed || replay.ID != original.ID || replay.Builds[0].ID != child || replay.Builds[0].RetryOf != parent || *replay.Builds[0].Number != *original.Builds[0].Number {
			t.Fatal("重放再次执行/分号")
		}
		for _, id := range []string{parent, child} {
			if _, err = s.Retry(testContext, localAdmin, RetryInput{BuildID: id, Key: "new-cleaned-retry"}); err != ErrRetentionRetired {
				t.Fatal("已清正文新retry必须拒绝", err)
			}
		}
	})
}
