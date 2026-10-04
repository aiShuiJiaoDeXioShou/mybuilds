package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/google/uuid"
	"mybuilds/internal/protocol"
	"testing"
	"time"
)

func claimed(t *testing.T, s *Store) (NodeActor, protocol.LeaseGrant, LeasePolicy) {
	t.Helper()
	actor, session, p, policy := leaseFixture(t, s)
	queueBuild(t, s, p, "event", "android", false)
	grant, err := s.Claim(testContext, actor, protocol.ClaimRequest{SessionID: session.SessionID, ClaimKey: uuid.NewString()}, policy)
	if err != nil || grant == nil {
		t.Fatal(err)
	}
	return actor, *grant, policy
}
func event(ref protocol.LeaseRef, seq int64, p protocol.ExecutionProgress) protocol.ExecutionEvent {
	p.At = time.Now().UTC()
	b, _ := json.Marshal(p)
	digest := sha256.Sum256(b)
	return protocol.ExecutionEvent{Ref: ref, Seq: seq, Digest: hex.EncodeToString(digest[:]), Progress: p}
}
func stepProgress(kind, status, phase, name string, index int) protocol.ExecutionProgress {
	return protocol.ExecutionProgress{Kind: kind, Status: status, Phase: phase, Name: name, StepKind: "run", Index: index, ExitCode: -1, RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}
}
func accept(t *testing.T, s *Store, a NodeActor, e protocol.ExecutionEvent) {
	t.Helper()
	ack, err := s.ApplyEvent(testContext, a, e)
	if err != nil || ack.Seq != e.Seq || ack.Digest != e.Digest {
		t.Fatalf("event %s/%s/%d: %v", e.Progress.Kind, e.Progress.Phase, e.Seq, err)
	}
}
func completeRunEvents(t *testing.T, s *Store, a NodeActor, grant protocol.LeaseGrant, failed bool) int64 {
	t.Helper()
	ref := grant.Ref
	p := stepProgress("intent", "", "ordinary", "compile", 1)
	accept(t, s, a, event(ref, 1, p))
	p.Kind = "started"
	p.Started = true
	accept(t, s, a, event(ref, 2, p))
	p.Kind = "finished"
	p.Status = "succeeded"
	p.StopConfirmed = true
	p.ExitCode = 0
	p.ElapsedNS = 1234567
	if failed {
		p.Status = "failed"
		p.Reason = "exit"
		p.ExitCode = 3
	}
	accept(t, s, a, event(ref, 3, p))
	p = protocol.ExecutionProgress{Kind: "post_selected", PostPhase: "success", RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}
	if failed {
		p.PostPhase = "failure"
	}
	accept(t, s, a, event(ref, 4, p))
	p = stepProgress("intent", "", "always", "cleanup", 1)
	accept(t, s, a, event(ref, 5, p))
	p.Kind = "started"
	p.Started = true
	accept(t, s, a, event(ref, 6, p))
	p.Kind = "finished"
	p.Status = "succeeded"
	p.StopConfirmed = true
	p.ExitCode = 0
	p.RemainingPostBudgetNS -= 100
	accept(t, s, a, event(ref, 7, p))
	return 7
}
func TestExecutionEventOrderReceiptBudgetAndTerminal(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, grant, _ := claimed(t, s)
		p := stepProgress("intent", "", "ordinary", "compile", 1)
		first := event(grant.Ref, 1, p)
		p.Kind = "started"
		p.Started = true
		if _, err := s.ApplyEvent(testContext, a, event(grant.Ref, 1, p)); err != ErrEventConflict {
			t.Fatal("started without intent", err)
		}
		accept(t, s, a, first)
		accept(t, s, a, first)
		p = stepProgress("intent", "", "ordinary", "compile", 1)
		p.Name = "other"
		if _, err := s.ApplyEvent(testContext, a, event(grant.Ref, 2, p)); err != ErrEventConflict {
			t.Fatal("wrong step", err)
		}
		p = stepProgress("started", "", "ordinary", "compile", 1)
		p.Started = true
		if _, err := s.ApplyEvent(testContext, a, event(grant.Ref, 3, p)); err != ErrSequenceInvalid {
			t.Fatal("sequence gap", err)
		}
		p.RemainingPostBudgetNS++
		if _, err := s.ApplyEvent(testContext, a, event(grant.Ref, 2, p)); err != ErrBudgetInvalid {
			t.Fatal("budget increased", err)
		}
		p.RemainingPostBudgetNS--
		accept(t, s, a, event(grant.Ref, 2, p))
		accept(t, s, a, first) // 完整历史ACK，不仅最后seq。
		conflict := first
		conflict.Progress.Reason = "exit"
		conflict = event(grant.Ref, 1, conflict.Progress)
		if _, err := s.ApplyEvent(testContext, a, conflict); err != ErrEventConflict {
			t.Fatal("conflicting replay", err)
		}
		terminal := protocol.ExecutionProgress{Kind: "build_finished", Status: "succeeded", Started: true, StopConfirmed: true, RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}
		if _, err := s.ApplyEvent(testContext, a, event(grant.Ref, 3, terminal)); err != ErrEventConflict {
			t.Fatal("unfinished intent terminal", err)
		}
		p.Kind = "finished"
		p.Status = "succeeded"
		p.StopConfirmed = true
		p.ExitCode = 0
		accept(t, s, a, event(grant.Ref, 3, p))
		selectPost := protocol.ExecutionProgress{Kind: "post_selected", PostPhase: "success", RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}
		accept(t, s, a, event(grant.Ref, 4, selectPost))
		p = stepProgress("skipped", "skipped", "always", "cleanup", 1)
		p.Reason = "condition"
		p.StopConfirmed = true
		accept(t, s, a, event(grant.Ref, 5, p))
		terminal.LastLogSeq = 1
		if _, err := s.ApplyEvent(testContext, a, event(grant.Ref, 6, terminal)); err != ErrEventConflict {
			t.Fatal("missing log cursor", err)
		}
		terminal.LastLogSeq = 0
		accept(t, s, a, event(grant.Ref, 6, terminal))
		if _, err := s.ApplyEvent(testContext, a, first); err != ErrLeaseInvalid {
			t.Fatal("terminal replay revived", err)
		}
		view, err := s.GetBuild(testContext, grant.Ref.BuildID)
		if err != nil || view.Status != "succeeded" || !view.Steps[0].Intent || !view.Steps[0].Started || !view.Steps[0].StopConfirmed || view.PostPhase != "success" || view.NodeID != a.ID {
			t.Fatal("safe evidence", view, err)
		}
	})
}
func TestExecutionFailurePreservesOrdinaryReason(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, grant, _ := claimed(t, s)
		seq := completeRunEvents(t, s, a, grant, true)
		terminal := protocol.ExecutionProgress{Kind: "build_finished", Status: "failed", Reason: "exit", Started: true, StopConfirmed: true, ExitCode: 3, RemainingPostBudgetNS: int64(2*time.Minute) - 100, ArtifactSteps: []protocol.ArtifactExpectation{}}
		accept(t, s, a, event(grant.Ref, seq+1, terminal))
		view, err := s.GetBuild(testContext, grant.Ref.BuildID)
		if err != nil || view.Reason != "exit" || view.PostPhase != "failure" || view.Steps[0].ExitCode != 3 || view.Steps[0].ElapsedNS != 1234567 {
			t.Fatal("failure evidence", view, err)
		}
	})
}

func TestPrecheckTerminalDerivesOnlyUntouchedSteps(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, grant, _ := claimed(t, s)
		p := protocol.ExecutionProgress{Kind: "build_finished", Status: "failed", Reason: "precheck_error", PostPhase: "none", StopConfirmed: true, RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}
		accept(t, s, a, event(grant.Ref, 1, p))
		view, err := s.GetBuild(testContext, grant.Ref.BuildID)
		if err != nil || view.Status != "failed" || view.Reason != "precheck_error" || view.PostPhase != "none" || view.Steps[0].Status != "skipped" || view.Steps[0].Intent || !view.Steps[0].StopConfirmed || view.Post[0].Reason != "precheck_error" {
			t.Fatal("precheck evidence", view, err)
		}
	})
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, grant, _ := claimed(t, s)
		accept(t, s, a, event(grant.Ref, 1, stepProgress("intent", "", "ordinary", "compile", 1)))
		p := protocol.ExecutionProgress{Kind: "build_finished", Status: "failed", Reason: "precheck_error", PostPhase: "none", StopConfirmed: true, RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}
		if _, err := s.ApplyEvent(testContext, a, event(grant.Ref, 2, p)); err != ErrEventConflict {
			t.Fatal("intent erased", err)
		}
	})
}

func TestCheckoutTerminalRequiresNoPriorUserAction(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, grant, _ := claimed(t, s)
		terminal := protocol.ExecutionProgress{Kind: "build_finished", Status: "failed", Reason: "checkout_error", PostPhase: "none", StopConfirmed: true, RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}
		accept(t, s, a, event(grant.Ref, 1, terminal))
		view, err := s.GetBuild(testContext, grant.Ref.BuildID)
		if err != nil || view.Reason != "checkout_error" || view.Steps[0].Reason != "checkout_error" || view.Steps[0].Started {
			t.Fatal("checkout evidence", err)
		}
	})
}

func TestEventCannotChangeUnlimitedBudgetSemantics(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		actor, grant, _ := claimed(t, s)
		p := stepProgress("intent", "", "ordinary", "compile", 1)
		zero := int64(0)
		p.RemainingBudgetNS = &zero
		if _, err := s.ApplyEvent(testContext, actor, event(grant.Ref, 1, p)); err != ErrBudgetInvalid {
			t.Fatal("unlimited changed to zero", err)
		}
		p.RemainingBudgetNS = nil
		p.ElapsedNS = -1
		if _, err := s.ApplyEvent(testContext, actor, event(grant.Ref, 1, p)); err != ErrInvalid {
			t.Fatal("negative ns", err)
		}
	})
}
