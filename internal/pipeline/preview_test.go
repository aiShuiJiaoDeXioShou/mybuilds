package pipeline

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"mybuilds/internal/config"
)

func document(t *testing.T, source string) *config.Document {
	t.Helper()
	d, err := config.Parse([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestPreviewConditionsAndSelection(t *testing.T) {
	d := document(t, `version: 1
builds:
  beta:
    params: {mode: release}
    when: {branches: ["release/*", main], params: {mode: release}, changes: ["src/**"]}
    steps:
      - {kind: run, name: compile, run: "exit 99"}
      - kind: run
        name: optional
        run: "exit 99"
        when: {params: {mode: debug}}
  alpha:
    params: {mode: release}
    steps: [{kind: run, name: compile, run: "exit 99"}]
`)
	if _, err := Preview(d, PreviewOptions{}); err == nil {
		t.Fatal("多 build 必须显式选择")
	}
	p, err := Preview(d, PreviewOptions{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if p.Builds[0].Name != "alpha" || p.Builds[1].Name != "beta" {
		t.Fatal("全选顺序不稳定")
	}
	b := p.Builds[1]
	if b.Condition != "pending" || b.Steps[0].Condition != "pending" || b.Steps[1].Condition != "skipped" {
		t.Fatalf("未知与已知失败组合错误: %+v", b)
	}
	if !strings.Contains(strings.Join(b.Reasons, " "), "changes") {
		t.Fatal("未解释手动忽略 changes")
	}
	for _, tc := range []struct{ branch, mode, want string }{{"release/v1", "release", "ready"}, {"feature/x", "release", "skipped"}, {"", "debug", "skipped"}} {
		facts := map[string]string{}
		if tc.branch != "" {
			facts["git.branch"] = tc.branch
		}
		p, err := Preview(d, PreviewOptions{Names: []string{"beta"}, Params: map[string]string{"mode": tc.mode}, Facts: facts})
		if err != nil {
			t.Fatal(err)
		}
		if p.Builds[0].Condition != tc.want {
			t.Fatalf("%+v 得到 %s", tc, p.Builds[0].Condition)
		}
	}
	if _, err := Preview(d, PreviewOptions{All: true, Step: "compile"}); err == nil {
		t.Fatal("批量选择不得指定步骤")
	}
	if _, err := Preview(d, PreviewOptions{Names: []string{"alpha"}, Step: "missing"}); err == nil {
		t.Fatal("未知步骤应拒绝")
	}
	p, err = Preview(d, PreviewOptions{Names: []string{"beta"}, Step: "optional"})
	if err != nil || len(p.Builds[0].Steps) != 1 || p.Builds[0].Steps[0].Index != 2 {
		t.Fatalf("步骤位置未保留: %+v, %v", p, err)
	}
}

func TestPreviewTemplatesAndFullValidation(t *testing.T) {
	base := `version: 1
params: {mode: release}
env: {MODE: "{{mode}}", SECRET: "${UNSET_PREVIEW_SECRET}", STEP: "{{step.name}}"}
steps:
  - {kind: run, name: first, run: "{{unknown}} ${INVALID:?shell expression}"}
  - {kind: artifact, name: collect, paths: ["out/{{mode}}/*.zip"]}
`
	d := document(t, base)
	p, err := Preview(d, PreviewOptions{Step: "first"})
	if err != nil || p.Builds[0].Condition != "ready" {
		t.Fatalf("声明参数应可解析，run 不扫描: %v", err)
	}
	d.Builds["default"].Steps[1].Paths[0] = "out/{{git.sha}}/*.zip"
	p, err = Preview(d, PreviewOptions{})
	if err != nil || p.Builds[0].Steps[1].Condition != "pending" {
		t.Fatalf("缺少上下文应 pending: %+v, %v", p, err)
	}
	p, err = Preview(d, PreviewOptions{Facts: map[string]string{"git.sha": "abc"}})
	if err != nil || p.Builds[0].Steps[1].Condition != "ready" {
		t.Fatalf("已有事实应 ready: %+v, %v", p, err)
	}
	for _, invalid := range []string{"out/{{undeclared}}/SECRET_LITERAL", "out/{{mode}/SECRET_LITERAL", "out/}}/SECRET_LITERAL"} {
		d.Builds["default"].Steps[1].Paths[0] = invalid
		_, err := Preview(d, PreviewOptions{Step: "first"})
		if err == nil {
			t.Fatalf("--step 未完整验证: %s", invalid)
		}
		if strings.Contains(err.Error(), "SECRET_LITERAL") || strings.Contains(err.Error(), "undeclared") {
			t.Fatalf("诊断泄露正文: %v", err)
		}
	}
	for _, mutate := range []func(*config.Document){
		func(d *config.Document) { d.Builds["default"].Env["SECRET"] = "${BAD-NAME} SECRET_LITERAL" },
		func(d *config.Document) {
			d.Builds["default"].Post = &config.Post{Always: []config.Step{{Kind: "run", Name: "cleanup", Run: "true", Env: map[string]string{"X": "{{unknown}} SECRET_LITERAL"}}}}
		},
		func(d *config.Document) {
			d.Notifications = &config.Notifications{Template: "{{unknown}} SECRET_LITERAL"}
		},
		func(d *config.Document) {
			d.Builds["default"].Notifications = &config.Notifications{Template: "${BAD-NAME} SECRET_LITERAL"}
		},
	} {
		d := document(t, base)
		mutate(d)
		_, err := Preview(d, PreviewOptions{Step: "first"})
		if err == nil || strings.Contains(err.Error(), "SECRET_LITERAL") {
			t.Fatalf("未安全验证完整配置: %v", err)
		}
	}
	d = document(t, base)
	d.Notifications = &config.Notifications{Template: "{{build.status}} {{build.url}} {{mode}}"}
	if _, err := Preview(d, PreviewOptions{}); err != nil {
		t.Fatalf("通知专用模板应有效: %v", err)
	}
	d.Builds["default"].Env["MODE"] = "{{build.status}}"
	if _, err := Preview(d, PreviewOptions{}); err == nil {
		t.Fatal("通知变量不应用于执行字段")
	}
}

func TestPreviewNoSideEffectsOrSensitiveOutput(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()
	marker := filepath.Join(t.TempDir(), "marker")
	t.Setenv("MYBUILD_SECRET", "ENV_SECRET_VALUE")
	d := document(t, fmt.Sprintf(`version: 1
params: {mode: PARAM_SECRET_VALUE}
env: {SECRET: "${MYBUILD_SECRET}", MODE: "{{mode}}"}
steps:
  - kind: run
    name: compile
    run: "touch %s # SCRIPT_SECRET_VALUE"
  - kind: artifact
    paths: ["out/{{mode}}/*.zip"]
  - kind: upload
    target: custom
    argv: ["touch", "%s", "ARGV_SECRET_VALUE"]
    credentials: "${MYBUILD_SECRET} CREDENTIAL_SECRET_VALUE"
    result_file: out/result.json
notifications:
  webhooks: [{type: feishu, url: "%s/WEBHOOK_SECRET_VALUE"}]
  template: "NOTIFY_SECRET_VALUE {{project}}"
`, marker, marker, server.URL))
	before, _ := json.Marshal(d)
	p, err := Preview(d, PreviewOptions{Facts: map[string]string{"project": "CONTEXT_SECRET_VALUE"}})
	if err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"PARAM_SECRET_VALUE", "ENV_SECRET_VALUE", "MYBUILD_SECRET", "SCRIPT_SECRET_VALUE", "ARGV_SECRET_VALUE", "CREDENTIAL_SECRET_VALUE", "WEBHOOK_SECRET_VALUE", "NOTIFY_SECRET_VALUE", "CONTEXT_SECRET_VALUE", server.URL, marker} {
		if strings.Contains(string(out), secret) {
			t.Fatalf("预览泄露 %s: %s", secret, out)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("预览执行了脚本")
	}
	if requests.Load() != 0 {
		t.Fatal("预览访问了网络")
	}
	after, _ := json.Marshal(d)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("预览修改了配置")
	}
	p2, err := Preview(d, PreviewOptions{Facts: map[string]string{"project": "CONTEXT_SECRET_VALUE"}})
	out2, _ := json.Marshal(p2)
	if err != nil || string(out) != string(out2) {
		t.Fatal("预览输出不稳定")
	}
}

func TestPreviewBatchParamsAndCommonTemplates(t *testing.T) {
	d := document(t, `version: 1
builds:
  a:
    params: {shared: value, only_a: value}
    steps: [{kind: run, run: "true"}]
  b:
    params: {shared: value}
    steps: [{kind: run, run: "true"}]
notifications:
  template: "{{only_a}}"
`)
	if _, err := Preview(d, PreviewOptions{All: true, Params: map[string]string{"only_a": "secret"}}); err == nil {
		t.Fatal("批量覆盖需共同声明")
	}
	if _, err := Preview(d, PreviewOptions{All: true}); err == nil {
		t.Fatal("公共通知需在全部选中参数上下文校验")
	}
	if _, err := Preview(d, PreviewOptions{Names: []string{"a"}}); err != nil {
		t.Fatal(err)
	}
}

func TestPreviewRenderedPaths(t *testing.T) {
	d := document(t, `version: 1
params: {output: out}
steps:
  - {kind: artifact, paths: ["{{output}}/*.zip"]}
`)
	for _, value := range []string{"../private", "/private", "C:/private", `C:\private`, "out[", "a/../b", "out\nprivate", "out\x00private"} {
		if _, err := Preview(d, PreviewOptions{Params: map[string]string{"output": value}}); err == nil {
			t.Fatalf("渲染后非法路径应拒绝: %s", value)
		}
	}
	if _, err := Preview(d, PreviewOptions{Params: map[string]string{"output": "out/{{unknown}}"}}); err != nil {
		t.Fatalf("参数不应递归解释为模板: %v", err)
	}
}

func TestPreviewConstructedModelRemainsUnchanged(t *testing.T) {
	d := &config.Document{Version: 1, Builds: map[string]*config.Build{"default": {
		Env:   map[string]string{"STEP": "{{step.name}}"},
		Steps: []config.Step{{Kind: "run", Run: "true"}},
		Post:  &config.Post{Always: []config.Step{{Kind: "run", Run: "true"}}},
	}}}
	before, _ := json.Marshal(d)
	p, err := Preview(d, PreviewOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Builds[0].Steps[0].Name != "run-1" || p.Builds[0].Post.Always[0].Name != "post-always-run-1" {
		t.Fatal("缺省名称不稳定")
	}
	if p.Builds[0].Condition != "ready" {
		t.Fatal("实际 step.name 不应待确定")
	}
	after, _ := json.Marshal(d)
	if string(before) != string(after) {
		t.Fatal("预览改写了构造模型")
	}
}

func TestPreviewRenderedPathFieldRules(t *testing.T) {
	for _, tc := range []struct{ step, value string }{
		{`{kind: run, run: "true", working_dir: "{{output}}"}`, "out/*"},
		{`{kind: upload, target: custom, argv: ["true"], result_file: "{{output}}"}`, "out/?.json"},
		{`{kind: upload, target: google_play, app_identifier: com.example.app, credentials: "${CREDS}", file: "{{output}}"}`, "out["},
	} {
		d := document(t, "version: 1\nparams: {output: out}\nsteps: ["+tc.step+"]\n")
		if _, err := Preview(d, PreviewOptions{Params: map[string]string{"output": tc.value}}); err == nil {
			t.Fatalf("字段渲染后路径应拒绝: %s", tc.step)
		}
	}
	d := document(t, `version: 1
params: {output: "out/*.aab"}
steps: [{kind: upload, target: google_play, app_identifier: com.example.app, credentials: "${CREDS}", file: "{{output}}"}]
`)
	if _, err := Preview(d, PreviewOptions{}); err != nil {
		t.Fatalf("file 支持合法 glob: %v", err)
	}
}
