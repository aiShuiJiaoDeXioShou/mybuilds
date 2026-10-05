package store

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"mybuilds/internal/protocol"
)

func recoveryRows(t *testing.T, s *Store) []buildRecord {
	t.Helper()
	var rows []buildRecord
	if err := s.db.Order("id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	return rows
}
func TestRecoverPreservesQueuedTerminalAndValidLease(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		actor, grant, _ := claimed(t, s)
		var running buildRecord
		if err := s.db.First(&running, "id = ?", grant.Ref.BuildID).Error; err != nil {
			t.Fatal(err)
		}
		project, err := s.GetProject(testContext, "app")
		if err != nil {
			t.Fatal(err)
		}
		queueBuild(t, s, project, "queued", "other", false)
		input := enqueueInput(project, "skipped")
		input.Builds[0].Name = "skip"
		input.Builds[0].Status = "skipped"
		input.Builds[0].Snapshot.Condition = "skipped"
		if _, err = s.Enqueue(testContext, input); err != nil {
			t.Fatal(err)
		}
		// 原定义和实际进度全部经真实API创建，不以手改状态冒充完整终态。
		completeRunEvents(t, s, actor, grant, true)
		terminal := protocol.ExecutionProgress{Kind: "build_finished", Status: "failed", Reason: "exit", Started: true, StopConfirmed: true, RemainingPostBudgetNS: int64(2*time.Minute) - 100, ArtifactSteps: []protocol.ArtifactExpectation{}}
		accept(t, s, actor, event(grant.Ref, 8, terminal))
		_, active, _ := claimedRecoveryOther(t, s, project)
		before := recoveryRows(t, s)
		if err = s.Recover(testContext); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, recoveryRows(t, s)) {
			t.Fatal("有效租约/终态/排队记录被改写")
		}
		var after buildRecord
		s.db.First(&after, "id = ?", active.Ref.BuildID)
		if buildRef(after) != active.Ref || after.LeaseExpiresAt == nil {
			t.Fatal("有效身份丢失")
		}
	})
}
func claimedRecoveryOther(t *testing.T, s *Store, p Project) (NodeActor, protocol.LeaseGrant, LeasePolicy) {
	t.Helper()
	a, session := openSession(t, s, "recovery-node", 2)
	if err := s.writer.Model(&projectRecord{}).Where("id = ?", p.ID).Updates(map[string]any{"nodes_json": "[\"linux\",\"recovery-node\"]", "default_node": "recovery-node"}).Error; err != nil {
		t.Fatal(err)
	}
	p.AllowedNodes = []string{"linux", "recovery-node"}
	p.DefaultNode = "recovery-node"
	queueBuild(t, s, p, "active-other", "active", false)
	policy := LeasePolicy{Concurrency: 2, Heartbeat: time.Second, Duration: 10 * time.Second}
	grant, err := s.Claim(testContext, a, protocol.ClaimRequest{SessionID: session.SessionID, ClaimKey: uuid.NewString()}, policy)
	if err != nil || grant == nil {
		t.Fatal("claim", err)
	}
	return a, *grant, policy
}
func TestRecoverRejectsCorruptDurableStateWithoutChanges(t *testing.T) {
	cases := []struct {
		name    string
		changes map[string]any
	}{
		{"snapshot", map[string]any{"snapshot_json": "broken-secret-marker"}},
		{"unknown-status", map[string]any{"status": "unknown"}},
		{"queued-guard", map[string]any{"stop_unconfirmed": true}},
		{"budget", map[string]any{"remaining_budget_ns": int64(-1)}},
		{"post-budget", map[string]any{"remaining_post_budget_ns": int64(-1)}},
		{"partial-ref", map[string]any{"lease_id": uuid.NewString()}},
		{"condition", map[string]any{"condition": "pending"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stores(t, func(t *testing.T, s *Store, opt Options) {
				p, err := s.CreateProject(testContext, localAdmin, projectInput("app"))
				if err != nil {
					t.Fatal(err)
				}
				id := queueBuild(t, s, p, "queued", "build", false)
				if err = s.writer.Model(&buildRecord{}).Where("id = ?", id).Updates(c.changes).Error; err != nil {
					t.Fatal(err)
				}
				before := recoveryRows(t, s)
				if err = s.Recover(testContext); err != errDatabase {
					t.Fatalf("损坏状态未安全拒绝: %v", err)
				}
				if !reflect.DeepEqual(before, recoveryRows(t, s)) {
					t.Fatal("拒绝时修改记录")
				}
			})
		})
	}
}
func TestRecoverExpiredLeaseRetainsEvidenceAndGuard(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, g, _ := claimed(t, s)
		p := stepProgress("intent", "", "ordinary", "compile", 1)
		accept(t, s, a, event(g.Ref, 1, p))
		p.Kind = "started"
		p.Started = true
		accept(t, s, a, event(g.Ref, 2, p))
		now := time.Now().UTC()
		if err := s.writer.Model(&buildRecord{}).Where("id = ?", g.Ref.BuildID).Updates(map[string]any{"lease_expires_at": now, "reason": "exit"}).Error; err != nil {
			t.Fatal(err)
		}
		if err := s.writer.Model(&attemptRecord{}).Where("id = ?", g.Ref.AttemptID).Update("lease_expires_at", now).Error; err != nil {
			t.Fatal(err)
		}
		before := recoveryRows(t, s)
		if err := s.Recover(testContext); err != nil {
			t.Fatal(err)
		}
		after := recoveryRows(t, s)
		if before[0].TerminalAt != nil {
			t.Fatal("活动构建提前保存终态时间")
		}
		retentionTimeWithin(t, after[0].TerminalAt, now, time.Now().UTC())
		before[0].TerminalAt = after[0].TerminalAt
		before[0].Status = "interrupted"
		before[0].StopUnconfirmed = true
		if !reflect.DeepEqual(before, after) {
			t.Fatal("过期覆盖原原因/身份/预算/序号")
		}
		if err := s.Recover(testContext); err != nil {
			t.Fatal("二次恢复", err)
		}
		if !reflect.DeepEqual(after, recoveryRows(t, s)) {
			t.Fatal("重复恢复刷新终态时间或原证据")
		}
		view, err := s.GetNode(testContext, localAdmin, "linux")
		if err != nil || !view.Quarantined || view.Running != 1 {
			t.Fatal("保护占用丢失", err)
		}
	})
}
func TestRecoverCancelledContext(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		ctx, cancel := context.WithCancel(testContext)
		cancel()
		if s.Recover(ctx) == nil {
			t.Fatal("取消仍通过恢复")
		}
	})
}
