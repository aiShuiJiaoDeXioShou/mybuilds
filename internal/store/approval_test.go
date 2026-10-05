package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"strings"
	"testing"
	"time"
)

func approvalClaimed(t *testing.T, s *Store) (NodeActor, protocol.LeaseGrant, LeasePolicy) {
	t.Helper()
	a, session, p, policy := leaseFixture(t, s)
	in := enqueueInput(p, "approval")
	in.Builds[0].Snapshot.Definition.Steps = append(in.Builds[0].Snapshot.Definition.Steps, config.Step{Kind: "approval", Name: "release"}, config.Step{Kind: "run", Name: "after", Run: "true"})
	in.Builds[0].Steps = append(in.Builds[0].Steps, StepProgress{Phase: "ordinary", Index: 2, Name: "release", Kind: "approval", Condition: "ready", Status: "pending"}, StepProgress{Phase: "ordinary", Index: 3, Name: "after", Kind: "run", Condition: "ready", Status: "pending"})
	if _, err := s.Enqueue(testContext, in); err != nil {
		t.Fatal("真实审批入队", err)
	}
	g, err := s.Claim(testContext, a, protocol.ClaimRequest{SessionID: session.SessionID, ClaimKey: uuid.NewString()}, policy)
	if err != nil || g == nil {
		t.Fatal("真实领取", err)
	}
	for i, kind := range []string{"intent", "started", "finished"} {
		p := stepProgress(kind, "", "ordinary", "compile", 1)
		if i > 0 {
			p.Started = true
		}
		if i == 2 {
			p.Status = "succeeded"
			p.StopConfirmed = true
			p.ExitCode = 0
		}
		accept(t, s, a, event(g.Ref, int64(i+1), p))
	}
	return a, *g, policy
}
func approvalCheckpoint(t *testing.T, g protocol.LeaseGrant) protocol.ExecutionEvent {
	t.Helper()
	p := protocol.ExecutionProgress{Kind: "approval_checkpoint", Phase: "ordinary", Index: 2, Name: "release", StepKind: "approval", PostPhase: "none", StopConfirmed: true, ExitCode: -1, At: time.Now().UTC(), RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}, Approval: &protocol.ApprovalCheckpointEvidence{ID: uuid.NewString(), Revision: 1, WorkspaceID: uuid.NewString(), ResultID: uuid.NewString(), NextOrdinaryIndex: 3, Artifacts: []protocol.ArtifactExpectation{}, PublishIntents: []protocol.PublishExpectation{}, SystemResourcesClosed: true}}
	snapshot, err := protocol.ApprovalSnapshotDigest(*g.Task)
	if err != nil {
		t.Fatal(err)
	}
	p.Approval.SnapshotDigest = snapshot
	d, err := protocol.ApprovalCheckpointDigest(g.Ref, 4, p)
	if err != nil {
		t.Fatal(err)
	}
	p.Approval.CheckpointDigest = d
	// event()会生成新的At；摘要必须绑定最终同一Progress。
	e := event(g.Ref, 4, p)
	d, err = protocol.ApprovalCheckpointDigest(g.Ref, 4, e.Progress)
	if err != nil {
		t.Fatal(err)
	}
	e.Progress.Approval.CheckpointDigest = d
	e.Digest = progressDigest(e.Progress)
	return e
}
func TestApprovalCheckpointActuallyPausesAndFreezes(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, g, _ := approvalClaimed(t, s)
		e := approvalCheckpoint(t, g)
		accept(t, s, a, e)
		v, err := s.GetBuild(testContext, g.Ref.BuildID)
		if err != nil || v.Status != "waiting_approval" {
			t.Fatal("没有真实挂起", v.Status, err)
		}
		if err = s.CheckExecution(testContext, a, g.Ref); err != ErrLeaseInvalid {
			t.Fatal("旧权仍可执行", err)
		}
		var r executionReceiptRecord
		if err = s.db.First(&r, "build_id = ? AND seq = ?", g.Ref.BuildID, 4).Error; err != nil || r.Kind != "approval_checkpoint" || !r.StopKnown {
			t.Fatal("缺少真实安全收据", err)
		}
		var paused approvalRecord
		if err = s.db.First(&paused, "id = ?", e.Progress.Approval.ID).Error; err != nil || paused.State != "pending" || paused.CheckpointDigest != e.Progress.Approval.CheckpointDigest {
			t.Fatal("检查点未持久", err)
		}
		if err = s.Close(); err != nil {
			t.Fatal(err)
		}
		reopened, err := Open(testContext, opt)
		if err != nil {
			t.Fatal(err)
		}
		defer reopened.Close()
		if err = reopened.Migrate(testContext); err != nil {
			t.Fatal(err)
		}
		if err = reopened.Recover(testContext); err != nil {
			t.Fatal("真实控制重开无法恢复安全审批", err)
		}
		restored, err := reopened.GetBuild(testContext, g.Ref.BuildID)
		if err != nil || restored.Status != "waiting_approval" || restored.AttemptID != g.Ref.AttemptID || restored.LeaseEpoch != g.Ref.Epoch {
			t.Fatal("控制重开改变原检查点", err)
		}

	})
}

func TestApprovalDecisionResumeKeepsAttemptAndOriginalFence(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, g, policy := approvalClaimed(t, s)
		e := approvalCheckpoint(t, g)
		accept(t, s, a, e)
		decision := ApprovalDecision{ApprovalID: e.Progress.Approval.ID, Revision: 1, CheckpointDigest: e.Progress.Approval.CheckpointDigest, Decision: "approve", Note: "已确认"}
		view, err := s.DecideApproval(testContext, localAdmin, g.Ref.BuildID, decision)
		if err != nil || view.State != "approved" {
			t.Fatal("实际批准", err)
		}
		if _, err = s.DecideApproval(testContext, localAdmin, g.Ref.BuildID, decision); err != nil {
			t.Fatal("同内容审批重放", err)
		}
		decision.Decision = "reject"
		if _, err = s.DecideApproval(testContext, localAdmin, g.Ref.BuildID, decision); err != ErrConflict {
			t.Fatal("决定被覆盖", err)
		}
		next, err := s.Claim(testContext, a, protocol.ClaimRequest{SessionID: g.Ref.SessionID, ClaimKey: uuid.NewString()}, policy)
		if err != nil || next == nil || next.Ref.AttemptID != g.Ref.AttemptID || next.Ref.Epoch != g.Ref.Epoch+1 || next.Task.Resume == nil {
			t.Fatal("同attempt真实恢复", err)
		}
		if next.Task.Resume.CheckpointRef != g.Ref || next.Task.Resume.EventSeq != 4 || next.Task.Resume.NextOrdinaryIndex != 3 {
			t.Fatal("恢复证据未保留原归属")
		}
		if err = s.CheckExecution(testContext, a, g.Ref); err != ErrLeaseInvalid {
			t.Fatal("旧fence复活", err)
		}
		if err = s.CheckExecution(testContext, a, next.Ref); err != nil {
			t.Fatal("新权非法", err)
		}
		lookup := protocol.ApprovalCheckpointLookup{Ref: g.Ref, ApprovalID: e.Progress.Approval.ID, Revision: 1, Seq: 4, Digest: e.Digest, CheckpointDigest: e.Progress.Approval.CheckpointDigest}
		receipt, err := s.ReadApprovalCheckpoint(testContext, a, lookup)
		if err != nil || receipt.State != "resumed" {
			t.Fatal("旧checkpoint只读不可核验", err)
		}
	})
}

// 同一真实挂起检查点的二十个决定只允许一个持久决定，其余相同请求返回同事实。
func TestApprovalConcurrentDecisionsAndCurrentRole(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		a, g, _ := approvalClaimed(t, s)
		e := approvalCheckpoint(t, g)
		accept(t, s, a, e)
		decision := ApprovalDecision{ApprovalID: e.Progress.Approval.ID, Revision: 1, CheckpointDigest: e.Progress.Approval.CheckpointDigest, Decision: "approve"}
		token, err := s.CreateToken(testContext, localAdmin, "trigger")
		if err != nil {
			t.Fatal(err)
		}
		trigger, err := s.Authenticate(testContext, token.Token)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.DecideApproval(testContext, trigger, g.Ref.BuildID, decision); err != ErrForbidden {
			t.Fatal("trigger获得审批权", err)
		}
		done := make(chan error, 20)
		for i := 0; i < 20; i++ {
			go func() { _, err := s.DecideApproval(testContext, localAdmin, g.Ref.BuildID, decision); done <- err }()
		}
		for i := 0; i < 20; i++ {
			if err := <-done; err != nil {
				t.Fatal("一致决定未幂等", err)
			}
		}
		var count int64
		if err := s.db.Model(&approvalRecord{}).Where("build_id=? AND state=?", g.Ref.BuildID, "approved").Count(&count).Error; err != nil || count != 1 {
			t.Fatal("重复实际决定", err)
		}
	})
}

func TestApprovalOpinionPublicOnlyDigest(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		a, g, _ := approvalClaimed(t, s)
		e := approvalCheckpoint(t, g)
		accept(t, s, a, e)
		note := "own-sensitive-opinion-marker-74219"
		view, err := s.DecideApproval(testContext, localAdmin, g.Ref.BuildID, ApprovalDecision{ApprovalID: e.Progress.Approval.ID, Revision: 1, CheckpointDigest: e.Progress.Approval.CheckpointDigest, Decision: "approve", Note: note})
		raw, _ := json.Marshal(view)
		h := sha256.Sum256([]byte(note))
		if err != nil || strings.Contains(string(raw), note) || view.NoteDigest != hex.EncodeToString(h[:]) {
			t.Fatal("公共意见泄漏或摘要不准确", err)
		}
		var private approvalRecord
		if err = s.db.First(&private, "id=?", view.ID).Error; err != nil || private.Note != note {
			t.Fatal("原审计意见丢失", err)
		}
		if validApprovalNote("${PRIVATE}") || validApprovalNote("token=private") {
			t.Fatal("明确材料格式被保存")
		}
	})
}
