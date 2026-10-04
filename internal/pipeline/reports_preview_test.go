package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReportsPreviewRenderedPathBounds(t *testing.T) {
	longPath := strings.Repeat("a/", 385) + strings.Repeat("b", 250) + ".xml"
	for _, tc := range []struct {
		name, value string
		valid       bool
	}{
		{"1024字节", longPath, true}, {"1025字节", longPath + "x", false},
		{"255字节叶", strings.Repeat("a", 251) + ".xml", true},
		{"256字节叶", strings.Repeat("a", 252) + ".xml", false},
		{"多字节叶", strings.Repeat("报告", 42) + ".xml", false},
		{"绝对", "/REPORT_PRIVATE_VALUE.xml", false},
		{"盘符", "C:REPORT_PRIVATE_VALUE.xml", false},
		{"反斜杠", `results\REPORT_PRIVATE_VALUE.xml`, false},
		{"控制字符", "results/REPORT_PRIVATE_VALUE\n.xml", false},
		{"向上段", "results/../REPORT_PRIVATE_VALUE.xml", false},
		{"非法glob", "results/[REPORT_PRIVATE_VALUE.xml", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := document(t, `version: 1
params: {report_path: 'results/*.xml'}
reports: {junit: {paths: ['{{report_path}}']}}
steps: [{kind: run, name: first, run: 'exit 99'}]
`)
			_, err := Preview(d, PreviewOptions{Step: "first", Params: map[string]string{"report_path": tc.value}})
			if (err == nil) != tc.valid {
				t.Fatalf("有效性=%v，错误=%v", tc.valid, err)
			}
			if err != nil && strings.Contains(err.Error(), "REPORT_PRIVATE_VALUE") {
				t.Fatal("错误回显路径参数")
			}
		})
	}
}

func TestReportsPreviewValidatesWholeSelection(t *testing.T) {
	d := document(t, `version: 1
builds:
  first:
    params: {report_path: 'results/*.xml'}
    reports: {junit: {paths: ['{{report_path}}']}}
    steps:
      - {kind: run, name: first, run: 'exit 99'}
      - {kind: run, name: later, run: 'exit 99'}
  other:
    reports: {junit: {paths: ['{{REPORT_UNKNOWN_PRIVATE}}/*.xml']}}
    steps: [{kind: run, run: 'exit 99'}]
`)
	if _, err := Preview(d, PreviewOptions{All: true}); err == nil || strings.Contains(err.Error(), "REPORT_UNKNOWN_PRIVATE") {
		t.Fatalf("未校验全部报告模板：%v", err)
	}
	d.Builds["first"].Reports.JUnit.Paths = []string{"{{REPORT_UNKNOWN_PRIVATE}}/*.xml"}
	if _, err := Preview(d, PreviewOptions{Names: []string{"first"}, Step: "first"}); err == nil {
		t.Fatal("--step绕过报告校验")
	}
	d.Builds["first"].Reports.JUnit.Paths = []string{"results/*.xml"}
	d.Builds["first"].Steps[1].WorkingDir = "{{REPORT_UNKNOWN_PRIVATE}}"
	if _, err := Preview(d, PreviewOptions{Names: []string{"first"}, Step: "first"}); err == nil {
		t.Fatal("--step绕过未显示步骤模板")
	}
}

func TestReportsPreviewPendingSinglePassAndNoEffects(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	marker := filepath.Join(dir, "must-not-run")
	if err := os.Mkdir("results", 0700); err != nil {
		t.Fatal(err)
	}
	original := []byte("<invalid REPORT_SECRET_VALUE")
	if err := os.WriteFile("results/old.xml", original, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REPORT_HOST_SECRET", "REPORT_SECRET_VALUE")
	d := document(t, `version: 1
params: {segment: '${REPORT_HOST_SECRET}{{UNDECLARED_INSERTED}}'}
env: {DECLARED: '${REPORT_HOST_SECRET}'}
reports: {junit: {paths: ['results/{{segment}}/{{build.id}}/*.xml', 'results/*.xml']}}
steps: [{kind: run, name: first, run: 'exit 99'}]
`)
	d.Builds["default"].Steps[0].Run = "touch " + marker
	before, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := Preview(d, PreviewOptions{})
	if err != nil || plan.Builds[0].Condition != "pending" || !plan.Builds[0].Reports {
		t.Fatalf("合法未确定报告事实应pending：%+v %v", plan, err)
	}
	plan, err = Preview(d, PreviewOptions{Facts: map[string]string{"build.id": "REPORT_FACT_VALUE"}})
	if err != nil || plan.Builds[0].Condition != "ready" {
		t.Fatalf("插入文本被再次解释：%+v %v", plan, err)
	}
	output, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"REPORT_HOST_SECRET", "REPORT_SECRET_VALUE", "REPORT_FACT_VALUE", "UNDECLARED_INSERTED", marker, "results/"} {
		if strings.Contains(string(output), value) {
			t.Fatal("预览泄露报告/参数/上下文正文")
		}
	}
	after, err := json.Marshal(d)
	if err != nil || string(before) != string(after) {
		t.Fatal("预览修改输入")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("预览启动了脚本")
	}
	current, err := os.ReadFile("results/old.xml")
	if err != nil || string(current) != string(original) {
		t.Fatal("预览修改原报告")
	}
}

func TestReportsPreviewSkippedConditionStillValidates(t *testing.T) {
	d := document(t, `version: 1
params: {report_path: 'results/*.xml'}
when: {branches: [main]}
reports: {junit: {paths: ['{{report_path}}/{{build.id}}/*.xml']}}
steps: [{kind: run, name: first, run: 'exit 99'}]
`)
	// 明确分支不匹配优先于报告模板待定，不能把pending当when false。
	p, err := Preview(d, PreviewOptions{Step: "first", Facts: map[string]string{"git.branch": "feature"}})
	if err != nil || p.Builds[0].Condition != "skipped" {
		t.Fatalf("已知when false组合错误：%+v %v", p, err)
	}
	// 整个build跳过仍校验完整报告模板，不把越界当成待定事实。
	d.Builds["default"].Reports.JUnit.Paths = []string{"{{report_path}}"}
	_, err = Preview(d, PreviewOptions{Step: "first", Facts: map[string]string{"git.branch": "feature"}, Params: map[string]string{"report_path": "results/../REPORT_PRIVATE_VALUE.xml"}})
	if err == nil || strings.Contains(err.Error(), "REPORT_PRIVATE_VALUE") {
		t.Fatalf("跳过build绕过路径校验：%v", err)
	}
}
