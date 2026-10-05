//go:build darwin || linux

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"mybuilds/internal/protocol"
)

// 旧Ref只作归属证据；新session管理清理不重新领取原Task或授用户执行权。
func TestRetentionDeleteServeCurrentIdentityHeartbeatWithoutTask(t *testing.T) {
	var serving, inDeletion atomic.Bool
	var heartbeats, overlap, claims, tasks, execution, authorizations, confirmations atomic.Int64
	var newSession string
	var sessionMu sync.Mutex
	completed := make(chan struct{})
	var completedOnce sync.Once
	client, lock, item, st, admin, cfg := actualDeletionFixture(t, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !serving.Load() {
				next.ServeHTTP(w, r)
				return
			}
			if r.URL.Path == "/api/agent/heartbeat" {
				heartbeats.Add(1)
				if inDeletion.Load() {
					overlap.Add(1)
				}
			}
			if r.URL.Path == "/api/agent/events" || r.URL.Path == "/api/agent/renew" || r.URL.Path == "/api/agent/logs" || strings.HasPrefix(r.URL.Path, "/api/agent/artifacts/") {
				execution.Add(1)
			}
			if strings.HasSuffix(r.URL.Path, "/authorize") {
				authorizations.Add(1)
				inDeletion.Store(true)
			}
			if r.URL.Path != "/api/agent/session" && r.URL.Path != "/api/agent/claim" && !strings.HasSuffix(r.URL.Path, "/authorize") && !strings.HasSuffix(r.URL.Path, "/confirm") {
				next.ServeHTTP(w, r)
				return
			}
			var in protocol.NodeDeletionConfirmation
			if strings.HasSuffix(r.URL.Path, "/confirm") {
				data, e := io.ReadAll(io.LimitReader(r.Body, 32769))
				if e != nil {
					t.Error(e)
					return
				}
				r.Body = io.NopCloser(bytes.NewReader(data))
				if json.Unmarshal(data, &in) != nil {
					t.Error("confirmation JSON")
					return
				}
				confirmations.Add(1)
			}
			recorded := httptest.NewRecorder()
			next.ServeHTTP(recorded, r)
			if r.URL.Path == "/api/agent/session" && recorded.Code == 200 {
				var grant protocol.SessionGrant
				if json.Unmarshal(recorded.Body.Bytes(), &grant) != nil {
					t.Error("actual session JSON")
				}
				sessionMu.Lock()
				newSession = grant.SessionID
				sessionMu.Unlock()
			}
			if r.URL.Path == "/api/agent/claim" {
				claims.Add(1)
				var grant protocol.LeaseGrant
				if recorded.Code == 200 && json.Unmarshal(recorded.Body.Bytes(), &grant) == nil && grant.Task != nil {
					tasks.Add(1)
				}
			}
			if strings.HasSuffix(r.URL.Path, "/authorize") || strings.HasSuffix(r.URL.Path, "/confirm") {
				// 每次仍是原Store真正应答；网络期间的心跳必须独立前进。
				timer := time.NewTimer(450 * time.Millisecond)
				select {
				case <-timer.C:
				case <-r.Context().Done():
					timer.Stop()
					return
				}
			}
			for key, values := range recorded.Header() {
				for _, value := range values {
					w.Header().Add(key, value)
				}
			}
			w.WriteHeader(recorded.Code)
			_, _ = w.Write(recorded.Body.Bytes())
			if strings.HasSuffix(r.URL.Path, "/confirm") && recorded.Code == 200 && in.WorkspaceState == "deleted" && in.ResultsState == "not_applicable" {
				inDeletion.Store(false)
				completedOnce.Do(func() { close(completed) })
			}
		})
	})
	data, _, _, e := deletionPrivateFile(context.Background(), lock, "resources", item.ResourceID)
	if e != nil {
		t.Fatal(e)
	}
	var resource resourceRecord
	if json.Unmarshal(data, &resource) != nil {
		t.Fatal("original resource")
	}
	for i := range 230 {
		if e = os.WriteFile(filepath.Join(lock.root.Name(), resource.Workspace.RelativePath, uuid.NewString()), []byte{byte(i)}, 0644); e != nil {
			t.Fatal(e)
		}
	}
	cfg.DataDir = lock.root.Name()
	if e = lock.Close(); e != nil {
		t.Fatal(e)
	}
	rotated, e := st.RotateNodeToken(context.Background(), admin, cfg.Node)
	if e != nil {
		t.Fatal(e)
	}
	cfg.RuntimeToken = rotated.Token
	// 真实session替换门要求旧心跳已过2倍lease；不改数据库时钟或会话字段。
	node, e := st.GetNode(context.Background(), admin, cfg.Node)
	if e != nil || node.LastHeartbeat == nil {
		t.Fatal("original heartbeat", e)
	}
	until := node.LastHeartbeat.Add(2*cfg.LeaseDuration + 50*time.Millisecond)
	if wait := time.Until(until); wait > 0 {
		timer := time.NewTimer(wait)
		<-timer.C
	}
	t.Setenv("JAVA_HOME", filepath.Join(t.TempDir(), "missing-java"))
	t.Setenv("ANDROID_HOME", filepath.Join(t.TempDir(), "missing-sdk"))
	t.Setenv("ANDROID_SDK_ROOT", "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serving.Store(true)
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, cfg) }()
	// 无queued构建，旧ref已经interrupted+真实StopKnown，删除本身不能依赖Task。
	select {
	case <-completed:
	case e := <-done:
		t.Fatalf("Serve ended before real deletion: %v", e)
	case <-time.After(15 * time.Second):
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("Serve did not stop")
		}
		t.Fatal("Serve never processed the scheduled independent deletion")
	}
	// 等真实客户端确认后本地原pending已安全移除，再正常停止本次Serve。
	acknowledged := false
	untilAck := time.Now().Add(3 * time.Second)
	for time.Now().Before(untilAck) {
		if _, e := os.Lstat(filepath.Join(cfg.DataDir, "deletions", item.ID+".json")); os.IsNotExist(e) {
			acknowledged = true
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve cancellation blocked")
	}
	if !acknowledged {
		t.Fatal("central real receipt did not clear local exact pending")
	}
	if authorizations.Load() < 3 || confirmations.Load() < 3 {
		t.Fatal("actual bounded deletion did not run", authorizations.Load(), confirmations.Load())
	}
	if heartbeats.Load() < 2 || overlap.Load() < 1 {
		t.Fatal("management loop blocked actual heartbeat", heartbeats.Load(), overlap.Load())
	}
	if tasks.Load() != 0 || execution.Load() != 0 {
		t.Fatal("management deletion used user execution", tasks.Load(), execution.Load())
	}
	sessionMu.Lock()
	sid := newSession
	sessionMu.Unlock()
	if sid == "" || sid == resource.Ref.SessionID {
		t.Fatal("rotation reused old execution session")
	}
	if _, e = os.Lstat(filepath.Join(cfg.DataDir, resource.Workspace.RelativePath)); !os.IsNotExist(e) {
		t.Fatal("original registered root remains", e)
	}
	cfg.Server = client.endpoint
	current, e := newAgentHTTP(cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer current.close()
	var remaining []protocol.NodeDeletion
	if e = current.get(context.Background(), "/api/agent/deletions?limit=10", &remaining); e != nil || len(remaining) != 0 {
		t.Fatal("central real receipt not complete", e, len(remaining))
	}
	t.Logf("actual sessions differ; heartbeats=%d during_delete=%d grants=%d confirmations=%d empty_claims=%d task_grants=0 user_events=0", heartbeats.Load(), overlap.Load(), authorizations.Load(), confirmations.Load(), claims.Load())
}

func TestRetentionDeleteServeUnknownJournalPreservedWithoutClaim(t *testing.T) {
	var claims, authorizations atomic.Int64
	st, admin, cfg, next := agentControl(t)
	control := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/agent/claim" {
			claims.Add(1)
		}
		if strings.HasSuffix(r.URL.Path, "/authorize") {
			authorizations.Add(1)
		}
		next.ServeHTTP(w, r)
	}))
	defer control.Close()
	cfg.Server = control.URL
	cfg.CAFile = filepath.Join(t.TempDir(), "ca.pem")
	if e := os.WriteFile(cfg.CAFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: control.Certificate().Raw}), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.MkdirAll(filepath.Join(cfg.DataDir, "journal"), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.Mkdir(filepath.Join(cfg.DataDir, "deletions"), 0700); e != nil {
		t.Fatal(e)
	}
	name := filepath.Join(cfg.DataDir, "deletions", uuid.NewString()+".json")
	original := []byte(`{"unknown":true,"PID":1}`)
	if e := os.WriteFile(name, original, 0600); e != nil {
		t.Fatal(e)
	}
	t.Setenv("JAVA_HOME", filepath.Join(t.TempDir(), "missing-java"))
	t.Setenv("ANDROID_HOME", filepath.Join(t.TempDir(), "missing-sdk"))
	t.Setenv("ANDROID_SDK_ROOT", "")
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	e := Serve(ctx, cfg)
	if e == nil || e.Error() != "agent_resource_unconfirmed" {
		t.Fatal("unknown delete journal was accepted", e)
	}
	if claims.Load() != 0 || authorizations.Load() != 0 {
		t.Fatal("unknown proof created claim or new permission", claims.Load(), authorizations.Load())
	}
	after, e := os.ReadFile(name)
	if e != nil || !bytes.Equal(after, original) {
		t.Fatal("unknown journal changed", e)
	}
	entries, e := os.ReadDir(filepath.Join(cfg.DataDir, "journal"))
	if e != nil || len(entries) != 0 {
		t.Fatal("unknown state created execution journal", e, len(entries))
	}
	if _, e = st.GetNode(context.Background(), admin, cfg.Node); e != nil {
		t.Fatal("own node was changed", e)
	}
}
