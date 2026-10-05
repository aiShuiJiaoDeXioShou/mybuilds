//go:build darwin || linux

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
	"mybuilds/internal/protocol"
)

// 此夹具只准备私有文件保护事实，不发管理授权、不调用unlink或旧PID信号。
func deletionGuardFixture(t *testing.T) (*executionJournal, protocol.NodeDeletion) {
	t.Helper()
	j, stop := stoppedResourceFixture(t)
	registration, e := j.prepareResourceCompletion(context.Background(), stop)
	if e != nil {
		t.Fatal(e)
	}
	if e = j.ackResourceCompletion(registration); e != nil {
		t.Fatal(e)
	}
	// 本地保护门需要空执行journal；只移除本测试已知零动作文件。
	if e = j.remove(); e != nil {
		t.Fatal(e)
	}
	item := protocol.NodeDeletion{ID: uuid.NewString(), ResourceID: registration.ID, BuildID: registration.Ref.BuildID, AttemptID: registration.Ref.AttemptID, OwnershipDigest: registration.OwnershipDigest, HasWorkspace: registration.HasWorkspace, HasResults: registration.HasResults}
	return j, item
}

func TestRetentionDeleteUnknownJournalNeverInterpretsPID(t *testing.T) {
	j, item := deletionGuardFixture(t)
	name := filepath.Join(j.lock.root.Name(), "journal", uuid.NewString()+".json")
	// 不是真的进程身份，也不允许consumer解释它；断言仅检查拒绝和字节保持。
	original := []byte(`{"PID":1,"PGID":1,"unknown":true}`)
	if e := os.WriteFile(name, original, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := deletionResourceGuard(context.Background(), j.lock, j.state.Ref.NodeID, item); e == nil {
		t.Fatal("unknown execution journal allowed deletion")
	}
	after, e := os.ReadFile(name)
	if e != nil || !bytes.Equal(after, original) {
		t.Fatal("unknown journal was changed", e)
	}
}

func TestRetentionDeleteLocalProtectionAndIdentityBoundaries(t *testing.T) {
	for _, mode := range []string{"spool", "resource_symlink", "resource_hardlink", "resource_fifo", "workspace_replaced", "parent_replaced", "foreign_item", "pending_registration"} {
		t.Run(mode, func(t *testing.T) {
			j, item := deletionGuardFixture(t)
			resource := filepath.Join(j.lock.root.Name(), "resources", item.ResourceID+".json")
			record, _, e := j.readResource()
			if e != nil {
				t.Fatal(e)
			}
			workspace := filepath.Join(j.lock.root.Name(), record.Workspace.RelativePath)
			switch mode {
			case "spool":
				if e = os.MkdirAll(filepath.Join(j.lock.root.Name(), "spool"), 0700); e != nil {
					t.Fatal(e)
				}
				if e = os.WriteFile(filepath.Join(j.lock.root.Name(), "spool", j.state.ClaimKey+"-1.json"), []byte("{}"), 0600); e != nil {
					t.Fatal(e)
				}
			case "resource_symlink":
				if e = os.Rename(resource, resource+"-original"); e != nil {
					t.Fatal(e)
				}
				if e = os.Symlink(resource+"-original", resource); e != nil {
					t.Fatal(e)
				}
			case "resource_hardlink":
				if e = os.Link(resource, resource+"-other"); e != nil {
					t.Fatal(e)
				}
			case "resource_fifo":
				if e = os.Rename(resource, resource+"-original"); e != nil {
					t.Fatal(e)
				}
				if e = unix.Mkfifo(resource, 0600); e != nil {
					t.Fatal(e)
				}
			case "workspace_replaced":
				if e = os.Rename(workspace, workspace+"-original"); e != nil {
					t.Fatal(e)
				}
				if e = os.Mkdir(workspace, 0700); e != nil {
					t.Fatal(e)
				}
			case "parent_replaced":
				parent := filepath.Dir(workspace)
				if e = os.Rename(parent, parent+"-original"); e != nil {
					t.Fatal(e)
				}
				if e = os.Mkdir(parent, 0700); e != nil {
					t.Fatal(e)
				}
				if e = os.Rename(filepath.Join(parent+"-original", filepath.Base(workspace)), workspace); e != nil {
					t.Fatal(e)
				}
			case "foreign_item":
				item.AttemptID = uuid.NewString()
			case "pending_registration":
				record.RegistrationPending = true
				record.Completion = nil
				record.CompletionConfirmed = false
				_, info, e := j.readResource()
				if e != nil {
					t.Fatal(e)
				}
				j.mu.Lock()
				e = j.saveResourceLocked(context.Background(), record, info)
				j.mu.Unlock()
				if e != nil {
					t.Fatal(e)
				}
			}
			neighbor := filepath.Join(j.lock.root.Name(), "neighbor")
			original := []byte("unrelated private fixture")
			if e = os.WriteFile(neighbor, original, 0600); e != nil {
				t.Fatal(e)
			}
			if _, e = deletionResourceGuard(context.Background(), j.lock, j.state.Ref.NodeID, item); e == nil {
				t.Fatal("unsafe local resource allowed deletion")
			}
			after, e := os.ReadFile(neighbor)
			if e != nil || !bytes.Equal(after, original) {
				t.Fatal("neighbor changed", e)
			}
		})
	}
}

func TestRetentionDeleteDeadlineStartsAtRequestNotArrival(t *testing.T) {
	j, item := deletionGuardFixture(t)
	requested := time.Now().Add(-6 * time.Second)
	authority := protocol.DeletionAuthority{ID: item.ID, NodeID: j.state.Ref.NodeID, ResourceID: item.ResourceID, OwnershipDigest: item.OwnershipDigest, Nonce: uuid.NewString(), ExpiresAt: time.Now().Add(5 * time.Second)}
	if _, e := deletionDeadline(requested, authority, j.state.Ref.NodeID, item); e == nil {
		t.Fatal("late response restarted the five-second permission")
	}
}

func TestRetentionDeleteCancelledContextCannotReadGrant(t *testing.T) {
	j, item := deletionGuardFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := deletionResourceGuard(ctx, j.lock, j.state.Ref.NodeID, item); e == nil {
		t.Fatal("cancelled deletion continued")
	}
}

func TestRetentionDeleteJournalRejectsUnknownConfirmation(t *testing.T) {
	for _, mode := range []string{"duplicate", "null", "unknown_field", "fifo", "hardlink"} {
		t.Run(mode, func(t *testing.T) {
			j, item := deletionGuardFixture(t)
			dir := filepath.Join(j.lock.root.Name(), "deletions")
			if e := os.Mkdir(dir, 0700); e != nil {
				t.Fatal(e)
			}
			name := filepath.Join(dir, item.ID+".json")
			data := []byte(`{"id":"` + item.ID + `","id":"` + item.ID + `"}`)
			switch mode {
			case "null":
				data = []byte(`{"pending":null}`)
			case "unknown_field":
				data = []byte(`{"unknown":true}`)
			case "fifo":
				if e := unix.Mkfifo(name, 0600); e != nil {
					t.Fatal(e)
				}
			}
			if mode != "fifo" {
				if e := os.WriteFile(name, data, 0600); e != nil {
					t.Fatal(e)
				}
			}
			if mode == "hardlink" {
				if e := os.Link(name, name+"-linked"); e != nil {
					t.Fatal(e)
				}
			}
			if _, e := readNodeDeletionJournal(j.lock, item.ID); e == nil {
				t.Fatal("unknown deletion confirmation accepted")
			}
			if _, e := os.Lstat(name); e != nil {
				t.Fatal("unknown confirmation removed", e)
			}
		})
	}
}

// 这是合法的本地“失败待补传”文法，不是远端授权，也没有删除动作。
func TestRetentionDeletePendingFailureJournalCanonicalBinding(t *testing.T) {
	for _, mode := range []string{"valid", "foreign_node", "wrong_digest", "wrong_slot", "wrong_case", "wrong_reason", "zero_seq", "deleted_without_progress"} {
		t.Run(mode, func(t *testing.T) {
			j, item := deletionGuardFixture(t)
			if e := os.Mkdir(filepath.Join(j.lock.root.Name(), "deletions"), 0700); e != nil {
				t.Fatal(e)
			}
			root, e := resourceDirectory(context.Background(), j.lock, ".")
			if e != nil {
				t.Fatal(e)
			}
			dir, e := resourceDirectory(context.Background(), j.lock, "deletions")
			if e != nil {
				t.Fatal(e)
			}
			p := protocol.NodeDeletionConfirmation{ID: item.ID, ResourceID: item.ResourceID, OwnershipDigest: item.OwnershipDigest, Nonce: uuid.NewString(), Seq: 1, WorkspaceState: "failed", ResultsState: "not_applicable", Reason: "resource_unconfirmed"}
			p.Digest, e = protocol.NodeDeletionDigest(p)
			if e != nil {
				t.Fatal(e)
			}
			state := nodeDeletionJournalState{Item: item, NodeID: j.state.Ref.NodeID, RootIdentity: root, DirectoryIdentity: dir, Pending: &p}
			switch mode {
			case "foreign_node":
				state.NodeID = uuid.NewString()
			case "wrong_digest":
				p.Digest = strings.Repeat("f", 64)
			case "wrong_slot":
				p.ResultsState = "deleted"
			case "wrong_reason":
				p.Reason = "PRIVATE_OS_ERROR"
			case "zero_seq":
				p.Seq = 0
			case "deleted_without_progress":
				p.WorkspaceState = "deleted"
				p.Reason = ""
				p.Digest, _ = protocol.NodeDeletionDigest(p)
			}
			data, e := json.Marshal(state)
			if e != nil {
				t.Fatal(e)
			}
			if mode == "wrong_case" {
				data = bytes.Replace(data, []byte(`"node_id"`), []byte(`"Node_ID"`), 1)
			}
			path := filepath.Join(j.lock.root.Name(), "deletions", item.ID+".json")
			if e = os.WriteFile(path, data, 0600); e != nil {
				t.Fatal(e)
			}
			read, e := readNodeDeletionJournal(j.lock, item.ID)
			if mode == "valid" {
				if e != nil || read.digest == "" || read.state.Pending.Digest != p.Digest {
					t.Fatal("valid failed journal grammar rejected", e)
				}
			} else if e == nil {
				t.Fatal("invalid pending failure binding accepted")
			}
			after, e := os.ReadFile(path)
			if e != nil || !bytes.Equal(after, data) {
				t.Fatal("reader changed private confirmation", e)
			}
		})
	}
}

func TestRetentionDeleteDeadlineCannotUseRemoteWallClock(t *testing.T) {
	j, item := deletionGuardFixture(t)
	requested := time.Now()
	authority := protocol.DeletionAuthority{ID: item.ID, NodeID: j.state.Ref.NodeID, ResourceID: item.ResourceID, OwnershipDigest: item.OwnershipDigest, Nonce: uuid.NewString(), ExpiresAt: requested.Add(time.Hour)}
	deadline, e := deletionDeadline(requested, authority, j.state.Ref.NodeID, item)
	if e != nil || !deadline.After(requested) || !deadline.Before(requested.Add(5*time.Second)) {
		t.Fatal("remote audit clock extended local permission", e)
	}
	authority.NodeID = uuid.NewString()
	if _, e = deletionDeadline(requested, authority, j.state.Ref.NodeID, item); e == nil {
		t.Fatal("foreign node permission accepted")
	}
	authority.NodeID = j.state.Ref.NodeID
	authority.Nonce = "not-a-manager-nonce"
	if _, e = deletionDeadline(requested, authority, j.state.Ref.NodeID, item); e == nil {
		t.Fatal("malformed management nonce accepted")
	}
}
