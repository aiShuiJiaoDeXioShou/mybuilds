package store

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
)

func claimedPostBudget(t *testing.T, s *Store, finite, artifact bool) (NodeActor, protocol.LeaseGrant) {
	t.Helper()
	actor, session, project, policy := leaseFixture(t, s)
	in := enqueueInput(project, "post-budget")
	if finite {
		initial := int64(time.Second)
		in.Builds[0].Snapshot.Definition.Timeout = "1s"
		in.Builds[0].InitialBudgetNS = &initial
	}
	if artifact {
		in.Builds[0].Snapshot.Definition.Steps = []config.Step{{Kind: "artifact", Name: "package", Paths: []string{"output/*.apk"}}}
		in.Builds[0].Steps[0].Kind = "artifact"
		in.Builds[0].Steps[0].Name = "package"
	}
	in.Builds[0].Snapshot.Definition.Post.Success = []config.Step{{Kind: "run", Name: "success", Run: "true"}}
	in.Builds[0].Snapshot.Definition.Post.Failure = []config.Step{{Kind: "run", Name: "failure", Run: "true"}}
	in.Builds[0].Steps = append(in.Builds[0].Steps,
		StepProgress{Phase: "success", Index: 1, Name: "success", Kind: "run", Condition: "ready", Status: "pending"},
		StepProgress{Phase: "failure", Index: 1, Name: "failure", Kind: "run", Condition: "ready", Status: "pending"})
	if _, err := s.Enqueue(testContext, in); err != nil {
		t.Fatal(err)
	}
	grant, err := s.Claim(testContext, actor, protocol.ClaimRequest{SessionID: session.SessionID, ClaimKey: uuid.NewString()}, policy)
	if err != nil || grant == nil {
		t.Fatal("claim", err)
	}
	return actor, *grant
}

func TestPostSelectionTimeoutAfterConfirmedArtifactAndLogDelay(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		actor, grant := claimedPostBudget(t, s, true, true)
		remaining := int64(10 * time.Millisecond)
		progress := stepProgress("intent", "", "ordinary", "package", 1)
		progress.StepKind = "artifact"
		progress.RemainingBudgetNS = &remaining
		accept(t, s, actor, event(grant.Ref, 1, progress))
		progress.Kind = "started"
		progress.Started = true
		accept(t, s, actor, event(grant.Ref, 2, progress))
		id := uuid.NewString()
		progress.Kind = "finished"
		progress.Status = "succeeded"
		progress.StopConfirmed = true
		progress.ExitCode = 0
		progress.ArtifactIDs = []string{id}
		accept(t, s, actor, event(grant.Ref, 3, progress))
		confirmed := time.Now()
		// 普通collector完成已ACK，真实确认延迟仍消耗普通预算；不修改已确认step。
		timer := time.NewTimer(20 * time.Millisecond)
		<-timer.C
		declaration := protocol.ArtifactDeclaration{Ref: grant.Ref, ID: id, Seq: 1, Phase: "ordinary", Step: "package", Index: 1, Name: "app.apk", Size: 3, SHA256: strings.Repeat("a", 64)}
		if _, err := s.CommitArtifact(testContext, actor, ArtifactCommit{Declaration: declaration, StorageID: uuid.NewString()}); err != nil {
			t.Fatal("complete artifact metadata", err)
		}
		if _, err := s.CommitLogChunk(testContext, actor, LogCommit{Ref: grant.Ref, Seq: 1, Offset: 0, Size: 100, Digest: strings.Repeat("b", 64), StorageID: uuid.NewString(), RecordCount: 1}); err != nil {
			t.Fatal("log metadata", err)
		}
		zero := max(0, remaining-int64(time.Since(confirmed)))
		if zero != 0 {
			t.Fatal("实际确认延迟未耗尽预算")
		}
		selection := protocol.ExecutionProgress{Kind: "post_selected", PostPhase: "failure", Reason: "timeout", RemainingBudgetNS: &zero, RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}
		accept(t, s, actor, event(grant.Ref, 4, selection))
		view, err := s.GetBuild(testContext, grant.Ref.BuildID)
		if err != nil || view.Reason != "timeout" || view.PostPhase != "failure" || view.Steps[0].Status != "succeeded" || view.Steps[0].Reason != "" || !view.Steps[0].StopConfirmed || view.RemainingBudgetNS == nil || *view.RemainingBudgetNS != 0 {
			t.Fatal("普通成功证据或预算被改写", err)
		}
		skip := stepProgress("skipped", "skipped", "success", "success", 1)
		skip.Reason = "not_selected"
		skip.StopConfirmed = true
		skip.RemainingBudgetNS = &zero
		accept(t, s, actor, event(grant.Ref, 5, skip))
		seq := int64(6)
		for _, post := range []struct{ phase, name string }{{"failure", "failure"}, {"always", "cleanup"}} {
			p := stepProgress("intent", "", post.phase, post.name, 1)
			p.RemainingBudgetNS = &zero
			accept(t, s, actor, event(grant.Ref, seq, p))
			seq++
			p.Kind, p.Started = "started", true
			accept(t, s, actor, event(grant.Ref, seq, p))
			seq++
			p.Kind, p.Status, p.StopConfirmed, p.ExitCode = "finished", "succeeded", true, 0
			accept(t, s, actor, event(grant.Ref, seq, p))
			seq++
		}
		terminal := protocol.ExecutionProgress{Kind: "build_finished", Status: "failed", Reason: "timeout", Started: true, StopConfirmed: true, RemainingBudgetNS: &zero, RemainingPostBudgetNS: int64(2 * time.Minute), LastLogSeq: 1, LastLogOffset: 100, LastArtifactSeq: 1, ArtifactSteps: []protocol.ArtifactExpectation{{Phase: "ordinary", Index: 1, Count: 1, IDs: []string{id}}}}
		for _, status := range []string{"succeeded", "cancelled"} {
			bad := terminal
			bad.Status = status
			if status == "cancelled" {
				// 即使用户随后cancel，也不能把先确认的timeout变成取消终态。
				if _, err := s.Cancel(testContext, localAdmin, grant.Ref.BuildID); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.ApplyEvent(testContext, actor, event(grant.Ref, seq, bad)); err != ErrEventConflict {
				t.Fatalf("timeout became %s: %v", status, err)
			}
		}
		accept(t, s, actor, event(grant.Ref, seq, terminal))
		view, err = s.GetBuild(testContext, grant.Ref.BuildID)
		if err != nil || view.Status != "failed" || view.Reason != "timeout" || view.Steps[0].Status != "succeeded" || view.StopUnconfirmed {
			t.Fatal("超时终态错误", err)
		}
		files, err := s.ListArtifacts(testContext, localAdmin, grant.Ref.BuildID, Page{})
		if err != nil || len(files) != 1 || files[0].ID != id {
			t.Fatal("完整诊断副本丢失", err)
		}
	})
}

func TestPostSelectionTimeoutCannotForgeOrOverwriteOrdinaryResult(t *testing.T) {
	for _, tc := range []struct {
		name, status, post, reason string
		finite, skipped, cancel    bool
		remaining                  int64
	}{
		{name: "unlimited", status: "succeeded", post: "failure", reason: "timeout"},
		{name: "nonzero", finite: true, remaining: 1, status: "succeeded", post: "failure", reason: "timeout"},
		{name: "all_skipped", finite: true, skipped: true, post: "failure", reason: "timeout"},
		{name: "original_failure", finite: true, status: "failed", post: "failure", reason: "timeout"},
		{name: "original_cancelled", finite: true, status: "cancelled", post: "failure", reason: "timeout"},
		{name: "cancel_requested", finite: true, status: "succeeded", cancel: true, post: "failure", reason: "timeout"},
		{name: "success_with_timeout", finite: true, status: "succeeded", post: "success", reason: "timeout"},
		{name: "success_without_timeout", finite: true, status: "succeeded", post: "success"},
		{name: "failure_without_timeout", finite: true, status: "succeeded", post: "failure"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stores(t, func(t *testing.T, s *Store, opt Options) {
				actor, grant := claimedPostBudget(t, s, tc.finite, false)
				var remaining *int64
				if tc.finite {
					remaining = &tc.remaining
				}
				p := stepProgress("intent", "", "ordinary", "compile", 1)
				p.RemainingBudgetNS = remaining
				seq := int64(1)
				if tc.skipped {
					p.Kind, p.Status, p.Reason, p.StopConfirmed = "skipped", "skipped", "condition", true
					accept(t, s, actor, event(grant.Ref, seq, p))
					seq++
				} else {
					accept(t, s, actor, event(grant.Ref, seq, p))
					seq++
					p.Kind, p.Started = "started", true
					accept(t, s, actor, event(grant.Ref, seq, p))
					seq++
					p.Kind, p.Status, p.StopConfirmed = "finished", tc.status, true
					if tc.status == "failed" {
						p.Reason = "exit"
					} else if tc.status == "cancelled" {
						p.Reason = "cancelled"
					}
					accept(t, s, actor, event(grant.Ref, seq, p))
					seq++
				}
				if tc.cancel {
					if _, err := s.Cancel(testContext, localAdmin, grant.Ref.BuildID); err != nil {
						t.Fatal(err)
					}
				}
				original, err := s.GetBuild(testContext, grant.Ref.BuildID)
				if err != nil {
					t.Fatal(err)
				}
				bad := protocol.ExecutionProgress{Kind: "post_selected", PostPhase: tc.post, Reason: tc.reason, RemainingBudgetNS: remaining, RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}
				if _, err := s.ApplyEvent(testContext, actor, event(grant.Ref, seq, bad)); err != ErrEventConflict {
					t.Fatal("伪造选择或原因覆盖", err)
				}
				var row buildRecord
				if err := s.db.First(&row, "id = ?", grant.Ref.BuildID).Error; err != nil || row.LastEventSeq != seq-1 || row.PostPhase != "" || row.Reason != original.Reason {
					t.Fatal("拒绝事件未回滚", err)
				}
			})
		})
	}
}
