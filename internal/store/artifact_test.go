package store

import (
	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"strings"
	"testing"
	"time"
)

func artifactFixture(t *testing.T, s *Store) (NodeActor, protocol.LeaseGrant, []string) {
	t.Helper()
	a, session, p, policy := leaseFixture(t, s)
	in := enqueueInput(p, "artifact")
	in.Builds[0].Snapshot.Definition.Steps = []config.Step{{Kind: "artifact", Name: "package", Paths: []string{"output/*.apk"}}}
	in.Builds[0].Steps[0].Kind = "artifact"
	in.Builds[0].Steps[0].Name = "package"
	out, err := s.Enqueue(testContext, in)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := s.Claim(testContext, a, protocol.ClaimRequest{SessionID: session.SessionID, ClaimKey: uuid.NewString()}, policy)
	if err != nil || grant == nil || grant.Ref.BuildID != out.Builds[0].ID {
		t.Fatal(err)
	}
	ids := []string{uuid.NewString(), uuid.NewString()}
	progress := stepProgress("intent", "", "ordinary", "package", 1)
	progress.StepKind = "artifact"
	accept(t, s, a, event(grant.Ref, 1, progress))
	progress.Kind = "started"
	progress.Started = true
	accept(t, s, a, event(grant.Ref, 2, progress))
	progress.Kind = "finished"
	progress.Status = "succeeded"
	progress.StopConfirmed = true
	progress.ExitCode = 0
	progress.ArtifactIDs = ids
	accept(t, s, a, event(grant.Ref, 3, progress))
	selectPost := protocol.ExecutionProgress{Kind: "post_selected", PostPhase: "success", RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}
	accept(t, s, a, event(grant.Ref, 4, selectPost))
	skip := stepProgress("skipped", "skipped", "always", "cleanup", 1)
	skip.Reason = "condition"
	skip.StopConfirmed = true
	accept(t, s, a, event(grant.Ref, 5, skip))
	return a, *grant, ids
}
func TestArtifactCanonicalLimitsAndCompleteTerminal(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		actor, grant, ids := artifactFixture(t, s)
		declaration := protocol.ArtifactDeclaration{Ref: grant.Ref, ID: ids[0], Seq: 1, Phase: "ordinary", Step: "package", Index: 1, Name: "app.apk", Size: 7, SHA256: strings.Repeat("a", 64)}
		first := ArtifactCommit{Declaration: declaration, StorageID: uuid.NewString()}
		out, err := s.CommitArtifact(testContext, actor, first)
		if err != nil || !out.Created || out.View.ID != ids[0] {
			t.Fatal("first artifact", err)
		}
		replay := first
		replay.StorageID = uuid.NewString()
		retry, err := s.CommitArtifact(testContext, actor, replay)
		if err != nil || retry.Created || retry.StorageID != first.StorageID {
			t.Fatal("canonical artifact", err)
		}
		replay.Declaration.Name = "other.apk"
		if _, err = s.CommitArtifact(testContext, actor, replay); err != ErrArtifactConflict {
			t.Fatal("conflict overwritten", err)
		}
		for _, bad := range []protocol.ArtifactDeclaration{{Ref: grant.Ref, ID: uuid.NewString(), Seq: 2, Phase: "ordinary", Step: "package", Index: 1, Name: "app.apk", Size: 7, SHA256: strings.Repeat("a", 64)}, {Ref: grant.Ref, ID: ids[1], Seq: 2, Phase: "ordinary", Step: "other", Index: 1, Name: "app.apk", Size: 7, SHA256: strings.Repeat("a", 64)}} {
			if _, err = s.CommitArtifact(testContext, actor, ArtifactCommit{Declaration: bad, StorageID: uuid.NewString()}); err != ErrArtifactConflict {
				t.Fatal("unapproved artifact", err)
			}
		}
		terminal := protocol.ExecutionProgress{Kind: "build_finished", Status: "succeeded", Started: true, StopConfirmed: true, RemainingPostBudgetNS: int64(2 * time.Minute), LastArtifactSeq: 1, ArtifactSteps: []protocol.ArtifactExpectation{{Phase: "ordinary", Index: 1, Count: 1, IDs: ids[:1]}}}
		if _, err = s.ApplyEvent(testContext, actor, event(grant.Ref, 6, terminal)); err != ErrEventConflict {
			t.Fatal("ACK subset terminal", err)
		}
		declaration.ID = ids[1]
		declaration.Seq = 2
		second := ArtifactCommit{Declaration: declaration, StorageID: uuid.NewString()}
		bad := second
		bad.Declaration.Name = "../app.apk"
		if _, err = s.CommitArtifact(testContext, actor, bad); err != ErrInvalid {
			t.Fatal("path accepted", err)
		}
		bad = second
		bad.Declaration.Size = (1 << 30) + 1
		if _, err = s.CommitArtifact(testContext, actor, bad); err != ErrInvalid {
			t.Fatal("file size", err)
		}
		if _, err = s.CommitArtifact(testContext, actor, second); err != nil {
			t.Fatal(err)
		}
		view, err := s.FindNodeArtifact(testContext, actor, grant.Ref, ids[1])
		if err != nil || view.Name != "app.apk" {
			t.Fatal("node canonical query", err)
		}
		terminal.LastArtifactSeq = 2
		terminal.ArtifactSteps[0].Count = 2
		terminal.ArtifactSteps[0].IDs = ids
		for _, manifest := range [][]protocol.ArtifactExpectation{nil, {}, {{Phase: "ordinary", Index: 1, Count: 2, IDs: []string{ids[0], ids[0]}}}, {{Phase: "ordinary", Index: 1, Count: 1, IDs: ids}}} {
			bad := terminal
			bad.ArtifactSteps = manifest
			if _, err = s.ApplyEvent(testContext, actor, event(grant.Ref, 6, bad)); err != ErrEventConflict {
				t.Fatal("bad manifest", err)
			}
		}
		accept(t, s, actor, event(grant.Ref, 6, terminal))
		if _, err = s.FindNodeArtifact(testContext, actor, grant.Ref, ids[0]); err != ErrLeaseInvalid {
			t.Fatal("terminal node access", err)
		}
		files, err := s.ListArtifacts(testContext, localAdmin, grant.Ref.BuildID, Page{})
		if err != nil || len(files) != 2 {
			t.Fatal("central files", err)
		}
		stored, err := s.GetArtifact(testContext, localAdmin, ids[0])
		if err != nil || stored.StorageID != first.StorageID || stored.View.Size != 7 {
			t.Fatal("stored file", err)
		}
		token, err := s.CreateToken(testContext, localAdmin, "trigger")
		if err != nil {
			t.Fatal(err)
		}
		user, err := s.Authenticate(testContext, token.Token)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.GetArtifact(testContext, user, ids[0]); err != ErrForbidden {
			t.Fatal("trigger file access", err)
		}
	})
}

func TestCompleteArtifactsSurviveFailedOrCancelledCollectionStep(t *testing.T) {
	for _, status := range []string{"failed", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			stores(t, func(t *testing.T, s *Store, opt Options) {
				a, session, p, policy := leaseFixture(t, s)
				in := enqueueInput(p, "diagnostic")
				in.Builds[0].Snapshot.Definition.Steps = []config.Step{{Kind: "artifact", Name: "package", Paths: []string{"output/*.apk"}}}
				in.Builds[0].Steps[0].Kind = "artifact"
				in.Builds[0].Steps[0].Name = "package"
				if _, err := s.Enqueue(testContext, in); err != nil {
					t.Fatal(err)
				}
				grant, err := s.Claim(testContext, a, protocol.ClaimRequest{SessionID: session.SessionID, ClaimKey: uuid.NewString()}, policy)
				if err != nil || grant == nil {
					t.Fatal(err)
				}
				id := uuid.NewString()
				progress := stepProgress("intent", "", "ordinary", "package", 1)
				progress.StepKind = "artifact"
				accept(t, s, a, event(grant.Ref, 1, progress))
				progress.Kind = "started"
				progress.Started = true
				accept(t, s, a, event(grant.Ref, 2, progress))
				progress.Kind = "finished"
				progress.Status = status
				progress.StopConfirmed = true
				progress.ExitCode = -1
				progress.Reason = "timeout"
				if status == "cancelled" {
					progress.Reason = "cancelled"
				}
				progress.ArtifactIDs = []string{id}
				accept(t, s, a, event(grant.Ref, 3, progress))
				postPhase := "failure"
				if status == "cancelled" {
					postPhase = "none"
				}
				accept(t, s, a, event(grant.Ref, 4, protocol.ExecutionProgress{Kind: "post_selected", PostPhase: postPhase, RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}))
				skip := stepProgress("skipped", "skipped", "always", "cleanup", 1)
				skip.Reason = "condition"
				skip.StopConfirmed = true
				accept(t, s, a, event(grant.Ref, 5, skip))
				declaration := protocol.ArtifactDeclaration{Ref: grant.Ref, ID: id, Seq: 1, Phase: "ordinary", Step: "package", Index: 1, Name: "diagnostic.apk", Size: 1, SHA256: strings.Repeat("a", 64)}
				if _, err = s.CommitArtifact(testContext, a, ArtifactCommit{Declaration: declaration, StorageID: uuid.NewString()}); err != nil {
					t.Fatal("complete diagnostic artifact refused", err)
				}
				terminal := protocol.ExecutionProgress{Kind: "build_finished", Status: status, Reason: progress.Reason, Started: true, StopConfirmed: true, RemainingPostBudgetNS: int64(2 * time.Minute), LastArtifactSeq: 1, ArtifactSteps: []protocol.ArtifactExpectation{{Phase: "ordinary", Index: 1, Count: 1, IDs: []string{id}}}}
				accept(t, s, a, event(grant.Ref, 6, terminal))
				view, err := s.GetBuild(testContext, grant.Ref.BuildID)
				if err != nil || view.Status != status || view.Reason != progress.Reason {
					t.Fatal("failure changed", view, err)
				}
				files, err := s.ListArtifacts(testContext, localAdmin, grant.Ref.BuildID, Page{})
				if err != nil || len(files) != 1 {
					t.Fatal("diagnostic evidence lost", err)
				}
			})
		})
	}
}
