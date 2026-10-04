package client

import (
	"bytes"
	"context"
	"encoding/json"
	"mybuilds/internal/store"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTriggerNamedParameters(t *testing.T) {
	shared, named, err := triggerParameters([]string{"version=1.0", "android:channel=prod", "channel=dev", "android:token=x=y"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if shared["channel"] != "dev" || named["android"]["channel"] != "prod" || named["android"]["token"] != "x=y" {
		t.Fatal("命名参数或首等号解析错误")
	}
	for _, input := range [][]string{{"x=SECRET", "x=SECRET"}, {"a:x=SECRET", "a:x=SECRET"}, {"=SECRET"}, {"a:=SECRET"}, {"a:b:c=SECRET"}, {"x"}} {
		_, _, err := triggerParameters(input, "", "")
		if err == nil || strings.Contains(err.Error(), "SECRET") {
			t.Fatal("非法参数被接受或泄露", err)
		}
	}
	if _, _, err := triggerParameters([]string{"version=SECRET"}, "2.0", ""); err == nil {
		t.Fatal("快捷参数重复未失败")
	}
}

func TestActualTriggerFixedSnapshotAndQuery(t *testing.T) {
	db, address := realRemoteAPI(t)
	repository := t.TempDir()
	marker := filepath.Join(repository, "must-not-execute")
	git := func(args ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, "git", append([]string{"-c", "core.hooksPath=" + t.TempDir(), "-c", "user.name=Self", "-c", "user.email=self@example.test"}, args...)...)
		command.Dir = repository
		command.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("自有Git失败: %v", err)
		}
		return strings.TrimSpace(string(output))
	}
	git("init", "--initial-branch=main")
	pipeline := `version: 1
builds:
  android:
    runner: {platform: android}
    params:
      version: {required: true}
      channel: {default: dev}
    env: {SIGNING_TOKEN: '${CONTROL_ENV_SECRET}'}
    timeout: 1m
    when: {params: {channel: prod}, changes: ['src/**']}
    steps:
      - kind: run
        run: 'printf CONTROL_SCRIPT_SECRET > ` + marker + `'
      - kind: artifact
        paths: ['{{build.number}}/*.apk']
    post:
      always:
        - kind: run
          run: printf CONTROL_POST_SECRET
  ios:
    runner: {platform: ios}
    params: {version: {required: true}}
    when: {branches: ['release/*']}
    steps:
      - kind: run
        run: printf CONTROL_IOS_SECRET
`
	if err := os.WriteFile(filepath.Join(repository, "mybuilds.yml"), []byte(pipeline), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "mybuilds.yml")
	git("commit", "-m", "fixture")
	firstSHA := git("rev-parse", "HEAD")
	call := func(args ...string) string {
		t.Helper()
		out, err := executeRemote(t, append([]string{"--server-url", address}, args...)...)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out, "CONTROL_") {
			t.Fatal("公开输出泄露值或脚本", out)
		}
		return out
	}
	call("group", "create", "apps")
	call("project", "init", "mobile", "--repo", repository, "--nodes", "android-1,mac-1", "--group", "apps")
	out := call("trigger", "mobile", "--all", "--param", "version=CONTROL_PARAM_SECRET", "--param", "android:channel=prod", "--idempotency-key", "cli-request-1", "--json")
	var first struct {
		RequestKey string            `json:"request_key"`
		BatchID    string            `json:"batch_id"`
		SHA        string            `json:"sha"`
		Builds     []store.BuildView `json:"builds"`
	}
	if err := json.Unmarshal([]byte(out), &first); err != nil {
		t.Fatal(err)
	}
	if first.SHA != firstSHA || first.RequestKey != "cli-request-1" || len(first.Builds) != 2 || first.Builds[0].Status != "queued" || first.Builds[0].Number == nil || *first.Builds[0].Number != 1 || first.Builds[1].Status != "skipped" || first.Builds[1].Number != nil {
		t.Fatal("固定SHA/条件/编号错误", out)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("控制端执行了仓库shell")
	}
	if err := os.WriteFile(filepath.Join(repository, "advanced"), []byte("next"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "advanced")
	git("commit", "-m", "advance")
	replay := call("trigger", "mobile", "--all", "--param", "version=CONTROL_PARAM_SECRET", "--param", "android:channel=prod", "--idempotency-key", "cli-request-1", "--json")
	if replay != out {
		t.Fatal("幂等重放改变固定结果")
	}
	out = call("build", "ls", "--project", "mobile", "--group", "apps", "--json")
	if !strings.Contains(out, first.Builds[0].ID) {
		t.Fatal("缺少排队证据")
	}
	out = call("build", "show", first.Builds[0].ID, "--json")
	if !strings.Contains(out, "post_budget_ns") || !strings.Contains(out, "parameter_keys") {
		t.Fatal("详情缺少预算/参数键")
	}
	out = call("build", "show", first.Builds[0].ID)
	if !strings.Contains(out, "always[1]") || !strings.Contains(out, "initial_budget_ns") {
		t.Fatal("默认详情缺少进度/预算")
	}
	if _, err := executeRemote(t, "--server-url", address, "trigger", "mobile", "--build", "android", "--param", "version=CONTROL_PARAM_SECRET", "--param", "ios:version=CONTROL_PARAM_SECRET"); err == nil {
		t.Fatal("未选scope被接受")
	}
	trigger, err := db.CreateToken(context.Background(), store.Actor{ID: "local-admin", Role: "admin"}, "trigger")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("MYBUILDS_CLIENT_TOKEN", trigger.Token)
	call("status", "--json")
	if _, err := executeRemote(t, "--server-url", address, "build", "ls"); err == nil {
		t.Fatal("trigger身份读取构建证据")
	}
	approver, err := db.CreateToken(context.Background(), store.Actor{ID: "local-admin", Role: "admin"}, "approver")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("MYBUILDS_CLIENT_TOKEN", approver.Token)
	call("build", "show", first.Builds[0].ID, "--json")
	if _, err := executeRemote(t, "--server-url", address, "trigger", "mobile", "--all", "--param", "version=x"); err == nil {
		t.Fatal("approver触发被接受")
	}
	if err := db.RevokeToken(context.Background(), store.Actor{ID: "local-admin", Role: "admin"}, approver.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := executeRemote(t, "--server-url", address, "status"); err == nil {
		t.Fatal("撤销token继续有效")
	}
}

func TestTriggerKeyAndTransportPrechecks(t *testing.T) {
	t.Setenv("MYBUILDS_CLIENT_TOKEN", strings.Repeat("token", 8))
	requests := 0
	observedKey := ""
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		observedKey = r.Header.Get("Idempotency-Key")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"batch_id":"batch","sha":"fixed","builds":[]}`))
	}))
	defer api.Close()
	cmd := NewCommand()
	var out, diagnostics bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&diagnostics)
	cmd.SetArgs([]string{"--server-url", api.URL, "trigger", "app", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var result struct {
		RequestKey string `json:"request_key"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || len(observedKey) != 64 || result.RequestKey != observedKey || !strings.Contains(diagnostics.String(), observedKey) || requests != 1 {
		t.Fatal("生成/交付一次key错误", err)
	}
	for _, args := range [][]string{{"trigger", "app", "--all", "--build", "android"}, {"trigger", "app", "--idempotency-key", ""}, {"trigger", "app", "--param", "version=SECRET", "--version", ""}, {"trigger", "app", "--param", "bad:x=1", "--param", "bad:x=2"}} {
		if _, err := executeRemote(t, append([]string{"--server-url", api.URL}, args...)...); err == nil {
			t.Fatal("CLI预检查未拒绝")
		}
	}
	if requests != 1 {
		t.Fatal("非法输入发起了网络请求")
	}
}
