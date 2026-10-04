package store

import (
	"fmt"
	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"strings"
	"testing"
)

func TestArtifactMetadataCountAndAggregateLimits(t *testing.T) {
	for _, limit := range []string{"count", "size"} {
		t.Run(limit, func(t *testing.T) {
			stores(t, func(t *testing.T, s *Store, opt Options) {
				actor, session, p, policy := leaseFixture(t, s)
				in := enqueueInput(p, "limits")
				in.Builds[0].Snapshot.Definition.Steps = []config.Step{{Kind: "artifact", Name: "first", Paths: []string{"output/*"}}, {Kind: "artifact", Name: "second", Paths: []string{"output/*"}}}
				in.Builds[0].Steps = []StepProgress{{Phase: "ordinary", Index: 1, Name: "first", Kind: "artifact", Status: "pending", Condition: "ready"}, {Phase: "ordinary", Index: 2, Name: "second", Kind: "artifact", Status: "pending", Condition: "ready"}, in.Builds[0].Steps[1]}
				if _, err := s.Enqueue(testContext, in); err != nil {
					t.Fatal(err)
				}
				grant, err := s.Claim(testContext, actor, protocol.ClaimRequest{SessionID: session.SessionID, ClaimKey: uuid.NewString()}, policy)
				if err != nil || grant == nil {
					t.Fatal(err)
				}
				count := 128
				fileSize := int64(0)
				if limit == "size" {
					count = 4
					fileSize = 1 << 30
				}
				ids := make([]string, count)
				for i := range ids {
					ids[i] = uuid.NewString()
				}
				extra := uuid.NewString()
				seq := int64(0)
				for index, name := range []string{"first", "second"} {
					progress := stepProgress("intent", "", "ordinary", name, index+1)
					progress.StepKind = "artifact"
					seq++
					accept(t, s, actor, event(grant.Ref, seq, progress))
					progress.Kind = "started"
					progress.Started = true
					seq++
					accept(t, s, actor, event(grant.Ref, seq, progress))
					progress.Kind = "finished"
					progress.Status = "succeeded"
					progress.ExitCode = 0
					progress.StopConfirmed = true
					progress.ArtifactIDs = ids
					if index == 1 {
						progress.ArtifactIDs = []string{extra}
					}
					seq++
					accept(t, s, actor, event(grant.Ref, seq, progress))
				}
				for i, id := range ids {
					d := protocol.ArtifactDeclaration{Ref: grant.Ref, ID: id, Seq: int64(i + 1), Phase: "ordinary", Index: 1, Step: "first", Name: fmt.Sprintf("file-%d.apk", i), Size: fileSize, SHA256: strings.Repeat("a", 64)}
					if _, err = s.CommitArtifact(testContext, actor, ArtifactCommit{Declaration: d, StorageID: uuid.NewString()}); err != nil {
						t.Fatal("within limit", err)
					}
				}
				d := protocol.ArtifactDeclaration{Ref: grant.Ref, ID: extra, Seq: int64(count + 1), Phase: "ordinary", Index: 2, Step: "second", Name: "extra.apk", Size: fileSize, SHA256: strings.Repeat("a", 64)}
				if _, err = s.CommitArtifact(testContext, actor, ArtifactCommit{Declaration: d, StorageID: uuid.NewString()}); err != ErrArtifactConflict {
					t.Fatal("aggregate limit exceeded", err)
				}
				files, err := s.ListArtifacts(testContext, localAdmin, grant.Ref.BuildID, Page{Limit: 200})
				if err != nil || len(files) != count {
					t.Fatal("limit rollback", len(files), err)
				}
			})
		})
	}
}
