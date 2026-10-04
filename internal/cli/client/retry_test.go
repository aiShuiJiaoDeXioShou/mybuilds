package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/store"
)

func TestRetryCLIExplicitKeyNoAutomaticResend(t *testing.T) {
	t.Setenv("MYBUILDS_CLIENT_TOKEN", strings.Repeat("client-token", 4))
	id := uuid.NewString()
	var requests atomic.Int64
	var drop atomic.Bool
	drop.Store(true)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != "POST" || r.URL.Path != "/api/builds/"+id+"/retry" || r.Header.Get("Idempotency-Key") != "explicit-key" {
			t.Error("请求身份或地址改变")
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"allow_upload":true}` {
			t.Error("请求包含未允许的输入")
		}
		if drop.Swap(false) {
			c, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			c.Close()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"batch_id":"new-batch","sha":"fixed","builds":[{"id":"new-build","build_name":"compile","number":2,"status":"queued","retry_of":"`+id+`"}]}`)
	}))
	defer api.Close()
	args := []string{"--server-url", api.URL, "build", "retry", id, "--idempotency-key", "explicit-key", "--allow-upload", "--json"}
	cmd := NewCommand()
	var out, diagnostics bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&diagnostics)
	cmd.SetArgs(args)
	err := cmd.Execute()
	if err == nil || requests.Load() != 1 || !strings.Contains(diagnostics.String(), "request_key: explicit-key") {
		t.Fatal("丢响应被自动重发或key未交付", err, requests.Load())
	}
	cmd = NewCommand()
	out.Reset()
	diagnostics.Reset()
	cmd.SetOut(&out)
	cmd.SetErr(&diagnostics)
	cmd.SetArgs(args)
	if err = cmd.Execute(); err != nil || requests.Load() != 2 {
		t.Fatal("显式原请求重发失败", err, requests.Load())
	}
	var decoded struct {
		RequestKey string `json:"request_key"`
	}
	if json.Unmarshal(out.Bytes(), &decoded) != nil || decoded.RequestKey != "explicit-key" || !strings.Contains(out.String(), id) {
		t.Fatal("输出未保留请求/关系", out.String())
	}
	for _, invalid := range [][]string{{"build", "retry", id}, {"build", "retry", id, "--idempotency-key", ""}, {"build", "retry", "not-uuid", "--idempotency-key", "explicit-key"}, {"build", "retry", id, "--idempotency-key", "x/y"}, {"build", "retry", id, "--idempotency-key", "explicit-key", "--param", "version=PRIVATE"}, {"build", "retry", id, "--idempotency-key", "explicit-key", "--branch", "main"}} {
		_, err = executeRemote(t, append([]string{"--server-url", api.URL}, invalid...)...)
		if err == nil {
			t.Fatal("非法retry输入被接受")
		}
	}
	if requests.Load() != 2 {
		t.Fatal("非法输入连接API")
	}
}

func TestActualRetryCLIOriginalCancelledQueuedSnapshot(t *testing.T) {
	st, address := realRemoteAPI(t)
	ctx := context.Background()
	admin, err := st.Authenticate(ctx, os.Getenv("MYBUILDS_CLIENT_TOKEN"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.CreateProject(ctx, admin, store.ProjectInput{Name: "retry-app", Repository: "https://example.org/repo.git", AllowedNodes: []string{"worker"}, DefaultNode: "worker"})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := st.Enqueue(ctx, store.EnqueueInput{Actor: admin, ProjectID: p.ID, ProjectVersion: p.PolicyVersion, Key: "original", RequestDigest: strings.Repeat("a", 64), SHA: strings.Repeat("b", 40), Branch: "main", Source: "repo", File: "mybuilds.yml", SourceDigest: strings.Repeat("c", 64), Builds: []store.PreparedBuild{{Name: "compile", Status: "queued", PostBudgetNS: int64(2 * time.Minute), Snapshot: store.BuildSnapshot{Definition: config.Build{Steps: []config.Step{{Name: "shell", Kind: "run", Run: "printf PRIVATE_SCRIPT"}}}, Condition: "ready"}, Steps: []store.StepProgress{{Phase: "ordinary", Index: 1, Name: "shell", Kind: "run", Condition: "ready", Status: "pending"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	id := batch.Builds[0].ID
	_, err = st.Cancel(ctx, admin, id)
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"--server-url", address, "build", "retry", id, "--idempotency-key", "original-retry", "--json"}
	out, err := executeRemote(t, args...)
	if err != nil || strings.Contains(out, "PRIVATE") {
		t.Fatal(out, err)
	}
	var result struct {
		RequestKey string            `json:"request_key"`
		BatchID    string            `json:"batch_id"`
		SHA        string            `json:"sha"`
		Builds     []store.BuildView `json:"builds"`
	}
	if err = json.Unmarshal([]byte(out), &result); err != nil || result.RequestKey != "original-retry" || result.SHA != batch.SHA || len(result.Builds) != 1 || result.Builds[0].RetryOf != id || result.Builds[0].Number == nil || *result.Builds[0].Number != 2 {
		t.Fatal("原快照/新号/关联错误", out, err)
	}
	repeated, err := executeRemote(t, args...)
	if err != nil || repeated != out {
		t.Fatal("幂等重发改变结果", err)
	}
	for _, args := range [][]string{{"build", "show", result.Builds[0].ID}, {"build", "ls", "--project", p.Name}, {"build", "ls", "--project", p.Name, "--json"}} {
		out, err = executeRemote(t, append([]string{"--server-url", address}, args...)...)
		if err != nil || !strings.Contains(out, id) || strings.Contains(out, "PRIVATE") {
			t.Fatal("查询缺关联或秘密泄漏", out, err)
		}
	}
}
