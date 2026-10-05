//go:build darwin || linux

package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
)

// 原事项全部经真实Claim、登记、停止、Completion和退役；不伪造SQL状态或授权。
func actualDeletionFixture(t *testing.T, wrap func(http.Handler) http.Handler) (*agentHTTP, *dataLock, protocol.NodeDeletion, *store.Store, store.Actor, config.AgentConfig) {
	t.Helper()
	ctx := context.Background()
	st, admin, cfg, handler := agentControl(t)
	if wrap != nil {
		handler = wrap(handler)
	}
	control := httptest.NewTLSServer(handler)
	t.Cleanup(control.Close)
	cfg.Server = control.URL
	cfg.CAFile = filepath.Join(t.TempDir(), "ca.pem")
	if e := os.WriteFile(cfg.CAFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: control.Certificate().Raw}), 0600); e != nil {
		t.Fatal(e)
	}
	client, e := newAgentHTTP(cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(client.close)
	checks := []protocol.ToolCheck{{Name: "node_journal", Status: "passed"}}
	for _, tool := range []struct {
		name, exe string
		args      []string
	}{{"shell", "sh", []string{"-c", "exit 0"}}, {"git", "git", []string{"--version"}}} {
		output, reason := toolOutput(ctx, tool.exe, tool.args, t.TempDir())
		if reason != "" {
			t.Fatal(reason)
		}
		check := protocol.ToolCheck{Name: tool.name, Status: "passed"}
		if tool.name == "git" {
			match := gitVersion.FindStringSubmatch(strings.TrimSpace(output))
			if len(match) < 2 {
				t.Fatal("git version")
			}
			check.Version = match[1]
		}
		checks = append(checks, check)
	}
	sid := uuid.NewString()
	var session protocol.SessionGrant
	if e = client.post(ctx, "/api/agent/session", protocol.SessionRequest{SessionID: sid, Report: protocol.NodeReport{OS: runtime.GOOS, Arch: runtime.GOARCH, Capacity: 1, Tools: checks}, HeartbeatNS: int64(cfg.HeartbeatInterval), LeaseNS: int64(cfg.LeaseDuration)}, &session); e != nil {
		t.Fatal(e)
	}
	project, e := st.CreateProject(ctx, admin, store.ProjectInput{Name: "delete-only", Repository: t.TempDir(), Branches: []string{"main"}, AllowedNodes: []string{cfg.Node}, DefaultNode: cfg.Node})
	if e != nil {
		t.Fatal(e)
	}
	definition := config.Build{Steps: []config.Step{{Kind: "run", Name: "never-started", Run: "exit 0"}}}
	data, _ := json.Marshal(definition)
	h := sha256.Sum256(data)
	digest := hex.EncodeToString(h[:])
	enqueue := func(key string) string {
		p, e := st.GetProject(ctx, project.Name)
		if e != nil {
			t.Fatal(e)
		}
		out, e := st.Enqueue(ctx, store.EnqueueInput{Actor: admin, ProjectID: p.ID, ProjectVersion: p.PolicyVersion, Key: key, RequestDigest: digest, SHA: hex.EncodeToString(h[:20]), Branch: "main", Source: "repo", File: "mybuilds.yml", SourceDigest: digest, Builds: []store.PreparedBuild{{Name: "default", Status: "queued", PostBudgetNS: int64(2 * time.Minute), Snapshot: store.BuildSnapshot{Definition: definition, Params: map[string]string{}, Facts: map[string]string{}, Condition: "ready", Reasons: []string{"条件已满足"}, AllowedNodes: p.AllowedNodes, DefaultNode: p.DefaultNode}, Steps: []store.StepProgress{{Phase: "ordinary", Index: 1, Name: "never-started", Kind: "run", Condition: "ready", Status: "pending", Reasons: []string{"条件已满足"}}}}}})
		if e != nil {
			t.Fatal(e)
		}
		return out.Builds[0].ID
	}
	enqueue("original")
	key := uuid.NewString()
	var grant protocol.LeaseGrant
	if e = client.post(ctx, "/api/agent/claim", protocol.ClaimRequest{SessionID: sid, ClaimKey: key}, &grant); e != nil || grant.Task == nil {
		t.Fatal("claim", e)
	}
	root := t.TempDir()
	if e = os.Chmod(root, 0700); e != nil {
		t.Fatal(e)
	}
	lock, e := lockDataDir(root)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { lock.Close() })
	j, e := newJournal(lock, key, sid)
	if e != nil {
		t.Fatal(e)
	}
	if e = j.setLease(grant.Ref, grant.RemainingBudgetNS, grant.RemainingPostBudgetNS); e != nil {
		t.Fatal(e)
	}
	workspace := filepath.Join(root, "scm", "checkout-delete", "workspace")
	if e = os.MkdirAll(workspace, 0700); e != nil {
		t.Fatal(e)
	}
	j.state.StopConfirmed = true
	if e = j.save(); e != nil {
		t.Fatal(e)
	}
	in, e := j.stageWorkspace(ctx, workspace)
	if e != nil {
		t.Fatal(e)
	}
	if e = client.post(ctx, "/api/agent/resources", in, nil); e != nil {
		t.Fatal(e)
	}
	if e = j.ackResource(in); e != nil {
		t.Fatal(e)
	}
	if e = st.SetNodeState(ctx, admin, cfg.Node, "disabled"); e != nil {
		t.Fatal(e)
	}
	if e = st.SetNodeState(ctx, admin, cfg.Node, "enabled"); e != nil {
		t.Fatal(e)
	}
	stop := protocol.StopConfirmation{Ref: grant.Ref, EvidenceCode: "process_group_reaped", Note: "本次执行进程组已完成真实回收"}
	j.state.PendingStop = &stop
	if e = j.save(); e != nil {
		t.Fatal(e)
	}
	if e = client.post(ctx, "/api/agent/stop-confirmation", stop, nil); e != nil {
		t.Fatal(e)
	}
	if e = completeStoppedResource(ctx, client, j, stop); e != nil {
		t.Fatal(e)
	}
	if e = j.remove(); e != nil {
		t.Fatal(e)
	}
	newer := enqueue("newer")
	if _, e = st.Cancel(ctx, admin, newer); e != nil {
		t.Fatal(e)
	}
	if e = st.SyncGlobalRetention(ctx, config.Retention{Builds: 1, Days: 30}); e != nil {
		t.Fatal(e)
	}
	if _, e = st.ScheduleRetention(ctx, admin, project.ID, 100); e != nil {
		t.Fatal(e)
	}
	var items []protocol.NodeDeletion
	if e = client.get(ctx, "/api/agent/deletions?limit=10", &items); e != nil || len(items) != 1 {
		t.Fatal("deletion actual claim", e, len(items))
	}
	if items[0].ResourceID != in.ID {
		t.Fatal("wrong resource")
	}
	return client, lock, items[0], st, admin, cfg
}

func TestRetentionDeleteActualHTTPRemovesOnlyRegisteredTree(t *testing.T) {
	var receipts []protocol.NodeDeletionConfirmation
	var mu sync.Mutex
	client, lock, item, _, _, _ := actualDeletionFixture(t, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/confirm") {
				body, e := io.ReadAll(io.LimitReader(r.Body, 32769))
				if e != nil {
					t.Error(e)
					return
				}
				r.Body = io.NopCloser(bytes.NewReader(body))
				var in protocol.NodeDeletionConfirmation
				if json.Unmarshal(body, &in) != nil {
					t.Error("confirmation JSON")
					return
				}
				mu.Lock()
				receipts = append(receipts, in)
				mu.Unlock()
			}
			next.ServeHTTP(w, r)
		})
	})
	r, e := deletionResourceRecord(context.Background(), lock, func() string {
		data, _, _, e := deletionPrivateFile(context.Background(), lock, "resources", item.ResourceID)
		if e != nil {
			t.Fatal(e)
		}
		var r resourceRecord
		json.Unmarshal(data, &r)
		return r.Ref.NodeID
	}(), item)
	if e != nil {
		t.Fatal(e)
	}
	for i := range 230 {
		if e = os.WriteFile(filepath.Join(lock.root.Name(), r.Workspace.RelativePath, fmt.Sprintf("file-%03d", i)), []byte("owned"), 0644); e != nil {
			t.Fatal(e)
		}
	}
	neighbor := filepath.Join(lock.root.Name(), "neighbor")
	if e = os.WriteFile(neighbor, []byte("keep"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = advanceNodeDeletion(context.Background(), client, lock, r.Ref.NodeID, item); e != nil {
		t.Fatal(e)
	}
	if _, e = lock.root.Lstat(r.Workspace.RelativePath); !os.IsNotExist(e) {
		t.Fatal("registered tree remains", e)
	}
	if b, e := os.ReadFile(neighbor); e != nil || string(b) != "keep" {
		t.Fatal("neighbor changed", e)
	}
	var remaining []protocol.NodeDeletion
	if e = client.get(context.Background(), "/api/agent/deletions?limit=10", &remaining); e != nil || len(remaining) != 0 {
		t.Fatal("completion not confirmed", e, len(remaining))
	}
	mu.Lock()
	if len(receipts) < 3 {
		t.Error("230 files did not produce bounded partial segments", len(receipts))
	}
	for i, in := range receipts {
		if in.Seq != int64(i+1) || (i < len(receipts)-1 && (in.WorkspaceState != "partial" || in.Reason != "partial")) {
			t.Error("segment evidence", i, in.Seq, in.WorkspaceState, in.Reason)
		}
	}
	mu.Unlock()
	if e = recoverNodeDeletionConfirmations(context.Background(), client, lock, r.Ref.NodeID); e != nil {
		t.Fatal(e)
	}
}

func TestRetentionDeleteActualLostACKOnlyReplaysReceipt(t *testing.T) {
	var lose atomic.Bool
	var confirmations atomic.Int64
	var authorizations atomic.Int64
	var pending protocol.NodeDeletionConfirmation
	var mu sync.Mutex
	client, lock, item, st, admin, cfg := actualDeletionFixture(t, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/authorize") {
				authorizations.Add(1)
			}
			if !strings.HasSuffix(r.URL.Path, "/confirm") {
				next.ServeHTTP(w, r)
				return
			}
			data, e := io.ReadAll(io.LimitReader(r.Body, 32769))
			if e != nil {
				t.Error(e)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(data))
			var in protocol.NodeDeletionConfirmation
			if json.Unmarshal(data, &in) != nil {
				t.Error("invalid receipt")
				return
			}
			mu.Lock()
			if confirmations.Add(1) == 1 {
				pending = in
			} else if in != pending {
				t.Error("changed original receipt")
			}
			mu.Unlock()
			recorded := httptest.NewRecorder()
			next.ServeHTTP(recorded, r)
			if recorded.Code != 200 {
				t.Errorf("actual confirmation %d", recorded.Code)
			}
			if lose.Load() {
				conn, _, e := w.(http.Hijacker).Hijack()
				if e != nil {
					t.Error(e)
					return
				}
				conn.Close()
				return
			}
			for key, values := range recorded.Header() {
				for _, value := range values {
					w.Header().Add(key, value)
				}
			}
			w.WriteHeader(recorded.Code)
			w.Write(recorded.Body.Bytes())
		})
	})
	data, _, _, e := deletionPrivateFile(context.Background(), lock, "resources", item.ResourceID)
	if e != nil {
		t.Fatal(e)
	}
	var record resourceRecord
	if json.Unmarshal(data, &record) != nil {
		t.Fatal("resource")
	}
	path := filepath.Join(lock.root.Name(), record.Workspace.RelativePath)
	if e = os.WriteFile(filepath.Join(path, "once"), []byte("owned"), 0644); e != nil {
		t.Fatal(e)
	}
	lose.Store(true)
	if e = advanceNodeDeletion(context.Background(), client, lock, record.Ref.NodeID, item); !temporaryNetwork(e) {
		t.Fatal("lost actual ACK not retained", e)
	}
	journal, e := readNodeDeletionJournal(lock, item.ID)
	if e != nil || journal.state.Pending == nil || journal.state.Pending.WorkspaceState != "deleted" {
		t.Fatal("real deleted pending proof", e)
	}
	if _, e = lock.root.Lstat(record.Workspace.RelativePath); !os.IsNotExist(e) {
		t.Fatal("original tree not physically gone", e)
	}
	// ACK丢失后插入同名新目录，只证明恢复没有任何删除动作。
	if e = os.Mkdir(path, 0700); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(path, "unrelated"), []byte("keep"), 0600); e != nil {
		t.Fatal(e)
	}
	rotated, e := st.RotateNodeToken(context.Background(), admin, cfg.Node)
	if e != nil {
		t.Fatal(e)
	}
	cfg.RuntimeToken = rotated.Token
	current, e := newAgentHTTP(cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer current.close()
	lose.Store(false)
	original := *journal.state.Pending
	for range 20 {
		var receipt protocol.NodeDeletionReceipt
		if e = current.post(context.Background(), "/api/agent/deletions/"+item.ID+"/confirm", original, &receipt); e != nil || receipt.ID != original.ID || receipt.Seq != original.Seq || receipt.Digest != original.Digest {
			t.Fatal("exact original repeat", e)
		}
	}
	if e = recoverNodeDeletionConfirmations(context.Background(), current, lock, record.Ref.NodeID); e != nil {
		t.Fatal(e)
	}
	if authorizations.Load() != 1 || confirmations.Load() != 22 {
		t.Fatal("recovery reacquired permission or did not repeat exact receipt", authorizations.Load(), confirmations.Load())
	}
	if _, e = lock.root.Lstat("deletions/" + item.ID + ".json"); !os.IsNotExist(e) {
		t.Fatal("exact ACK did not clear own pending", e)
	}
	if b, e := os.ReadFile(filepath.Join(path, "unrelated")); e != nil || string(b) != "keep" {
		t.Fatal("recovery touched new object", e)
	}
}

func TestRetentionDeleteActualPausedRevokedAndUnknownGuard(t *testing.T) {
	for _, mode := range []string{"paused", "revoked", "unknown_journal", "foreign_node"} {
		t.Run(mode, func(t *testing.T) {
			var authorizations atomic.Int64
			client, lock, item, st, admin, cfg := actualDeletionFixture(t, func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if strings.HasSuffix(r.URL.Path, "/authorize") {
						authorizations.Add(1)
					}
					next.ServeHTTP(w, r)
				})
			})
			data, _, _, e := deletionPrivateFile(context.Background(), lock, "resources", item.ResourceID)
			if e != nil {
				t.Fatal(e)
			}
			var record resourceRecord
			json.Unmarshal(data, &record)
			nodeID := record.Ref.NodeID
			switch mode {
			case "paused":
				client.paused.Store(true)
			case "revoked":
				if _, e = st.RotateNodeToken(context.Background(), admin, cfg.Node); e != nil {
					t.Fatal(e)
				}
			case "unknown_journal":
				if e = os.WriteFile(filepath.Join(lock.root.Name(), "journal", uuid.NewString()+".json"), []byte(`{"PID":1,"unknown":true}`), 0600); e != nil {
					t.Fatal(e)
				}
			case "foreign_node":
				nodeID = uuid.NewString()
			}
			sentinel := filepath.Join(lock.root.Name(), record.Workspace.RelativePath, "unchanged")
			if e = os.WriteFile(sentinel, []byte("keep"), 0644); e != nil {
				t.Fatal(e)
			}
			if e = advanceNodeDeletion(context.Background(), client, lock, nodeID, item); e == nil {
				t.Fatal("untrusted state received permission")
			}
			if b, e := os.ReadFile(sentinel); e != nil || string(b) != "keep" {
				t.Fatal("unauthorized physical action", e)
			}
			if mode != "revoked" && authorizations.Load() != 0 {
				t.Fatal("local guard did not precede network", authorizations.Load())
			}
			if _, e = lock.root.Lstat("deletions/" + item.ID + ".json"); !os.IsNotExist(e) {
				t.Fatal("denied grant created action journal", e)
			}
		})
	}
}

func TestRetentionDeleteActualIsolatedRestartResumesNewSegment(t *testing.T) {
	client, lock, item, _, _, _ := actualDeletionFixture(t, nil)
	data, _, _, e := deletionPrivateFile(context.Background(), lock, "resources", item.ResourceID)
	if e != nil {
		t.Fatal(e)
	}
	var record resourceRecord
	json.Unmarshal(data, &record)
	for i := range 2000 {
		if e = os.WriteFile(filepath.Join(lock.root.Name(), record.Workspace.RelativePath, fmt.Sprintf("file-%04d", i)), []byte("owned"), 0644); e != nil {
			t.Fatal(e)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	observed := make(chan bool, 1)
	go func() {
		until := time.Now().Add(10 * time.Second)
		for time.Now().Before(until) {
			data, e := lock.root.ReadFile("deletions/" + item.ID + ".json")
			if e == nil {
				var state nodeDeletionJournalState
				if json.Unmarshal(data, &state) == nil && state.Progress != nil && state.Progress.Active && state.Progress.WorkspaceState == "quarantined" && state.Pending == nil {
					cancel()
					observed <- true
					return
				}
			}
			if ctx.Err() != nil {
				observed <- false
				return
			}
			time.Sleep(time.Millisecond)
		}
		cancel()
		observed <- false
	}()
	if e = advanceNodeDeletion(ctx, client, lock, record.Ref.NodeID, item); e == nil {
		t.Fatal("actual interruption did not stop advance")
	}
	if !<-observed {
		t.Fatal("fixture did not observe persisted isolated intent")
	}
	journal, e := readNodeDeletionJournal(lock, item.ID)
	if e != nil || journal.state.Pending != nil || journal.state.Progress == nil || !journal.state.Progress.Active {
		t.Fatal("no exact active isolated journal", e)
	}
	if _, e = lock.root.Lstat(record.Workspace.RelativePath); !os.IsNotExist(e) {
		t.Fatal("original was not isolated", e)
	}
	root := lock.root.Name()
	if e = lock.Close(); e != nil {
		t.Fatal(e)
	}
	reopened, e := lockDataDir(root)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	if e = recoverNodeDeletionConfirmations(context.Background(), client, reopened, record.Ref.NodeID); e != nil {
		t.Fatal("known isolated intent blocks restart", e)
	}
	if e = advanceNodeDeletion(context.Background(), client, reopened, record.Ref.NodeID, item); e != nil {
		t.Fatal("fresh actual grant did not resume isolated tree", e)
	}
	var remaining []protocol.NodeDeletion
	if e = client.get(context.Background(), "/api/agent/deletions?limit=10", &remaining); e != nil || len(remaining) != 0 {
		t.Fatal("resumed deletion not confirmed", e)
	}
}

func TestRetentionDeleteActualTraversalLimitAndSpecialObjects(t *testing.T) {
	for _, mode := range []string{"visited", "depth", "symlink", "hardlink", "fifo"} {
		t.Run(mode, func(t *testing.T) {
			client, lock, item, _, _, _ := actualDeletionFixture(t, nil)
			data, _, _, e := deletionPrivateFile(context.Background(), lock, "resources", item.ResourceID)
			if e != nil {
				t.Fatal(e)
			}
			var record resourceRecord
			json.Unmarshal(data, &record)
			workspace := filepath.Join(lock.root.Name(), record.Workspace.RelativePath)
			switch mode {
			case "depth":
				name := workspace
				for range 65 {
					name = filepath.Join(name, "d")
					if e = os.Mkdir(name, 0755); e != nil {
						t.Fatal(e)
					}
				}
				if e = os.WriteFile(filepath.Join(name, "leaf"), []byte("keep"), 0644); e != nil {
					t.Fatal(e)
				}
			case "symlink":
				if e = os.Symlink("../../neighbor", filepath.Join(workspace, "link")); e != nil {
					t.Fatal(e)
				}
			case "hardlink":
				if e = os.WriteFile(filepath.Join(workspace, "file"), []byte("keep"), 0644); e != nil {
					t.Fatal(e)
				}
				if e = os.Link(filepath.Join(workspace, "file"), filepath.Join(lock.root.Name(), "neighbor")); e != nil {
					t.Fatal(e)
				}
			case "fifo":
				if e = unix.Mkfifo(filepath.Join(workspace, "pipe"), 0600); e != nil {
					t.Fatal(e)
				}
			case "visited":
				for i := range 2 {
					if e = os.WriteFile(filepath.Join(workspace, fmt.Sprint(i)), []byte("keep"), 0644); e != nil {
						t.Fatal(e)
					}
				}
			}
			if mode == "visited" {
				// 只验证真实有限遍历到边界，不声称真实删除了100000项。
				if e = lock.prepareDirectory("deletions"); e != nil {
					t.Fatal(e)
				}
				root, e := resourceDirectory(context.Background(), lock, ".")
				if e != nil {
					t.Fatal(e)
				}
				dir, e := resourceDirectory(context.Background(), lock, "deletions")
				if e != nil {
					t.Fatal(e)
				}
				requested := time.Now()
				var a protocol.DeletionAuthority
				if e = client.post(context.Background(), "/api/agent/deletions/"+item.ID+"/authorize", struct{}{}, &a); e != nil {
					t.Fatal(e)
				}
				deadline, e := deletionDeadline(requested, a, record.Ref.NodeID, item)
				if e != nil {
					t.Fatal(e)
				}
				j := &nodeDeletionJournal{lock: lock, state: nodeDeletionJournalState{Item: item, NodeID: record.Ref.NodeID, RootIdentity: root, DirectoryIdentity: dir, Progress: &nodeDeletionProgress{WorkspaceState: "pending", ResultsState: "not_applicable", Nonce: a.Nonce, Active: true}}}
				if e = j.save(context.Background()); e != nil {
					t.Fatal(e)
				}
				visited := 99999
				s := deletionSegment{ctx: context.Background(), client: client, j: j, deadline: deadline, started: time.Now(), visited: &visited}
				if e = s.run(record); e == nil || e.Error() != "agent_invalid_object" || visited != 100000 {
					t.Fatal("actual traversal crossed fixed bound", visited, e)
				}
				for i := range 2 {
					if b, e := lock.root.ReadFile("retention/" + item.ID + "/workspace/" + fmt.Sprint(i)); e != nil || string(b) != "keep" {
						t.Fatal("boundary deleted extra child", e)
					}
				}
			} else {
				if e = advanceNodeDeletion(context.Background(), client, lock, record.Ref.NodeID, item); e == nil || e.Error() != "agent_invalid_object" {
					t.Fatal("unsafe object was accepted", e)
				}
				var remaining []protocol.NodeDeletion
				if e = client.get(context.Background(), "/api/agent/deletions?limit=10", &remaining); e != nil || len(remaining) != 1 {
					t.Fatal("invalid remaining tree falsely completed", e, len(remaining))
				}
			}
			if mode == "hardlink" {
				if b, e := os.ReadFile(filepath.Join(lock.root.Name(), "neighbor")); e != nil || string(b) != "keep" {
					t.Fatal("foreign hardlink bytes changed", e)
				}
			}
		})
	}
}
