package store

import (
	"testing"
	"time"

	"mybuilds/internal/protocol"
)

func TestReportFieldsStayAtTheirLifecycleBoundary(t *testing.T) {
	for _, kind := range []string{"intent", "started", "finished", "post_selected", "skipped", "build_finished"} {
		t.Run(kind, func(t *testing.T) {
			base := protocol.ExecutionProgress{Kind: kind, At: time.Now().UTC()}
			if !validProgress(base) {
				t.Fatal("旧无报告消息形状被改变")
			}
			local := base
			local.LocalReports = []protocol.CollectedReport{{SnapshotPath: "private/snapshot"}}
			if validProgress(local) {
				t.Fatal("Store接纳私有节点路径")
			}
			withEvidence := base
			withEvidence.Reports = &protocol.ReportEvidence{Files: []protocol.ReportFile{}, Diagnostics: []protocol.JUnitDiagnostic{}}
			if validProgress(withEvidence) {
				t.Fatal("非checked/sealed事件夹带报告证据")
			}
			if kind != "build_finished" {
				withManifest := base
				withManifest.ReportManifest = &protocol.ReportManifest{IDs: []string{}}
				if validProgress(withManifest) {
					t.Fatal("非terminal事件夹带报告manifest")
				}
			}
		})
	}
}
