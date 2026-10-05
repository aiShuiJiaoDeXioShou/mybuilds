//go:build darwin || linux

package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
)

// 仅这条真实网络故障门消费；不替换Context或远端响应。
type completionDeadlineBody struct {
	ctx    context.Context
	reader *bytes.Reader
	once   sync.Once
	t      *testing.T
}

func (b *completionDeadlineBody) Read(p []byte) (int, error) {
	b.once.Do(func() {
		select {
		case <-b.ctx.Done():
		case <-time.After(3 * time.Second):
			b.t.Error("real completion request did not reach original deadline")
		}
	})
	return b.reader.Read(p)
}
func (*completionDeadlineBody) Close() error { return nil }

// 本门确实没有用户动作：真实Claim之后停止零动作租约，不伪造进程Started或终态。
func TestRetentionResourceCompletionActualHTTPAndLostACKRecovery(t *testing.T) {
	ctx := context.Background()
	st, admin, cfg, handler := agentControl(t)
	var lose atomic.Bool
	var attempts atomic.Int64
	var committed atomic.Int64
	var recoveredACK atomic.Int64
	var cancelledAttempts atomic.Int64
	var first []byte
	var messageMu sync.Mutex
	control := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		completion := false
		if r.URL.Path == "/api/agent/resources" {
			data, e := io.ReadAll(io.LimitReader(r.Body, 32769))
			if e != nil {
				t.Error(e)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(data))
			var in protocol.NodeResourceRegistration
			if json.Unmarshal(data, &in) == nil && in.Completion != nil {
				completion = true
				messageMu.Lock()
				if attempts.Add(1) == 1 {
					first = append([]byte(nil), data...)
				} else if !bytes.Equal(first, data) {
					t.Error("completion retry changed original message")
				}
				messageMu.Unlock()
			}
		}
		if !completion {
			handler.ServeHTTP(w, r)
			return
		}
		if lose.Load() && attempts.Load() == 2 {
			// 原Handler真实鉴权后才读body；只延迟原bytes直到同一个请求真的取消。
			data, e := io.ReadAll(io.LimitReader(r.Body, 32769))
			if e != nil {
				t.Error(e)
				return
			}
			r.Body = &completionDeadlineBody{ctx: r.Context(), reader: bytes.NewReader(data), t: t}
		}
		recorded := httptest.NewRecorder()
		handler.ServeHTTP(recorded, r)
		if recorded.Code != http.StatusNoContent || recorded.Body.Len() != 0 {
			code := "unknown_error"
			var envelope struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if json.Unmarshal(recorded.Body.Bytes(), &envelope) == nil && safeNodeError(envelope.Error.Code) {
				code = envelope.Error.Code
			}
			ctxState := "active"
			if r.Context().Err() == context.Canceled {
				ctxState = "cancelled"
			} else if r.Context().Err() == context.DeadlineExceeded {
				ctxState = "deadline"
			}
			// 已有真实204提交的丢ACK窗口内，原请求实际取消可被Store拒绝；
			// 活跃请求、鉴权前500或任何其它拒绝仍是本门失败。
			if recorded.Code == http.StatusConflict && code == "retention_cancelled" &&
				r.Context().Err() == context.Canceled && committed.Load() > 0 && lose.Load() {
				cancelledAttempts.Add(1)
				t.Logf("actual cancelled retry status=%d code=%s request_ctx=%s prior_empty204=%d", recorded.Code, code, ctxState, committed.Load())
			} else {
				t.Errorf("actual completion response status=%d code=%s request_ctx=%s committed_204=%d dropped_ack=%t", recorded.Code, code, ctxState, committed.Load(), lose.Load())
			}
		}
		if recorded.Code == http.StatusNoContent && recorded.Body.Len() == 0 {
			committed.Add(1)
			if !lose.Load() {
				recoveredACK.Add(1)
			}
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
		_, _ = w.Write(recorded.Body.Bytes())
	}))
	defer control.Close()
	cfg.Server = control.URL
	cfg.CAFile = filepath.Join(t.TempDir(), "ca.pem")
	if e := os.WriteFile(cfg.CAFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: control.Certificate().Raw}), 0600); e != nil {
		t.Fatal(e)
	}
	client, e := newAgentHTTP(cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer client.close()
	data := t.TempDir()
	if e = os.Chmod(data, 0700); e != nil {
		t.Fatal(e)
	}
	lock, e := lockDataDir(data)
	if e != nil {
		t.Fatal(e)
	}
	if e = lock.Close(); e != nil {
		t.Fatal(e)
	}
	if e = os.Mkdir(filepath.Join(data, "journal"), 0700); e != nil {
		t.Fatal(e)
	}
	if inspectData(data) != "" {
		t.Fatal("initialized empty journal was not inspected as clear")
	}
	checks := []protocol.ToolCheck{{Name: "node_journal", Status: "passed"}}
	for _, tool := range []struct {
		name, exe string
		args      []string
	}{{"shell", "sh", []string{"-c", "exit 0"}}, {"git", "git", []string{"--version"}}} {
		output, reason := toolOutput(ctx, tool.exe, tool.args, t.TempDir())
		if reason != "" {
			t.Fatal("actual bounded tool check", tool.name, reason)
		}
		check := protocol.ToolCheck{Name: tool.name, Status: "passed"}
		if tool.name == "git" {
			match := gitVersion.FindStringSubmatch(strings.TrimSpace(output))
			if len(match) < 2 {
				t.Fatal("actual git version")
			}
			check.Version = match[1]
		}
		checks = append(checks, check)
	}
	sid := uuid.NewString()
	session := protocol.SessionRequest{SessionID: sid, Report: protocol.NodeReport{OS: runtime.GOOS, Arch: runtime.GOARCH, Capacity: 1, Tools: checks}, HeartbeatNS: int64(cfg.HeartbeatInterval), LeaseNS: int64(cfg.LeaseDuration)}
	var sessionGrant protocol.SessionGrant
	if e = client.post(ctx, "/api/agent/session", session, &sessionGrant); e != nil {
		t.Fatal(e)
	}
	project, e := st.CreateProject(ctx, admin, store.ProjectInput{Name: "completion-only", Repository: t.TempDir(), Branches: []string{"main"}, AllowedNodes: []string{cfg.Node}, DefaultNode: cfg.Node})
	if e != nil {
		t.Fatal(e)
	}
	definition := config.Build{Steps: []config.Step{{Kind: "run", Name: "never-started", Run: "exit 0"}}}
	frozen, _ := json.Marshal(definition)
	hash := sha256.Sum256(frozen)
	digest := hex.EncodeToString(hash[:])
	_, e = st.Enqueue(ctx, store.EnqueueInput{
		Actor: admin, ProjectID: project.ID, ProjectVersion: project.PolicyVersion,
		Key: "completion-zero-action", RequestDigest: digest, SHA: hex.EncodeToString(hash[:20]),
		Branch: "main", Source: "repo", File: "mybuilds.yml", SourceDigest: digest,
		Builds: []store.PreparedBuild{{
			Name: "default", Status: "queued", PostBudgetNS: int64(2 * time.Minute),
			Snapshot: store.BuildSnapshot{Definition: definition, Params: map[string]string{}, Facts: map[string]string{}, Condition: "ready", Reasons: []string{"条件已满足"}, AllowedNodes: project.AllowedNodes, DefaultNode: project.DefaultNode},
			Steps:    []store.StepProgress{{Phase: "ordinary", Index: 1, Name: "never-started", Kind: "run", Condition: "ready", Status: "pending", Reasons: []string{"条件已满足"}}},
		}},
	})
	if e != nil {
		t.Fatal(e)
	}
	key := uuid.NewString()
	var grant protocol.LeaseGrant
	if e = client.post(ctx, "/api/agent/claim", protocol.ClaimRequest{SessionID: sid, ClaimKey: key}, &grant); e != nil || grant.Task == nil {
		t.Fatal("actual claim", e)
	}
	lock, e = lockDataDir(data)
	if e != nil {
		t.Fatal(e)
	}
	defer lock.Close()
	j, e := newJournal(lock, key, sid)
	if e != nil {
		t.Fatal(e)
	}
	if e = j.setLease(grant.Ref, grant.RemainingBudgetNS, grant.RemainingPostBudgetNS); e != nil {
		t.Fatal(e)
	}
	// 只创建本测试的空目录用于登记文件协议，不声称完成了SCM checkout或Run。
	workspace := filepath.Join(data, "scm", "checkout-http", "workspace")
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
	lose.Store(true)
	if e = completeStoppedResource(ctx, client, j, stop); !temporaryNetwork(e) {
		t.Fatal("actual lost ACK was not retained", e)
	}
	record, _, e := j.readResource()
	if e != nil || record.Completion == nil || record.CompletionConfirmed {
		t.Fatal("lost ACK falsely cleared local completion", e)
	}
	before, e := st.GetBuild(ctx, grant.Ref.BuildID)
	if e != nil || before.Status != "interrupted" || before.StopUnconfirmed {
		t.Fatal("actual stopped result", e)
	}
	if attempts.Load() < 2 {
		t.Fatal("no real identical retransmission")
	}
	rotated, e := st.RotateNodeToken(ctx, admin, cfg.Node)
	if e != nil {
		t.Fatal(e)
	}
	cfg.RuntimeToken = rotated.Token
	currentClient, e := newAgentHTTP(cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer currentClient.close()
	lose.Store(false)
	recovered, e := readStoppedResourceJournal(lock, j.name)
	if e != nil {
		t.Fatal(e)
	}
	if e = restoreStoppedResource(ctx, currentClient, recovered); e != nil {
		t.Fatal(e)
	}
	record, _, e = recovered.readResource()
	if e != nil || !record.CompletionConfirmed {
		t.Fatal("current identity did not confirm original completion", e)
	}
	after, e := st.GetBuild(ctx, grant.Ref.BuildID)
	if e != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("completion recovery changed stopped build", e)
	}
	if cancelledAttempts.Load() != 1 {
		t.Fatal("controlled real cancelled attempt was not observed", cancelledAttempts.Load())
	}
	if committed.Load() == 0 || recoveredACK.Load() == 0 {
		t.Fatal("first actual completion commit or recovery ACK missing")
	}
	t.Logf("actual empty204 completion responses=%d current_identity_recovery_ack=%d", committed.Load(), recoveredACK.Load())
	if _, e = os.Stat(filepath.Join(data, "journal", j.name)); e != nil {
		t.Fatal("helper removed journal before root final same-inode check", e)
	}
}
