package client

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mybuilds/internal/pipeline"
)

func artifactWorkspace(t *testing.T) (string, string) {
	t.Helper()
	requireLocalShell(t)
	workspace, results := t.TempDir(), t.TempDir()
	t.Chdir(workspace)
	t.Setenv("TMPDIR", results)
	return workspace, results
}

func decodeArtifactResult(t *testing.T, stdout, results string) pipeline.RunResult {
	t.Helper()
	var result pipeline.RunResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("运行JSON无效：%q，%v", stdout, err)
	}
	if !filepath.IsAbs(result.ResultDir) {
		t.Fatalf("结果目录不是绝对路径：%q", result.ResultDir)
	}
	// macOS的系统临时根可能经/var链接指向/private/var，按真实目录核对归属。
	realResults, err := filepath.EvalSymlinks(results)
	if err != nil {
		t.Fatal(err)
	}
	realDirectory, err := filepath.EvalSymlinks(result.ResultDir)
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(realResults, realDirectory)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		t.Fatalf("结果不在本次测试临时根：%q", result.ResultDir)
	}
	t.Cleanup(func() { os.RemoveAll(result.ResultDir) })
	info, err := os.Stat(result.ResultDir)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("结果目录权限错误：%v", err)
	}
	return result
}

func checkArtifactRecord(t *testing.T, directory string, record pipeline.ArtifactRecord, want string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(record.SnapshotPath)))
	if err != nil || string(data) != want || record.Size != int64(len(data)) || record.SHA256 != fmt.Sprintf("%x", sha256.Sum256(data)) {
		t.Fatalf("产物快照/大小/hash错误：%+v，%q，%v", record, data, err)
	}
	info, err := os.Stat(filepath.Join(directory, filepath.FromSlash(record.SnapshotPath)))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("产物权限错误：%v", err)
	}
}

func TestLocalArtifactsJSONSnapshotsPostAndLogs(t *testing.T) {
	_, results := artifactWorkspace(t)
	t.Setenv("LOCAL_ARTIFACT_LOG_SECRET", "private-artifact-secret")
	writeConfig(t, "user-file", "用户修改")
	writeConfig(t, "mybuilds.yml", `version: 1
env:
  EXPLICIT_SECRET: ${LOCAL_ARTIFACT_LOG_SECRET}
steps:
  - kind: run
    name: package
    run: |
      mkdir -p output/deep
      printf alpha > output/root.bin
      printf beta > output/deep/root.bin
      printf '%s\n' "$EXPLICIT_SECRET"
      printf 'stdout-note\n'
      printf 'stderr-note\n' >&2
  - kind: artifact
    name: collect
    paths: [output/**/*.bin, output/**/root.bin]
post:
  always:
    - kind: run
      name: rewrite
      run: printf changed > output/root.bin; printf changed > output/deep/root.bin
    - kind: artifact
      name: collect-post
      paths: [output/**/*.bin]
`)
	stdout, stderr, err := localExecute(t, context.Background(), "run")
	if err != nil {
		t.Fatalf("产物执行失败：%q，%q，%v", stdout, stderr, err)
	}
	result := decodeArtifactResult(t, stdout, results)
	if len(result.Builds) != 1 || result.Builds[0].Status != "succeeded" {
		t.Fatalf("构建结果错误：%+v", result.Builds)
	}
	build := result.Builds[0]
	if len(build.Steps) != 2 || len(build.Post) != 2 || len(build.Steps[1].Artifacts) != 2 || len(build.Post[1].Artifacts) != 2 {
		t.Fatalf("去重/步骤/收尾清单错误：%+v", build)
	}
	want := map[string]string{"output/root.bin": "alpha", "output/deep/root.bin": "beta"}
	for i, record := range build.Steps[1].Artifacts {
		checkArtifactRecord(t, result.ResultDir, record, want[record.SourcePath])
		post := build.Post[1].Artifacts[i]
		checkArtifactRecord(t, result.ResultDir, post, "changed")
		if record.SnapshotPath == post.SnapshotPath {
			t.Fatal("普通与post共用产物路径")
		}
	}
	for _, step := range append(build.Steps, build.Post...) {
		if step.LogPath == "" {
			t.Fatalf("步骤日志没有实际路径：%+v", step)
		}
		data, err := os.ReadFile(filepath.Join(result.ResultDir, filepath.FromSlash(step.LogPath)))
		if err != nil || len(data) == 0 || strings.Contains(string(data), "private-artifact-secret") {
			t.Fatalf("步骤日志缺失或泄露秘密：%s，%v", step.Name, err)
		}
		info, err := os.Stat(filepath.Join(result.ResultDir, filepath.FromSlash(step.LogPath)))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("日志权限错误：%v", err)
		}
	}
	log, _ := os.ReadFile(filepath.Join(result.ResultDir, filepath.FromSlash(build.Steps[0].LogPath)))
	if !strings.Contains(string(log), "[stream=stdout]") || !strings.Contains(string(log), "[stream=stderr]") || !strings.Contains(string(log), "stdout-note") || !strings.Contains(string(log), "stderr-note") {
		t.Fatal("步骤日志没有保留两流来源")
	}
	if strings.Contains(stdout+stderr, "private-artifact-secret") {
		t.Fatal("JSON/终端泄露秘密")
	}
	data, _ := os.ReadFile("user-file")
	if string(data) != "用户修改" {
		t.Fatal("产物保存修改了用户文件")
	}
}

func TestLocalArtifactsStepOnlyAndFailureEvidence(t *testing.T) {
	_, results := artifactWorkspace(t)
	if err := os.Mkdir("output", 0700); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, "output/ready.bin", "独立输入")
	writeConfig(t, "mybuilds.yml", `version: 1
steps:
  - kind: run
    name: package
    run: ': > forbidden-marker'
  - kind: artifact
    name: collect
    paths: [output/*.bin]
`)
	stdout, _, err := localExecute(t, context.Background(), "run", "--step", "collect")
	if err != nil {
		t.Fatal(err)
	}
	result := decodeArtifactResult(t, stdout, results)
	if len(result.Builds[0].Steps) != 1 || len(result.Builds[0].Steps[0].Artifacts) != 1 {
		t.Fatal("--step没有独立收集")
	}
	checkArtifactRecord(t, result.ResultDir, result.Builds[0].Steps[0].Artifacts[0], "独立输入")
	if _, err := os.Stat("forbidden-marker"); !os.IsNotExist(err) {
		t.Fatal("独立artifact自动运行了前序步骤")
	}
	if err := os.Remove("output/ready.bin"); err != nil {
		t.Fatal(err)
	}
	stdout, _, err = localExecute(t, context.Background(), "run", "--step", "collect")
	if err == nil {
		t.Fatal("零匹配应该失败")
	}
	failed := decodeArtifactResult(t, stdout, results)
	step := failed.Builds[0].Steps[0]
	if failed.Builds[0].Status != "failed" || len(step.Artifacts) != 0 || step.LogPath == "" {
		t.Fatalf("失败未保留安全日志或泄露部分产物：%+v", step)
	}
	if _, err := os.Stat(filepath.Join(failed.ResultDir, filepath.FromSlash(step.LogPath))); err != nil {
		t.Fatal("失败日志不存在")
	}
}

func TestLocalArtifactsNoResultForDryRunPrecheckOrSkipped(t *testing.T) {
	_, results := artifactWorkspace(t)
	for _, tc := range []struct {
		name, contents string
		args           []string
		wantErr        bool
	}{
		{"预览", "version: 1\nsteps:\n  - kind: artifact\n    paths: [output/**/*.bin]\n", []string{"run", "--dry-run"}, false},
		{"全部跳过", "version: 1\nparams: {release: 'no'}\nsteps:\n  - kind: artifact\n    paths: [output/**/*.bin]\n    when:\n      params: {release: 'yes'}\n", []string{"run"}, false},
		{"审批纯预览", "version: 1\nsteps:\n  - kind: run\n    run: ': > forbidden-marker'\n  - kind: approval\n    notify: false\n", []string{"run", "--dry-run"}, false},
		{"通知预检查失败", "version: 1\nsteps:\n  - kind: run\n    run: ': > forbidden-marker'\n  - kind: approval\n    notify: true\n", []string{"run"}, true},
	} {
		writeConfig(t, "mybuilds.yml", tc.contents)
		stdout, _, err := localExecute(t, context.Background(), tc.args...)
		if (err != nil) != tc.wantErr || strings.Contains(stdout, "result_dir") {
			t.Fatalf("%s结果错误：%q，%v", tc.name, stdout, err)
		}
		entries, err := os.ReadDir(results)
		if err != nil || len(entries) != 0 {
			t.Fatalf("%s创建了结果目录：%v，%v", tc.name, entries, err)
		}
	}
	if _, err := os.Stat("forbidden-marker"); !os.IsNotExist(err) {
		t.Fatal("预检查失败执行了脚本")
	}
}

func TestLocalArtifactsExample(t *testing.T) {
	filename, err := filepath.Abs("../../../examples/local-artifacts.yml")
	if err != nil {
		t.Fatal(err)
	}
	_, results := artifactWorkspace(t)
	stdout, _, err := localExecute(t, context.Background(), "run", "--file", filename)
	if err != nil {
		t.Fatal(err)
	}
	result := decodeArtifactResult(t, stdout, results)
	build := result.Builds[0]
	if build.Status != "succeeded" || len(build.Steps[1].Artifacts) != 2 || len(build.Post[1].Artifacts) != 2 {
		t.Fatalf("示例未生成独立产物：%+v", build)
	}
}
