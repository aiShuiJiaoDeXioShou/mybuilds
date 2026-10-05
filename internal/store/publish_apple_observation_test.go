package store

import (
	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"strings"
	"testing"
	"time"
)

func applePublishFixture(t *testing.T, s *Store) (NodeActor, protocol.LeaseGrant, protocol.PublishAuthorization) {
	t.Helper()
	a, session, p, policy := leaseFixture(t, s)
	binding, err := s.BindApplication(testContext, localAdmin, BindApplicationInput{ProjectID: p.ID, NodeID: a.ID, Store: "app_store", AppIdentifier: "com.example.publish", CredentialRef: "${APPLE_JSON}", AllowedTracks: []string{"internal"}})
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.ClaimPublishQuery(testContext, a, session.SessionID)
	if err != nil || task == nil {
		t.Fatal(err)
	}
	_, err = s.CompletePublishQuery(testContext, a, protocol.PublishQueryResult{ID: task.ID, Nonce: task.Nonce, Kind: "doctor", BindingID: binding.ID, NodeID: a.ID, SessionID: session.SessionID, ObservedAt: time.Now().UTC(), Matches: []protocol.PublishMatch{}, ToolLockDigest: strings.Repeat("f", 64), DoctorChecks: []protocol.ToolCheck{{Name: "fastlane", Status: "passed"}, {Name: "apple_transport", Status: "passed"}, {Name: "app_store_credentials", Status: "passed"}, {Name: "app_store_application", Status: "passed"}}})
	if err != nil {
		t.Fatal(err)
	}
	in := enqueueInput(p, "publish")
	in.AllowUpload = true
	in.HasUpload = true
	version := "1.2.3"
	submit := true
	in.Builds[0].Snapshot.Definition = config.Build{Params: map[string]config.Parameter{"version": {Default: &version}}, Steps: []config.Step{{Kind: "artifact", Name: "package", Paths: []string{"output/app.ipa"}}, {Kind: "upload", Name: "publish", Target: "app_store", File: "output/app.ipa", AppIdentifier: "com.example.publish", Credentials: "${APPLE_JSON}", SubmitForReview: &submit}}}
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
	_, err = s.CommitArtifact(testContext, a, ArtifactCommit{Declaration: protocol.ArtifactDeclaration{Ref: g.Ref, ID: artifactID, Seq: 1, Phase: "ordinary", Step: "package", Index: 1, Name: "app.ipa", Size: 123, SHA256: strings.Repeat("b", 64)}, StorageID: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	progress = stepProgress("intent", "", "ordinary", "publish", 2)
	progress.StepKind = "upload"
	accept(t, s, a, event(g.Ref, 4, progress))
	return a, *g, protocol.PublishAuthorization{IntentID: uuid.NewString(), Ref: g.Ref, Index: 2, StepName: "publish", ArtifactID: artifactID, ArtifactSize: 123, ArtifactSHA256: strings.Repeat("b", 64), ReportIDs: []string{}, VersionName: version, VersionCode: *out.Builds[0].Number}
}

func TestAppleQueryResolvesOriginalActionWithDecisionAndClosedSlot(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		actor, lease, request := applePublishFixture(t, s)
		var previous string
		var final protocol.PublishGrant
		for _, action := range appleActions {
			request.IntentID = uuid.NewString()
			request.Apple = &protocol.ApplePublishAuthorization{Action: action, PreviousIntentID: previous, RequestSHA256: strings.Repeat("c", 64), SubmitForReview: true, AppStoreVersionID: "version-owned", BuildID: "build-owned", ReviewSubmissionID: "review-owned", ReviewItemID: "item-owned"}
			grant, e := s.AuthorizePublish(testContext, actor, request)
			if e != nil {
				t.Fatalf("authorize %s: %v", action, e)
			}
			remote := protocol.AppleRemoteEvidence{AppID: "app-owned", AppStoreVersionID: "version-owned", BuildID: "build-owned", ReviewSubmissionID: "review-owned", ReviewItemID: "item-owned", RequestSHA256: request.Apple.RequestSHA256, ResponseSHA256: strings.Repeat("d", 64), TransportID: uuid.NewString(), ReleaseType: "MANUAL", ReviewState: "WAITING_FOR_REVIEW", ActionConfirmed: true}
			now := time.Now().UTC()
			remote.UploadedAt = &now
			receipt := protocol.PublishReceipt{IntentID: grant.IntentID, Ref: lease.Ref, AuthorizationDigest: grant.AuthorizationDigest, Status: "confirmed", Started: true, StopConfirmed: true, EvidenceCode: "remote_receipt", MutationStage: action, Remote: protocol.PublishRemoteEvidence{Apple: &remote}}
			if action == "submit_review" {
				wrong := receipt
				copyRemote := remote
				copyRemote.BuildID = "foreign-build"
				wrong.Remote.Apple = &copyRemote
				wrong.Digest, _ = protocol.PublishReceiptDigest(wrong)
				if _, e = s.RecordPublish(testContext, actor, wrong); e != ErrConflict {
					t.Fatal("wrong submitted relationship accepted", e)
				}
				receipt.Status = "unknown"
				receipt.EvidenceCode = "transport_unconfirmed"
				receipt.Remote.Apple.ActionConfirmed = false
			}
			receipt.Digest, _ = protocol.PublishReceiptDigest(receipt)
			if _, e = s.RecordPublish(testContext, actor, receipt); e != nil {
				t.Fatalf("record %s: %v", action, e)
			}
			previous, final = grant.IntentID, grant
		}
		p := stepProgress("started", "", "ordinary", "publish", 2)
		p.StepKind = "upload"
		p.Started = true
		accept(t, s, actor, event(lease.Ref, 5, p))
		p.Kind = "finished"
		p.Status = "failed"
		p.Reason = "publish_unknown"
		p.StopConfirmed = true
		p.ExitCode = -1
		accept(t, s, actor, event(lease.Ref, 6, p))
		for _, wrong := range []bool{true, false} {
			if _, e := s.RequestPublishQuery(testContext, localAdmin, final.IntentID); e != nil {
				t.Fatal(e)
			}
			task, e := s.ClaimPublishQuery(testContext, actor, lease.Ref.SessionID)
			if e != nil || task == nil || task.AppleAuthorization == nil {
				t.Fatal("missing original action", e)
			}
			remote := protocol.AppleRemoteEvidence{AppID: "app-owned", AppStoreVersionID: "version-owned", BuildID: "build-owned", ReviewSubmissionID: "review-owned", ReviewItemID: "item-owned", RequestSHA256: final.Apple.RequestSHA256, ResponseSHA256: strings.Repeat("e", 64), ReviewState: "WAITING_FOR_REVIEW", ActionConfirmed: true}
			if wrong {
				remote.ReviewItemID = "foreign-item"
			}
			result := protocol.PublishQueryResult{ID: task.ID, Nonce: task.Nonce, Kind: task.Kind, BindingID: task.BindingID, IntentID: task.IntentID, NodeID: task.NodeID, SessionID: task.SessionID, ObservedAt: time.Now().UTC(), Matches: []protocol.PublishMatch{{VersionCodes: []int64{final.VersionCode}, Lifecycle: "WAITING_FOR_REVIEW", Apple: &remote}}}
			if _, e = s.CompletePublishQuery(testContext, actor, result); e != nil {
				t.Fatal("complete observation", e)
			}
			got, e := s.GetPublish(testContext, final.IntentID)
			if e != nil {
				t.Fatal(e)
			}
			if wrong {
				if got.Status != "unknown" || !got.ApplicationProtected {
					t.Fatal("weak observation released", got.Status)
				}
				continue
			}
			if got.Status != "confirmed" || got.EvidenceCode != "remote_state" || got.ApplicationProtected {
				t.Fatal("confirmed action still guarded", got.Status, got.ApplicationProtected)
			}
			var decision publishDecisionRecord
			if e = s.db.First(&decision, "intent_id = ?", final.IntentID).Error; e != nil || decision.ActorID != localAdmin.ID || decision.Key != task.ID {
				t.Fatal("missing original query actor audit", e)
			}
			var count int64
			s.db.Model(&auditRecord{}).Where("action = ? AND object_id = ?", "publish_query_confirmed", final.IntentID).Count(&count)
			if count != 1 {
				t.Fatal("query audit count", count)
			}
		}
	})
}
