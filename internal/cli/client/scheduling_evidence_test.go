//go:build darwin || linux

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	node "mybuilds/internal/agent"
	agentcli "mybuilds/internal/cli/agent"
	"mybuilds/internal/config"
	"mybuilds/internal/server"
	"mybuilds/internal/store"
)

type schedulingEvidence struct {
	t                         *testing.T
	root, address, repository string
	st                        *store.Store
}

// 只构造实际入口所需的自有资源；脚本屏障由本测试创建文件解除。
func newSchedulingEvidence(t *testing.T, concurrency, nodes int) *schedulingEvidence {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MYBUILDS_AGENT_TOKEN", "")
	if err := os.Unsetenv("MYBUILDS_AGENT_TOKEN"); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(context.Background(), store.Options{Driver: "sqlite", DSN: filepath.Join(root, "control.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err = st.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	created, err := st.CreateToken(context.Background(), store.Actor{ID: "local-admin", Role: "admin"}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("MYBUILDS_CLIENT_TOKEN", created.Token)
	api := httptest.NewServer(server.New(st, config.ServerConfig{DataDir: root, Concurrency: concurrency, HeartbeatInterval: time.Second, LeaseDuration: 10 * time.Second}).Handler())
	t.Cleanup(api.Close)
	f := &schedulingEvidence{t: t, root: root, address: api.URL, st: st, repository: filepath.Join(root, "source")}
	if err = os.Mkdir(f.repository, 0700); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "core.hooksPath=" + filepath.Join(root, "empty-hooks")}, args...)...)
		cmd.Dir = f.repository
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + root, "GIT_CONFIG_SYSTEM=/dev/null", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid"}
		if _, err := cmd.CombinedOutput(); err != nil {
			t.Fatal("自有Git失败", err)
		}
	}
	git("init", "--initial-branch=main", "--template=")
	var source strings.Builder
	source.WriteString("version: 1\nbuilds:\n")
	for _, name := range []string{"flex", "single", "labels"} {
		source.WriteString("  " + name + ":\n")
		if name != "single" {
			source.WriteString("    runner: {platform: android")
			if name == "labels" {
				source.WriteString(", labels: [missing-required-label]")
			}
			source.WriteString("}\n")
		}
		source.WriteString(`    params: {gate: {required: true}, counter: {required: true}}
    env: {GATE: '{{gate}}', COUNTER: '{{counter}}'}
    timeout: 30s
    steps:
      - kind: run
        name: held
        run: 'printf "once\n" >> "$COUNTER"; while ! test -f "$GATE"; do sleep 0.02; done'
`)
	}
	source.WriteString(`  failed:
    timeout: 10s
    steps:
      - kind: run
        name: original
        run: 'exit 7'
    post:
      failure:
        - kind: run
          name: secondary
          run: 'exit 9'
      always:
        - kind: run
          name: final
          run: printf completed
`)
	if err = os.WriteFile(filepath.Join(f.repository, "mybuilds.yml"), []byte(source.String()), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "mybuilds.yml")
	git("commit", "-m", "own scheduling fixture")
	for i := 1; i <= nodes; i++ {
		name := fmt.Sprintf("schedule-%d", i)
		var nodeCreated store.NodeCreated
		if err = json.Unmarshal([]byte(f.call("node", "create", name, "--capacity", "1", "--labels", "sdk", "--json")), &nodeCreated); err != nil {
			t.Fatal(err)
		}
		filename := filepath.Join(root, name+".yml")
		content := fmt.Sprintf("server: %s\nnode: %s\ntoken: '%s'\ndata_dir: %s-data\ncapacity: 2\nheartbeat_interval: 1s\nlease_duration: 10s\n", api.URL, name, nodeCreated.Token, name)
		if err = os.WriteFile(filename, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		live, stop := context.WithCancel(context.Background())
		cmd := agentcli.NewCommand()
		cmd.SetArgs([]string{"--config", filename, "serve"})
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		done := make(chan error, 1)
		go func() { done <- cmd.ExecuteContext(live) }()
		t.Cleanup(func() {
			stop()
			select {
			case err := <-done:
				if err != nil {
					t.Error("实际Agent退出", err)
				}
			case <-time.After(4 * time.Second):
				t.Error("实际Agent退出超时")
			}
		})
	}
	f.until("实际节点注册", func() bool {
		for i := 1; i <= nodes; i++ {
			v, err := st.GetNode(context.Background(), store.Actor{ID: "local-admin", Role: "admin"}, fmt.Sprintf("schedule-%d", i))
			if err != nil {
				t.Fatal(err)
			}
			if !v.Healthy || !v.SessionActive || v.EffectiveCapacity != 1 {
				return false
			}
		}
		return true
	})
	return f
}
func (f *schedulingEvidence) call(args ...string) string {
	f.t.Helper()
	out, err := executeRemote(f.t, append([]string{"--server-url", f.address}, args...)...)
	if err != nil {
		f.t.Fatalf("真实CLI %v: %v", args, err)
	}
	return out
}
func (f *schedulingEvidence) project(name, allowed, defaultNode string) {
	f.t.Helper()
	args := []string{"project", "init", name, "--repo", f.repository, "--nodes", allowed}
	if defaultNode != "" {
		args = append(args, "--default-node", defaultNode)
	}
	f.call(args...)
}
func (f *schedulingEvidence) trigger(project, build, key string) string {
	f.t.Helper()
	args := []string{"trigger", project, "--build", build, "--idempotency-key", key, "--json"}
	if build != "failed" {
		args = append(args, "--param", "gate="+filepath.Join(f.root, key+".gate"), "--param", "counter="+filepath.Join(f.root, key+".count"))
	}
	var batch server.BatchView
	if err := json.Unmarshal([]byte(f.call(args...)), &batch); err != nil || len(batch.Builds) != 1 {
		f.t.Fatal(batch, err)
	}
	return batch.Builds[0].ID
}
func (f *schedulingEvidence) until(description string, ready func() bool) {
	f.t.Helper()
	until := time.Now().Add(15 * time.Second)
	for time.Now().Before(until) {
		if ready() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	f.t.Fatal("真实状态等待超时", description)
}
func (f *schedulingEvidence) view(id string) store.BuildView {
	f.t.Helper()
	v, err := f.st.GetBuild(context.Background(), id)
	if err != nil {
		f.t.Fatal(err)
	}
	return v
}
func (f *schedulingEvidence) started(id string) store.BuildView {
	f.t.Helper()
	f.until("步骤实际开始", func() bool {
		v := f.view(id)
		if v.Status == "failed" || v.Status == "interrupted" {
			f.t.Fatal(v.Status, v.Reason)
		}
		return len(v.Steps) > 0 && v.Steps[0].Started
	})
	return f.view(id)
}
func (f *schedulingEvidence) queued(id, reason string) {
	f.t.Helper()
	f.until("未分配原因持久化", func() bool {
		v := f.view(id)
		if v.Status != "queued" || v.AttemptID != "" || v.NodeID != "" {
			f.t.Fatal("不应分配", v.Status, v.NodeName)
		}
		return v.Reason == reason
	})
}
func (f *schedulingEvidence) release(key string) {
	f.t.Helper()
	if err := os.WriteFile(filepath.Join(f.root, key+".gate"), []byte("release"), 0600); err != nil {
		f.t.Fatal(err)
	}
}
func (f *schedulingEvidence) finished(id string) {
	f.t.Helper()
	f.until("真实成功及停止确认", func() bool {
		v := f.view(id)
		if v.Status == "failed" || v.Status == "interrupted" {
			f.t.Fatal(v.Status, v.Reason)
		}
		return v.Status == "succeeded" && len(v.Steps) > 0 && v.Steps[0].StopConfirmed
	})
}
func requireActualSchedulingAndroid(t *testing.T) {
	t.Helper()
	report, err := node.Doctor(context.Background(), filepath.Join(t.TempDir(), "uninitialized"))
	if err != nil {
		t.Fatal(err)
	}
	passed := map[string]bool{}
	for _, tool := range report.Tools {
		passed[tool.Name] = tool.Status == "passed"
	}
	for _, name := range []string{"shell", "git", "java", "android_aapt2", "android_apksigner"} {
		if !passed[name] {
			t.Skip("真实Android工具未就绪：", name)
		}
	}
}

func TestActualCLISchedulingSerialParallelAndCapacity(t *testing.T) {
	requireActualSchedulingAndroid(t)
	f := newSchedulingEvidence(t, 2, 3)
	for _, name := range []string{"same", "other", "third"} {
		f.project(name, "schedule-1,schedule-2,schedule-3", "")
	}
	a := f.trigger("same", "flex", "serial-first")
	first := f.started(a)
	second := f.trigger("same", "flex", "serial-second")
	f.queued(second, "build_name_locked")
	b := f.trigger("other", "flex", "parallel")
	other := f.started(b)
	if first.NodeID == other.NodeID {
		t.Fatal("节点容量1允许两实际运行")
	}
	c := f.trigger("third", "flex", "global-limit")
	f.queued(c, "capacity_wait")
	status, err := f.st.Status(context.Background())
	if err != nil || status.Running != 2 {
		t.Fatal("全局容量不准确", status, err)
	}
	for i := 1; i <= 3; i++ {
		v, err := f.st.GetNode(context.Background(), store.Actor{ID: "local-admin", Role: "admin"}, fmt.Sprintf("schedule-%d", i))
		if err != nil || v.Running > 1 {
			t.Fatal("节点容量越界", v.Running, err)
		}
	}
	f.release("serial-first")
	f.finished(a)
	f.started(second)
	if f.view(b).Status != "running" {
		t.Fatal("不同项目未保留并行")
	}
	f.queued(c, "capacity_wait")
	f.release("serial-second")
	f.finished(second)
	f.started(c)
	f.release("parallel")
	f.release("global-limit")
	f.finished(b)
	f.finished(c)
	for _, key := range []string{"serial-first", "serial-second", "parallel", "global-limit"} {
		data, err := os.ReadFile(filepath.Join(f.root, key+".count"))
		if err != nil || string(data) != "once\n" {
			t.Fatal("重复实际执行", key, string(data), err)
		}
	}
}

func TestActualCLISchedulingDefaultAndIneligible(t *testing.T) {
	f := newSchedulingEvidence(t, 2, 2)
	f.project("default-only", "schedule-1,schedule-2", "schedule-1")
	held := f.trigger("default-only", "single", "default-held")
	if f.started(held).NodeName != "schedule-1" {
		t.Fatal("无runner分配到非default_node")
	}
	f.project("default-wait", "schedule-1,schedule-2", "schedule-1")
	waiting := f.trigger("default-wait", "single", "default-waiting")
	// schedule-2虽空闲且同样健康，不能替代指定default_node。
	f.queued(waiting, "capacity_wait")
	f.project("no-default", "schedule-1,schedule-2", "")
	if _, err := executeRemote(t, "--server-url", f.address, "trigger", "no-default", "--build", "single", "--idempotency-key", "no-default", "--param", "gate="+filepath.Join(f.root, "no-default.gate"), "--param", "counter="+filepath.Join(f.root, "no-default.count")); err == nil {
		t.Fatal("无runner且无default_node仍入队")
	}
	missing, err := f.st.ListBuilds(context.Background(), store.BuildFilter{Project: "no-default", Page: store.Page{Limit: 200}})
	if err != nil || len(missing) != 0 {
		t.Fatal("无default_node错误留下批次", len(missing), err)
	}
	f.project("no-eligible", "not-registered", "not-registered")
	ineligible := f.trigger("no-eligible", "single", "no-eligible")
	f.queued(ineligible, "node_unauthorized")
	f.project("wrong-label", "schedule-1,schedule-2", "")
	labels := f.trigger("wrong-label", "labels", "wrong-label")
	f.queued(labels, "capability_mismatch")
	f.release("default-held")
	f.finished(held)
	if f.started(waiting).NodeName != "schedule-1" {
		t.Fatal("default_node等待后被替换")
	}
	f.release("default-waiting")
	f.finished(waiting)
	for _, id := range []string{ineligible, labels} {
		v := f.view(id)
		if v.Status != "queued" || v.AttemptID != "" {
			t.Fatal("无eligible节点仍执行", v.Status, v.NodeName)
		}
		f.call("build", "cancel", id)
	}
}

func TestActualCLITwentyIdempotentTriggersExecuteOnce(t *testing.T) {
	f := newSchedulingEvidence(t, 2, 2)
	f.project("idempotent", "schedule-1,schedule-2", "schedule-1")
	const key = "twenty-same-request"
	f.release(key)
	type result struct {
		body string
		err  error
	}
	results := make(chan result, 20)
	triggerContext, stopTriggers := context.WithTimeout(context.Background(), 15*time.Second)
	defer stopTriggers()
	for i := 0; i < 20; i++ {
		go func() {
			cmd := NewCommand()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(io.Discard)
			cmd.SetArgs([]string{"--server-url", f.address, "trigger", "idempotent", "--build", "single", "--idempotency-key", key, "--param", "gate=" + filepath.Join(f.root, key+".gate"), "--param", "counter=" + filepath.Join(f.root, key+".count"), "--json"})
			err := cmd.ExecuteContext(triggerContext)
			results <- result{out.String(), err}
		}()
	}
	ids := map[string]bool{}
	batches := map[string]bool{}
	for i := 0; i < 20; i++ {
		select {
		case result := <-results:
			if result.err != nil {
				t.Fatal("真实并发触发", result.err)
			}
			var batch server.BatchView
			if err := json.Unmarshal([]byte(result.body), &batch); err != nil || len(batch.Builds) != 1 {
				t.Fatal(result.body, err)
			}
			ids[batch.Builds[0].ID] = true
			batches[batch.ID] = true
		case <-time.After(15 * time.Second):
			t.Fatal("并发触发超时")
		}
	}
	if len(ids) != 1 || len(batches) != 1 {
		t.Fatal("重复批次或build", len(ids), len(batches))
	}
	for id := range ids {
		f.finished(id)
	}
	rows, err := f.st.ListBuilds(context.Background(), store.BuildFilter{Project: "idempotent", Page: store.Page{Limit: 200}})
	if err != nil || len(rows) != 1 || rows[0].Number == nil || *rows[0].Number != 1 {
		t.Fatal("重复编号/构建", len(rows), err)
	}
	data, err := os.ReadFile(filepath.Join(f.root, key+".count"))
	if err != nil || string(data) != "once\n" {
		t.Fatal("20幂等请求实际执行多次", string(data), err)
	}
}

func TestActualCLIFailedRunRetainsOriginalEvidenceReason(t *testing.T) {
	f := newSchedulingEvidence(t, 1, 1)
	f.project("failed-evidence", "schedule-1", "schedule-1")
	id := f.trigger("failed-evidence", "failed", "failed-evidence")
	f.until("实际失败、failure和always完成", func() bool { return f.view(id).Status == "failed" })
	raw := f.call("build", "show", id, "--json")
	var view store.BuildView
	if err := json.Unmarshal([]byte(raw), &view); err != nil {
		t.Fatal(err)
	}
	if view.Reason != "exit" || view.PostPhase != "failure" || view.StopUnconfirmed || view.NodeName != "schedule-1" || view.SessionID == "" || view.AttemptID == "" || view.LeaseID == "" || view.LeaseEpoch != 1 || len(view.Steps) != 1 || len(view.Post) != 2 {
		t.Fatal("真实失败证据不完整", view)
	}
	original := view.Steps[0]
	if original.Phase != "ordinary" || original.Index != 1 || original.Status != "failed" || original.Reason != "exit" || original.ExitCode != 7 || !original.Started || !original.StopConfirmed || original.CleanupFailed || original.ElapsedNS <= 0 {
		t.Fatal("普通失败被覆盖或误报停止", original)
	}
	secondary, always := view.Post[0], view.Post[1]
	if secondary.Phase != "failure" || secondary.Status != "failed" || secondary.ExitCode != 9 || !secondary.Started || !secondary.StopConfirmed || secondary.CleanupFailed || always.Phase != "always" || always.Status != "succeeded" || !always.Started || !always.StopConfirmed {
		t.Fatal("实际post证据不正确", view.Post)
	}
	if view.InitialBudgetNS == nil || view.RemainingBudgetNS == nil || *view.RemainingBudgetNS <= 0 || *view.RemainingBudgetNS >= *view.InitialBudgetNS || view.RemainingPostBudgetNS <= 0 || view.RemainingPostBudgetNS >= view.PostBudgetNS {
		t.Fatal("实际ns预算未累计", view.RemainingBudgetNS, view.RemainingPostBudgetNS)
	}
	text := f.call("build", "show", id)
	for _, evidence := range []string{"post_phase", "failure[1]", "always[1]", "exit_code=7", "exit_code=9", "started=true", "stop_confirmed=true", "cleanup_failed=false", "elapsed_ns="} {
		if !strings.Contains(text, evidence) {
			t.Fatal("文本缺实际失败证据", evidence, text)
		}
	}
	for _, private := range []string{f.root, f.repository, "exit 7", "exit 9", os.Getenv("MYBUILDS_CLIENT_TOKEN"), "snapshot_json", "runtime_token", "storage_id"} {
		if private != "" && strings.Contains(raw+text, private) {
			t.Fatal("失败视图泄露私有值", private)
		}
	}
}
