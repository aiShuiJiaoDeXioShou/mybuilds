package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mybuilds/internal/protocol"
)

// 运行真实脚本、collector和报告封存；本门不声称完成 APK/IPA 签名构建。
func TestFlutterActualToolsRunFrozenNumberAndReports(t *testing.T) {
	sdk := os.Getenv("MYBUILDS_FLUTTER_REAL_SDK")
	if sdk == "" {
		t.Skip("未声明已初始化实际 Flutter SDK")
	}
	t.Setenv("FLUTTER_TEST_SDK", sdk)
	for _, remoteMode := range []bool{false, true} {
		t.Run(map[bool]string{false: "local", true: "remote"}[remoteMode], func(t *testing.T) {
			root := t.TempDir()
			document := localDocument(t, `version: 1
runner: {platform: android, framework: flutter}
params: {version: "2.3.4", channel: "internal", flavor: "", build_number: "7"}
env:
  FLUTTER_ROOT: "${FLUTTER_TEST_SDK}"
  APP_VERSION: "{{version}}"
  BUILD_NUMBER: "{{build.number}}"
  APP_CHANNEL: "{{channel}}"
  APP_FLAVOR: "{{flavor}}"
reports: {junit: {paths: [test.xml]}}
steps:
  - kind: run
    name: prepare
    run: |
      printf '%s/%s/%s/%s' "$APP_VERSION" "$BUILD_NUMBER" "$APP_CHANNEL" "$APP_FLAVOR" > values
      printf '<testsuite tests="1"><testcase name="actual"/></testsuite>' > test.xml
  - kind: artifact
    name: collect
    paths: [values]
post:
  always:
    - kind: run
      name: change-working-tree
      run: printf '<testsuite tests="0"/>' > test.xml
`)
			options := RunOptions{Workspace: root}
			var final protocol.ExecutionProgress
			if remoteMode {
				options.Remote = remoteOptions(t)
				options.Remote.Facts = map[string]string{"build.number": "99", "git.sha": strings.Repeat("a", 40)}
				options.Remote.Secrets = map[string]string{"FLUTTER_TEST_SDK": sdk}
				options.Remote.Progress = func(_ context.Context, p protocol.ExecutionProgress) error {
					if p.Kind == "build_finished" {
						final = p
					}
					return nil
				}
			}
			result, err := runWithCleanup(t, context.Background(), document, options)
			if err != nil || result == nil || result.Builds[0].Status != "succeeded" {
				t.Fatal(result, err)
			}
			number := "7"
			if remoteMode {
				number = "99"
			}
			requireFile(t, root, "values", "2.3.4/"+number+"/internal/")
			build := result.Builds[0]
			if build.Reports == nil || build.Reports.Counts.Tests != 1 || build.ReportSealDigest == "" || len(build.Steps[1].Artifacts) != 1 {
				t.Fatal("报告/完整快照缺失")
			}
			snapshot := filepath.Join(result.ResultDir, build.Steps[1].Artifacts[0].SnapshotPath)
			value, e := os.ReadFile(snapshot)
			if e != nil || string(value) != "2.3.4/"+number+"/internal/" {
				t.Fatal("完整实际 collector 快照", e)
			}
			if remoteMode && (final.ReportManifest == nil || final.ReportManifest.SealDigest != build.ReportSealDigest) {
				t.Fatal("终态没有原封存证据")
			}
		})
	}
}
