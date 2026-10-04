package pipeline

import (
	"bytes"
	"context"
	"crypto/sha256"
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

func runWithCleanup(t *testing.T, ctx context.Context, d *config.Document, options RunOptions) (*RunResult, error) {
	t.Helper()
	result, err := Run(ctx, d, options)
	if result != nil && result.ResultDir != "" {
		t.Cleanup(func() { _ = os.RemoveAll(result.ResultDir) })
	}
	return result, err
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
	result, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: root, Output: &output, PreviewOptions: PreviewOptions{Params: map[string]string{"value": literal}}})
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
	result, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: root})
	if err != nil || result.Builds[0].Status != "succeeded" {
		t.Fatalf("插入值被重新解释: %+v %v", result, err)
	}
	requireFile(t, root, "param", "${EXTRA_SECRET} {{unknown}}")
	requireFile(t, root, "secret", "${EXTRA_SECRET} {{unknown}}")
}

func TestRunBatchPrecheckHasNoScriptSideEffects(t *testing.T) {
	for _, invalid := range []string{
		"steps: [{kind: artifact, paths: ['out/{../outside,good}']}]",
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
			result, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: root, PreviewOptions: PreviewOptions{All: true}})
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
	result, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: root})
	if err != nil || result.Builds[0].Status != "succeeded" || result.Builds[0].Steps[1].Status != "skipped" {
		t.Fatalf("未生效能力应可跳过: %+v %v", result, err)
	}
	if err := os.Remove(filepath.Join(root, "marker")); err != nil {
		t.Fatal(err)
	}
	_, err = runWithCleanup(t, context.Background(), d, RunOptions{Workspace: root, PreviewOptions: PreviewOptions{Step: "selected", Params: map[string]string{"mode": "release"}}})
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
	result, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: root, PreviewOptions: PreviewOptions{Names: []string{"first", "second"}}})
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
	result, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: root})
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
	result, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: root})
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
	result, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: root})
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
	result, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: root})
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
	result, err := runWithCleanup(t, ctx, d, RunOptions{Workspace: root, PreviewOptions: PreviewOptions{Names: []string{"first", "second"}}})
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
	result, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: root})
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
	if _, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: root}); err != nil {
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
	result, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: root, PreviewOptions: PreviewOptions{Step: "selected"}})
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
	if _, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: root}); err != nil {
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
	result, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: root, PreviewOptions: PreviewOptions{Names: []string{"first", "second"}}})
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
	result, err := runWithCleanup(t, ctx, d, RunOptions{Workspace: root})
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
	result, err := runWithCleanup(t, context.Background(), d, options)
	if err == nil || result != nil || !strings.Contains(err.Error(), "notifications") {
		t.Fatalf("通知能力应整批拒绝: %+v %v", result, err)
	}
	requireAbsent(t, root, "first")
	if strings.Contains(err.Error(), "UNSET_NOTIFY_SECRET") {
		t.Fatal("通知错误泄露引用")
	}
	disabled := false
	d.Builds["second"].Notifications.Enabled = &disabled
	if _, err := runWithCleanup(t, context.Background(), d, options); err != nil {
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
	result, err = runWithCleanup(t, context.Background(), d, options)
	if err != nil || result.Builds[1].Status != "skipped" {
		t.Fatalf("build 禁用覆盖公共通知，跳过build无需通知: %+v %v", result, err)
	}
}

func TestRunArtifactSnapshotsAndFailureDiagnostics(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(fmt.Sprint(failed), func(t *testing.T) {
			root := t.TempDir()
			ending, phase := "true", "success"
			if failed {
				ending, phase = "exit 7", "failure"
			}
			d := localDocument(t, fmt.Sprintf(`version: 1
steps:
  - {kind: run, name: create, run: "mkdir -p nested; printf original > nested/app.zip"}
  - {kind: artifact, name: original, paths: ["**/*.zip"]}
  - {kind: run, name: ending, run: "%s"}
post:
  %s:
    - {kind: run, name: rewrite, run: "printf changed > nested/app.zip"}
    - {kind: artifact, name: post-copy, paths: [nested/app.zip]}
`, ending, phase))
			var output bytes.Buffer
			result, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: root, Output: &output})
			if result == nil || (err != nil) != failed || result.ResultDir == "" {
				t.Fatalf("快照结果错误: %+v %v", result, err)
			}
			info, err := os.Stat(result.ResultDir)
			if err != nil || info.Mode().Perm() != 0700 {
				t.Fatalf("结果根权限错误: %v %v", info, err)
			}
			realRoot, _ := filepath.EvalSymlinks(root)
			if rel, _ := filepath.Rel(realRoot, result.ResultDir); rel != ".." && !strings.HasPrefix(rel, "../") {
				t.Fatal("结果目录进入工作区")
			}
			b := result.Builds[0]
			for _, step := range append(append([]StepRun{}, b.Steps...), b.Post...) {
				if step.started && step.LogPath == "" {
					t.Fatalf("静默脚本缺少实际日志: %+v", step)
				}
			}
			if failed && b.Reason != "exit" {
				t.Fatalf("收尾覆盖原失败: %+v", b)
			}
			for i, step := range []StepRun{b.Steps[1], b.Post[1]} {
				want := []string{"original", "changed"}[i]
				if step.Status != "succeeded" || !step.started || len(step.Artifacts) != 1 || step.LogPath == "" {
					t.Fatalf("产物步骤结果错误: %+v", step)
				}
				record := step.Artifacts[0]
				requireFile(t, result.ResultDir, record.SnapshotPath, want)
				sum := sha256.Sum256([]byte(want))
				if record.SourcePath != "nested/app.zip" || record.Size != int64(len(want)) || record.SHA256 != fmt.Sprintf("%x", sum) {
					t.Fatalf("快照清单错误: %+v", record)
				}
				log, err := os.ReadFile(filepath.Join(result.ResultDir, step.LogPath))
				if err != nil || !strings.Contains(string(log), "stream=system") || !strings.Contains(output.String(), string(log)) {
					t.Fatalf("实际保存日志错误: %q %v", log, err)
				}
			}
		})
	}
}

func TestRunArtifactSelectionAndNoResultBeforePrecheck(t *testing.T) {
	root, temporary := t.TempDir(), t.TempDir()
	t.Setenv("TMPDIR", temporary)
	if err := os.WriteFile(filepath.Join(root, "app.zip"), []byte("standalone"), 0600); err != nil {
		t.Fatal(err)
	}
	d := localDocument(t, `version: 1
params: {path: app.zip, mode: release}
steps:
  - {kind: run, name: unused, run: "touch forbidden"}
  - {kind: artifact, name: selected, paths: ["{{path}}"], when: {params: {mode: release}}}
`)
	result, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: root, PreviewOptions: PreviewOptions{Step: "selected"}})
	if err != nil || len(result.Builds[0].Steps) != 1 || len(result.Builds[0].Steps[0].Artifacts) != 1 {
		t.Fatalf("独立产物步骤失败: %+v %v", result, err)
	}
	requireAbsent(t, root, "forbidden")
	if err := os.RemoveAll(result.ResultDir); err != nil {
		t.Fatal(err)
	}
	for _, options := range []PreviewOptions{
		{Step: "unused", Params: map[string]string{"path": "../outside"}},
		{Step: "selected", Params: map[string]string{"mode": "debug"}},
	} {
		result, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: root, PreviewOptions: options})
		if options.Step == "unused" && (err == nil || result != nil) {
			t.Fatalf("--step 绕过其他生效模式校验: %+v %v", result, err)
		}
		if options.Step == "selected" && (err != nil || result.ResultDir != "" || result.Builds[0].Status != "skipped") {
			t.Fatalf("全部选中步骤跳过仍创建结果: %+v %v", result, err)
		}
		entries, err := os.ReadDir(temporary)
		if err != nil || len(entries) != 0 {
			t.Fatalf("预检查失败/全跳过创建了目录: %v %v", entries, err)
		}
	}
	requireAbsent(t, root, "forbidden")
}

func TestRunArtifactTimeoutStillRunsFailurePost(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.zip"), []byte("diagnostic"), 0600); err != nil {
		t.Fatal(err)
	}
	d := localDocument(t, `version: 1
timeout: 1ns
steps:
  - {kind: artifact, name: timed, paths: ["**/*.zip"]}
  - {kind: run, run: "touch forbidden"}
post:
  failure: [{kind: artifact, name: diagnostic, paths: [app.zip]}]
`)
	result, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: root})
	if err == nil || result.Builds[0].Reason != "timeout" || !result.Builds[0].Steps[0].started || len(result.Builds[0].Post) != 1 || len(result.Builds[0].Post[0].Artifacts) != 1 {
		t.Fatalf("产物预算/诊断收尾失败: %+v %v", result, err)
	}
	if len(result.Builds[0].Steps[0].Artifacts) != 0 {
		t.Fatal("超时返回部分清单")
	}
	requireAbsent(t, result.ResultDir, "artifacts/default/timed")
	requireAbsent(t, root, "forbidden")
}

func TestRunResultDirectoryIgnoresWorkspaceTempAndLogFailurePreservesExit(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TMPDIR", root)
	for _, command := range []string{"printf hello", "printf hello; exit 7"} {
		d := localDocument(t, fmt.Sprintf("version: 1\nsteps: [{kind: run, run: '%s'}]\n", command))
		result, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: root, Output: failingLogWriter{}})
		if err == nil || result.Builds[0].Status != "failed" {
			t.Fatalf("日志失败未记录: %+v %v", result, err)
		}
		if strings.Contains(command, "exit") && result.Builds[0].Reason != "exit" {
			t.Fatalf("日志错误覆盖命令退出: %+v", result.Builds[0])
		}
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 0 {
			t.Fatalf("结果数据进入源码: %v %v", entries, err)
		}
	}
}

func TestRunArtifactMatchFailureContinuesBatchAndPost(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "diagnostic.txt"), []byte("evidence"), 0600); err != nil {
		t.Fatal(err)
	}
	d := localDocument(t, `version: 1
builds:
  first:
    steps:
      - {kind: artifact, name: missing, paths: [missing/*.zip]}
      - {kind: run, run: "touch forbidden"}
    post:
      failure: [{kind: artifact, name: diagnostic, paths: [diagnostic.txt]}]
  second:
    steps: [{kind: run, run: "touch continued"}]
`)
	var output bytes.Buffer
	result, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: root, Output: &output, PreviewOptions: PreviewOptions{Names: []string{"first", "second"}}})
	if err == nil || result.Builds[0].Reason != "artifact_error" || !result.Builds[0].Steps[0].started || len(result.Builds[0].Post[0].Artifacts) != 1 || result.Builds[1].Status != "succeeded" {
		t.Fatalf("匹配失败执行边界错误: %+v %v", result, err)
	}
	if strings.Contains(output.String(), "missing/*.zip") {
		t.Fatal("系统日志泄露产物模式")
	}
	requireAbsent(t, result.ResultDir, "artifacts/first/missing")
	requireAbsent(t, root, "forbidden")
	requireFile(t, root, "continued", "")
}
