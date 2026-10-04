package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// 使用真实Run和脚本，报告成功必须来自本次生成的不可变快照。
func TestReportRunConfiguredSnapshot(t *testing.T) {
	work := t.TempDir()
	d := localDocument(t, `version: 1
reports: {junit: {paths: ['result.xml']}}
steps:
  - kind: run
    name: tests
    run: printf '<testsuite tests="1"><testcase name="actual" time="0.125"/></testsuite>' > result.xml
post:
  always:
    - kind: run
      name: diagnostic
      run: printf '<testsuite tests="0"/>' > result.xml
`)
	result, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: work})
	if err != nil {
		t.Fatalf("真实报告流水线应成功: %v", err)
	}
	if result == nil || len(result.Builds) != 1 || result.Builds[0].Reports == nil {
		t.Fatal("缺少报告")
	}
	b := result.Builds[0]
	if b.Status != "succeeded" || !b.Reports.Sealed || b.Reports.Counts.Tests != 1 || b.Reports.Counts.DurationNS != 125000000 || len(b.Reports.Files) != 1 || b.ReportSealDigest == "" {
		t.Fatalf("报告不完整: %+v", b)
	}
	source, err := os.ReadFile(filepath.Join(work, "result.xml"))
	if err != nil || string(source) != `<testsuite tests="0"/>` {
		t.Fatal("post没有真实改写源")
	}
}
