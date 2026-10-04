package store

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
)

func TestReportsAndOrdinaryArtifactsShareAttemptQuotas(t *testing.T) {
	for _, limit := range []string{"count", "size"} {
		t.Run(limit, func(t *testing.T) {
			stores(t, func(t *testing.T, s *Store, opt Options) {
				a, session, project, policy := leaseFixture(t, s)
				in := enqueueInput(project, "shared-quota")
				in.Builds[0].Snapshot.Definition.Reports = &config.Reports{JUnit: &config.JUnitReport{Paths: []string{"results/*.xml"}}}
				in.Builds[0].Snapshot.Definition.Steps = append(in.Builds[0].Snapshot.Definition.Steps, config.Step{Kind: "artifact", Name: "package", Paths: []string{"output/*"}})
				in.Builds[0].Steps = append(in.Builds[0].Steps, StepProgress{Phase: "ordinary", Index: 2, Name: "package", Kind: "artifact", Condition: "ready", Status: "pending"})
				if _, err := s.Enqueue(testContext, in); err != nil {
					t.Fatal(err)
				}
				g, err := s.Claim(testContext, a, protocol.ClaimRequest{SessionID: session.SessionID, ClaimKey: uuid.NewString()}, policy)
				if err != nil || g == nil {
					t.Fatal(err)
				}
				reportFinished(t, s, a, *g)
				e, parsed := actualReport(t, `<testsuite><testcase/></testsuite>`)
				n := 64
				ordinaryCount := 65
				ordinarySize := int64(0)
				if limit == "size" {
					n = 1
					ordinaryCount = 4
					ordinarySize = 1 << 30
				}
				base := e.Files[0]
				e.Files = []protocol.ReportFile{}
				e.Counts.Tests = int64(n)
				for i := 0; i < n; i++ {
					f := base
					f.Path = fmt.Sprintf("results/%03d.xml", i)
					f.Key = reportKey(f.Path)
					f.ArtifactID = uuid.NewString()
					e.Files = append(e.Files, f)
				}
				accept(t, s, a, checkedEvent(*g, 4, &e, false))
				ids := make([]string, ordinaryCount)
				for i := range ids {
					ids[i] = uuid.NewString()
				}
				p := stepProgress("intent", "", "ordinary", "package", 2)
				p.StepKind = "artifact"
				accept(t, s, a, event(g.Ref, 5, p))
				p.Kind = "started"
				p.Started = true
				accept(t, s, a, event(g.Ref, 6, p))
				p.Kind = "finished"
				p.Status = "succeeded"
				p.ExitCode = 0
				p.StopConfirmed = true
				p.ArtifactIDs = ids
				accept(t, s, a, event(g.Ref, 7, p))
				for i, id := range ids {
					d := protocol.ArtifactDeclaration{Ref: g.Ref, ID: id, Seq: int64(i + 1), Phase: "ordinary", Index: 2, Step: "package", Name: fmt.Sprintf("file-%03d.apk", i), Size: ordinarySize, SHA256: reportKey("ordinary")}
					if _, err = s.CommitArtifact(testContext, a, ArtifactCommit{Declaration: d, StorageID: uuid.NewString()}); err != nil {
						t.Fatal("original artifact within shared quota", err)
					}
				}
				e.Revision = 2
				e.Outcome = "passed"
				accept(t, s, a, checkedEvent(*g, 8, &e, true))
				allowed := n - 1
				if limit == "size" {
					allowed = 0
				}
				for i, f := range e.Files {
					single := e
					single.Files = []protocol.ReportFile{f}
					upload := reportCommit(*g, single, &parsed)
					upload.Declaration.Seq = int64(ordinaryCount + i + 1)
					_, err = s.CommitArtifact(testContext, a, upload)
					if i < allowed {
						if err != nil {
							t.Fatal("combined quota prematurely rejected", err)
						}
					} else if err != ErrArtifactConflict {
						t.Fatal("purpose bypassed shared attempt quota", err)
					}
				}
				var row buildRecord
				s.db.First(&row, "id = ?", g.Ref.BuildID)
				var count int64
				s.db.Model(&artifactRecord{}).Where("attempt_id = ?", g.Ref.AttemptID).Count(&count)
				if count != int64(ordinaryCount+allowed) || row.LastArtifactSeq != count {
					t.Fatal("quota rejection advanced confirmation cursor")
				}
			})
		})
	}
}
