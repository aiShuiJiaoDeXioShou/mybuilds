package store

import (
	"slices"
	"testing"
	"time"

	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
)

func retentionProjectForBuild(t *testing.T, s *Store, id string) string {
	t.Helper()
	var b buildRecord
	if err := s.db.First(&b, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	return b.ProjectID
}
func TestRetentionProtectionExecutionAndIndependentStop(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 1, Days: 1}); err != nil {
			t.Fatal(err)
		}
		a, g, _ := claimed(t, s)
		project := retentionProjectForBuild(t, s, g.Ref.BuildID)
		accept(t, s, a, event(g.Ref, 1, stepProgress("intent", "", "ordinary", "compile", 1)))
		p := stepProgress("started", "", "ordinary", "compile", 1)
		p.Started = true
		accept(t, s, a, event(g.Ref, 2, p))
		running, err := s.EvaluateRetention(testContext, localAdmin, project, Page{})
		if err != nil {
			t.Fatal(err)
		}
		if e := retentionEntry(t, running, g.Ref.BuildID); !slices.Contains(e.ProtectReasons, "active") || !slices.Contains(e.ProtectReasons, "execution_unconfirmed") {
			t.Fatal("真实未确认执行未保护", e)
		}
		// 真实节点权撤销产生中断/停止保护，而非手改业务状态。
		if err = s.SetNodeState(testContext, localAdmin, "linux", "disabled"); err != nil {
			t.Fatal(err)
		}
		var row buildRecord
		s.db.First(&row, "id = ?", g.Ref.BuildID)
		original := *row.TerminalAt
		at := original.Add(48 * time.Hour)
		held := retentionEvaluateAt(t, s, project, at, Page{Limit: 200})
		e := retentionEntry(t, held, g.Ref.BuildID)
		if !e.Candidate || !slices.Contains(e.ProtectReasons, "stop_unconfirmed") || !slices.Contains(e.ProtectReasons, "resource_unconfirmed") {
			t.Fatal("实际停止/旧资源保护", e)
		}
		if err = s.ConfirmStopped(testContext, localAdmin, protocol.StopConfirmation{Ref: g.Ref, EvidenceCode: "admin_observed_stopped", Note: "独立实证夹具"}); err != nil {
			t.Fatal(err)
		}
		stopped := retentionEvaluateAt(t, s, project, at, Page{Limit: 200})
		e = retentionEntry(t, stopped, g.Ref.BuildID)
		if slices.Contains(e.ProtectReasons, "stop_unconfirmed") || slices.Contains(e.ProtectReasons, "execution_unconfirmed") || !slices.Contains(e.ProtectReasons, "ownership_unknown") || !e.TerminalAt.Equal(original) {
			t.Fatal("独立stop未解除执行或误解除归属/刷新时间", e)
		}
	})
}
func TestRetentionProtectionZeroActionReportAndSealedFiles(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 1, Days: 1}); err != nil {
			t.Fatal(err)
		}
		a, g := queuedReports(t, s, true, []string{"results/*.xml"}, false)
		p := protocol.ExecutionProgress{Kind: "build_finished", Status: "failed", Reason: "precheck_error", PostPhase: "none", StopConfirmed: true, RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}
		accept(t, s, a, event(g.Ref, 1, p))
		page, err := s.EvaluateRetention(testContext, localAdmin, retentionProjectForBuild(t, s, g.Ref.BuildID), Page{})
		if err != nil {
			t.Fatal(err)
		}
		if e := retentionEntry(t, page, g.Ref.BuildID); slices.Contains(e.ProtectReasons, "reports_unconfirmed") || slices.Contains(e.ProtectReasons, "execution_unconfirmed") {
			t.Fatal("合法零动作revision0永久保护", e)
		}
	})
	stores(t, func(t *testing.T, s *Store, _ Options) {
		if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 1, Days: 1}); err != nil {
			t.Fatal(err)
		}
		_, last := completeReportsTerminal(t, s)
		page, err := s.EvaluateRetention(testContext, localAdmin, retentionProjectForBuild(t, s, last.Ref.BuildID), Page{})
		if err != nil {
			t.Fatal(err)
		}
		if e := retentionEntry(t, page, last.Ref.BuildID); slices.Contains(e.ProtectReasons, "reports_unconfirmed") || slices.Contains(e.ProtectReasons, "artifacts_unconfirmed") || slices.Contains(e.ProtectReasons, "execution_unconfirmed") {
			t.Fatal("完整真实报告/receipt误保护", e)
		}
		// 真实已封存历史的元数据丢失必须闭锁；不读取或猜测文件系统。
		if err = s.writer.Model(&artifactRecord{}).Where("build_id = ?", last.Ref.BuildID).Update("verified_junit_json", "{}").Error; err != nil {
			t.Fatal(err)
		}
		page, err = s.EvaluateRetention(testContext, localAdmin, retentionProjectForBuild(t, s, last.Ref.BuildID), Page{})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(retentionEntry(t, page, last.Ref.BuildID).ProtectReasons, "reports_unconfirmed") {
			t.Fatal("缺真实解析证据仍被当确认")
		}
	})
}
