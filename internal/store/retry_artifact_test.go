package store

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"mybuilds/internal/protocol"
)

func TestRetryLeavesOriginalCentralFilesAndLogCursors(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, g, ids := artifactFixture(t, s)
		for i, id := range ids {
			d := protocol.ArtifactDeclaration{Ref: g.Ref, ID: id, Seq: int64(i + 1), Phase: "ordinary", Index: 1, Step: "package", Name: "app.apk", Size: 7, SHA256: strings.Repeat("a", 64)}
			if _, err := s.CommitArtifact(testContext, a, ArtifactCommit{Declaration: d, StorageID: uuid.NewString()}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := s.CommitLogChunk(testContext, a, LogCommit{Ref: g.Ref, Seq: 1, Size: 17, Digest: strings.Repeat("b", 64), StorageID: uuid.NewString(), RecordCount: 1}); err != nil {
			t.Fatal(err)
		}
		terminal := protocol.ExecutionProgress{Kind: "build_finished", Status: "succeeded", Started: true, StopConfirmed: true, RemainingPostBudgetNS: int64(2 * time.Minute), LastLogSeq: 1, LastLogOffset: 17, LastArtifactSeq: 2, ArtifactSteps: []protocol.ArtifactExpectation{{Phase: "ordinary", Index: 1, Count: 2, IDs: ids}}}
		accept(t, s, a, event(g.Ref, 6, terminal))
		before := recoveryRows(t, s)
		var oldFiles []artifactRecord
		var oldLogs []logChunkRecord
		var oldSteps []stepRecord
		s.db.Order("id").Find(&oldFiles)
		s.db.Order("id").Find(&oldLogs)
		s.db.Where("build_id = ?", g.Ref.BuildID).Order("id").Find(&oldSteps)
		out, err := s.Retry(testContext, localAdmin, RetryInput{BuildID: g.Ref.BuildID, Key: "artifact-retry"})
		if err != nil {
			t.Fatal(err)
		}
		var original buildRecord
		s.db.First(&original, "id = ?", g.Ref.BuildID)
		var files []artifactRecord
		var logs []logChunkRecord
		var steps []stepRecord
		s.db.Order("id").Find(&files)
		s.db.Order("id").Find(&logs)
		s.db.Where("build_id = ?", g.Ref.BuildID).Order("id").Find(&steps)
		if !reflect.DeepEqual(before[0], original) || !reflect.DeepEqual(oldFiles, files) || !reflect.DeepEqual(oldLogs, logs) || !reflect.DeepEqual(oldSteps, steps) {
			t.Fatal("重试改写原完整证据")
		}
		var child buildRecord
		s.db.First(&child, "id = ?", out.Builds[0].ID)
		if child.LastArtifactSeq != 0 || child.LastLogSeq != 0 || child.LastLogOffset != 0 || child.LastEventSeq != 0 || child.RemainingPostBudgetNS != 0 || child.RemainingBudgetNS != nil {
			t.Fatal("新执行继承原游标/消耗预算")
		}
	})
}
