package pipeline

import (
	"encoding/json"
	"mybuilds/internal/config"
	"strings"
	"testing"
)

func TestIOSPreviewOnlyDeclaredValuesAndSystemPending(t *testing.T) {
	const text = `version: 1
params:
  bundle: {default: com.example.preview}
  method: {default: release-testing}
ios_signing: {p12: '${PREVIEW_P12}', profile: '${PREVIEW_PROFILE}', password: '${PREVIEW_PASSWORD}', bundle_id: '{{bundle}}', export_method: '{{method}}'}
steps:
  - {kind: run, run: 'echo safe'}
  - {kind: artifact, paths: ['{{ios.output_dir}}/*.ipa']}
`
	doc, err := config.Parse([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PREVIEW_PASSWORD", "PRIVATE_PREVIEW_PASSWORD")
	t.Setenv("PREVIEW_P12", "PRIVATE_MATERIAL_PATH")
	plan, err := Preview(doc, PreviewOptions{Facts: map[string]string{"ios.output_dir": "PRIVATE_FORGED_OUTPUT"}})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(plan)
	if len(plan.Builds[0].Steps) != 2 || plan.Builds[0].Steps[1].Condition != "pending" || strings.Contains(string(data), "PRIVATE_") || strings.Contains(string(data), "PREVIEW_P12") {
		t.Fatal("纯预览接受外部系统事实或泄露原配置")
	}
	for _, params := range []map[string]string{{"bundle": "PRIVATE_BAD_BUNDLE"}, {"method": "PRIVATE_BAD_METHOD"}, {"bundle": "{{workspace}}"}, {"method": "{{method}}"}} {
		_, err = Preview(doc, PreviewOptions{Params: params})
		if err == nil || strings.Contains(err.Error(), "PRIVATE_") {
			t.Fatal("渲染后非法字面量被接受或泄露")
		}
	}
	_, err = Preview(doc, PreviewOptions{Params: map[string]string{"bundle": "com.example.valid", "method": "app-store-connect"}})
	if err != nil {
		t.Fatal("合法签名单次渲染被拒", err)
	}
	noSigning, err := config.Parse([]byte("version: 1\nsteps: [{kind: artifact, paths: ['{{ios.output_dir}}/*.ipa']}]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Preview(noSigning, PreviewOptions{}); err == nil {
		t.Fatal("没有实际签名生成者却接受系统目录")
	}
}
