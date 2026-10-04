package pipeline

import (
	"context"
	"fmt"
	"testing"
)

func TestReportRunFailureBlocksSentinelAndPreservesCommand(t *testing.T) {
	for _, exit := range []int{0, 7} {
		t.Run(fmt.Sprintf("exit%d", exit), func(t *testing.T) {
			work := t.TempDir()
			d := localDocument(t, fmt.Sprintf(`version: 1
reports: {junit: {paths: ['results/*.xml']}}
steps:
  - kind: run
    name: prepare
    run: mkdir results
  - kind: run
    name: test
    run: |
      printf '<testsuite tests="1" failures="1"><testcase name="failure"><failure message="actual mismatch"/></testcase></testsuite>' > results/report.xml
      exit %d
  - kind: run
    name: sentinel
    run: printf unexpected > sentinel
post:
  success:
    - kind: run
      name: success
      run: printf unexpected > success
  failure:
    - kind: run
      name: observed-failure
      run: printf actual > failure
  always:
    - kind: run
      name: diagnostic
      run: printf '<testsuite tests="0"/>' > results/report.xml
`, exit))
			result, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: work})
			if err == nil || result == nil || len(result.Builds) != 1 {
				t.Fatalf("缺少真实失败结果: %v", err)
			}
			b := result.Builds[0]
			want := "report_failed"
			if exit == 7 {
				want = "exit"
			}
			if b.Status != "failed" || b.Reason != want || b.Reports == nil || !b.Reports.Sealed || b.Reports.Counts.Failures != 1 {
				t.Fatalf("原失败和报告不一致: %+v", b)
			}
			if len(b.Steps) != 3 || !b.Steps[0].started || !b.Steps[1].started || b.Steps[1].ExitCode != exit || b.Steps[2].started {
				t.Fatal("动作启动事实错误")
			}
			requireAbsent(t, work, "sentinel")
			requireAbsent(t, work, "success")
			requireFile(t, work, "failure", "actual")
		})
	}
}

func TestReportRunZeroActionsDoNotScanOrCreateSeal(t *testing.T) {
	work := t.TempDir()
	// 匹配超大旧文件；如果错误地建立基线就会失败。
	reportScript(t, work, `dd if=/dev/zero of=old.xml bs=1024 count=8193 2>/dev/null`)
	d := localDocument(t, `version: 1
params: {do_it: no}
reports: {junit: {paths: ['*.xml']}}
steps:
  - kind: run
    name: skipped
    when: {params: {do_it: yes}}
    run: printf unexpected > sentinel
`)
	result, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: work})
	if err != nil || result == nil || result.ResultDir != "" || len(result.Builds) != 1 || result.Builds[0].Status != "skipped" || result.Builds[0].Reports != nil || result.Builds[0].ReportSealDigest != "" {
		t.Fatalf("零动作不应扫描或造seal: %+v %v", result, err)
	}
	requireAbsent(t, work, "sentinel")
}
