package client

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func execute(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := NewCommand()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return output.String(), err
}

func writeConfig(t *testing.T, filename, contents string) {
	t.Helper()
	if err := os.WriteFile(filename, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestInitAndPreview(t *testing.T) {
	t.Chdir(t.TempDir())
	if _, err := execute(t, "init"); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile("mybuilds.yml")
	if err != nil {
		t.Fatal(err)
	}
	out, err := execute(t, "run", "--dry-run")
	if err != nil || !json.Valid([]byte(out)) || !strings.Contains(out, "default") || !strings.Contains(out, "run") {
		t.Fatalf("默认预览：%q，%v", out, err)
	}
	if _, err := execute(t, "init"); err == nil {
		t.Fatal("重复初始化应失败")
	}
	after, err := os.ReadFile("mybuilds.yml")
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("重复初始化修改了原文件")
	}
}

func TestInitExclusiveCreation(t *testing.T) {
	t.Chdir(t.TempDir())
	var successful atomic.Int32
	var group sync.WaitGroup
	for range 16 {
		group.Go(func() {
			if _, err := execute(t, "init"); err == nil {
				successful.Add(1)
			}
		})
	}
	group.Wait()
	if successful.Load() != 1 {
		t.Fatalf("初始化成功次数 = %d，需要 1", successful.Load())
	}
	if _, err := execute(t, "run", "--dry-run"); err != nil {
		t.Fatalf("竞态后的文件无效：%v", err)
	}
}

func TestInitRejectsExistingSymlink(t *testing.T) {
	t.Chdir(t.TempDir())
	writeConfig(t, "existing", "不应修改")
	if err := os.Symlink("existing", "mybuilds.yml"); err != nil {
		t.Fatal(err)
	}
	if _, err := execute(t, "init"); err == nil {
		t.Fatal("符号链接目标应拒绝")
	}
	data, _ := os.ReadFile("existing")
	if string(data) != "不应修改" {
		t.Fatal("链接原文件被修改")
	}
	if err := os.Remove("existing"); err != nil {
		t.Fatal(err)
	}
	if _, err := execute(t, "init"); err == nil {
		t.Fatal("悬空符号链接目标应拒绝")
	}
}

func TestInitTemplate(t *testing.T) {
	t.Chdir(t.TempDir())
	valid := "version: 1\nbuilds:\n  demo:\n    steps:\n      - kind: run\n        run: touch template-marker\n"
	writeConfig(t, "template.yml", valid)
	if _, err := execute(t, "init", "--template", "template.yml"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile("mybuilds.yml")
	if string(data) != valid {
		t.Fatal("模板原字节未保留")
	}
	if _, err := os.Stat("template-marker"); !os.IsNotExist(err) {
		t.Fatal("初始化执行了模板脚本")
	}
}

func TestInitErrorsDoNotCreateFile(t *testing.T) {
	for _, args := range [][]string{
		{"init", "extra"},
		{"init", "--template", "missing.yml"},
		{"init", "--template", "invalid.yml"},
		{"init", "--template", ""},
		{"init", "--framework", "flutter"},
		{"init", "--framework", ""},
		{"init", "--platform", "unknown"},
		{"init", "--template", "valid.yml", "--framework", "flutter"},
		{"init", "--template", "valid.yml", "--platform", "android"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Chdir(t.TempDir())
			writeConfig(t, "invalid.yml", "version: 1\nprivate: secret-template-invalid\n")
			writeConfig(t, "valid.yml", "version: 1\nsteps:\n  - kind: run\n    run: echo demo\n")
			out, err := execute(t, args...)
			if err == nil {
				t.Fatal("需要初始化错误")
			}
			if strings.Contains(out+err.Error(), "secret-template-invalid") {
				t.Fatal("模板错误泄露原标量")
			}
			if _, err := os.Lstat("mybuilds.yml"); !os.IsNotExist(err) {
				t.Fatalf("错误初始化遗留目标：%v", err)
			}
		})
	}
}

const multiBuildConfig = `version: 1
builds:
  android:
    params:
      channel:
        default: internal
        choices: [internal, production]
      token: ""
      android_only: ""
    steps:
      - kind: run
        name: package
        run: echo android
  ios:
    params:
      channel:
        default: internal
        choices: [internal, production]
      token: ""
    steps:
      - kind: run
        name: package
        run: echo ios
`

func TestRunSelectionAndParameters(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.Mkdir("configs", 0700); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, "configs/pipeline.yml", multiBuildConfig)
	for _, args := range [][]string{
		{"--build", "android"},
		{"--build", "ios,android"},
		{"--all"},
		{"--build", "android", "--step", "package"},
		{"--build", "android", "--param", "token=secret=a,b", "--param", "channel=production"},
		{"--all", "--param", "token="},
	} {
		out, err := execute(t, append([]string{"run", "--dry-run", "--file", "configs/pipeline.yml"}, args...)...)
		if err != nil || !json.Valid([]byte(out)) {
			t.Fatalf("%v：%q，%v", args, out, err)
		}
		if strings.Contains(out, "secret=a,b") {
			t.Fatal("参数值泄露")
		}
	}
	first, err := execute(t, "run", "--dry-run", "--file", "configs/pipeline.yml", "--all")
	if err != nil {
		t.Fatal(err)
	}
	second, _ := execute(t, "run", "--dry-run", "--file", "configs/pipeline.yml", "--all")
	if first != second {
		t.Fatal("相同输入预览输出不稳定")
	}
	for _, args := range [][]string{
		{}, {"--build", "missing"}, {"--build", "android,"}, {"--build", ""},
		{"--build", "android,android"}, {"--build", "android", "--all"},
		{"--build", "android", "--all=false"}, {"--build", "android", "--step", ""},
		{"--all", "--step", "package"}, {"--build", "android", "--step", "missing"},
		{"--build", "android", "--param", "token=one", "--param", "token=two"},
		{"--build", "android", "--param", "=secret-empty-key"},
		{"--build", "android", "--param", "secret-no-equals"},
		{"--build", "android", "--param", "unknown=secret-unknown"},
		{"--build", "android", "--param", "channel=secret-choice"},
		{"--all", "--param", "android_only=value"}, {"extra"},
	} {
		out, err := execute(t, append([]string{"run", "--dry-run", "--file", "configs/pipeline.yml"}, args...)...)
		if err == nil {
			t.Fatalf("%v：需要错误", args)
		}
		if strings.Contains(out+err.Error(), "secret-") {
			t.Fatalf("参数错误泄露值：%q，%v", out, err)
		}
	}
}

func TestRunPreviewHasNoSideEffectsOrSensitiveOutput(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("PREVIEW_SECRET", "secret-env-value")
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()
	writeConfig(t, "git", "#!/bin/sh\n: > git-marker\n")
	if err := os.Chmod("git", 0700); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", cwd)
	contents := `version: 1
params:
  token: secret-default-value
env:
  SECRET: ${PREVIEW_SECRET}
  LITERAL: secret-literal-env
when:
  branches: [main]
notifications:
  webhooks:
    - type: feishu
      url: ` + server.URL + `/secret-webhook
steps:
  - kind: run
    name: package
    run: ": > run-marker # secret-run-body"
  - kind: artifact
    paths: [output/*.aab]
  - kind: approval
    notify: false
  - kind: upload
    target: google_play
    file: output/*.aab
    track: internal
    credentials: ${PREVIEW_SECRET}
post:
  always:
    - kind: run
      run: ": > post-marker"
`
	writeConfig(t, "mybuilds.yml", contents)
	out, err := execute(t, "run", "--dry-run")
	if err != nil || !json.Valid([]byte(out)) {
		t.Fatalf("敏感配置预览：%q，%v", out, err)
	}
	for _, secret := range []string{"secret-env-value", "secret-default-value", "secret-literal-env", "secret-webhook", "secret-run-body", "${PREVIEW_SECRET}", server.URL} {
		if strings.Contains(out, secret) {
			t.Fatalf("输出包含敏感值 %q", secret)
		}
	}
	for _, name := range []string{"run-marker", "post-marker", "git-marker"} {
		if _, err := os.Stat(name); !os.IsNotExist(err) {
			t.Fatalf("预览产生副作用 %s", name)
		}
	}
	if requests.Load() != 0 {
		t.Fatal("预览发送了网络请求")
	}
	entries, err := os.ReadDir(".")
	if err != nil || len(entries) != 2 {
		t.Fatalf("预览创建了额外文件：%v，%v", entries, err)
	}
}

func TestExampleConfiguration(t *testing.T) {
	filename, err := filepath.Abs("../../../examples/pipeline-preview.yml")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	out, err := execute(t, "run", "--dry-run", "--file", filename, "--all")
	if err != nil || !json.Valid([]byte(out)) || !strings.Contains(out, "android") || !strings.Contains(out, "ios") {
		t.Fatalf("双平台示例：%q，%v", out, err)
	}
}

func TestRunWithoutConfiguration(t *testing.T) {
	t.Chdir(t.TempDir())
	out, err := execute(t, "run")
	if err == nil || strings.Contains(out, "mybuilds.yml") {
		t.Fatalf("普通 run 应拒绝缺失配置且不暴露文件名：%q，%v", out, err)
	}
	entries, err := os.ReadDir(".")
	if err != nil || len(entries) != 0 {
		t.Fatal("普通 run 创建了文件")
	}
}

func TestInitRejectsOversizedTemplate(t *testing.T) {
	t.Chdir(t.TempDir())
	writeConfig(t, "large.yml", "version: 1\nsteps:\n  - kind: run\n    run: echo demo\n#"+strings.Repeat("x", 1<<20))
	if _, err := execute(t, "init", "--template", "large.yml"); err == nil {
		t.Fatal("超限模板应拒绝")
	}
	if _, err := os.Lstat("mybuilds.yml"); !os.IsNotExist(err) {
		t.Fatal("超限模板遗留目标")
	}
}

func TestRunParamRetainsEqualsCommaAndEmptyValue(t *testing.T) {
	t.Chdir(t.TempDir())
	writeConfig(t, "mybuilds.yml", `version: 1
params:
  token:
    choices: ["a=b,c", ""]
steps:
  - kind: run
    name: verify
    when:
      params: {token: "a=b,c"}
    run: echo demo
`)
	for _, tc := range []struct {
		value, condition string
	}{{"a=b,c", "ready"}, {"", "skipped"}} {
		out, err := execute(t, "run", "--dry-run", "--param", "token="+tc.value)
		if err != nil || !strings.Contains(out, `"condition": "`+tc.condition+`"`) {
			t.Fatalf("参数分割/空值处理失败：%q，%v", out, err)
		}
	}
}
