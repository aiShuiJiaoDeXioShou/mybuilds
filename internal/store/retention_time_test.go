package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"mybuilds/internal/protocol"
)

func retentionTimeBuild(t *testing.T, s *Store, id string) buildRecord {
	t.Helper()
	var row buildRecord
	if err := s.db.First(&row, "id = ?", id).Error; err != nil {
		t.Fatal("读取实际终态", safeError(err))
	}
	return row
}
func retentionTimeWithin(t *testing.T, value *time.Time, start, end time.Time) {
	t.Helper()
	// PostgreSQL真实timestamp精度为微秒，边界仅按该精度比较。
	if value == nil || value.Before(start.Truncate(time.Microsecond)) || value.After(end.Add(time.Microsecond)) {
		t.Fatal("终态时间不是本次服务端转换时间")
	}
}
func TestRetentionTimeTerminalAndReplay(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		a, grant, _ := claimed(t, s)
		if retentionTimeBuild(t, s, grant.Ref.BuildID).TerminalAt != nil {
			t.Fatal("活动构建提前记录终态时间")
		}
		completeRunEvents(t, s, a, grant, false)
		if retentionTimeBuild(t, s, grant.Ref.BuildID).TerminalAt != nil {
			t.Fatal("步骤结束不能冒充构建结束")
		}
		p := protocol.ExecutionProgress{Kind: "build_finished", Status: "succeeded", Started: true, StopConfirmed: true, RemainingPostBudgetNS: int64(2*time.Minute) - 100, ArtifactSteps: []protocol.ArtifactExpectation{}}
		e := event(grant.Ref, 8, p)
		// 节点时间不作为可信TerminalAt来源。
		e.Progress.At = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		wire, err := json.Marshal(e.Progress)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(wire)
		e.Digest = hex.EncodeToString(digest[:])
		start := time.Now().UTC()
		accept(t, s, a, e)
		before := retentionTimeBuild(t, s, grant.Ref.BuildID)
		retentionTimeWithin(t, before.TerminalAt, start, time.Now().UTC())
		if _, err := s.ApplyEvent(testContext, a, e); err != ErrLeaseInvalid {
			t.Fatal("终态重放不能恢复旧执行权", err)
		}
		if _, err := s.TerminalReceipt(testContext, a, protocol.TerminalReceiptRequest{Ref: e.Ref, Seq: e.Seq, Digest: e.Digest}); err != nil {
			t.Fatal(err)
		}
		if err := s.Migrate(testContext); err != nil {
			t.Fatal(err)
		}
		after := retentionTimeBuild(t, s, grant.Ref.BuildID)
		if after.TerminalAt == nil || !after.TerminalAt.Equal(*before.TerminalAt) {
			t.Fatal("回执重放或一般迁移刷新终态时间")
		}
	})
}
func TestRetentionTimeQueuedCancelAndSkipped(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		project, err := s.CreateProject(testContext, localAdmin, projectInput("retention-time"))
		if err != nil {
			t.Fatal(err)
		}
		id := queueBuild(t, s, project, "queued-cancel", "queued", false)
		start := time.Now().UTC()
		view, err := s.Cancel(testContext, localAdmin, id)
		if err != nil {
			t.Fatal(err)
		}
		before := retentionTimeBuild(t, s, id)
		retentionTimeWithin(t, before.TerminalAt, start, time.Now().UTC())
		if view.TerminalAt == nil || !view.TerminalAt.Equal(*before.TerminalAt) {
			t.Fatal("视图缺真实终态时间")
		}
		if _, err = s.Cancel(testContext, localAdmin, id); err != nil {
			t.Fatal(err)
		}
		after := retentionTimeBuild(t, s, id)
		if after.TerminalAt == nil || !after.TerminalAt.Equal(*before.TerminalAt) {
			t.Fatal("重复取消刷新时间")
		}
		in := enqueueInput(project, "created-skipped")
		in.Builds[0].Status = "skipped"
		in.Builds[0].Snapshot.Condition = "skipped"
		out, err := s.Enqueue(testContext, in)
		if err != nil {
			t.Fatal(err)
		}
		row := retentionTimeBuild(t, s, out.Builds[0].ID)
		if row.TerminalAt == nil || !row.TerminalAt.Equal(row.CreatedAt) {
			t.Fatal("创建skipped未用实际创建时间")
		}
	})
}
func TestRetentionTimeInterruptAndStopConfirmation(t *testing.T) {
	for _, cause := range []string{"expire", "disable", "rotate", "revoke"} {
		t.Run(cause, func(t *testing.T) {
			stores(t, func(t *testing.T, s *Store, _ Options) {
				_, grant, _ := claimed(t, s)
				start := time.Now().UTC()
				var err error
				switch cause {
				case "expire":
					err = s.writer.Model(&buildRecord{}).Where("id = ?", grant.Ref.BuildID).Update("lease_expires_at", start.Add(-time.Hour)).Error
					if err == nil {
						err = s.ExpireLeases(testContext)
					}
				case "disable":
					err = s.SetNodeState(testContext, localAdmin, "linux", "disabled")
				case "rotate":
					_, err = s.RotateNodeToken(testContext, localAdmin, "linux")
				case "revoke":
					err = s.RevokeNodeToken(testContext, localAdmin, "linux")
				}
				if err != nil {
					t.Fatal(err)
				}
				before := retentionTimeBuild(t, s, grant.Ref.BuildID)
				if before.Status != "interrupted" || !before.StopUnconfirmed {
					t.Fatal("未保持真实停止保护")
				}
				retentionTimeWithin(t, before.TerminalAt, start, time.Now().UTC())
				confirmation := protocol.StopConfirmation{Ref: grant.Ref, EvidenceCode: "admin_observed_stopped", Note: "真实测试确认本次执行实例已停止"}
				if err = s.ConfirmStopped(testContext, localAdmin, confirmation); err != nil {
					t.Fatal(err)
				}
				if err = s.ExpireLeases(testContext); err != nil {
					t.Fatal(err)
				}
				after := retentionTimeBuild(t, s, grant.Ref.BuildID)
				if after.StopUnconfirmed || after.TerminalAt == nil || !after.TerminalAt.Equal(*before.TerminalAt) {
					t.Fatal("停止确认或重复过期刷新终态时间")
				}
			})
		})
	}
}
func TestRetentionTimeLegacySources(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		_, terminal := terminalReceiptFixture(t, s)
		var receipt executionReceiptRecord
		if err := s.db.First(&receipt, "build_id = ? AND seq = ?", terminal.Ref.BuildID, terminal.Seq).Error; err != nil {
			t.Fatal(err)
		}
		oldTerminal := retentionTimeBuild(t, s, terminal.Ref.BuildID)
		if err := s.writer.Model(&oldTerminal).Update("terminal_at", nil).Error; err != nil {
			t.Fatal(err)
		}
		project, err := s.CreateProject(testContext, localAdmin, projectInput("legacy-times"))
		if err != nil {
			t.Fatal(err)
		}
		knownSkipped := queueBuild(t, s, project, "old-skipped", "skip", false)
		knownCancel := queueBuild(t, s, project, "old-cancel", "cancel", false)
		unknown := queueBuild(t, s, project, "old-unknown", "unknown", false)
		if err = s.writer.Model(&buildRecord{}).Where("id = ?", knownSkipped).Updates(map[string]any{"status": "skipped", "reason": "condition_skipped", "terminal_at": nil}).Error; err != nil {
			t.Fatal(err)
		}
		skipped := retentionTimeBuild(t, s, knownSkipped)
		if err = s.writer.Model(&buildRecord{}).Where("id = ?", knownCancel).Updates(map[string]any{"status": "cancelled", "terminal_at": nil}).Error; err != nil {
			t.Fatal(err)
		}
		cancelTime := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
		for _, at := range []time.Time{cancelTime, cancelTime.Add(time.Minute)} {
			if err = s.writer.Create(&auditRecord{ID: uuid.NewString(), ActorID: localAdmin.ID, Action: "build_cancel", ObjectID: knownCancel, To: "cancelled", CreatedAt: at}).Error; err != nil {
				t.Fatal(err)
			}
		}
		if err = s.writer.Model(&buildRecord{}).Where("id = ?", unknown).Updates(map[string]any{"status": "interrupted", "stop_unconfirmed": true, "terminal_at": nil, "lease_expires_at": cancelTime}).Error; err != nil {
			t.Fatal(err)
		}
		for pass := 0; pass < 2; pass++ {
			if err = s.Migrate(testContext); err != nil {
				t.Fatal(err)
			}
			row := retentionTimeBuild(t, s, terminal.Ref.BuildID)
			if row.TerminalAt == nil || !row.TerminalAt.Equal(receipt.CreatedAt) {
				t.Fatal("精确末尾build_finished receipt未回填")
			}
			row = retentionTimeBuild(t, s, knownSkipped)
			if row.TerminalAt == nil || !row.TerminalAt.Equal(skipped.CreatedAt) {
				t.Fatal("无attempt skipped未回填真实CreatedAt")
			}
			row = retentionTimeBuild(t, s, knownCancel)
			if row.TerminalAt == nil || !row.TerminalAt.Equal(cancelTime) {
				t.Fatal("queued取消未用最早真正取消审计")
			}
			if retentionTimeBuild(t, s, unknown).TerminalAt != nil {
				t.Fatal("旧中断时间借lease expiry虚构")
			}
		}
	})
}

func TestRetentionTimeLegacyEvidenceMustBeExact(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		_, terminal := terminalReceiptFixture(t, s)
		before := retentionTimeBuild(t, s, terminal.Ref.BuildID)
		var receipt executionReceiptRecord
		if err := s.db.First(&receipt, "build_id = ? AND seq = ?", before.ID, terminal.Seq).Error; err != nil {
			t.Fatal(err)
		}
		for _, c := range []struct {
			column string
			value  any
		}{
			{"last_event_seq", before.LastEventSeq + 1},
			{"lease_epoch", before.LeaseEpoch + 1},
			{"status", "running"},
		} {
			if err := s.writer.Model(&buildRecord{}).Where("id = ?", before.ID).Updates(map[string]any{"terminal_at": nil, c.column: c.value}).Error; err != nil {
				t.Fatal(err)
			}
			if err := s.Migrate(testContext); err != nil {
				t.Fatal(err)
			}
			if retentionTimeBuild(t, s, before.ID).TerminalAt != nil {
				t.Fatalf("旧%s不精确仍回填", c.column)
			}
			if err := s.writer.Model(&buildRecord{}).Where("id = ?", before.ID).Updates(map[string]any{"last_event_seq": before.LastEventSeq, "lease_epoch": before.LeaseEpoch, "status": before.Status}).Error; err != nil {
				t.Fatal(err)
			}
		}
		for _, kind := range []string{"", "step_finished", "finished"} {
			if err := s.writer.Model(&buildRecord{}).Where("id = ?", before.ID).Update("terminal_at", nil).Error; err != nil {
				t.Fatal(err)
			}
			if err := s.writer.Model(&executionReceiptRecord{}).Where("id = ?", receipt.ID).Update("kind", kind).Error; err != nil {
				t.Fatal(err)
			}
			if err := s.Migrate(testContext); err != nil {
				t.Fatal(err)
			}
			if retentionTimeBuild(t, s, before.ID).TerminalAt != nil {
				t.Fatal("非明确build_finished回执借时间回填")
			}
		}
		if err := s.writer.Model(&executionReceiptRecord{}).Where("id = ?", receipt.ID).Updates(map[string]any{"kind": "build_finished", "stop_known": false}).Error; err != nil {
			t.Fatal(err)
		}
		if err := s.Migrate(testContext); err != nil {
			t.Fatal(err)
		}
		row := retentionTimeBuild(t, s, before.ID)
		if row.TerminalAt == nil || !row.TerminalAt.Equal(receipt.CreatedAt) {
			t.Fatal("可信终态时间不能要求已经确认物理停止")
		}
		project, err := s.CreateProject(testContext, localAdmin, projectInput("legacy-no-audit"))
		if err != nil {
			t.Fatal(err)
		}
		id := queueBuild(t, s, project, "cancel-no-audit", "cancel", false)
		if err = s.writer.Model(&buildRecord{}).Where("id = ?", id).Updates(map[string]any{"status": "cancelled", "terminal_at": nil}).Error; err != nil {
			t.Fatal(err)
		}
		if err = s.writer.Create(&auditRecord{ID: uuid.NewString(), ActorID: localAdmin.ID, Action: "build_cancel", ObjectID: id, To: "running", CreatedAt: time.Now().UTC()}).Error; err != nil {
			t.Fatal(err)
		}
		if err = s.Migrate(testContext); err != nil {
			t.Fatal(err)
		}
		if retentionTimeBuild(t, s, id).TerminalAt != nil {
			t.Fatal("请求取消审计不是已取消终态事实")
		}
		if _, err = s.Cancel(testContext, localAdmin, id); err != nil {
			t.Fatal(err)
		}
		for pass := 0; pass < 2; pass++ {
			if err = s.Migrate(testContext); err != nil {
				t.Fatal(err)
			}
			if retentionTimeBuild(t, s, id).TerminalAt != nil {
				t.Fatal("重复取消审计虚构旧首次终态时间")
			}
		}
	})
}
