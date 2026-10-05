package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const minimal = "version: 1\nsteps:\n  - kind: run\n    run: echo safe\n"

func TestParseAndResolve(t *testing.T) {
	d, err := Parse([]byte(`version: 1
runner: {platform: android, labels: [sdk]}
timeout: 1h
params:
  optional: ""
  channel: {default: internal, required: true, choices: [internal, production]}
  supplied: {required: true}
env: {CHANNEL: "{{channel}}"}
when: {branches: [main, 'release/*'], params: {channel: internal}, changes: ['src/**']}
steps:
  - {kind: run, run: 'echo ${SECRET}', shell: bash, working_dir: '.', timeout: 1m}
  - {kind: artifact, paths: ['build/**/*.aab']}
  - {kind: approval, name: 发布审核, notify: false}
  - {kind: upload, target: google_play, app_identifier: com.example.app, file: '*.aab', credentials: '${CREDS}', track: internal}
post:
  timeout: 2m
  failure: [{kind: run, run: diagnostics}]
  always: [{kind: artifact, paths: ['logs/*.txt']}]
reports: {junit: {paths: ['test/**/*.xml'], required: true}}
notifications:
  enabled: true
  on: [success, failure, cancelled]
  webhooks: [{type: feishu, url: '${HOOK}'}]
  template: '{{build.status}}'
`))
	if err != nil {
		t.Fatal(err)
	}
	b := d.Builds["default"]
	if b.Steps[0].Name != "run-1" || b.Steps[1].Name != "artifact-2" {
		t.Fatal("步骤未获得稳定名称")
	}
	if _, err = ResolveParams(b, nil); err == nil {
		t.Fatal("缺失必填参数未拒绝")
	}
	values, err := ResolveParams(b, map[string]string{"supplied": "yes", "channel": "production"})
	if err != nil || values["channel"] != "production" || values["optional"] != "" {
		t.Fatalf("参数合并失败: %v", err)
	}
	for _, overrides := range []map[string]string{{"supplied": ""}, {"supplied": "x", "channel": "bad"}, {"unknown": "x"}} {
		if _, err := ResolveParams(b, overrides); err == nil {
			t.Fatal("非法覆盖未拒绝")
		}
	}
}

func TestSelection(t *testing.T) {
	d, err := Parse([]byte("version: 1\nbuilds:\n  z: {steps: [{kind: run, run: z}]}\n  a: {steps: [{kind: run, run: a}]}\n"))
	if err != nil {
		t.Fatal(err)
	}
	names, err := d.Select(nil, true)
	if err != nil || !reflect.DeepEqual(names, []string{"a", "z"}) {
		t.Fatal(names, err)
	}
	names, err = d.Select([]string{"z", "a"}, false)
	if err != nil || !reflect.DeepEqual(names, []string{"z", "a"}) {
		t.Fatal(names, err)
	}
	for _, tc := range []struct {
		names []string
		all   bool
	}{{nil, false}, {[]string{"z"}, true}, {[]string{"z", "z"}, false}, {[]string{""}, false}, {[]string{"missing"}, false}} {
		if _, err := d.Select(tc.names, tc.all); err == nil {
			t.Fatal("非法选择未拒绝")
		}
	}
	single, _ := Parse([]byte(minimal))
	names, err = single.Select(nil, false)
	if err != nil || !reflect.DeepEqual(names, []string{"default"}) {
		t.Fatal(names, err)
	}
}

func TestRejectInvalidConfiguration(t *testing.T) {
	cases := map[string]string{
		"empty": "", "missing-version": "steps: [{kind: run, run: echo}]", "version": "version: 2\nsteps: [{kind: run, run: echo}]", "multi": minimal + "---\n" + minimal,
		"unknown": minimal + "secret_unknown: secret_value\n", "duplicate": minimal + "version: 1\n", "null": minimal + "env: null\n", "alias": "version: 1\nsteps: &s [{kind: run, run: echo}]\npost: {always: *s}\n", "merge": "version: 1\nsteps: [{kind: run, run: echo, <<: {shell: sh}}]\n",
		"number-run": "version: 1\nsteps: [{kind: run, run: 123}]", "bool-env": minimal + "env: {SECRET: true}\n", "numeric-param": minimal + "params: {secret: 12}\n", "numeric-key": minimal + "env: {12: text}\n", "bool-required": minimal + "params: {secret: {required: 'true'}}\n", "string-version": "version: '1'\nsteps: [{kind: run, run: echo}]",
		"empty-builds": "version: 1\nbuilds: {}", "mixed": minimal + "builds: {}\n", "build-name": "version: 1\nbuilds: {'../bad': {steps: [{kind: run, run: echo}]}}", "empty-steps": "version: 1\nsteps: []", "missing-run": "version: 1\nsteps: [{kind: run}]", "blank-run": "version: 1\nsteps: [{kind: run, run: ' '}]", "kind": "version: 1\nsteps: [{kind: checkout}]",
		"exclusive": "version: 1\nsteps: [{kind: run, run: echo, notify: false}]", "shell": "version: 1\nsteps: [{kind: run, run: echo, shell: fish}]", "empty-shell": "version: 1\nsteps: [{kind: run, run: echo, shell: ''}]", "duration": minimal + "timeout: 0s\n", "negative-duration": minimal + "timeout: -1s\n", "bad-duration": minimal + "timeout: forever\n", "empty-duration": minimal + "timeout: ''\n",
		"working-dir": "version: 1\nsteps: [{kind: run, run: echo, working_dir: '../outside'}]", "windows-path": "version: 1\nsteps: [{kind: run, run: echo, working_dir: 'C:\\root'}]", "absolute-path": "version: 1\nsteps: [{kind: artifact, paths: ['/tmp/a']}]", "glob": "version: 1\nsteps: [{kind: artifact, paths: ['[broken']}]", "empty-paths": "version: 1\nsteps: [{kind: artifact, paths: []}]", "step-name": "version: 1\nsteps: [{kind: run, name: '..', run: echo}]", "duplicate-name": "version: 1\nsteps: [{kind: run, name: x, run: echo}, {kind: artifact, name: x, paths: [a]}]",
		"param-name": minimal + "params: {'bad-name': value}\n", "reserved-param": minimal + "params: {project: value}\n", "bad-default": minimal + "params: {channel: {default: bad, choices: [good]}}\n", "empty-required-default": minimal + "params: {channel: {default: '', required: true}}\n", "empty-choices": minimal + "params: {channel: {choices: []}}\n", "duplicate-choices": minimal + "params: {channel: {choices: [x, x]}}\n",
		"empty-when": minimal + "when: {}\n", "empty-branches": minimal + "when: {branches: []}\n", "unknown-param": minimal + "when: {params: {missing: x}}\n", "empty-when-params": minimal + "when: {params: {}}\n", "changes-traverse": minimal + "when: {changes: ['../**']}\n",
		"post-kind": minimal + "post: {always: [{kind: approval}]}\n", "post-empty": minimal + "post: {}\n", "reports-empty": minimal + "reports: {}\n", "reports-paths": minimal + "reports: {junit: {paths: []}}\n", "bad-platform": minimal + "runner: {platform: linux}\n", "engine-env": minimal + "env: {MYBUILDS_BUILD_ID: value}\n", "bad-env-name": minimal + "env: {'bad-name': value}\n",
		"notify-type": minimal + "notifications: {webhooks: [{type: bad, url: 'https://example.org'}]}\n", "notify-url": minimal + "notifications: {webhooks: [{type: generic, url: 'file:///tmp/a'}]}\n", "notify-event": minimal + "notifications: {on: [unknown]}\n", "notify-null": minimal + "notifications: {enabled: null}\n", "notify-empty-url": minimal + "notifications: {webhooks: [{type: generic, url: ''}]}\n",
		"publish-order": "version: 1\nsteps: [{kind: approval}, {kind: run, run: echo}, {kind: upload, target: google_play, app_identifier: com.example.app, file: '*.aab', credentials: '${CREDS}'}]", "upload-required": "version: 1\nsteps: [{kind: upload, target: google_play}]", "upload-fields": "version: 1\nsteps: [{kind: upload, target: google_play, app_identifier: com.example.app, file: '*.aab', credentials: '${CREDS}', argv: [secret]}]", "custom-required": "version: 1\nsteps: [{kind: upload, target: custom, argv: []}]",
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(input)); err == nil {
				t.Fatal("非法配置被接受")
			}
		})
	}
}

func TestSafeErrorsAndLimits(t *testing.T) {
	secret := "DO_NOT_LEAK_7eJx"
	for _, input := range []string{minimal + secret + ": x\n", minimal + "params: {" + secret + ": {choices: []}}", "version: 1\nsteps: [{kind: run, run: '" + secret + "}]", minimal + "env: {KEY: [" + secret + "]}", minimal + "notifications: {webhooks: [{type: generic, url: '" + secret + "'}]}"} {
		_, err := Parse([]byte(input))
		if err == nil || strings.Contains(err.Error(), secret) {
			t.Fatalf("诊断泄露或未拒绝: %v", err)
		}
	}
	if _, err := Parse([]byte(strings.Repeat(" ", MaxConfigBytes+1))); err == nil {
		t.Fatal("大小超限未拒绝")
	}
	if _, err := Parse([]byte("version: 1\nsteps: " + strings.Repeat("[", MaxDepth+1) + "x" + strings.Repeat("]", MaxDepth+1))); err == nil {
		t.Fatal("深度超限未拒绝")
	}
	if _, err := Parse([]byte(minimal + "env: {" + strings.Repeat("k: x,", MaxNodes) + "}")); err == nil {
		t.Fatal("节点超限未拒绝")
	}
	dir := t.TempDir()
	filename := filepath.Join(dir, "secret.yml")
	if err := os.WriteFile(filename, []byte(minimal), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(filename); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(filepath.Join(dir, secret)); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatal("加载诊断泄露路径")
	}
}

func TestTargetAndOrdinaryApproval(t *testing.T) {
	for _, steps := range []string{
		"[{kind: approval}, {kind: run, run: echo}]",
		"[{kind: upload, target: app_store, app_identifier: com.example.app, file: '*.ipa', credentials: '${CREDS}', submit_for_review: true, automatic_release: false}]",
		"[{kind: upload, target: custom, argv: [bash, ci/publish.sh], result_file: publish/result.json, query_argv: [bash, ci/query.sh], working_dir: '.', timeout: 1m}]",
	} {
		if _, err := Parse([]byte("version: 1\nsteps: " + steps)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestNestedTypesAndPresence(t *testing.T) {
	cases := []string{
		"runner: []", "runner: {platform: 3}", "runner: {platform: android, labels: [true]}", "runner: {platform: android, labels: ['', sdk]}", "runner: {platform: android, labels: [sdk, sdk]}",
		"params: []", "params: {x: {unknown: secret}}", "params: {x: {default: true}}", "params: {x: {description: 5}}", "params: {x: {choices: [x, 3]}}", "params: {x: {choices: [null]}}", "params: {x: {default: x, default: secret}}", "params: {'1': value}",
		"env: []", "env: {X: {value: secret}}", "when: []", "when: {unknown: secret}", "when: {branches: main}", "when: {branches: ['[bad']}", "when: {branches: [true]}", "when: {changes: []}", "when: {changes: [true]}", "when: {params: []}",
		"post: []", "post: {unknown: secret}", "post: {always: [{kind: run, run: echo, shell: fish}]}", "post: {always: [{kind: artifact, paths: ['../logs']}]}", "post: {always: [{kind: run, run: echo}], timeout: '0s'}",
		"reports: []", "reports: {unknown: secret}", "reports: {junit: {paths: [x], required: 'false'}}", "reports: {junit: {paths: [x], unknown: secret}}", "reports: {junit: {paths: [true]}}",
		"notifications: []", "notifications: {unknown: secret}", "notifications: {enabled: 'false'}", "notifications: {on: [success, success]}", "notifications: {webhooks: [{type: generic}]}", "notifications: {webhooks: [{url: 'https://example.org'}]}", "notifications: {webhooks: [{type: generic, url: true}]}", "notifications: {webhooks: [{type: generic, url: 'https://user:pass@example.org'}]}", "notifications: {webhooks: [{type: generic, url: '${bad-name}'}]}", "notifications: {template: false}",
	}
	for _, tail := range cases {
		t.Run(tail, func(t *testing.T) {
			if _, err := Parse([]byte(minimal + tail + "\n")); err == nil {
				t.Fatal("非法嵌套配置被接受")
			}
		})
	}
	steps := []string{
		"{kind: run, run: echo, shell: null}", "{kind: run, run: echo, name: ''}", "{kind: run, run: echo, working_dir: ''}", "{kind: run, run: echo, paths: []}", "{kind: artifact, paths: [x], timeout: 1m}", "{kind: approval, notify: 'false'}", "{kind: approval, run: ''}", "{kind: approval, env: {}}", "{kind: upload, target: custom, argv: [3], result_file: x}", "{kind: upload, target: custom, argv: [''], result_file: x}", "{kind: upload, target: custom, argv: [publish], result_file: '../x'}", "{kind: upload, target: custom, argv: [publish], result_file: '*.json'}", "{kind: upload, target: custom, argv: [publish], result_file: x, query_argv: []}", "{kind: upload, target: google_play, app_identifier: com.example.app, file: x, credentials: y, submit_for_review: false}", "{kind: upload, target: app_store, app_identifier: com.example.app, file: x, credentials: y, track: internal}",
	}
	for _, step := range steps {
		t.Run(step, func(t *testing.T) {
			if _, err := Parse([]byte("version: 1\nsteps: [" + step + "]")); err == nil {
				t.Fatal("非法步骤被接受")
			}
		})
	}
}

func TestValidateConstructedModel(t *testing.T) {
	falseValue := false
	defaultValue := "bad"
	cases := []*Document{
		nil, {Version: 1}, {Version: 2, Builds: map[string]*Build{"default": {Steps: []Step{{Kind: "run", Run: "echo"}}}}},
		{Version: 1, Builds: map[string]*Build{"default": nil}},
		{Version: 1, Builds: map[string]*Build{"default": {Steps: []Step{{Kind: "run", Run: "echo", Notify: &falseValue}}}}},
		{Version: 1, Builds: map[string]*Build{"default": {Steps: []Step{{Kind: "run", Run: "echo", Paths: []string{}}}}}},
		{Version: 1, Builds: map[string]*Build{"default": {Steps: []Step{{Kind: "run", Run: "echo"}}, When: &When{Branches: []string{}}}}},
		{Version: 1, Builds: map[string]*Build{"default": {Steps: []Step{{Kind: "run", Run: "echo"}}, Params: map[string]Parameter{"x": {Default: &defaultValue, Choices: []string{"good"}}}}}},
	}
	for i, d := range cases {
		if err := Validate(d); err == nil {
			t.Fatalf("构造模型 %d 未被拒绝", i)
		}
	}
	d := &Document{Version: 1, Builds: map[string]*Build{"default": {Steps: []Step{{Kind: "run", Run: "echo"}}, Post: &Post{Always: []Step{{Kind: "run", Run: "cleanup"}}}}}}
	if err := Validate(d); err != nil {
		t.Fatal(err)
	}
	if err := Validate(d); err != nil {
		t.Fatal("重复校验不稳定", err)
	}
	if d.Builds["default"].Post.Always[0].Name != "post-always-run-1" {
		t.Fatal("收尾名称不稳定")
	}
}

func TestActualResourceBounds(t *testing.T) {
	var text strings.Builder
	text.WriteString(minimal + "env:\n")
	for i := 0; i < MaxNodes; i++ {
		fmt.Fprintf(&text, "  KEY_%d: value\n", i)
	}
	if _, err := Parse([]byte(text.String())); err == nil || !strings.Contains(err.Error(), "节点上限") {
		t.Fatalf("真实节点数量未限制: %v", err)
	}
	input := "version: 1\nsteps: " + strings.Repeat("[", MaxDepth+1) + "x" + strings.Repeat("]", MaxDepth+1)
	if _, err := Parse([]byte(input)); err == nil || !strings.Contains(err.Error(), "嵌套上限") {
		t.Fatalf("真实深度未限制: %v", err)
	}
	filename := filepath.Join(t.TempDir(), "large.yml")
	if err := os.WriteFile(filename, []byte(strings.Repeat(" ", MaxConfigBytes+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(filename); err == nil || !strings.Contains(err.Error(), "大小上限") {
		t.Fatalf("读取未限制: %v", err)
	}
}

func TestFullCollectionValidated(t *testing.T) {
	input := "version: 1\nbuilds:\n  selected: {steps: [{kind: run, run: echo}]}\n  unused: {steps: [{kind: run, run: 2}]}"
	if _, err := Parse([]byte(input)); err == nil {
		t.Fatal("未选择构建未通过完整校验")
	}
	for _, input := range []string{
		minimal + "notifications: {enabled: false, webhooks: []}",
		minimal + "params: {empty: {default: '', choices: ['']}}",
		"version: 1\nsteps: [{kind: upload, target: google_play, app_identifier: com.example.app, file: '{{channel}}/*.aab', credentials: '${CREDS}', track: named-testing-track}]",
		"version: 1\nbuilds:\n  中文: {steps: [{kind: run, run: echo}]}\nnotifications: {webhooks: [{type: generic, url: 'https://example.org/hook'}]}",
	} {
		if _, err := Parse([]byte(input)); err != nil {
			t.Fatal("合法配置未接受", err)
		}
	}
}

func FuzzParse(f *testing.F) {
	for _, input := range []string{minimal, "", "version: 1\nsteps: [{kind: run, run: 2}]", "version: 1\nsteps: [{kind: run, run: echo, notify: false}]", minimal + "params: {x: ''}"} {
		f.Add([]byte(input))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		d, err := Parse(data)
		if err == nil {
			if err := Validate(d); err != nil {
				t.Fatal("解析成功的模型未通过校验", err)
			}
		}
	})
}
