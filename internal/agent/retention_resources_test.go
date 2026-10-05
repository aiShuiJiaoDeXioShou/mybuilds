//go:build darwin || linux

package agent

import (
	"bytes"
	"context"
	"github.com/google/uuid"
	"golang.org/x/sys/unix"
	"mybuilds/internal/protocol"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func resourceFixture(t *testing.T) (*executionJournal, string, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	lock, err := lockDataDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { lock.Close() })
	sid := uuid.NewString()
	j, err := newJournal(lock, uuid.NewString(), sid)
	if err != nil {
		t.Fatal(err)
	}
	ref := protocol.LeaseRef{NodeID: uuid.NewString(), SessionID: sid, BuildID: uuid.NewString(), AttemptID: uuid.NewString(), LeaseID: uuid.NewString(), Epoch: 1}
	if err = j.setLease(ref, nil, 1000000000); err != nil {
		t.Fatal(err)
	}
	j.state.StopConfirmed = true
	if err = j.save(); err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(dir, "scm", "checkout-real", "workspace")
	results := filepath.Join(dir, "results", "mybuilds-real")
	for _, p := range []string{filepath.Dir(workspace), workspace, results} {
		if err = os.MkdirAll(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	return j, workspace, results
}
func TestRetentionResourceStagesFixedSlotsAndExactACK(t *testing.T) {
	j, w, r := resourceFixture(t)
	a, err := j.stageWorkspace(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	if !a.HasWorkspace || a.HasResults || !exactUUID(a.ID) {
		t.Fatalf("invalid initial registration: %+v", a)
	}
	again, err := j.stageWorkspace(context.Background(), w)
	if err != nil || again != a {
		t.Fatal("same attempt changed registration", err)
	}
	if err = j.ackResource(a); err != nil {
		t.Fatal(err)
	}
	b, err := j.stageResult(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if b.ID != a.ID || !b.HasResults || b.OwnershipDigest == a.OwnershipDigest {
		t.Fatal("results did not extend fixed registration")
	}
	if err = j.ackResource(a); err == nil {
		t.Fatal("stale acknowledgement accepted")
	}
	if err = j.ackResource(b); err != nil {
		t.Fatal(err)
	}
	record, _, err := j.readResource()
	if err != nil || record.RegistrationPending || record.Workspace.RelativePath != "scm/checkout-real" {
		t.Fatal("registration was not persisted", err)
	}
	if info, err := j.lock.root.Lstat("resources/" + b.ID + ".json"); err != nil || !privateInfo(info, false) {
		t.Fatal("resource file is not private", err)
	}
	data, err := os.ReadFile(filepath.Join(j.lock.root.Name(), "resources", b.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("empty resource record")
	}
	if err = j.remove(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(j.lock.root.Name(), "resources", b.ID+".json")); err != nil {
		t.Fatal("journal removal lost durable resource", err)
	}
}
func TestRetentionResourceRejectsReplacementAndForeignPath(t *testing.T) {
	j, w, r := resourceFixture(t)
	a, err := j.stageWorkspace(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	if err = j.ackResource(a); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	if _, err = j.stageResult(context.Background(), other); err == nil {
		t.Fatal("foreign result accepted")
	}
	if err = os.Rename(filepath.Dir(w), filepath.Dir(w)+"-old"); err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(w, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = j.stageResult(context.Background(), r); err == nil {
		t.Fatal("replaced workspace accepted")
	}
}
func TestRetentionResourceRequiresRealStoppedCheckout(t *testing.T) {
	j, w, _ := resourceFixture(t)
	j.state.StopConfirmed = false
	if _, err := j.stageWorkspace(context.Background(), w); err == nil {
		t.Fatal("unknown checkout accepted")
	}
	j.state.StopConfirmed = true
	j.state.CleanupFailed = true
	if _, err := j.stageWorkspace(context.Background(), w); err == nil {
		t.Fatal("failed cleanup accepted")
	}
}

func TestRetentionResourcePendingAndPrivateFileBoundaries(t *testing.T) {
	for _, mode := range []string{"symlink", "hardlink", "fifo", "duplicate", "wrong_ref", "directory_replaced"} {
		t.Run(mode, func(t *testing.T) {
			j, w, _ := resourceFixture(t)
			in, err := j.stageWorkspace(context.Background(), w)
			if err != nil {
				t.Fatal(err)
			}
			r, _, err := j.readResource()
			if err != nil || !r.RegistrationPending {
				t.Fatal("lost acknowledgement did not retain pending", err)
			}
			file := filepath.Join(j.lock.root.Name(), "resources", in.ID+".json")
			switch mode {
			case "symlink":
				if err = os.Rename(file, file+"-owned"); err != nil {
					t.Fatal(err)
				}
				err = os.Symlink(file+"-owned", file)
			case "hardlink":
				err = os.Link(file, file+"-owned")
			case "fifo":
				if err = os.Remove(file); err != nil {
					t.Fatal(err)
				}
				err = unix.Mkfifo(file, 0600)
			case "duplicate":
				data, e := os.ReadFile(file)
				if e != nil {
					t.Fatal(e)
				}
				data = append(data[:len(data)-1], []byte(",\"id\":\""+in.ID+"\"}")...)
				err = os.WriteFile(file, data, 0600)
			case "wrong_ref":
				data, e := os.ReadFile(file)
				if e != nil {
					t.Fatal(e)
				}
				data = bytes.Replace(data, []byte(in.Ref.BuildID), []byte(uuid.NewString()), 1)
				err = os.WriteFile(file, data, 0600)
			case "directory_replaced":
				parent := filepath.Dir(file)
				if err = os.Rename(parent, parent+"-owned"); err != nil {
					t.Fatal(err)
				}
				if err = os.Mkdir(parent, 0700); err != nil {
					t.Fatal(err)
				}
				data, e := os.ReadFile(filepath.Join(parent+"-owned", in.ID+".json"))
				if e != nil {
					t.Fatal(e)
				}
				err = os.WriteFile(file, data, 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = j.ackResource(in); err == nil {
				t.Fatal("invalid private resource accepted")
			}
			if _, err = j.lock.root.Lstat("journal/" + j.name); err != nil {
				t.Fatal("failure removed unknown journal", err)
			}
		})
	}
}
func TestRetentionResourcePendingCannotExtendAndExpiredContextCannotSave(t *testing.T) {
	j, w, r := resourceFixture(t)
	in, err := j.stageWorkspace(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = j.stageResult(context.Background(), r); err == nil {
		t.Fatal("pending registration allowed result expansion")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = j.stageWorkspace(ctx, w); err == nil {
		t.Fatal("cancelled authority accepted")
	}
	persisted, _, err := j.readResource()
	if err != nil || resourceRegistration(persisted) != in || !persisted.RegistrationPending {
		t.Fatal("failure lost exact pending", err)
	}
}

func TestRetentionResourceRejectedSaveRetainsPendingAndJournal(t *testing.T) {
	j, w, _ := resourceFixture(t)
	in, err := j.stageWorkspace(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(j.lock.root.Name(), "resources")
	if err = os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	if err = j.ackResource(in); err == nil {
		t.Fatal("invalid private parent falsely confirmed registration")
	}
	if err = os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	r, _, err := j.readResource()
	if err != nil || !r.RegistrationPending || j.state.CleanupFailed {
		t.Fatal("save failure erased pending or fabricated cleanup failure", err)
	}
	if _, err = os.Stat(filepath.Join(j.lock.root.Name(), "journal", j.name)); err != nil {
		t.Fatal("save failure removed journal", err)
	}
}
func TestRetentionResourceRejectsMovedSlotUnderReplacedParent(t *testing.T) {
	j, w, r := resourceFixture(t)
	in, err := j.stageWorkspace(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	if err = j.ackResource(in); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Dir(filepath.Dir(w))
	if err = os.Rename(parent, parent+"-old"); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(parent, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(filepath.Join(parent+"-old", "checkout-real"), filepath.Join(parent, "checkout-real")); err != nil {
		t.Fatal(err)
	}
	if _, err = j.stageResult(context.Background(), r); err == nil {
		t.Fatal("same leaf under different parent accepted")
	}
}
func TestRetentionResourceTerminalProofBeforeJournalRemoval(t *testing.T) {
	j, w, r := resourceFixture(t)
	in, err := j.stageWorkspace(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	if err = j.ackResource(in); err != nil {
		t.Fatal(err)
	}
	in, err = j.stageResult(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if err = j.ackResource(in); err != nil {
		t.Fatal(err)
	}
	p := protocol.ExecutionProgress{Kind: "build_finished", Status: "skipped", Reason: "condition", StopConfirmed: true, ArtifactSteps: []protocol.ArtifactExpectation{}}
	event, err := j.prepareEvent(p)
	if err != nil {
		t.Fatal(err)
	}
	ack := protocol.EventAck{Seq: event.Seq, Digest: event.Digest}
	bad := ack
	bad.Digest = strings.Repeat("0", 64)
	if err = recordResourceTerminal(j, event, bad); err == nil {
		t.Fatal("unmatched terminal acknowledgement accepted")
	}
	if err = j.ackEvent(ack); err != nil {
		t.Fatal(err)
	}
	changed := event
	changed.Progress.Status = "succeeded"
	if err = recordResourceTerminal(j, changed, ack); err == nil {
		t.Fatal("terminal body changed without digest accepted")
	}
	if err = recordResourceTerminal(j, event, ack); err != nil {
		t.Fatal(err)
	}
	resource, _, err := j.readResource()
	if err != nil || resource.TerminalSeq != event.Seq || resource.TerminalDigest != event.Digest || resource.PendingLog {
		t.Fatal("exact terminal proof was not durable", err)
	}
	if _, err = j.stageResult(context.Background(), r); err == nil {
		t.Fatal("terminal registration can expand/redeclare slots")
	}
	if err = j.remove(); err != nil {
		t.Fatal(err)
	}
	if _, _, err = j.readResource(); err != nil {
		t.Fatal("terminal resource lost when journal removed", err)
	}
}

func TestRetentionResourceTerminalSaveFailureRetainsExactPending(t *testing.T) {
	j, w, _ := resourceFixture(t)
	in, err := j.stageWorkspace(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	if err = j.ackResource(in); err != nil {
		t.Fatal(err)
	}
	event, err := j.prepareEvent(protocol.ExecutionProgress{Kind: "build_finished", Status: "skipped", Reason: "condition", StopConfirmed: true, ArtifactSteps: []protocol.ArtifactExpectation{}})
	if err != nil {
		t.Fatal(err)
	}
	ack := protocol.EventAck{Seq: event.Seq, Digest: event.Digest}
	dir := filepath.Join(j.lock.root.Name(), "resources")
	if err = os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	if err = recordResourceTerminal(j, event, ack); err == nil {
		t.Fatal("terminal proof save failure ignored")
	}
	if j.state.PendingEvent == nil || j.state.PendingEvent.Seq != event.Seq || j.state.PendingEvent.Digest != event.Digest || j.state.LastEventSeq != 0 {
		t.Fatal("failed resource persist erased original pending terminal")
	}
	if err = os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(j.lock.root.Name(), "journal", j.name)); err != nil {
		t.Fatal("failed terminal persist removed journal", err)
	}
	if err = recordResourceTerminal(j, event, ack); err != nil {
		t.Fatal(err)
	}
	if err = j.ackEvent(ack); err != nil {
		t.Fatal(err)
	}
	record, _, err := j.readResource()
	if err != nil || record.TerminalSeq != event.Seq || record.TerminalDigest != event.Digest {
		t.Fatal("exact terminal persist recovery failed", err)
	}
}
