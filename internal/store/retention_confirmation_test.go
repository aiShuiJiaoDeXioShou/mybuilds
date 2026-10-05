package store

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
)

func TestRetentionConfirmationArtifactsLogsAndReceipt(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 1, Days: 1}); err != nil {
			t.Fatal(err)
		}
		a, g, ids := artifactFixture(t, s)
		project := retentionProjectForBuild(t, s, g.Ref.BuildID)
		page, err := s.EvaluateRetention(testContext, localAdmin, project, Page{})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(retentionEntry(t, page, g.Ref.BuildID).ProtectReasons, "artifacts_unconfirmed") {
			t.Fatal("实际完整collector尚未上传不保护")
		}
		for i, id := range ids {
			_, err = s.CommitArtifact(testContext, a, ArtifactCommit{Declaration: protocol.ArtifactDeclaration{Ref: g.Ref, ID: id, Seq: int64(i + 1), Phase: "ordinary", Step: "package", Index: 1, Name: "app.apk", Size: 7, SHA256: strings.Repeat("a", 64)}, StorageID: uuid.NewString()})
			if err != nil {
				t.Fatal(err)
			}
			page, err = s.EvaluateRetention(testContext, localAdmin, project, Page{})
			if err != nil {
				t.Fatal(err)
			}
			if slices.Contains(retentionEntry(t, page, g.Ref.BuildID).ProtectReasons, "artifacts_unconfirmed") != (i == 0) {
				t.Fatal("确认部分/完整副本未重新评估")
			}
		}
		log := LogCommit{Ref: g.Ref, Seq: 1, Offset: 0, Size: 10, Digest: strings.Repeat("b", 64), StorageID: uuid.NewString(), RecordCount: 1}
		if _, err = s.CommitLogChunk(testContext, a, log); err != nil {
			t.Fatal(err)
		}
		terminal := protocol.ExecutionProgress{Kind: "build_finished", Status: "succeeded", Started: true, StopConfirmed: true, RemainingPostBudgetNS: int64(2 * time.Minute), LastLogSeq: 1, LastLogOffset: 10, LastArtifactSeq: 2, ArtifactSteps: []protocol.ArtifactExpectation{{Phase: "ordinary", Index: 1, Count: 2, IDs: ids}}}
		accept(t, s, a, event(g.Ref, 6, terminal))
		page, err = s.EvaluateRetention(testContext, localAdmin, project, Page{})
		if err != nil {
			t.Fatal(err)
		}
		e := retentionEntry(t, page, g.Ref.BuildID)
		for _, reason := range []string{"execution_unconfirmed", "artifacts_unconfirmed", "logs_unconfirmed"} {
			if slices.Contains(e.ProtectReasons, reason) {
				t.Fatal("真实完整manifest/cursors被误判", e)
			}
		}
		if err = s.writer.Model(&logChunkRecord{}).Where("build_id = ?", g.Ref.BuildID).Update("offset", int64(1)).Error; err != nil {
			t.Fatal(err)
		}
		page, err = s.EvaluateRetention(testContext, localAdmin, project, Page{})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(retentionEntry(t, page, g.Ref.BuildID).ProtectReasons, "logs_unconfirmed") {
			t.Fatal("日志真实cursor元数据缺失未保护")
		}
		if err = s.writer.Where("build_id = ? AND seq = ?", g.Ref.BuildID, 1).Delete(&executionReceiptRecord{}).Error; err != nil {
			t.Fatal(err)
		}
		page, err = s.EvaluateRetention(testContext, localAdmin, project, Page{})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(retentionEntry(t, page, g.Ref.BuildID).ProtectReasons, "execution_unconfirmed") {
			t.Fatal("旧动作完整receipt链缺口未保护")
		}
	})
}
func TestRetentionConfirmationReportPendingVersusOptionalMissing(t *testing.T) {
	for _, required := range []bool{true, false} {
		t.Run(map[bool]string{true: "required", false: "optional"}[required], func(t *testing.T) {
			stores(t, func(t *testing.T, s *Store, _ Options) {
				if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 1, Days: 1}); err != nil {
					t.Fatal(err)
				}
				a, g := queuedReports(t, s, required, []string{"results/*.xml"}, false)
				reportFinished(t, s, a, g)
				empty := protocol.ReportEvidence{Revision: 1, Required: required, Outcome: "pending", Files: []protocol.ReportFile{}, Diagnostics: []protocol.JUnitDiagnostic{}}
				accept(t, s, a, checkedEvent(g, 4, &empty, false))
				if err := s.SetNodeState(testContext, localAdmin, "linux", "disabled"); err != nil {
					t.Fatal(err)
				}
				if err := s.ConfirmStopped(testContext, localAdmin, protocol.StopConfirmation{Ref: g.Ref, EvidenceCode: "admin_observed_stopped", Note: "明确停止仍保留报告检查事实"}); err != nil {
					t.Fatal(err)
				}
				page, err := s.EvaluateRetention(testContext, localAdmin, retentionProjectForBuild(t, s, g.Ref.BuildID), Page{})
				if err != nil {
					t.Fatal(err)
				}
				e := retentionEntry(t, page, g.Ref.BuildID)
				if !slices.Contains(e.ProtectReasons, "reports_unconfirmed") || slices.Contains(e.ProtectReasons, "execution_unconfirmed") {
					t.Fatal("独立stop不得消除尚未seal报告", e)
				}
			})
		})
	}
}

func TestRetentionConfirmationOptionalMissingSealIsComplete(t *testing.T) {
	for _, required := range []bool{true, false} {
		stores(t, func(t *testing.T, s *Store, _ Options) {
			if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 1, Days: 1}); err != nil {
				t.Fatal(err)
			}
			a, g := queuedReports(t, s, required, []string{"results/*.xml"}, false)
			reportFinished(t, s, a, g)
			e := protocol.ReportEvidence{Revision: 1, Required: required, Outcome: "pending", Files: []protocol.ReportFile{}, Diagnostics: []protocol.JUnitDiagnostic{}}
			accept(t, s, a, checkedEvent(g, 4, &e, false))
			e.Revision = 2
			e.Outcome = "missing"
			if required {
				e.Outcome = "failed"
				e.Reason = "report_missing"
			}
			accept(t, s, a, checkedEvent(g, 5, &e, true))
			e.Sealed = true
			progress := stepProgress("reports_sealed", "", "", "", 0)
			progress.StepKind = ""
			progress.Reports = &e
			accept(t, s, a, event(g.Ref, 6, progress))
			page, err := s.EvaluateRetention(testContext, localAdmin, retentionProjectForBuild(t, s, g.Ref.BuildID), Page{})
			if err != nil {
				t.Fatal(err)
			}
			if slices.Contains(retentionEntry(t, page, g.Ref.BuildID).ProtectReasons, "reports_unconfirmed") {
				t.Fatal("合法0文件封存误作副本未确认")
			}
		})
	}
}
func TestRetentionConfirmationExpireUsesOriginalTimeAndProtectedRank(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 1, Days: 30}); err != nil {
			t.Fatal(err)
		}
		a, g, _ := claimed(t, s)
		id := retentionProjectForBuild(t, s, g.Ref.BuildID)
		p, err := s.GetProject(testContext, "app")
		if err != nil || p.ID != id {
			t.Fatal(err)
		}
		others := retentionSkipped(t, s, p, 2)
		accept(t, s, a, event(g.Ref, 1, stepProgress("intent", "", "ordinary", "compile", 1)))
		// 固定已到期边界仅调整原lease时间，两端Ref仍一致；业务终态由真实Expire生成。
		expired := time.Now().UTC().Add(-time.Second)
		if err = s.writer.Model(&buildRecord{}).Where("id = ?", g.Ref.BuildID).Update("lease_expires_at", expired).Error; err != nil {
			t.Fatal(err)
		}
		if err = s.writer.Model(&attemptRecord{}).Where("id = ?", g.Ref.AttemptID).Update("lease_expires_at", expired).Error; err != nil {
			t.Fatal(err)
		}
		if err = s.ExpireLeases(testContext); err != nil {
			t.Fatal(err)
		}
		var row buildRecord
		if err = s.db.First(&row, "id = ?", g.Ref.BuildID).Error; err != nil {
			t.Fatal(err)
		}
		at := *row.TerminalAt
		page := retentionEvaluateAt(t, s, p.ID, at, Page{Limit: 200})
		for _, other := range others {
			if !retentionEntry(t, page, other).Candidate {
				t.Fatal("可信受保护终态未占配额")
			}
		}
		if e := retentionEntry(t, page, g.Ref.BuildID); e.Candidate || !slices.Contains(e.ProtectReasons, "stop_unconfirmed") {
			t.Fatal("真实Expire保护/排序失效", e)
		}
		if err = s.ConfirmNodeStopped(testContext, a, protocol.StopConfirmation{Ref: g.Ref, EvidenceCode: "process_group_reaped", Note: "当前独立节点真实停止证据夹具"}); err != nil {
			t.Fatal(err)
		}
		if err = s.ExpireLeases(testContext); err != nil {
			t.Fatal(err)
		}
		page = retentionEvaluateAt(t, s, p.ID, at, Page{Limit: 200})
		e := retentionEntry(t, page, g.Ref.BuildID)
		if !e.TerminalAt.Equal(at) || slices.Contains(e.ProtectReasons, "stop_unconfirmed") || slices.Contains(e.ProtectReasons, "execution_unconfirmed") || !slices.Contains(e.ProtectReasons, "ownership_unknown") {
			t.Fatal("重复Expire/独立Stop更新终态或清无关保护", e)
		}
	})
}
