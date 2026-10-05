package agent

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"mybuilds/internal/distribute"
	"mybuilds/internal/pipeline"
	"mybuilds/internal/process"
	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
)

type publishCheckpoint struct {
	IntentID string                   `json:"intent_id"`
	Index    int                      `json:"index"`
	Grant    *protocol.PublishGrant   `json:"grant,omitempty"`
	Once     bool                     `json:"once"`
	Receipt  *protocol.PublishReceipt `json:"receipt,omitempty"`
}

func (e *taskExecution) publisherCandidate(index int) (string, error) {
	j := e.journal
	j.mu.Lock()
	defer j.mu.Unlock()
	if len(j.state.Publishes) >= 384 || j.state.Ref == nil {
		return "", failure("persistence_error")
	}
	id := uuid.NewString()
	j.state.Publishes = append(j.state.Publishes, publishCheckpoint{IntentID: id, Index: index})
	if err := j.saveLocked(); err != nil {
		return "", err
	}
	return id, nil
}
func (e *taskExecution) publisherGrant(grant protocol.PublishGrant) error {
	j := e.journal
	j.mu.Lock()
	defer j.mu.Unlock()
	for i := range j.state.Publishes {
		item := &j.state.Publishes[i]
		if item.IntentID == grant.IntentID {
			if item.Once || item.Grant != nil {
				return failure("persistence_error")
			}
			copyGrant := grant
			item.Grant = &copyGrant
			item.Once = true
			return j.saveLocked()
		}
	}
	return failure("persistence_error")
}
func (e *taskExecution) publisherReceipt(ctx context.Context, receipt protocol.PublishReceipt) error {
	digest, err := protocol.PublishReceiptDigest(receipt)
	if err != nil || digest != receipt.Digest {
		return failure("invalid_response")
	}
	j := e.journal
	j.mu.Lock()
	found := false
	for i := range j.state.Publishes {
		item := &j.state.Publishes[i]
		if item.IntentID == receipt.IntentID {
			copyReceipt := receipt
			item.Receipt = &copyReceipt
			found = true
			break
		}
	}
	if !found {
		j.mu.Unlock()
		return failure("persistence_error")
	}
	err = j.saveLocked()
	j.mu.Unlock()
	if err != nil {
		return err
	}
	if err = e.savePublishEvidence(receipt.IntentID); err != nil {
		return err
	}
	var view store.PublishView
	// 只重发精确回执；绝不重发authorize或执行命令。
	return e.client.retryExecutionPost(ctx, "/api/agent/publishes/"+receipt.IntentID+"/receipt", receipt, &view)
}
func (e *taskExecution) publish(ctx context.Context, input pipeline.PublishInput) (receipt protocol.PublishReceipt, returnErr error) {
	if e.cfg.PublishTools == nil || e.task == nil || e.lease.check() != nil {
		return receipt, failure("publish_precheck")
	}
	j := e.journal
	j.mu.Lock()
	var artifact *localArtifact
	resultDir := j.state.ResultDir
	for i := range j.state.Artifacts {
		a := j.state.Artifacts[i]
		if a.Confirmed && a.Declaration.Phase == "ordinary" && (a.Declaration.Purpose == "" || a.Declaration.Purpose == "artifact") && a.Declaration.Index < input.Index && a.SnapshotPath == input.Artifact.SnapshotPath && a.Declaration.SHA256 == input.Artifact.SHA256 && a.Declaration.Size == input.Artifact.Size {
			if artifact != nil {
				j.mu.Unlock()
				return receipt, failure("publish_precheck")
			}
			copyArtifact := a
			artifact = &copyArtifact
		}
	}
	j.mu.Unlock()
	if artifact == nil {
		return receipt, failure("publish_precheck")
	}
	input.ArtifactID = artifact.Declaration.ID
	name := strings.TrimSuffix(strings.TrimPrefix(input.Step.Credentials, "${"), "}")
	credential, ok := e.publishSecrets[name]
	if !ok || credential == "" {
		return receipt, failure("publish_precheck")
	}
	var binding store.NodeApplicationMaterial
	preflight := struct {
		Ref           protocol.LeaseRef `json:"ref"`
		Target        string            `json:"target"`
		AppIdentifier string            `json:"app_identifier"`
	}{e.lease.ref, input.Step.Target, input.Step.AppIdentifier}
	if err := e.client.post(ctx, "/api/agent/publishes/preflight", preflight, &binding); err != nil {
		return receipt, err
	}
	if binding.CredentialRef != input.Step.Credentials {
		return receipt, failure("invalid_response")
	}
	path := filepath.Join(resultDir, filepath.FromSlash(input.Artifact.SnapshotPath))
	version := e.task.Parameters["version"]
	authority := func(apple *protocol.ApplePublishAuthorization) (protocol.PublishGrant, error) {
		if ctx.Err() != nil || e.lease.check() != nil {
			return protocol.PublishGrant{}, failure("authority_lost")
		}
		id, err := e.publisherCandidate(input.Index)
		if err == nil {
			err = e.savePublishEvidence(id)
		}
		if err != nil {
			return protocol.PublishGrant{}, err
		}
		receipt = protocol.PublishReceipt{IntentID: id, Ref: e.lease.ref, Status: "unknown", StopConfirmed: true}
		track := input.Step.Track
		if track == "" && input.Step.Target == "google_play" {
			track = "internal"
		}
		request := protocol.PublishAuthorization{IntentID: id, Ref: e.lease.ref, Index: input.Index, StepName: input.Step.Name, ArtifactID: input.ArtifactID, ArtifactSHA256: input.Artifact.SHA256, ArtifactSize: input.Artifact.Size, ReportSealDigest: input.ReportSealDigest, ReportIDs: input.ReportIDs, VersionName: version, VersionCode: e.task.Number, Track: track, ExplicitProduction: track == "production", Apple: apple}
		var grant protocol.PublishGrant
		// 服务端可能已经提交。任何丢响应只保存原候选并闭锁，不再申请另一个grant。
		if err = e.client.post(ctx, "/api/agent/publishes/authorize", request, &grant); err != nil {
			return grant, err
		}
		digest, digestErr := protocol.PublishGrantDigest(grant)
		if digestErr != nil || digest != grant.AuthorizationDigest || grant.IntentID != id || grant.Ref != e.lease.ref || grant.ArtifactID != input.ArtifactID || grant.ArtifactSHA256 != input.Artifact.SHA256 || grant.ArtifactSize != input.Artifact.Size || grant.VersionCode != e.task.Number || grant.VersionName != version || grant.AppIdentifier != input.Step.AppIdentifier || grant.Track != track {
			return grant, failure("invalid_response")
		}
		if err = e.publisherGrant(grant); err == nil {
			err = e.savePublishEvidence(grant.IntentID)
		}
		if err != nil {
			e.lease.cancel()
			return grant, err
		}
		return grant, nil
	}
	onStart := func(info process.StartInfo) error {
		if ctx.Err() != nil || e.lease.check() != nil {
			return failure("authority_lost")
		}
		j.mu.Lock()
		j.state.PID, j.state.PGID = info.PID, info.PGID
		j.state.StopConfirmed = false
		err := j.saveLocked()
		j.mu.Unlock()
		if err != nil {
			e.lease.cancel()
			return err
		}
		return input.OnStart(info)
	}
	cleanup := func(close func() error) {
		if err := close(); err != nil {
			receipt.CleanupFailed = true
			receipt.StopConfirmed = false
			returnErr = failure("cleanup_error")
		}
	}
	if input.Step.Target == "google_play" {
		prepared, err := distribute.PrepareGooglePlay(ctx, distribute.GooglePlayOptions{BundleDir: e.cfg.PublishTools.BundleDir, Bundletool: e.cfg.PublishTools.Bundletool, DataDir: e.cfg.DataDir, CredentialFile: credential, AppIdentifier: input.Step.AppIdentifier, VersionName: version, UploadCertificateSHA256: binding.UploadCertificateSHA256, Number: e.task.Number, ArtifactPath: path, ArtifactSHA256: input.Artifact.SHA256, ArtifactSize: input.Artifact.Size})
		if err != nil {
			if strings.Contains(err.Error(), "cleanup") {
				receipt.CleanupFailed = true
				receipt.StopConfirmed = false
			}
			return receipt, failure("publish_precheck")
		}
		defer func() { cleanup(prepared.Close) }()
		grant, err := authority(nil)
		if err != nil {
			return receipt, err
		}
		receipt, err = distribute.UploadGooglePlay(ctx, prepared, grant, onStart)
		if recordErr := e.publisherReceipt(ctx, receipt); recordErr != nil {
			e.lease.cancel()
			return receipt, recordErr
		}
		return receipt, err
	}
	if input.Step.Target == "app_store" {
		prepared, err := distribute.PrepareApple(ctx, distribute.AppleOptions{BundleDir: e.cfg.PublishTools.BundleDir, DataDir: e.cfg.DataDir, CredentialFile: credential, AppIdentifier: input.Step.AppIdentifier, VersionName: version, DistributionTeamID: input.DistributionTeamID, Number: e.task.Number, ArtifactPath: path, ArtifactSHA256: input.Artifact.SHA256, ArtifactSize: input.Artifact.Size})
		if err != nil {
			if strings.Contains(err.Error(), "cleanup") {
				receipt.CleanupFailed = true
				receipt.StopConfirmed = false
			}
			return receipt, failure("publish_precheck")
		}
		defer func() { cleanup(prepared.Close) }()
		previous := []protocol.PublishReceipt{}
		submit := input.Step.SubmitForReview != nil && *input.Step.SubmitForReview
		automatic := input.Step.AutomaticRelease != nil && *input.Step.AutomaticRelease
		for {
			action, err := prepared.NextAction(ctx, previous, submit, automatic)
			if err != nil {
				return receipt, failure("publish_precheck")
			}
			if action == nil {
				return receipt, nil
			}
			if len(previous) > 0 {
				action.PreviousIntentID = previous[len(previous)-1].IntentID
			}
			grant, err := authority(action)
			if err != nil {
				return receipt, err
			}
			receipt, err = distribute.UploadApple(ctx, prepared, grant, onStart)
			if recordErr := e.publisherReceipt(ctx, receipt); recordErr != nil {
				e.lease.cancel()
				return receipt, recordErr
			}
			if err != nil {
				return receipt, err
			}
			previous = append(previous, receipt)
			if len(previous) > 6 {
				return receipt, errors.New("publish_precheck")
			}
		}
	}
	return receipt, failure("publish_precheck")
}
func (e *taskExecution) publishManifest(ctx context.Context, p *protocol.ExecutionProgress) error {
	if p.Kind != "build_finished" {
		return nil
	}
	j := e.journal
	j.mu.Lock()
	items := append([]publishCheckpoint{}, j.state.Publishes...)
	j.mu.Unlock()
	for _, item := range items {
		var state store.NodePublishState
		lookup := protocol.PublishLookup{Ref: e.lease.ref, Index: item.Index, IntentID: item.IntentID}
		if err := e.client.post(ctx, "/api/agent/publishes/lookup", lookup, &state); err != nil {
			return err
		}
		if state.IntentID != item.IntentID || state.Ref != e.lease.ref || !state.StepClosed {
			return failure("invalid_response")
		}
		if state.Authorized {
			p.PublishIntents = append(p.PublishIntents, protocol.PublishExpectation{IntentID: item.IntentID, Status: state.Status, ReceiptDigest: state.ReceiptDigest})
		}
	}
	return nil
}
