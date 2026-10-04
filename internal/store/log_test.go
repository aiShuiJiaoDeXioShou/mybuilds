package store

import (
	"github.com/google/uuid"
	"mybuilds/internal/protocol"
	"strings"
	"testing"
	"time"
)

func TestLogCommitSequenceCanonicalAndRole(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		actor, grant, _ := claimed(t, s)
		first := LogCommit{Ref: grant.Ref, Seq: 1, Offset: 0, Size: 100, Digest: strings.Repeat("a", 64), StorageID: uuid.NewString(), RecordCount: 1}
		committed, err := s.CommitLogChunk(testContext, actor, first)
		if err != nil || !committed.Created || committed.Ack.NextOffset != 100 {
			t.Fatal("first log", err)
		}
		second := first
		second.Seq = 2
		second.Offset = 100
		second.StorageID = uuid.NewString()
		second.Digest = strings.Repeat("b", 64)
		if _, err = s.CommitLogChunk(testContext, actor, second); err != nil {
			t.Fatal(err)
		}
		retry := first
		retry.StorageID = uuid.NewString()
		replay, err := s.CommitLogChunk(testContext, actor, retry)
		if err != nil || replay.Created || replay.StorageID != first.StorageID || replay.Ack != committed.Ack {
			t.Fatal("canonical replay", err)
		}
		retry.Digest = strings.Repeat("c", 64)
		if _, err = s.CommitLogChunk(testContext, actor, retry); err != ErrLogConflict {
			t.Fatal("conflict", err)
		}
		next := second
		next.Seq = 4
		next.Offset = 200
		if _, err = s.CommitLogChunk(testContext, actor, next); err != ErrSequenceInvalid {
			t.Fatal("gap", err)
		}
		next.Seq = 3
		next.Offset = 199
		if _, err = s.CommitLogChunk(testContext, actor, next); err != ErrSequenceInvalid {
			t.Fatal("offset", err)
		}
		next.Offset = 200
		next.Size = 65537
		if _, err = s.CommitLogChunk(testContext, actor, next); err != ErrInvalid {
			t.Fatal("oversized", err)
		}
		trigger, e := s.CreateToken(testContext, localAdmin, "trigger")
		if e != nil {
			t.Fatal(e)
		}
		user, e := s.Authenticate(testContext, trigger.Token)
		if e != nil {
			t.Fatal(e)
		}
		if _, err = s.ListLogChunks(testContext, user, grant.Ref.BuildID, 0, Page{}); err != ErrForbidden {
			t.Fatal("trigger read", err)
		}
		approver, e := s.CreateToken(testContext, localAdmin, "approver")
		if e != nil {
			t.Fatal(e)
		}
		reader, e := s.Authenticate(testContext, approver.Token)
		if e != nil {
			t.Fatal(e)
		}
		logs, err := s.ListLogChunks(testContext, reader, grant.Ref.BuildID, 1, Page{Limit: 1})
		if err != nil || len(logs) != 1 || logs[0].Seq != 2 || logs[0].Offset != 100 {
			t.Fatal("log page", logs, err)
		}
		seq := completeRunEvents(t, s, actor, grant, false)
		terminal := protocol.ExecutionProgress{Kind: "build_finished", Status: "succeeded", Started: true, StopConfirmed: true, RemainingPostBudgetNS: int64(2*time.Minute) - 100, LastLogSeq: 2, LastLogOffset: 200, ArtifactSteps: []protocol.ArtifactExpectation{}}
		accept(t, s, actor, event(grant.Ref, seq+1, terminal))
		if _, err = s.CommitLogChunk(testContext, actor, first); err != ErrLeaseInvalid {
			t.Fatal("terminal log replay", err)
		}
		logs, err = s.ListLogChunks(testContext, localAdmin, grant.Ref.BuildID, 0, Page{})
		if err != nil || len(logs) != 2 {
			t.Fatal("central history", err)
		}
	})
}
