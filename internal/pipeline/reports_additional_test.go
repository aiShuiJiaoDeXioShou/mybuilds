package pipeline

import (
	"context"
	"crypto/sha256"
	"mybuilds/internal/config"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mybuilds/internal/protocol"
)

func TestReportCollectionRequiredEachPatternKeepsActualCounts(t *testing.T) {
	work, wr, dr := reportRoots(t)
	c, err := newReportCollection(context.Background(), wr, dr, []string{"results/*.xml", "missing/*.xml"}, true, nil, config.DefaultJUnitMaxFiles)
	if err != nil {
		t.Fatal(err)
	}
	reportScript(t, work, `mkdir results; printf '<testsuite tests="1"><testcase/></testsuite>' > results/actual.xml`)
	checked, _ := reportCheck(t, c, 1, false)
	if checked.Outcome != "pending" || checked.Counts.Tests != 1 {
		t.Fatal("prepare阶段缺一模式不应立即missing")
	}
	final, files := reportCheck(t, c, 0, true)
	if final.Outcome != "failed" || final.Reason != "report_missing" || final.Counts.Tests != 1 || len(files) != 1 {
		t.Fatal("required必须核每个模式，保留实际合法计数")
	}
}

func TestReportRunSelectedPreparationKeepsOldXMLExcluded(t *testing.T) {
	work := t.TempDir()
	filename := filepath.Join(work, "old.xml")
	original := []byte(`<testsuite tests="1"><testcase/></testsuite>`)
	if err := os.WriteFile(filename, original, 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(filename)
	if err != nil {
		t.Fatal(err)
	}
	d := localDocument(t, `version: 1
reports: {junit: {paths: ['old.xml']}}
steps:
 - kind: run
   name: prepare
   run: touch prepared
 - kind: run
   name: tests
   run: printf '<testsuite tests="0"/>' > old.xml; touch forbidden
`)
	result, err := runWithCleanup(t, context.Background(), d, RunOptions{PreviewOptions: PreviewOptions{Step: "prepare"}, Workspace: work})
	if err == nil || result == nil {
		t.Fatal("只运行prepare不能消费旧报告")
	}
	b := result.Builds[0]
	if b.Reason != "report_missing" || b.Reports == nil || !b.Reports.Sealed || b.Reports.Counts.Tests != 0 || len(b.Reports.Files) != 0 {
		t.Fatal("旧报告被计为当前报告")
	}
	requireAbsent(t, work, "forbidden")
	after, err := os.Stat(filename)
	data, readErr := os.ReadFile(filename)
	if err != nil || readErr != nil || !os.SameFile(before, after) || sha256.Sum256(data) != sha256.Sum256(original) || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("读取基线不应删除或改写旧源")
	}
}

func TestReportRunPostFailureAndCancelPreserveOriginalReportReason(t *testing.T) {
	for _, mode := range []string{"exit", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			work := t.TempDir()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			remote := remoteOptions(t)
			_, persist := reportEventFile(t)
			remote.Progress = persist
			remote.Log = func(_ context.Context, record protocol.LogRecord) error {
				if strings.Contains(record.Text, "post-ready") {
					cancel()
				}
				return nil
			}
			post := "exit 9"
			if mode == "cancel" {
				post = "printf 'post-ready\\n'; sleep 60"
			}
			d := localDocument(t, `version: 1
reports: {junit: {paths: ['result.xml']}}
steps:
 - kind: run
   name: tests
   run: printf '<testsuite tests="1" failures="1"><testcase><failure/></testcase></testsuite>' > result.xml
post:
 failure:
  - kind: run
    name: failure
    run: `+post+`
 always:
  - kind: run
    name: cleanup
    run: printf actual > always
`)
			result, err := runWithCleanup(t, ctx, d, RunOptions{Workspace: work, Remote: remote})
			if err == nil || result == nil {
				t.Fatal("缺少原失败")
			}
			b := result.Builds[0]
			if b.Status != "failed" || b.Reason != "report_failed" || b.Reports == nil || !b.Reports.Sealed || b.Reports.Counts.Failures != 1 || len(b.Post) != 2 || !b.Post[0].started || b.Post[0].Status == "succeeded" || b.Post[0].CleanupFailed {
				t.Fatal("post自身失败或取消覆盖了普通封存结论")
			}
			requireFile(t, work, "always", "actual")
		})
	}
}

func TestReportRunActualLogSaveFailureClosesReportAndAlways(t *testing.T) {
	work := t.TempDir()
	output, err := os.OpenFile(filepath.Join(t.TempDir(), "terminal.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err = output.Close(); err != nil {
		t.Fatal(err)
	}
	d := localDocument(t, `version: 1
reports: {junit: {paths: ['result.xml']}}
steps:
 - kind: run
   name: tests
   run: printf '<testsuite tests="1"><testcase/></testsuite>' > result.xml; printf actual-log
post:
 always:
  - kind: run
    run: touch forbidden-always
`)
	result, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: work, Output: output})
	if err == nil || result == nil {
		t.Fatal("实际closed日志文件错误未失败")
	}
	b := result.Builds[0]
	if b.Reason != "log_error" || b.Reports != nil || b.ReportSealDigest != "" || !b.Steps[0].started || b.Steps[0].CleanupFailed {
		t.Fatal("日志保存失败不得伪造报告或物理清理失败")
	}
	requireAbsent(t, work, "forbidden-always")
}

func TestReportRunOriginalExitSurvivesActualProgressSaveError(t *testing.T) {
	work := t.TempDir()
	remote := remoteOptions(t)
	file, persist := reportEventFile(t)
	remote.Progress = func(ctx context.Context, p protocol.ExecutionProgress) error {
		if p.Kind == "reports_checked" {
			_ = file.Close()
		}
		return persist(ctx, p)
	}
	source := strings.Replace(reportBudgetSource, " > result.xml", " > result.xml; exit 7", 1)
	result, err := runWithCleanup(t, context.Background(), localDocument(t, source), RunOptions{Workspace: work, Remote: remote})
	if err == nil || result == nil {
		t.Fatal("缺少实际exit失败")
	}
	b := result.Builds[0]
	if b.Status != "failed" || b.Reason != "exit" || b.Steps[0].ExitCode != 7 || b.Steps[0].CleanupFailed || b.ReportSealDigest != "" {
		t.Fatal("报告进度实际保存错误覆盖了原命令原因")
	}
	requireAbsent(t, work, "sentinel")
	requireAbsent(t, work, "always")
}

// 报告路径是build层事实，不能借私有workspace拼成服务端无法核对的相对路径。
func TestReportRunRemoteWorkspaceFactRejectedBeforeActions(t *testing.T) {
	work := t.TempDir()
	document := localDocument(t, `version: 1
reports: {junit: {paths: ['prefix{{workspace}}/result.xml']}}
steps:
 - kind: run
   name: tests
   run: touch forbidden
`)
	result, err := runWithCleanup(t, context.Background(), document, RunOptions{Workspace: work, Remote: remoteOptions(t)})
	if err == nil || result != nil {
		t.Fatal("私有报告路径事实必须在任何用户动作前拒绝")
	}
	requireAbsent(t, work, "forbidden")
}
