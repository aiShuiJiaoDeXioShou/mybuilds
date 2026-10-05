package store

import (
	"testing"

	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
)

// 使用实际Store事件、原XML解析与中央封存，不把生成的消息当远端商店验收。
func TestPublishReportsSealBeforePublisher(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		a, session, project, policy := leaseFixture(t, s)
		in := enqueueInput(project, "publish-reports")
		in.HasUpload, in.AllowUpload = true, true
		in.Builds[0].Snapshot.Definition.Reports = &config.Reports{JUnit: &config.JUnitReport{Paths: []string{"results/*.xml"}}}
		in.Builds[0].Snapshot.Definition.Steps = append(in.Builds[0].Snapshot.Definition.Steps, config.Step{Kind: "upload", Name: "publish", Target: "google_play", AppIdentifier: "com.example.app", File: "app.aab", Credentials: "${PLAY_JSON}"})
		in.Builds[0].Steps = append(in.Builds[0].Steps, StepProgress{Phase: "ordinary", Index: 2, Name: "publish", Kind: "upload", Status: "pending", Condition: "ready"})
		if _, err := s.Enqueue(testContext, in); err != nil {
			t.Fatal(err)
		}
		grant, err := s.Claim(testContext, a, protocol.ClaimRequest{SessionID: session.SessionID, ClaimKey: uuid.NewString()}, policy)
		if err != nil || grant == nil {
			t.Fatal(err)
		}
		g := *grant
		reportFinished(t, s, a, g)
		e, parsed := actualReport(t, `<testsuite><testcase name="before-publish"/></testsuite>`)
		accept(t, s, a, checkedEvent(g, 4, &e, false))
		e.Revision = 2
		e.Outcome = "passed"
		accept(t, s, a, checkedEvent(g, 5, &e, true))
		verifiedReportRow(t, s, a, g, reportCommit(g, e, &parsed))
		e.Sealed = true
		p := stepProgress("reports_sealed", "", "", "", 0)
		p.StepKind, p.Reports = "", &e
		accept(t, s, a, event(g.Ref, 6, p))
		p = stepProgress("intent", "", "ordinary", "publish", 2)
		p.StepKind = "upload"
		accept(t, s, a, event(g.Ref, 7, p))
		var row buildRecord
		if err := s.db.First(&row, "id = ?", g.Ref.BuildID).Error; err != nil || !row.ReportFinal || row.ReportSealDigest == "" {
			t.Fatal("发布前未保存中央封存", err)
		}
	})
}

func TestPublishReportsRejectUnfinishedOrdinaryScript(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		a, g := queuedReports(t, s, true, []string{"results/*.xml"}, true)
		reportFinished(t, s, a, g)
		e, _ := actualReport(t, `<testsuite><testcase/></testsuite>`)
		accept(t, s, a, checkedEvent(g, 4, &e, false))
		e.Revision, e.Outcome = 2, "passed"
		if _, err := s.ApplyEvent(testContext, a, checkedEvent(g, 5, &e, true)); err != ErrEventConflict {
			t.Fatal("仍未完成普通脚本却封存", err)
		}
	})
}

func TestPublishReceiptClosesGuardAfterPhysicalStep(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		a, g, in := publishFixture(t, s)
		grant, err := s.AuthorizePublish(testContext, a, in)
		if err != nil {
			t.Fatal(err)
		}
		// 合成有限协议证据只验证Store联动，不代表Google Play真实接收。
		receipt := protocol.PublishReceipt{IntentID: in.IntentID, Ref: in.Ref, AuthorizationDigest: grant.AuthorizationDigest, Status: "uploaded", EvidenceCode: "remote_receipt", MutationStage: grant.Action, Started: true, StopConfirmed: true, Remote: protocol.PublishRemoteEvidence{EditID: uuid.NewString(), ReleaseName: grant.ReleaseName, Track: grant.Track, BundleSHA256: grant.ArtifactSHA256, VersionCode: grant.VersionCode, BundleAccepted: true, TrackAccepted: true, CommitAccepted: true}}
		receipt.Digest, _ = protocol.PublishReceiptDigest(receipt)
		if _, err = s.RecordPublish(testContext, a, receipt); err != nil {
			t.Fatal("原回执", err)
		}
		if _, err = s.RecordPublish(testContext, a, receipt); err != nil {
			t.Fatal("完全相同回执幂等", err)
		}
		view, err := s.GetPublish(testContext, in.IntentID)
		if err != nil || !view.ApplicationProtected {
			t.Fatal("回执到达过早释放执行中应用", err)
		}
		p := stepProgress("started", "", "ordinary", "publish", 2)
		p.StepKind, p.Started = "upload", true
		accept(t, s, a, event(g.Ref, 5, p))
		p.Kind, p.Status, p.ExitCode, p.StopConfirmed = "finished", "succeeded", 0, true
		accept(t, s, a, event(g.Ref, 6, p))
		view, err = s.GetPublish(testContext, in.IntentID)
		if err != nil || view.ApplicationProtected || view.Status != "uploaded" {
			t.Fatal("真实关闭后未释放已知应用", err)
		}
		lookup, err := s.FindNodePublish(testContext, a, protocol.PublishLookup{IntentID: in.IntentID, Ref: g.Ref, Index: 2})
		if err != nil || !lookup.StepClosed || lookup.ReceiptDigest != receipt.Digest {
			t.Fatal("关闭后精确只读核对", err)
		}
		terminal := protocol.ExecutionProgress{Kind: "build_finished", PublishIntents: []protocol.PublishExpectation{{IntentID: in.IntentID, ReceiptDigest: receipt.Digest, Status: "uploaded"}}}
		var row buildRecord
		if err = s.db.First(&row, "id = ?", g.Ref.BuildID).Error; err != nil {
			t.Fatal(err)
		}
		if err = validatePublishManifest(s.db, row, terminal); err != nil {
			t.Fatal("完整终态manifest", err)
		}
		terminal.PublishIntents = nil
		if err = validatePublishManifest(s.db, row, terminal); err != ErrEventConflict {
			t.Fatal("遗失原意图被接受", err)
		}
	})
}
