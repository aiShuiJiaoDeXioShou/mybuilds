//go:build darwin || linux

package agent

import (
	"github.com/google/uuid"
	"mybuilds/internal/protocol"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSpoolActualFileAndACKBeforeRelease(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "data")
	lock, err := lockDataDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	lock.prepareJournal()
	journal, err := newJournal(lock, uuid.NewString(), uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	ref := protocol.LeaseRef{NodeID: uuid.NewString(), SessionID: journal.state.SessionID, BuildID: uuid.NewString(), AttemptID: uuid.NewString(), LeaseID: uuid.NewString(), Epoch: 1}
	if err = journal.setLease(ref, nil, 1234); err != nil {
		t.Fatal(err)
	}
	usage := &spoolUsage{}
	spool := newSpool(journal, usage)
	chunk, err := spool.append(protocol.LogRecord{UTC: time.Now().UTC(), Build: "real", Phase: "ordinary", Index: 1, Step: "step", Stream: "stdout", Text: "redacted actual log"})
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(directory, spool.pendingPath())
	info, err := os.Lstat(filename)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		t.Fatal(err)
	}
	if err = spool.ack(protocol.LogAck{Seq: chunk.Seq, NextOffset: 1, Digest: chunk.Digest}); err == nil {
		t.Fatal("bad cursor ACK")
	}
	if _, err = os.Stat(filename); err != nil {
		t.Fatal("unconfirmed file dropped")
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if err = spool.ack(protocol.LogAck{Seq: chunk.Seq, NextOffset: chunk.Offset + int64(len(data)), Digest: chunk.Digest}); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filename); !os.IsNotExist(err) {
		t.Fatal("confirmed spool retained")
	}
	if journal.state.LastLogSeq != 1 || journal.state.LastLogOffset != int64(len(data)) {
		t.Fatal(journal.state)
	}
	if usage.records != 0 || usage.bytes != 0 {
		t.Fatal(usage)
	}
}
