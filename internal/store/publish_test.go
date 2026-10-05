package store

import (
	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPublishBindingIdentityAndPermissions(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		node, _, project, _ := leaseFixture(t, s)
		in := BindApplicationInput{ProjectID: project.ID, NodeID: node.ID, Store: "google_play", AppIdentifier: "com.example.app", CredentialRef: "${PLAY_JSON}", UploadCertificateSHA256: strings.Repeat("a", 64), AllowedTracks: []string{"internal"}}
		if _, err := s.BindApplication(testContext, Actor{Role: "approver"}, in); err != ErrForbidden {
			t.Fatal("非管理员登记", err)
		}
		first, err := s.BindApplication(testContext, localAdmin, in)
		if err != nil || first.Status != "pending" {
			t.Fatal("初次登记未pending", err)
		}
		again, err := s.BindApplication(testContext, localAdmin, in)
		if err != nil || again.ID != first.ID {
			t.Fatal("重复登记", err)
		}
		other, err := s.CreateProject(testContext, localAdmin, projectInput("other"))
		if err != nil {
			t.Fatal(err)
		}
		in.ProjectID = other.ID
		if _, err = s.BindApplication(testContext, localAdmin, in); err != ErrConflict {
			t.Fatal("跨项目应用归属", err)
		}
	})
}

func TestPublishDoctorRequiresRealProtocolEvidence(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		a, session, p, _ := leaseFixture(t, s)
		binding, err := s.BindApplication(testContext, localAdmin, BindApplicationInput{ProjectID: p.ID, NodeID: a.ID, Store: "google_play", AppIdentifier: "com.example.doctor", CredentialRef: "${PLAY_JSON}", UploadCertificateSHA256: strings.Repeat("a", 64), AllowedTracks: []string{"internal"}})
		if err != nil {
			t.Fatal(err)
		}
		task, err := s.ClaimPublishQuery(testContext, a, session.SessionID)
		if err != nil || task == nil {
			t.Fatal("领取管理请求", err)
		}
		result := protocol.PublishQueryResult{ID: task.ID, Nonce: task.Nonce, Kind: task.Kind, BindingID: task.BindingID, NodeID: a.ID, SessionID: session.SessionID, ObservedAt: time.Now().UTC(), Matches: []protocol.PublishMatch{}, ToolLockDigest: strings.Repeat("b", 64), DoctorChecks: []protocol.ToolCheck{{Name: "wrong_one", Status: "passed"}, {Name: "wrong_two", Status: "passed"}, {Name: "wrong_three", Status: "passed"}, {Name: "wrong_four", Status: "passed"}}}
		if _, err = s.CompletePublishQuery(testContext, a, result); err != nil {
			t.Fatal(err)
		}
		apps, err := s.ListApplications(testContext, p.ID)
		if err != nil || len(apps) != 1 || apps[0].Status != "pending" {
			t.Fatal("错误检查集合伪verified", err)
		}
		if _, err = s.RequestApplicationDoctor(testContext, localAdmin, binding.ID); err != nil {
			t.Fatal(err)
		}
		task, err = s.ClaimPublishQuery(testContext, a, session.SessionID)
		if err != nil || task == nil {
			t.Fatal(err)
		}
		result.ID = task.ID
		result.Nonce = task.Nonce
		result.ObservedAt = time.Now().UTC()
		result.DoctorChecks = []protocol.ToolCheck{{Name: "fastlane", Status: "passed"}, {Name: "bundletool", Status: "passed"}, {Name: "google_play_credentials", Status: "passed"}, {Name: "google_play_application", Status: "passed"}}
		if _, err = s.CompletePublishQuery(testContext, a, result); err != nil {
			t.Fatal(err)
		}
		apps, err = s.ListApplications(testContext, p.ID)
		if err != nil || apps[0].Status != "verified" {
			t.Fatal("完整精确检查被拒", err)
		}
		result.Nonce = uuid.NewString()
		if _, err = s.CompletePublishQuery(testContext, a, result); err != ErrConflict {
			t.Fatal("不同nonce覆盖核验", err)
		}
	})
}

func publishFixture(t *testing.T, s *Store) (NodeActor, protocol.LeaseGrant, protocol.PublishAuthorization) {
	t.Helper()
	a, session, p, policy := leaseFixture(t, s)
	binding, err := s.BindApplication(testContext, localAdmin, BindApplicationInput{ProjectID: p.ID, NodeID: a.ID, Store: "google_play", AppIdentifier: "com.example.publish", CredentialRef: "${PLAY_JSON}", UploadCertificateSHA256: strings.Repeat("a", 64), AllowedTracks: []string{"internal"}})
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.ClaimPublishQuery(testContext, a, session.SessionID)
	if err != nil || task == nil {
		t.Fatal(err)
	}
	_, err = s.CompletePublishQuery(testContext, a, protocol.PublishQueryResult{ID: task.ID, Nonce: task.Nonce, Kind: "doctor", BindingID: binding.ID, NodeID: a.ID, SessionID: session.SessionID, ObservedAt: time.Now().UTC(), Matches: []protocol.PublishMatch{}, ToolLockDigest: strings.Repeat("f", 64), DoctorChecks: []protocol.ToolCheck{{Name: "fastlane", Status: "passed"}, {Name: "bundletool", Status: "passed"}, {Name: "google_play_credentials", Status: "passed"}, {Name: "google_play_application", Status: "passed"}}})
	if err != nil {
		t.Fatal(err)
	}
	in := enqueueInput(p, "publish")
	in.AllowUpload = true
	in.HasUpload = true
	version := "1.2.3"
	in.Builds[0].Snapshot.Definition = config.Build{Params: map[string]config.Parameter{"version": {Default: &version}}, Steps: []config.Step{{Kind: "artifact", Name: "package", Paths: []string{"output/app.aab"}}, {Kind: "upload", Name: "publish", Target: "google_play", File: "output/app.aab", AppIdentifier: "com.example.publish", Track: "internal", Credentials: "${PLAY_JSON}"}}}
	in.Builds[0].Snapshot.Params = map[string]string{"version": version}
	in.Builds[0].Steps = []StepProgress{{Phase: "ordinary", Index: 1, Name: "package", Kind: "artifact", Condition: "ready", Status: "pending"}, {Phase: "ordinary", Index: 2, Name: "publish", Kind: "upload", Condition: "ready", Status: "pending"}}
	out, err := s.Enqueue(testContext, in)
	if err != nil {
		t.Fatal(err)
	}
	g, err := s.Claim(testContext, a, protocol.ClaimRequest{SessionID: session.SessionID, ClaimKey: uuid.NewString()}, policy)
	if err != nil || g == nil {
		t.Fatal("领取含upload冻结配置", err)
	}
	artifactID := uuid.NewString()
	progress := stepProgress("intent", "", "ordinary", "package", 1)
	progress.StepKind = "artifact"
	accept(t, s, a, event(g.Ref, 1, progress))
	progress.Kind = "started"
	progress.Started = true
	accept(t, s, a, event(g.Ref, 2, progress))
	progress.Kind = "finished"
	progress.Status = "succeeded"
	progress.StopConfirmed = true
	progress.ExitCode = 0
	progress.ArtifactIDs = []string{artifactID}
	accept(t, s, a, event(g.Ref, 3, progress))
	_, err = s.CommitArtifact(testContext, a, ArtifactCommit{Declaration: protocol.ArtifactDeclaration{Ref: g.Ref, ID: artifactID, Seq: 1, Phase: "ordinary", Step: "package", Index: 1, Name: "app.aab", Size: 123, SHA256: strings.Repeat("b", 64)}, StorageID: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	progress = stepProgress("intent", "", "ordinary", "publish", 2)
	progress.StepKind = "upload"
	accept(t, s, a, event(g.Ref, 4, progress))
	return a, *g, protocol.PublishAuthorization{IntentID: uuid.NewString(), Ref: g.Ref, Index: 2, StepName: "publish", ArtifactID: artifactID, ArtifactSize: 123, ArtifactSHA256: strings.Repeat("b", 64), ReportIDs: []string{}, VersionName: version, VersionCode: *out.Builds[0].Number, Track: "internal"}
}
func TestPublishOnceUnknownAndExactDecision(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		a, g, in := publishFixture(t, s)
		results := make(chan error, 20)
		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); _, err := s.AuthorizePublish(testContext, a, in); results <- err }()
		}
		wg.Wait()
		close(results)
		grants := 0
		for err := range results {
			if err == nil {
				grants++
			} else if err != ErrConflict {
				t.Fatal(err)
			}
		}
		if grants != 1 {
			t.Fatal("一次动作授权数量", grants)
		}
		view, err := s.GetPublish(testContext, in.IntentID)
		if err != nil || view.Status != "unknown" || !view.ApplicationProtected {
			t.Fatal("未知状态未保护", err)
		}
		lookup, err := s.FindNodePublish(testContext, a, protocol.PublishLookup{Ref: g.Ref, Index: 2, IntentID: in.IntentID})
		if err != nil || !lookup.Authorized || lookup.StepClosed {
			t.Fatal("只读核对原归属", err)
		}
		held, err := protectPublishHistory(s.db, g.Ref.BuildID)
		if err != nil || !held {
			t.Fatal("发布unknown不保护保留", err)
		}
		decision := ConfirmPublishInput{IntentID: in.IntentID, Key: uuid.NewString(), ExpectedIntentDigest: view.IntentDigest, Outcome: "failed", EvidenceCode: "stop_confirmed", Note: "只有停止事实", EvidenceSHA256: strings.Repeat("d", 64)}
		if _, err = s.ConfirmPublish(testContext, localAdmin, decision); err != ErrInvalid {
			t.Fatal("把停止当未发送证明", err)
		}
		decision.EvidenceCode = "confirmed_not_sent"
		decision.ExpectedIntentDigest = strings.Repeat("e", 64)
		if _, err = s.ConfirmPublish(testContext, localAdmin, decision); err != ErrConflict {
			t.Fatal("过期决定摘要", err)
		}
		decision.ExpectedIntentDigest = view.IntentDigest
		confirmed, err := s.ConfirmPublish(testContext, localAdmin, decision)
		if err != nil || confirmed.Status != "failed" {
			t.Fatal("具体原证据决定", err)
		}
		if _, err = s.ConfirmPublish(testContext, localAdmin, decision); err != nil {
			t.Fatal("精确决定幂等", err)
		}
		decision.Note = "改变原决定"
		if _, err = s.ConfirmPublish(testContext, localAdmin, decision); err != ErrConflict {
			t.Fatal("决定key冲突", err)
		}
	})
}
