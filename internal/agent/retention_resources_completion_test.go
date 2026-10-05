//go:build darwin || linux

package agent

import (
	"context"
	"encoding/json"
	"mybuilds/internal/protocol"
	"os"
	"path/filepath"
	"testing"
)

func stoppedResourceFixture(t *testing.T) (*executionJournal, protocol.StopConfirmation) {
	t.Helper()
	j, w, _ := resourceFixture(t)
	in, e := j.stageWorkspace(context.Background(), w)
	if e != nil {
		t.Fatal(e)
	}
	if e = j.ackResource(in); e != nil {
		t.Fatal(e)
	}
	stop := protocol.StopConfirmation{Ref: *j.state.Ref, EvidenceCode: "process_group_reaped", Note: "本次执行进程组已完成真实回收"}
	j.state.PendingStop = &stop
	if e = j.save(); e != nil {
		t.Fatal(e)
	}
	if e = recordResourceStop(j, stop); e != nil {
		t.Fatal(e)
	}
	return j, stop
}

func TestRetentionResourceCompletionRejectsResidualSpool(t *testing.T) {
	j, stop := stoppedResourceFixture(t)
	spool := filepath.Join(j.lock.root.Name(), "spool")
	if e := os.MkdirAll(spool, 0700); e != nil {
		t.Fatal(e)
	}
	name := filepath.Join(spool, j.state.ClaimKey+"-1.json")
	if e := os.WriteFile(name, []byte("{}"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := j.prepareResourceCompletion(context.Background(), stop); e == nil {
		t.Fatal("nil PendingLog hid an unremoved spool file")
	}
	if _, e := readStoppedResourceJournal(j.lock, j.name); e == nil {
		t.Fatal("stopped recovery hid residual spool")
	}
	// 只移除本测试创建的残留；其它claim的文件仍保留。
	if e := os.Remove(name); e != nil {
		t.Fatal(e)
	}
	foreign := filepath.Join(spool, "other-claim-1.json")
	if e := os.WriteFile(foreign, []byte("{}"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := j.prepareResourceCompletion(context.Background(), stop); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(foreign); e != nil {
		t.Fatal("another claim spool was removed", e)
	}
}

func TestRetentionResourceCompletionSaveFailurePreservesProof(t *testing.T) {
	j, stop := stoppedResourceFixture(t)
	in, e := j.prepareResourceCompletion(context.Background(), stop)
	if e != nil {
		t.Fatal(e)
	}
	resources := filepath.Join(j.lock.root.Name(), "resources")
	if e = os.Chmod(resources, 0500); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = os.Chmod(resources, 0700) })
	if e = j.ackResourceCompletion(in); e == nil {
		t.Fatal("failed private save falsely acknowledged completion")
	}
	if e = os.Chmod(resources, 0700); e != nil {
		t.Fatal(e)
	}
	r, _, e := j.readResource()
	if e != nil || r.Completion == nil || r.CompletionConfirmed || *r.Completion != *in.Completion {
		t.Fatal("original pending proof was lost", e)
	}
	if _, e = os.Stat(filepath.Join(j.lock.root.Name(), "journal", j.name)); e != nil {
		t.Fatal("pending journal was removed", e)
	}
	again, e := j.prepareResourceCompletion(context.Background(), stop)
	if e != nil || !sameResourceRegistration(again, in) {
		t.Fatal("retry changed completion", e)
	}
	if e = j.ackResourceCompletion(again); e != nil {
		t.Fatal(e)
	}
}

func TestRetentionResourceStoppedJournalRejectsMalformedEvidence(t *testing.T) {
	for _, mode := range []string{"wrong_stop_ref", "cleanup", "unknown_field", "duplicate_field", "null", "negative_budget"} {
		t.Run(mode, func(t *testing.T) {
			j, _ := stoppedResourceFixture(t)
			switch mode {
			case "wrong_stop_ref":
				j.state.PendingStop.Ref.Epoch++
			case "cleanup":
				j.state.CleanupFailed = true
			case "negative_budget":
				j.state.RemainingPostBudgetNS = -1
			}
			data, e := json.Marshal(j.state)
			if e != nil {
				t.Fatal(e)
			}
			switch mode {
			case "unknown_field":
				data = append([]byte(`{"foreign":true,`), data[1:]...)
			case "duplicate_field":
				data = append([]byte(`{"claim_key":"duplicate",`), data[1:]...)
			case "null":
				data = []byte(`{"pending_stop":null}`)
			}
			if e = os.WriteFile(filepath.Join(j.lock.root.Name(), "journal", j.name), data, 0600); e != nil {
				t.Fatal(e)
			}
			if _, e = readStoppedResourceJournal(j.lock, j.name); e == nil {
				t.Fatal("malformed stopped journal accepted")
			}
			if _, e = os.Stat(filepath.Join(j.lock.root.Name(), "journal", j.name)); e != nil {
				t.Fatal("malformed journal removed", e)
			}
		})
	}
}
func TestRetentionResourceStoppedCompletionIsExactAndDurable(t *testing.T) {
	j, stop := stoppedResourceFixture(t)
	in, e := j.prepareResourceCompletion(context.Background(), stop)
	if e != nil {
		t.Fatal(e)
	}
	if in.Completion == nil || in.Completion.LastEventSeq != 0 || in.Completion.LastLogSeq != 0 || in.Completion.LastLogOffset != 0 || in.Completion.LastArtifactSeq != 0 || in.Completion.StopCode != "process_group_reaped" {
		t.Fatal("wrong completion cursor", in.Completion)
	}
	r, _, e := j.readResource()
	if e != nil || r.Completion == nil || r.CompletionConfirmed {
		t.Fatal("completion not pending on disk", e)
	}
	wrong := in
	wrong.Completion = &protocol.NodeResourceCompletion{LastEventSeq: 1, StopCode: "process_group_reaped"}
	if e = j.ackResourceCompletion(wrong); e == nil {
		t.Fatal("wrong completion acknowledgement accepted")
	}
	if e = j.ackResourceCompletion(in); e != nil {
		t.Fatal(e)
	}
	r, _, e = j.readResource()
	if e != nil || !r.CompletionConfirmed {
		t.Fatal("completion acknowledgement not durable", e)
	}
}
func TestRetentionResourceUnknownEvidenceCannotCreateCompletion(t *testing.T) {
	for _, mode := range []string{"event", "log", "artifact", "registration", "cleanup", "replacement"} {
		t.Run(mode, func(t *testing.T) {
			j, stop := stoppedResourceFixture(t)
			switch mode {
			case "event":
				j.state.PendingEvent = &protocol.ExecutionEvent{Ref: *j.state.Ref, Seq: 1}
			case "log":
				j.state.PendingLog = &protocol.LogChunk{}
			case "artifact":
				j.state.Artifacts = []localArtifact{{Confirmed: false}}
			case "registration":
				r, info, e := j.readResource()
				if e != nil {
					t.Fatal(e)
				}
				r.RegistrationPending = true
				j.mu.Lock()
				e = j.saveResourceLocked(context.Background(), r, info)
				j.mu.Unlock()
				if e != nil {
					t.Fatal(e)
				}
			case "cleanup":
				j.state.CleanupFailed = true
			case "replacement":
				r, _, e := j.readResource()
				if e != nil {
					t.Fatal(e)
				}
				p := filepath.Join(j.lock.root.Name(), r.Workspace.RelativePath)
				if e = os.Rename(p, p+"-old"); e != nil {
					t.Fatal(e)
				}
				if e = os.Mkdir(p, 0700); e != nil {
					t.Fatal(e)
				}
			}
			if _, e := j.prepareResourceCompletion(context.Background(), stop); e == nil {
				t.Fatal("unknown evidence falsely completed")
			}
			if _, e := os.Stat(filepath.Join(j.lock.root.Name(), "journal", j.name)); e != nil {
				t.Fatal("unknown journal removed", e)
			}
		})
	}
}
func TestRetentionResourceStoppedJournalStrictRecovery(t *testing.T) {
	j, _ := stoppedResourceFixture(t)
	recovered, e := readStoppedResourceJournal(j.lock, j.name)
	if e != nil || recovered.recoveryDigest == "" {
		t.Fatal("known stopped resource cannot be read", e)
	}
	j.state.PendingLog = &protocol.LogChunk{}
	if e = j.save(); e != nil {
		t.Fatal(e)
	}
	if _, e = readStoppedResourceJournal(j.lock, j.name); e == nil {
		t.Fatal("unknown log accepted for stopped recovery")
	}
}
