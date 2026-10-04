package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"mybuilds/internal/config"
)

func localDocument(t *testing.T, source string) *config.Document {
	t.Helper()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("本地执行只支持 macOS/Linux")
	}
	return document(t, source)
}

func requireFile(t *testing.T, root, name, want string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, name))
	if err != nil || string(data) != want {
		t.Fatalf("%s: 得到 %q，错误 %v，期望 %q", name, data, err, want)
	}
}

func requireAbsent(t *testing.T, root, name string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
		t.Fatalf("不应创建 %s: %v", name, err)
	}
}

func TestRunSequentialEnvironmentAndLiteralValues(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "sub"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOST_TOKEN", "host-secret")
	t.Setenv("BASH_ENV", filepath.Join(root, "startup"))
	if err := os.WriteFile(filepath.Join(root, "startup"), []byte("touch startup-leak"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DECLARED_SECRET", "redact-me")
	d := localDocument(t, `version: 1
params: {value: default}
env: {MAPPED: "{{value}}", OVERRIDE: build}
steps:
  - kind: run
    name: first
    env: {SECRET: "${DECLARED_SECRET}", OVERRIDE: step}
    run: |
      printf '%s' "$MAPPED" > literal
      test "$OVERRIDE" = step
      test -z "${HOST_TOKEN-}"
      test -z "${value-}"
      test "$MYBUILDS_BUILD_NAME" = default
      test "$MYBUILDS_STEP_NAME" = first
      printf '%s' "$SECRET"
      export LEAK=wrong
      cd sub
      printf shared > shared
  - kind: run
    name: second
    shell: bash
    run: |
      test -z "${LEAK-}"
      test -z "${SECRET-}"
      test "$OVERRIDE" = build
      test -f sub/shared
      printf done > done
`)
	var output bytes.Buffer
	literal := "$(touch injected)\n${HOST_TOKEN} {{unknown}} = value"
	result, err := Run(context.Background(), d, RunOptions{Workspace: root, Output: &output, PreviewOptions: PreviewOptions{Params: map[string]string{"value": literal}}})
	if err != nil || result.Builds[0].Status != "succeeded" {
		t.Fatalf("执行失败: %+v, %v", result, err)
	}
	requireFile(t, root, "literal", literal)
	requireFile(t, root, "done", "done")
	requireAbsent(t, root, "injected")
	requireAbsent(t, root, "startup-leak")
	if strings.Contains(output.String(), "redact-me") {
		t.Fatal("声明秘密未脱敏")
	}
}

func TestRunEnvironmentReferencesAreNotRecursive(t *testing.T) {
	root := t.TempDir()
	t.Setenv("INSERTED_SECRET", "${EXTRA_SECRET} {{unknown}}")
	t.Setenv("EXTRA_SECRET", "must-not-be-read")
	d := localDocument(t, `version: 1
params: {value: "${EXTRA_SECRET} {{unknown}}"}
env: {PARAM: "{{value}}", SECRET: "${INSERTED_SECRET}"}
steps:
  - kind: run
    run: |
      test -z "${EXTRA_SECRET-}"
      printf '%s' "$PARAM" > param
      printf '%s' "$SECRET" > secret
`)
	result, err := Run(context.Background(), d, RunOptions{Workspace: root})
	if err != nil || result.Builds[0].Status != "succeeded" {
		t.Fatalf("插入值被重新解释: %+v %v", result, err)
	}
	requireFile(t, root, "param", "${EXTRA_SECRET} {{unknown}}")
	requireFile(t, root, "secret", "${EXTRA_SECRET} {{unknown}}")
}

func TestRunBatchPrecheckHasNoScriptSideEffects(t *testing.T) {
	for _, invalid := range []string{
		"steps: [{kind: artifact, paths: [out/*.zip]}]",
		"steps: [{kind: approval}]",
		"steps: [{kind: upload, target: custom, argv: [true], result_file: result.json}]",
		"steps: [{kind: run, run: true, working_dir: missing}]",
		"env: {SECRET: '${MISSING_RUN_SECRET}'}\n    steps: [{kind: run, run: true}]",
		"env: {NUMBER: '{{build.number}}'}\n    steps: [{kind: run, run: true}]",
		"when: {branches: [main]}\n    steps: [{kind: run, run: true}]",
		"reports: {junit: {paths: [reports/*.xml]}}\n    steps: [{kind: run, run: true}]",
	} {
		t.Run(strings.Split(invalid, "\n")[0], func(t *testing.T) {
			root := t.TempDir()
			// YAML 中的 true 作为脚本字符串，保留严格标量校验。
			invalid = strings.ReplaceAll(invalid, "run: true", "run: 'true'")
			invalid = strings.ReplaceAll(invalid, "argv: [true]", "argv: ['true']")
			d := localDocument(t, "version: 1\nbuilds:\n  first:\n    steps: [{kind: run, run: 'touch marker'}]\n  second:\n    "+invalid+"\n")
			result, err := Run(context.Background(), d, RunOptions{Workspace: root, PreviewOptions: PreviewOptions{All: true}})
			if err == nil || result != nil {
				t.Fatalf("预检查应拒绝: %+v, %v", result, err)
			}
			requireAbsent(t, root, "marker")
			if strings.Contains(err.Error(), "MISSING_RUN_SECRET") {
				t.Fatal("诊断泄露引用名")
			}
		})
	}
}

func TestRunSkippedCapabilitiesAndFullStepPrecheck(t *testing.T) {
	root := t.TempDir()
	d := localDocument(t, `version: 1
params: {mode: debug}
steps:
  - {kind: run, name: selected, run: "touch marker"}
  - {kind: artifact, paths: [out/*.zip], when: {params: {mode: release}}}
  - {kind: upload, target: custom, argv: ["true"], result_file: result.json, when: {params: {mode: release}}}
`)
	result, err := Run(context.Background(), d, RunOptions{Workspace: root})
	if err != nil || result.Builds[0].Status != "succeeded" || result.Builds[0].Steps[1].Status != "skipped" {
		t.Fatalf("未生效能力应可跳过: %+v %v", result, err)
	}
	if err := os.Remove(filepath.Join(root, "marker")); err != nil {
		t.Fatal(err)
	}
	_, err = Run(context.Background(), d, RunOptions{Workspace: root, PreviewOptions: PreviewOptions{Step: "selected", Params: map[string]string{"mode": "release"}}})
	if err == nil {
		t.Fatal("--step 绕过生效发布能力预检查")
	}
	requireAbsent(t, root, "marker")
}

func TestRunFailureStopsBuildAndContinuesBatch(t *testing.T) {
	root := t.TempDir()
	d := localDocument(t, `version: 1
builds:
  first:
    steps:
      - {kind: run, name: fail, run: "exit 7"}
      - {kind: run, name: after, run: "touch forbidden"}
    post:
      success: [{kind: run, run: "touch wrong-success"}]
      failure:
        - {kind: run, name: fail-post, run: "exit 8"}
        - {kind: run, name: continue-post, run: "touch failure"}
      always: [{kind: run, run: "touch always"}]
  second:
    steps: [{kind: run, run: "touch second"}]
`)
	result, err := Run(context.Background(), d, RunOptions{Workspace: root, PreviewOptions: PreviewOptions{Names: []string{"first", "second"}}})
	if err == nil || result.Builds[0].Reason != "exit" || result.Builds[0].Steps[0].ExitCode != 7 || result.Builds[1].Status != "succeeded" {
		t.Fatalf("失败结果错误: %+v %v", result, err)
	}
	if len(result.Builds[0].Post) != 3 {
		t.Fatalf("post 未继续: %+v", result.Builds[0])
	}
	for _, name := range []string{"failure", "always", "second"} {
		requireFile(t, root, name, "")
	}
	for _, name := range []string{"forbidden", "wrong-success"} {
		requireAbsent(t, root, name)
	}
}

func TestRunPostFailureDoesNotEnterFailurePhase(t *testing.T) {
	root := t.TempDir()
	d := localDocument(t, `version: 1
steps: [{kind: run, run: "true"}]
post:
  success:
    - {kind: run, run: "exit 3"}
    - {kind: run, run: "touch remaining"}
  failure: [{kind: run, run: "touch wrong-failure"}]
  always: [{kind: run, run: "touch always"}]
`)
	result, err := Run(context.Background(), d, RunOptions{Workspace: root})
	if err == nil || result.Builds[0].Status != "failed" || result.Builds[0].Reason != "post_error" {
		t.Fatalf("成功转失败记录错误: %+v %v", result, err)
	}
	requireAbsent(t, root, "wrong-failure")
	requireFile(t, root, "remaining", "")
	requireFile(t, root, "always", "")
}

func TestRunAllSkippedDoesNotRunPostOrRequireBranch(t *testing.T) {
	root := t.TempDir()
	d := localDocument(t, `version: 1
params: {mode: debug}
steps:
  - {kind: run, run: "touch forbidden", when: {params: {mode: release}, branches: [main]}}
post:
  always: [{kind: run, run: "touch forbidden-post"}]
`)
	result, err := Run(context.Background(), d, RunOptions{Workspace: root})
	if err != nil || result.Builds[0].Status != "skipped" || len(result.Builds[0].Post) != 0 {
		t.Fatalf("全部跳过结果错误: %+v %v", result, err)
	}
	requireAbsent(t, root, "forbidden")
	requireAbsent(t, root, "forbidden-post")
}

func TestRunCumulativeBudgetAndIndependentPost(t *testing.T) {
	root := t.TempDir()
	d := localDocument(t, `version: 1
timeout: 150ms
steps:
  - {kind: run, run: "sleep 0.09"}
  - {kind: run, run: "sleep 0.09; touch forbidden"}
post:
  timeout: 1s
  failure: [{kind: run, run: "sleep 0.08; touch post"}]
`)
	result, err := Run(context.Background(), d, RunOptions{Workspace: root})
	if err == nil || result.Builds[0].Reason != "timeout" {
		t.Fatalf("累计预算错误: %+v %v", result, err)
	}
	requireAbsent(t, root, "forbidden")
	requireFile(t, root, "post", "")
}

func TestRunPostBudgetMarksUnstartedItems(t *testing.T) {
	root := t.TempDir()
	d := localDocument(t, `version: 1
steps: [{kind: run, run: "true"}]
post:
  timeout: 30ms
  success: [{kind: run, run: "sleep 1"}]
  always: [{kind: run, run: "touch forbidden"}]
`)
	result, err := Run(context.Background(), d, RunOptions{Workspace: root})
	if err == nil || len(result.Builds[0].Post) != 2 || result.Builds[0].Post[1].Reason != "budget_exhausted" {
		t.Fatalf("收尾预算结果错误: %+v %v", result, err)
	}
	requireAbsent(t, root, "forbidden")
}

func TestRunCancellationOnlyAlwaysAndStopsBatch(t *testing.T) {
	root := t.TempDir()
	d := localDocument(t, `version: 1
builds:
  first:
    steps: [{kind: run, run: "touch started; sleep 10"}]
    post:
      failure: [{kind: run, run: "touch forbidden-failure"}]
      always: [{kind: run, run: "touch always"}]
  second:
    steps: [{kind: run, run: "touch forbidden-second"}]
`)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		deadline := time.After(3 * time.Second)
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-deadline:
				cancel()
				return
			case <-ticker.C:
				if _, err := os.Stat(filepath.Join(root, "started")); err == nil {
					cancel()
					return
				}
			}
		}
	}()
	result, err := Run(ctx, d, RunOptions{Workspace: root, PreviewOptions: PreviewOptions{Names: []string{"first", "second"}}})
	if err == nil || result.Builds[0].Status != "cancelled" || result.Builds[1].Status != "cancelled" {
		t.Fatalf("取消结果错误: %+v %v", result, err)
	}
	requireFile(t, root, "always", "")
	requireAbsent(t, root, "forbidden-failure")
	requireAbsent(t, root, "forbidden-second")
}

func TestRunRechecksWorkingDirectorySymlinks(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "sub"), 0700); err != nil {
		t.Fatal(err)
	}
	d := localDocument(t, fmt.Sprintf(`version: 1
steps:
  - {kind: run, run: "rmdir sub; ln -s '%s' sub"}
  - {kind: run, working_dir: sub, run: "touch forbidden"}
`, outside))
	result, err := Run(context.Background(), d, RunOptions{Workspace: root})
	if err == nil || result.Builds[0].Steps[1].Reason != "directory_error" {
		t.Fatalf("目录启动前未复查: %+v %v", result, err)
	}
	requireAbsent(t, outside, "forbidden")
}

func TestRunReadsOnlyNecessaryGitFacts(t *testing.T) {
	root := t.TempDir()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("需要 Git")
	}
	for _, args := range [][]string{{"init", "-b", "main"}, {"-c", "user.name=test", "-c", "user.email=test@example.test", "commit", "--allow-empty", "-m", "baseline"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if data, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("初始化测试仓库: %v %s", err, data)
		}
	}
	d := localDocument(t, `version: 1
when: {branches: [main]}
env: {SHA: "{{git.sha}}"}
steps:
  - kind: run
    run: |
      test "$MYBUILDS_GIT_BRANCH" = main
      test "$SHA" = "$MYBUILDS_GIT_SHA"
      printf '%s' "$SHA" > sha
`)
	if _, err := Run(context.Background(), d, RunOptions{Workspace: root}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "sha"))
	if err != nil || len(data) != 40 {
		t.Fatalf("Git事实错误: %q %v", data, err)
	}
}

func TestRunExhaustedBudgetAndCancelledPostEntry(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("需要本地 shell")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if limit := durationLimit(config.Step{}, 0, true); limit >= 0 {
		t.Fatal("耗尽预算必须拒绝启动")
	}
	step := preparedStep{step: config.Step{Kind: "run", Name: "edge"}, relativeDir: ".", command: shellCommand{Path: "/bin/sh", Args: []string{"-c", "touch forbidden"}}}
	s := executeStep(context.Background(), root, "default", step, -1, newRunLogger(nil, nil))
	if s.started || s.Status != "failed" || s.Reason != "timeout" {
		t.Fatalf("耗尽预算仍启动: %+v", s)
	}
	requireAbsent(t, root, "forbidden")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s = executeStep(ctx, root, "default", step, 0, newRunLogger(nil, nil))
	if s.started || s.Status != "cancelled" {
		t.Fatalf("启动前取消应保持未启动: %+v", s)
	}
	for _, status := range []string{"succeeded", "failed"} {
		b := BuildRun{Status: status, Reason: "exit"}
		executePost(ctx, root, preparedBuild{postTimeout: time.Second}, &b, newRunLogger(nil, nil))
		if status == "succeeded" && b.Status != "cancelled" {
			t.Fatal("无 always 时也应保留取消")
		}
		if status == "failed" && (b.Status != "failed" || b.Reason != "exit") {
			t.Fatal("取消不能覆盖原失败")
		}
	}
}

func TestRunStepSelectionAndInputUnchanged(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("需要本地 shell")
	}
	root := t.TempDir()
	d := &config.Document{Version: 1, Builds: map[string]*config.Build{"default": {
		Steps: []config.Step{{Kind: "run", Name: "first", Run: "touch first"}, {Kind: "run", Name: "selected", Run: "touch selected"}},
		Post:  &config.Post{Always: []config.Step{{Kind: "run", Run: "touch cleanup"}}},
	}}}
	before, _ := json.Marshal(d)
	result, err := Run(context.Background(), d, RunOptions{Workspace: root, PreviewOptions: PreviewOptions{Step: "selected"}})
	if err != nil || result.Builds[0].Status != "succeeded" || len(result.Builds[0].Steps) != 1 {
		t.Fatalf("独立步骤执行错误: %+v %v", result, err)
	}
	requireAbsent(t, root, "first")
	requireFile(t, root, "selected", "")
	requireFile(t, root, "cleanup", "")
	after, _ := json.Marshal(d)
	if string(before) != string(after) {
		t.Fatal("执行改写了配置")
	}
}

func TestRunOverriddenSecretAndUnavailableShell(t *testing.T) {
	root := t.TempDir()
	d := localDocument(t, `version: 1
env: {SECRET: "${UNUSED_MISSING_SECRET}"}
steps: [{kind: run, env: {SECRET: safe}, run: "test \"$SECRET\" = safe"}]
`)
	if _, err := Run(context.Background(), d, RunOptions{Workspace: root}); err != nil {
		t.Fatalf("被覆盖引用不应解析: %v", err)
	}
	bin := t.TempDir()
	if err := os.Symlink("/bin/sh", filepath.Join(bin, "sh")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	d = localDocument(t, `version: 1
builds:
  first:
    steps: [{kind: run, run: "echo marker > marker"}]
  second:
    steps: [{kind: run, shell: bash, run: "true"}]
`)
	result, err := Run(context.Background(), d, RunOptions{Workspace: root, PreviewOptions: PreviewOptions{Names: []string{"first", "second"}}})
	if err == nil || result != nil {
		t.Fatalf("解释器预检查应整批失败: %+v %v", result, err)
	}
	requireAbsent(t, root, "marker")
}

func TestRunCancellationDuringPostRunsOnlyRemainingAlways(t *testing.T) {
	root := t.TempDir()
	d := localDocument(t, `version: 1
steps: [{kind: run, run: "true"}]
post:
  timeout: 2s
  success:
    - {kind: run, run: "exit 3"}
    - {kind: run, run: "touch post-started; sleep 10"}
    - {kind: run, run: "touch forbidden-success"}
  failure: [{kind: run, run: "touch forbidden-failure"}]
  always: [{kind: run, run: "touch cleanup"}]
`)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		deadline := time.After(3 * time.Second)
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-deadline:
				cancel()
				return
			case <-ticker.C:
				if _, err := os.Stat(filepath.Join(root, "post-started")); err == nil {
					cancel()
					return
				}
			}
		}
	}()
	result, err := Run(ctx, d, RunOptions{Workspace: root})
	if err == nil || result.Builds[0].Status != "cancelled" {
		t.Fatalf("post 取消结果错误: %+v %v", result, err)
	}
	requireFile(t, root, "cleanup", "")
	requireAbsent(t, root, "forbidden-success")
	requireAbsent(t, root, "forbidden-failure")
}

func TestRunNotificationPrecheckAndExplicitDisable(t *testing.T) {
	root := t.TempDir()
	d := localDocument(t, `version: 1
builds:
  first:
    steps: [{kind: run, run: "touch first"}]
  second:
    notifications:
      webhooks: [{type: feishu, url: "${UNSET_NOTIFY_SECRET}"}]
    steps: [{kind: run, run: "touch second"}]
`)
	options := RunOptions{Workspace: root, PreviewOptions: PreviewOptions{Names: []string{"first", "second"}}}
	result, err := Run(context.Background(), d, options)
	if err == nil || result != nil || !strings.Contains(err.Error(), "notifications") {
		t.Fatalf("通知能力应整批拒绝: %+v %v", result, err)
	}
	requireAbsent(t, root, "first")
	if strings.Contains(err.Error(), "UNSET_NOTIFY_SECRET") {
		t.Fatal("通知错误泄露引用")
	}
	disabled := false
	d.Builds["second"].Notifications.Enabled = &disabled
	if _, err := Run(context.Background(), d, options); err != nil {
		t.Fatalf("关闭通知不应解析秘密: %v", err)
	}
	requireFile(t, root, "first", "")
	requireFile(t, root, "second", "")
	d = localDocument(t, `version: 1
notifications:
  webhooks: [{type: generic, url: "${UNSET_NOTIFY_SECRET}"}]
builds:
  first:
    notifications: {enabled: false}
    steps: [{kind: run, run: "true"}]
  second:
    params: {mode: debug}
    when: {params: {mode: release}}
    steps: [{kind: run, run: "true"}]
`)
	result, err = Run(context.Background(), d, options)
	if err != nil || result.Builds[1].Status != "skipped" {
		t.Fatalf("build 禁用覆盖公共通知，跳过build无需通知: %+v %v", result, err)
	}
}
