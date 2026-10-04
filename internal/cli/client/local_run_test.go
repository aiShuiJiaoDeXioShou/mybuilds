package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func localExecute(t *testing.T, ctx context.Context, args ...string) (string, string, error) {
	t.Helper()
	cmd := NewCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(ctx)
	return stdout.String(), stderr.String(), err
}

func requireLocalShell(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("本地执行仅支持 macOS/Linux")
	}
	// 本地运行的结果跟随本次测试临时目录清理，不留宿主临时产物。
	t.Setenv("TMPDIR", t.TempDir())
}

func TestLocalRunJSONLogsAndCurrentWorkspace(t *testing.T) {
	requireLocalShell(t)
	t.Chdir(t.TempDir())
	if err := os.Mkdir("configs", 0700); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, "user-file", "用户尚未提交的修改")
	writeConfig(t, "configs/run.yml", `version: 1
params:
  message: hello
  release: "no"
env:
  MESSAGE: "{{message}}"
steps:
  - kind: run
    name: first
    run: printf '%s' "$MESSAGE" > first-marker; printf 'hello-log\n'; printf 'error-log\n' >&2
  - kind: run
    name: verify
    run: test -f first-marker; printf second > second-marker
  - kind: run
    name: release
    when:
      params: {release: "yes"}
    run: printf release > release-marker
post:
  success:
    - kind: run
      name: success
      run: printf success > success-marker
  always:
    - kind: run
      name: cleanup
      run: printf cleanup > cleanup-marker
`)
	stdout, stderr, err := localExecute(t, context.Background(), "run", "--file", "configs/run.yml", "--param", "message=hi=there,friend")
	if err != nil || !json.Valid([]byte(stdout)) || !strings.Contains(stdout, `"status": "succeeded"`) || !strings.Contains(stdout, `"status": "skipped"`) {
		t.Fatalf("本地执行：%q，%q，%v", stdout, stderr, err)
	}
	if strings.Contains(stdout, "hello-log") || !strings.Contains(stderr, "[stream=stdout] hello-log") || !strings.Contains(stderr, "[stream=stderr] error-log") {
		t.Fatalf("JSON/日志未分流：%q，%q", stdout, stderr)
	}
	data, _ := os.ReadFile("first-marker")
	if string(data) != "hi=there,friend" {
		t.Fatalf("映射参数失真：%q", data)
	}
	for _, name := range []string{"second-marker", "success-marker", "cleanup-marker"} {
		if _, err := os.Stat(name); err != nil {
			t.Fatalf("工作目录或收尾错误：%s，%v", name, err)
		}
	}
	if _, err := os.Stat("release-marker"); !os.IsNotExist(err) {
		t.Fatal("参数条件跳过步骤却执行了")
	}
	data, _ = os.ReadFile("user-file")
	if string(data) != "用户尚未提交的修改" {
		t.Fatal("本地执行修改了用户已有文件")
	}
}

func TestLocalRunFailureKeepsResultAndContinuesOtherBuild(t *testing.T) {
	requireLocalShell(t)
	t.Chdir(t.TempDir())
	writeConfig(t, "mybuilds.yml", `version: 1
builds:
  failed:
    steps:
      - kind: run
        name: fail
        run: exit 7
      - kind: run
        name: blocked
        run: ': > blocked-marker'
    post:
      failure:
        - kind: run
          name: diagnostics
          run: ': > diagnostics-marker'
      always:
        - kind: run
          name: cleanup
          run: ': > cleanup-marker'
  next:
    steps:
      - kind: run
        name: later
        run: ': > next-marker'
`)
	stdout, _, err := localExecute(t, context.Background(), "run", "--build", "failed,next")
	if err == nil || !json.Valid([]byte(stdout)) || !strings.Contains(stdout, `"exit_code": 7`) || !strings.Contains(stdout, `"status": "failed"`) || !strings.Contains(stdout, `"status": "succeeded"`) {
		t.Fatalf("执行失败未保留完整结果：%q，%v", stdout, err)
	}
	for _, name := range []string{"diagnostics-marker", "cleanup-marker", "next-marker"} {
		if _, err := os.Stat(name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat("blocked-marker"); !os.IsNotExist(err) {
		t.Fatal("失败后继续了同一 build")
	}
}

func TestLocalRunPrechecksAllBuildsAndStepScope(t *testing.T) {
	requireLocalShell(t)
	t.Chdir(t.TempDir())
	writeConfig(t, "mybuilds.yml", `version: 1
builds:
  first:
    steps:
      - kind: run
        name: package
        run: ': > first-marker'
  publish:
    steps:
      - kind: run
        name: package
        run: ': > publish-marker'
      - kind: upload
        target: google_play
        file: output/*.aab
        track: internal
        credentials: ${ABSENT_RELEASE_SECRET}
`)
	for _, args := range [][]string{{"--all"}, {"--build", "publish", "--step", "package"}} {
		stdout, _, err := localExecute(t, context.Background(), append([]string{"run"}, args...)...)
		if err == nil || stdout != "" {
			t.Fatalf("全批预检查未阻止执行：%q，%v", stdout, err)
		}
	}
	for _, name := range []string{"first-marker", "publish-marker"} {
		if _, err := os.Stat(name); !os.IsNotExist(err) {
			t.Fatal("预检查失败后产生了脚本副作用")
		}
	}
}

func TestLocalRunStepAndDryRun(t *testing.T) {
	requireLocalShell(t)
	t.Chdir(t.TempDir())
	writeConfig(t, "mybuilds.yml", `version: 1
steps:
  - kind: run
    name: first
    run: ': > first-marker'
  - kind: run
    name: second
    run: ': > second-marker'
`)
	if out, logs, err := localExecute(t, context.Background(), "run", "--dry-run"); err != nil || !json.Valid([]byte(out)) || logs != "" {
		t.Fatalf("dry-run 回归：%q，%q，%v", out, logs, err)
	}
	if entries, _ := os.ReadDir("."); len(entries) != 1 {
		t.Fatal("dry-run 产生了副作用")
	}
	if out, _, err := localExecute(t, context.Background(), "run", "--step", "second"); err != nil || !json.Valid([]byte(out)) {
		t.Fatal(err)
	}
	if _, err := os.Stat("first-marker"); !os.IsNotExist(err) {
		t.Fatal("--step 自动执行了前序步骤")
	}
	if _, err := os.Stat("second-marker"); err != nil {
		t.Fatal(err)
	}
}

func TestLocalRunCancellationAndSecrets(t *testing.T) {
	requireLocalShell(t)
	t.Chdir(t.TempDir())
	t.Setenv("LOCAL_RUN_TEST_SECRET", "private-run-secret")
	writeConfig(t, "mybuilds.yml", `version: 1
env:
  EXPLICIT_SECRET: ${LOCAL_RUN_TEST_SECRET}
steps:
  - kind: run
    name: active
    run: ': > active-marker; printf "%s\n" "$EXPLICIT_SECRET"; sleep 20'
post:
  always:
    - kind: run
      name: cleanup
      run: ': > cleanup-marker'
`)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := os.Stat("active-marker"); err == nil {
					cancel()
					return
				}
			}
		}
	}()
	stdout, stderr, err := localExecute(t, ctx, "run")
	if err == nil || !json.Valid([]byte(stdout)) || !strings.Contains(stdout, `"status": "cancelled"`) {
		t.Fatalf("取消未保留结果：%q，%q，%v", stdout, stderr, err)
	}
	if strings.Contains(stdout+stderr+err.Error(), "private-run-secret") {
		t.Fatal("执行结果或日志泄露密钥")
	}
	if _, err := os.Stat("cleanup-marker"); err != nil {
		t.Fatalf("取消未执行 always：%v", err)
	}
}

func TestLocalRunExample(t *testing.T) {
	requireLocalShell(t)
	filename, err := filepath.Abs("../../../examples/local-run.yml")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	stdout, _, err := localExecute(t, context.Background(), "run", "--file", filename)
	if err != nil || !json.Valid([]byte(stdout)) || !strings.Contains(stdout, `"status": "succeeded"`) {
		t.Fatalf("本地示例未成功执行：%q，%v", stdout, err)
	}
}

type failedResultWriter struct{}

func (failedResultWriter) Write([]byte) (int, error) {
	return 0, errors.New("private-result-writer-error")
}

func TestLocalRunResultOutputErrorIsSafe(t *testing.T) {
	requireLocalShell(t)
	t.Chdir(t.TempDir())
	writeConfig(t, "mybuilds.yml", "version: 1\nsteps:\n  - kind: run\n    run: 'true'\n")
	for _, args := range [][]string{{"run", "--dry-run"}, {"run"}} {
		cmd := NewCommand()
		var stderr bytes.Buffer
		cmd.SetOut(failedResultWriter{})
		cmd.SetErr(&stderr)
		cmd.SetArgs(args)
		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "写入") || strings.Contains(err.Error()+stderr.String(), "private-result-writer-error") {
			t.Fatalf("结果写入错误缺失或泄露底层错误：%v，%q", err, stderr.String())
		}
	}
}
