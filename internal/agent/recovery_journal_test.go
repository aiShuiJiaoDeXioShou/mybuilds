//go:build darwin || linux

package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
	"mybuilds/internal/protocol"
)

// 这里只验证真实私有文件读取，不假称中央已确认此终态。
func recoveryJournalFile(t *testing.T) (*dataLock, *executionJournal) {
	t.Helper()
	lock, err := lockDataDir(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { lock.Close() })
	if err = lock.prepareJournal(); err != nil {
		t.Fatal(err)
	}
	journal, err := newJournal(lock, uuid.NewString(), uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	ref := protocol.LeaseRef{NodeID: uuid.NewString(), SessionID: journal.state.SessionID, BuildID: uuid.NewString(), AttemptID: uuid.NewString(), LeaseID: uuid.NewString(), Epoch: 1}
	if err = journal.setLease(ref, nil, 1); err != nil {
		t.Fatal(err)
	}
	_, err = journal.prepareEvent(protocol.ExecutionProgress{Kind: "build_finished", Status: "succeeded", StopConfirmed: true, RemainingPostBudgetNS: 1, ArtifactSteps: []protocol.ArtifactExpectation{}})
	if err != nil {
		t.Fatal(err)
	}
	return lock, journal
}
func TestRecoveryReadTerminalJournalActualFile(t *testing.T) {
	lock, journal := recoveryJournalFile(t)
	got, err := readTerminalJournal(lock, journal.name)
	if err != nil || got.state.PendingEvent == nil || got.state.PendingEvent.Digest != journal.state.PendingEvent.Digest {
		t.Fatal("local proof", err)
	}
	if inspectData(lock.root.Name()) != "journal_unconfirmed" {
		t.Fatal("read deleted unconfirmed file")
	}
	if err = lock.Check(); err != nil {
		t.Fatal(err)
	}
}
func TestRecoveryReadJournalRejectsUnknownAndConflictingLocalProof(t *testing.T) {
	for _, mode := range []string{"claim", "nonterminal", "status", "journal_stop", "event_stop", "cleanup", "pendinglog", "pendingstop", "ref", "seq", "digest", "name", "null", "duplicate", "trailing", "unknownfield"} {
		t.Run(mode, func(t *testing.T) {
			lock, journal := recoveryJournalFile(t)
			filename := filepath.Join(lock.root.Name(), "journal", journal.name)
			state := journal.state
			switch mode {
			case "claim":
				state.Ref = nil
				state.PendingEvent = nil
			case "nonterminal":
				state.PendingEvent.Progress.Kind = "started"
			case "status":
				state.PendingEvent.Progress.Status = "running"
			case "journal_stop":
				state.StopConfirmed = false
			case "event_stop":
				state.PendingEvent.Progress.StopConfirmed = false
			case "cleanup":
				state.CleanupFailed = true
			case "pendinglog":
				state.PendingLog = &protocol.LogChunk{}
			case "pendingstop":
				state.PendingStop = &protocol.StopConfirmation{}
			case "ref":
				state.PendingEvent.Ref.BuildID = uuid.NewString()
			case "seq":
				state.PendingEvent.Seq = 0
			case "digest":
				state.PendingEvent.Digest = "invalid"
			case "name":
				state.ClaimKey = uuid.NewString()
			}
			data, _ := json.Marshal(state)
			switch mode {
			case "null":
				data = []byte(`{"ref":null}`)
			case "duplicate":
				data = []byte(`{"claim_key":"a","claim_key":"b"}`)
			case "trailing":
				data = append(data, []byte(` {}`)...)
			case "unknownfield":
				data = append(data[:len(data)-1], []byte(`,"unexpected":"private-marker"}`)...)
			}
			if err := os.WriteFile(filename, data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := readTerminalJournal(lock, journal.name); err == nil {
				t.Fatal("unsafe local evidence accepted")
			}
			after, err := os.ReadFile(filename)
			if err != nil || string(after) != string(data) {
				t.Fatal("read rewrote evidence", err)
			}
		})
	}
}
func TestRecoveryReadJournalSpecialFilesAndPermissionBoundary(t *testing.T) {
	for _, mode := range []string{"symlink", "fifo", "hardlink", "permissions", "oversized", "filename"} {
		t.Run(mode, func(t *testing.T) {
			lock, journal := recoveryJournalFile(t)
			filename := filepath.Join(lock.root.Name(), "journal", journal.name)
			switch mode {
			case "symlink":
				os.Remove(filename)
				if err := os.Symlink(filepath.Join(t.TempDir(), "outside"), filename); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				os.Remove(filename)
				if err := unix.Mkfifo(filename, 0600); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(filename, filepath.Join(t.TempDir(), "link")); err != nil {
					t.Fatal(err)
				}
			case "permissions":
				if err := os.Chmod(filename, 0644); err != nil {
					t.Fatal(err)
				}
			case "oversized":
				if err := os.WriteFile(filename, make([]byte, (1<<20)+1), 0600); err != nil {
					t.Fatal(err)
				}
			case "filename":
				journal.name = "../outside.json"
			}
			done := make(chan error, 1)
			go func() { _, err := readTerminalJournal(lock, journal.name); done <- err }()
			ctx, stop := context.WithTimeout(context.Background(), time.Second)
			defer stop()
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("unsafe file accepted")
				}
			case <-ctx.Done():
				t.Fatal("special file blocked")
			}
		})
	}
}

func TestRecoveryJournalEnumerationBound(t *testing.T) {
	lock, journal := recoveryJournalFile(t)
	names, err := lock.journalNames()
	if err != nil || len(names) != 1 || names[0] != journal.name {
		t.Fatal(names, err)
	}
	for i := 0; i < 128; i++ {
		name := uuid.NewString() + ".json"
		if err := os.WriteFile(filepath.Join(lock.root.Name(), "journal", name), []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = lock.journalNames(); err == nil {
		t.Fatal("over 128 journals accepted")
	}
}
