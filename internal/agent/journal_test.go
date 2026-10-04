//go:build darwin || linux

package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"mybuilds/internal/protocol"
)

func TestJournalActualDurableFenceAndStartedEvidence(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "data")
	lock, err := lockDataDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err = lock.prepareJournal(); err != nil {
		t.Fatal(err)
	}
	journal, err := newJournal(lock, uuid.NewString(), uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(directory, "journal", journal.name))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), journal.state.ClaimKey) {
		t.Fatal("claim key not durable")
	}
	ref := protocol.LeaseRef{NodeID: uuid.NewString(), SessionID: journal.state.SessionID, BuildID: uuid.NewString(), AttemptID: uuid.NewString(), LeaseID: uuid.NewString(), Epoch: 1}
	if err = journal.setLease(ref, nil, 1234); err != nil {
		t.Fatal(err)
	}
	p := protocol.ExecutionProgress{Kind: "started", Phase: "ordinary", Index: 1, Name: "real", StepKind: "run", Started: true, PID: os.Getpid(), PGID: os.Getpid(), RemainingPostBudgetNS: 1234}
	event, err := journal.prepareEvent(p)
	if err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(filepath.Join(directory, "journal", journal.name))
	if err != nil {
		t.Fatal(err)
	}
	var actual journalState
	if err = json.Unmarshal(data, &actual); err != nil {
		t.Fatal(err)
	}
	if actual.Ref == nil || *actual.Ref != ref || !actual.Started || actual.PID != os.Getpid() || actual.PendingEvent == nil || actual.PendingEvent.Seq != 1 {
		t.Fatal(actual)
	}
	public, _ := json.Marshal(event)
	if strings.Contains(string(public), "PID") || strings.Contains(string(public), "pid") {
		t.Fatal("private PID leaked")
	}
	if err = journal.ackEvent(protocol.EventAck{Seq: event.Seq, Digest: "wrong"}); err == nil {
		t.Fatal("incorrect ACK accepted")
	}
	if err = journal.ackEvent(protocol.EventAck{Seq: event.Seq, Digest: event.Digest}); err != nil {
		t.Fatal(err)
	}
	if inspectData(directory) != "journal_unconfirmed" {
		t.Fatal("journal silently recovered")
	}
}
func TestJournalRejectsReplacementAndNeverDropsIntent(t *testing.T) {
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
	filename := filepath.Join(directory, "journal", journal.name)
	if err = os.Remove(filename); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(filepath.Join(t.TempDir(), "outside"), filename); err != nil {
		t.Fatal(err)
	}
	if err = journal.save(); err == nil {
		t.Fatal("replacement was overwritten")
	}
	if _, err = os.Lstat(filename); err != nil {
		t.Fatal("unconfirmed evidence disappeared", err)
	}
}
