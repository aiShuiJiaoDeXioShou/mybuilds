package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mybuilds/internal/protocol"
)

// 具体回调把真实Run进度写入私有普通文件；不替代中央Store或HTTP验收。
func reportEventFile(t *testing.T) (*os.File, func(context.Context, protocol.ExecutionProgress) error) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(t.TempDir(), "events.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f, func(ctx context.Context, p protocol.ExecutionProgress) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		data, err := json.Marshal(p)
		if err != nil {
			return err
		}
		if _, err = f.Write(append(data, '\n')); err != nil {
			return err
		}
		return f.Sync()
	}
}

const reportBudgetSource = `version: 1
reports: {junit: {paths: ['result.xml']}}
steps:
 - kind: run
   name: tests
   run: printf '<testsuite tests="1"><testcase/></testsuite>' > result.xml
 - kind: run
   name: sentinel
   run: touch sentinel
post:
 always:
  - kind: run
    name: cleanup
    run: printf actual > always
`

func TestReportRunRemoteBudgetBoundsCheckpointPersistence(t *testing.T) {
	work := t.TempDir()
	remote := remoteOptions(t)
	budget := int64(600 * time.Millisecond)
	remote.RemainingBudgetNS = &budget
	_, persist := reportEventFile(t)
	checked := false
	remote.Progress = func(ctx context.Context, p protocol.ExecutionProgress) error {
		if err := persist(ctx, p); err != nil {
			return err
		}
		if p.Kind == "reports_checked" {
			checked = true
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) > time.Duration(budget) {
				t.Error("报告回执没有原普通预算期限")
			}
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	}
	start := time.Now()
	result, err := runWithCleanup(t, context.Background(), localDocument(t, reportBudgetSource), RunOptions{Workspace: work, Remote: remote})
	if err == nil || result == nil || !checked || time.Since(start) > 3*time.Second {
		t.Fatal("真实报告回执未有界结束", err)
	}
	b := result.Builds[0]
	if b.Status != "failed" || b.Reason != "timeout" {
		t.Fatalf("耗尽原普通预算应保留timeout: %s/%s", b.Status, b.Reason)
	}
	if b.Reports == nil || b.Reports.Sealed || b.ReportSealDigest != "" || b.Steps[0].CleanupFailed {
		t.Fatal("未确认报告不得伪seal或物理清理失败")
	}
	requireAbsent(t, work, "sentinel")
	requireAbsent(t, work, "always")
}

func TestReportRunAuthorityAndSaveFailureClosePost(t *testing.T) {
	for _, mode := range []string{"authority", "save"} {
		t.Run(mode, func(t *testing.T) {
			work := t.TempDir()
			remote := remoteOptions(t)
			authority, cancel := context.WithCancel(context.Background())
			defer cancel()
			remote.AuthorityContext = authority
			f, persist := reportEventFile(t)
			checked := false
			remote.Progress = func(ctx context.Context, p protocol.ExecutionProgress) error {
				if p.Kind == "reports_checked" {
					checked = true
					if mode == "authority" {
						cancel()
					} else {
						_ = f.Close()
					}
				}
				return persist(ctx, p)
			}
			result, err := runWithCleanup(t, context.Background(), localDocument(t, reportBudgetSource), RunOptions{Workspace: work, Remote: remote})
			if err == nil || result == nil || !checked {
				t.Fatal("报告失权或实际保存失败未失败")
			}
			b := result.Builds[0]
			if b.ReportSealDigest != "" || b.Reports == nil || b.Reports.Sealed || b.Steps[0].CleanupFailed {
				t.Fatal("报告闭锁不能伪造seal/cleanup")
			}
			requireAbsent(t, work, "sentinel")
			requireAbsent(t, work, "always")
		})
	}
}

func TestReportRunUserCancelCollectsWithinLiveAuthority(t *testing.T) {
	work := t.TempDir()
	remote := remoteOptions(t)
	budget := int64(3 * time.Second)
	remote.RemainingBudgetNS = &budget
	_, persist := reportEventFile(t)
	remote.Progress = persist
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	remote.Log = func(_ context.Context, p protocol.LogRecord) error {
		if strings.Contains(p.Text, "report-ready") {
			cancel()
		}
		return nil
	}
	source := strings.Replace(reportBudgetSource, `run: printf '<testsuite tests="1"><testcase/></testsuite>' > result.xml`, `run: printf '<testsuite tests="1"><testcase/></testsuite>' > result.xml; printf 'report-ready\n'; sleep 60`, 1)
	result, err := runWithCleanup(t, ctx, localDocument(t, source), RunOptions{Workspace: work, Remote: remote})
	if err == nil || result == nil {
		t.Fatal("取消缺少结果")
	}
	b := result.Builds[0]
	if b.Status != "cancelled" || b.Reason != "cancelled" || b.Reports == nil || !b.Reports.Sealed || b.Reports.Counts.Tests != 1 || b.ReportSealDigest == "" || b.Steps[0].CleanupFailed {
		t.Fatalf("有效运行权下取消应有限封存原报告: %+v", b)
	}
	requireAbsent(t, work, "sentinel")
	requireFile(t, work, "always", "actual")
}

func TestReportRunBaselineTimeoutStartsNoUserAction(t *testing.T) {
	work := t.TempDir()
	payload := make([]byte, 1<<20)
	for i := 0; i < 64; i++ {
		if err := os.WriteFile(filepath.Join(work, fmt.Sprintf("baseline-%d.xml", i)), payload, 0600); err != nil {
			t.Fatal(err)
		}
	}
	source := `version: 1
timeout: 1ns
reports: {junit: {paths: ['*.xml']}}
steps:
 - kind: run
   run: touch sentinel
post:
 always:
  - kind: run
    run: touch always
`
	result, err := runWithCleanup(t, context.Background(), localDocument(t, source), RunOptions{Workspace: work})
	if err == nil || result == nil || result.Builds[0].Reason != "timeout" || result.Builds[0].Reports != nil || result.Builds[0].ReportSealDigest != "" {
		t.Fatal("耗尽普通预算仍建立报告或执行", err)
	}
	requireAbsent(t, work, "sentinel")
	requireAbsent(t, work, "always")
}
