package store

import (
	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"testing"
)

// 审批中途保存的是已检查原XML，不把pending报告冒充最终通过。
func TestApprovalNonfinalReportUploadUsesActualCheckedXML(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		a, session, project, policy := leaseFixture(t, s)
		in := enqueueInput(project, "approval-reports")
		in.Builds[0].Snapshot.Definition.Reports = &config.Reports{JUnit: &config.JUnitReport{Paths: []string{"results/*.xml"}}}
		in.Builds[0].Snapshot.Definition.Steps = append(in.Builds[0].Snapshot.Definition.Steps, config.Step{Kind: "approval", Name: "release"}, config.Step{Kind: "run", Name: "after", Run: "true"})
		in.Builds[0].Steps = append(in.Builds[0].Steps, StepProgress{Phase: "ordinary", Index: 2, Name: "release", Kind: "approval", Condition: "ready", Status: "pending"}, StepProgress{Phase: "ordinary", Index: 3, Name: "after", Kind: "run", Condition: "ready", Status: "pending"})
		if _, err := s.Enqueue(testContext, in); err != nil {
			t.Fatal(err)
		}
		grant, err := s.Claim(testContext, a, protocol.ClaimRequest{SessionID: session.SessionID, ClaimKey: uuid.NewString()}, policy)
		if err != nil || grant == nil {
			t.Fatal(err)
		}
		g := *grant
		reportFinished(t, s, a, g)
		e, parsed := actualReport(t, `<testsuite><testcase name="before-approval"/></testsuite>`)
		accept(t, s, a, checkedEvent(g, 4, &e, false))
		if _, err = s.ReportUploadBudget(testContext, a, g.Ref); err != nil {
			t.Fatal("真实中途审批报告上传预算", err)
		}
		commit := reportCommit(g, e, &parsed)
		wrong := commit
		wrong.Declaration.ReportRevision++
		if _, err = s.CommitArtifact(testContext, a, wrong); err != ErrArtifactConflict {
			t.Fatal("非原revision通过", err)
		}
		verifiedReportRow(t, s, a, g, commit)
		var row buildRecord
		if err = s.db.First(&row, "id = ?", g.Ref.BuildID).Error; err != nil || row.ReportFinal || row.ReportSealDigest != "" {
			t.Fatal("中途报告被提前封存", err)
		}
		visible, err := s.ListArtifacts(testContext, localAdmin, g.Ref.BuildID, Page{})
		if err != nil || len(visible) != 0 {
			t.Fatal("未挂起/未封存XML提前公开", err)
		}
	})
}
