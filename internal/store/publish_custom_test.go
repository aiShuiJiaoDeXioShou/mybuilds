package store

import (
	"encoding/json"
	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"strings"
	"testing"
	"time"
)

func TestCustomBindingManualAttestationIsExplicitAndPrivate(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		a, session, p, _ := leaseFixture(t, s)
		in := BindApplicationInput{ProjectID: p.ID, NodeID: a.ID, Store: "custom", AppIdentifier: "org.example.custom", AllowedTracks: []string{}, Custom: &protocol.CustomBindingEvidence{Source: "manual_attested", EvidenceCode: "ownership_attested", Note: "private administrator ownership observation", EvidenceSHA256: strings.Repeat("a", 64)}}
		got, err := s.BindApplication(testContext, localAdmin, in)
		if err != nil || got.Status != "verified" || got.VerificationSource != "manual_attested" {
			t.Fatalf("manual binding: %v", err)
		}
		data, _ := json.Marshal(got)
		if strings.Contains(string(data), in.Custom.Note) {
			t.Fatal("private evidence in public view")
		}
		again, err := s.BindApplication(testContext, localAdmin, in)
		if err != nil || again.ID != got.ID {
			t.Fatal("idempotent binding", err)
		}
		if task, err := s.ClaimPublishQuery(testContext, a, session.SessionID); err != nil || task != nil {
			t.Fatal("created fake doctor", err)
		}
		bad := in
		bad.Custom = nil
		if _, e := s.BindApplication(testContext, localAdmin, bad); e == nil {
			t.Fatal("accepted missing attestation")
		}
		bad = in
		bad.Store = "app_store"
		bad.CredentialRef = "${APPLE}"
		if _, e := s.BindApplication(testContext, localAdmin, bad); e == nil {
			t.Fatal("manual bypassed store doctor")
		}
		if _, e := s.BindApplication(testContext, Actor{Role: "trigger"}, in); e != ErrForbidden {
			t.Fatal("trigger bound app", e)
		}
	})
}

func TestPublishIOSOutputUsesTrustedSnapshot(t *testing.T) {
	id := "927329b5-0726-42ef-9c73-295665d12daa"
	task := protocol.TaskSnapshot{Facts: map[string]string{"build.id": id, "ios.output_dir": "forged"}}
	task.Definition.IOSSigning = &config.IOSSigning{}
	value, err := renderPublishValue("{{ios.output_dir}}/export/*.ipa", task)
	if err != nil || value != ".mybuilds-ios-"+id+"/export/*.ipa" {
		t.Fatalf("trusted output: %q %v", value, err)
	}
	task.Definition.IOSSigning = nil
	if _, e := renderPublishValue("{{ios.output_dir}}/export/*.ipa", task); e == nil {
		t.Fatal("unsigned forged iOS fact accepted")
	}
	task.Definition.IOSSigning = &config.IOSSigning{}
	task.Facts["build.id"] = "not-uuid"
	if _, e := renderPublishValue("{{ios.output_dir}}/export/*.ipa", task); e == nil {
		t.Fatal("invalid build identity accepted")
	}
}

func customPublishFixture(t *testing.T, s *Store, query ...[]string) (NodeActor, protocol.LeaseGrant, protocol.PublishAuthorization) {
	t.Helper()
	a, session, p, policy := leaseFixture(t, s)
	_, err := s.BindApplication(testContext, localAdmin, BindApplicationInput{ProjectID: p.ID, NodeID: a.ID, Store: "custom", AppIdentifier: "com.example.publish", AllowedTracks: []string{}, Custom: &protocol.CustomBindingEvidence{Source: "manual_attested", EvidenceCode: "ownership_attested", Note: "owned app", EvidenceSHA256: strings.Repeat("a", 64)}})
	if err != nil {
		t.Fatal(err)
	}
	in := enqueueInput(p, "publish")
	in.AllowUpload = true
	in.HasUpload = true
	version := "1.2.3"
	in.Builds[0].Snapshot.Definition = config.Build{Params: map[string]config.Parameter{"version": {Default: &version}}, Steps: []config.Step{{Kind: "artifact", Name: "package", Paths: []string{"output/app.aab"}}, {Kind: "upload", Name: "publish", Target: "custom", File: "output/app.aab", AppIdentifier: "com.example.publish", Argv: []string{"./publish"}, QueryArgv: []string{"./query"}, WorkingDir: ".", ResultFile: "result.json"}}}
	if len(query) > 0 {
		in.Builds[0].Snapshot.Definition.Steps[1].QueryArgv = query[0]
	}
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
	request := protocol.PublishAuthorization{IntentID: uuid.NewString(), Ref: g.Ref, Index: 2, StepName: "publish", ArtifactID: artifactID, ArtifactSize: 123, ArtifactSHA256: strings.Repeat("b", 64), ReportIDs: []string{}, VersionName: version, VersionCode: *out.Builds[0].Number, Custom: &protocol.CustomPublishAuthorization{ResultSchemaVersion: 1}}
	digest, _ := protocol.CustomCommandDigest(g.Task.Definition.Steps[1].Argv, g.Task.Definition.Steps[1].QueryArgv, ".", "result.json", "com.example.publish", version, artifactID, request.ArtifactSHA256, request.VersionCode, request.ArtifactSize)
	request.Custom.CommandDigest = digest
	return a, *g, request
}

func TestCustomAuthorizationBindsOriginalCommandAndReceipt(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		a, g, in := customPublishFixture(t, s)
		bad := in
		bad.Custom = &protocol.CustomPublishAuthorization{ResultSchemaVersion: 1, CommandDigest: strings.Repeat("0", 64)}
		if _, e := s.AuthorizePublish(testContext, a, bad); e == nil {
			t.Fatal("wrong command digest")
		}
		grant, e := s.AuthorizePublish(testContext, a, in)
		if e != nil || grant.Custom == nil || grant.Action != "custom_upload" {
			t.Fatalf("custom grant %v", e)
		}
		receipt := protocol.PublishReceipt{IntentID: grant.IntentID, Ref: g.Ref, AuthorizationDigest: grant.AuthorizationDigest, Started: true, StopConfirmed: true, Status: "uploaded", EvidenceCode: "remote_receipt", MutationStage: "custom_upload", Remote: protocol.PublishRemoteEvidence{Custom: &protocol.CustomPublishEvidence{RemoteID: "receipt-1", Lifecycle: "uploaded", ActionConfirmed: true}}}
		receipt.Digest, _ = protocol.PublishReceiptDigest(receipt)
		got, e := s.RecordPublish(testContext, a, receipt)
		if e != nil || got.Status != "uploaded" || !got.ApplicationProtected {
			t.Fatalf("custom receipt %v", e)
		}
		receipt.EvidenceCode = "private arbitrary output"
		receipt.Digest, _ = protocol.PublishReceiptDigest(receipt)
		if _, e = s.RecordPublish(testContext, a, receipt); e == nil {
			t.Fatal("arbitrary evidence")
		}
	})
}

func TestCustomQueryOriginalContextAndExactResolution(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		a, g, in := customPublishFixture(t, s)
		grant, e := s.AuthorizePublish(testContext, a, in)
		if e != nil {
			t.Fatal(e)
		}
		unknown := protocol.PublishReceipt{IntentID: grant.IntentID, Ref: g.Ref, AuthorizationDigest: grant.AuthorizationDigest, Status: "unknown", Started: true, StopConfirmed: true, EvidenceCode: "result_unconfirmed", MutationStage: "custom_upload"}
		unknown.Digest, _ = protocol.PublishReceiptDigest(unknown)
		if _, e = s.RecordPublish(testContext, a, unknown); e != nil {
			t.Fatal(e)
		}
		q, e := s.RequestPublishQuery(testContext, localAdmin, grant.IntentID)
		if e != nil {
			t.Fatal(e)
		}
		task, e := s.ClaimPublishQuery(testContext, a, g.Ref.SessionID)
		if e != nil || task == nil || task.ID != q.ID || task.Custom == nil {
			t.Fatal("private query", e)
		}
		c := task.Custom
		if c.OriginalRef != grant.Ref || c.AuthorizationDigest != grant.AuthorizationDigest || c.SHA != g.Task.SHA || len(c.QueryArgv) != 1 || c.QueryArgv[0] != "./query" || c.ArtifactID != grant.ArtifactID || len(c.ReportIDs) != 0 {
			t.Fatal("wrong frozen context")
		}
		result := protocol.PublishQueryResult{ID: task.ID, Nonce: task.Nonce, Kind: task.Kind, BindingID: task.BindingID, IntentID: task.IntentID, NodeID: task.NodeID, SessionID: task.SessionID, ObservedAt: time.Now().UTC(), Matches: []protocol.PublishMatch{{VersionCodes: []int64{grant.VersionCode}, Lifecycle: "uploaded", Custom: &protocol.CustomPublishEvidence{RemoteID: "owned-remote", Lifecycle: "uploaded", ActionConfirmed: true}}}, Custom: &protocol.CustomQueryEvidence{IntentID: grant.IntentID, AuthorizationDigest: grant.AuthorizationDigest, ArtifactID: grant.ArtifactID, ArtifactSHA256: grant.ArtifactSHA256, VersionName: grant.VersionName, VersionCode: grant.VersionCode, EvidenceCode: "remote_receipt", Remote: protocol.CustomPublishEvidence{RemoteID: "owned-remote", Lifecycle: "uploaded", ActionConfirmed: true}}}
		result.Custom.ArtifactID = uuid.NewString()
		if _, e = s.CompletePublishQuery(testContext, a, result); e != ErrConflict {
			t.Fatal("forged context promoted", e)
		}
		result.Custom.ArtifactID = grant.ArtifactID
		result.Matches[0].Custom.RemoteID = "different-remote"
		if _, e = s.CompletePublishQuery(testContext, a, result); e != ErrConflict {
			t.Fatal("conflicting public observation promoted", e)
		}
		result.Matches[0].Custom.RemoteID = "owned-remote"
		view, e := s.CompletePublishQuery(testContext, a, result)
		if e != nil || view.Status != "completed" {
			t.Fatal("exact query", e)
		}
		pub, e := s.GetPublish(testContext, grant.IntentID)
		if e != nil || pub.Status != "uploaded" || pub.EvidenceCode != "remote_state" || !pub.ApplicationProtected {
			t.Fatal("query before step close", e)
		}
		data, _ := json.Marshal(view)
		if strings.Contains(string(data), c.Repository) || strings.Contains(string(data), "query_argv") || strings.Contains(string(data), "environment") {
			t.Fatal("private query context in safeview")
		}
	})
}

func TestCustomQueryWithoutDeclaredCommandDoesNotCreateTask(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		a, _, in := customPublishFixture(t, s, nil)
		grant, e := s.AuthorizePublish(testContext, a, in)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = s.RequestPublishQuery(testContext, localAdmin, grant.IntentID); e != ErrInvalid {
			t.Fatal("undeclared query accepted", e)
		}
		var count int64
		if e = s.db.Model(&publishQueryRecord{}).Where("intent_id = ?", grant.IntentID).Count(&count).Error; e != nil || count != 0 {
			t.Fatal("query partially created", count, e)
		}
	})
}
