package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mybuilds/internal/config"
)

func TestFlutterScopedParametersPurePreview(t *testing.T) {
	document, err := config.Parse([]byte(`version: 1
builds:
  android:
    runner: {platform: android, framework: flutter}
    params: {version: "1.0.0", build_number: "1", application_id: {required: true}}
    env: {ID: "{{application_id}}", VERSION: "{{version}}"}
    steps: [{kind: run, run: "echo literal"}]
  generic:
    params: {version: "1.0.0", identity: {required: true}}
    env: {ID: "{{identity}}", VERSION: "{{version}}"}
    steps: [{kind: run, run: "echo literal"}]
`))
	if err != nil {
		t.Fatal(err)
	}
	options := PreviewOptions{All: true, Params: map[string]string{"version": "2.3.4"}, BuildParams: map[string]map[string]string{"android": {"application_id": "private-app", "version": "3.4.5"}, "generic": {"identity": "private-other"}}}
	preview, err := Preview(document, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Builds) != 2 || preview.Builds[0].Platform != "android" || preview.Builds[0].Framework != "flutter" {
		t.Fatal(preview)
	}
	// 不修改共享 map，也不把一个 build 的必填字段送进另一份 ResolveParams。
	if options.Params["version"] != "2.3.4" || len(options.Params) != 1 {
		t.Fatal("参数合并改变调用方")
	}
	for _, invalid := range []PreviewOptions{
		{Names: []string{"android"}, BuildParams: map[string]map[string]string{"generic": {"identity": "x"}}},
		{All: true, BuildParams: map[string]map[string]string{"absent": {"version": "1"}}},
		{All: true, Params: map[string]string{"version": "1.2"}, BuildParams: map[string]map[string]string{"android": {"application_id": "x"}, "generic": {"identity": "y"}}},
	} {
		if _, e := Preview(document, invalid); e == nil {
			t.Fatal("非法scope或标准值未整批拒绝")
		}
	}
}

func TestFlutterLastInvalidParameterPreventsAllScripts(t *testing.T) {
	root := t.TempDir()
	document, err := config.Parse([]byte(`version: 1
builds:
  first:
    steps: [{kind: run, run: "touch forbidden"}]
  last:
    runner: {platform: android, framework: flutter}
    params: {version: "broken", build_number: "1"}
    steps: [{kind: run, run: "touch forbidden"}]
`))
	if err != nil {
		t.Fatal(err)
	}
	result, err := Run(context.Background(), document, RunOptions{PreviewOptions: PreviewOptions{All: true}, Workspace: root})
	if err == nil || result != nil {
		t.Fatal("整批参数预检未拒绝")
	}
	precheckErr := err
	if _, err = os.Stat(filepath.Join(root, "forbidden")); !os.IsNotExist(err) {
		t.Fatal("非法后续build之前已执行脚本")
	}
	if strings.Contains(precheckErr.Error(), "broken") {
		t.Fatal("参数值进入错误")
	}
}

func TestFlutterToolPrecheckBeforeEveryScript(t *testing.T) {
	root := t.TempDir()
	document, err := config.Parse([]byte(`version: 1
builds:
  first:
    steps: [{kind: run, run: "touch forbidden"}]
  last:
    runner: {platform: android, framework: flutter}
    env: {PATH: ""}
    steps: [{kind: run, run: "touch forbidden"}]
`))
	if err != nil {
		t.Fatal(err)
	}
	result, err := Run(context.Background(), document, RunOptions{PreviewOptions: PreviewOptions{All: true}, Workspace: root})
	if result != nil || err == nil || !strings.Contains(err.Error(), "flutter") {
		t.Fatalf("工具整批预检: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "forbidden")); !os.IsNotExist(err) {
		t.Fatal("预检之前执行了脚本")
	}
}

func TestFlutterSkippedDoesNotCheckTools(t *testing.T) {
	document, err := config.Parse([]byte(`version: 1
runner: {platform: android, framework: flutter}
params: {enabled: "no"}
when: {params: {enabled: "yes"}}
env: {PATH: ""}
steps: [{kind: run, run: "exit 0"}]
`))
	if err != nil {
		t.Fatal(err)
	}
	result, err := Run(context.Background(), document, RunOptions{Workspace: t.TempDir()})
	if err != nil || result == nil || result.ResultDir != "" || result.Builds[0].Status != "skipped" {
		t.Fatal(result, err)
	}
}
