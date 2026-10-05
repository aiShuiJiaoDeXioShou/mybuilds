package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
)

var publishApplication = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(?:\.[A-Za-z][A-Za-z0-9_-]*)+$`)
var publishVersion = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+){0,3}(?:[-+][A-Za-z0-9.-]+)?$`)
var publishTrack = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
var appleActions = []string{"upload_binary", "select_build", "set_release_policy", "create_review", "add_review_item", "submit_review"}

func applicationView(r applicationRecord) (ApplicationView, error) {
	out := ApplicationView{ID: r.ID, ProjectID: r.ProjectID, NodeID: r.NodeID, Store: r.Store, AppIdentifier: r.AppIdentifier, Status: r.Status, VerificationSource: r.VerificationSource, UploadCertificateSHA256: r.UploadCertificateSHA256, VerifiedAt: r.VerifiedAt, AllowedTracks: []string{}}
	if json.Unmarshal([]byte(r.TracksJSON), &out.AllowedTracks) != nil || out.AllowedTracks == nil {
		return out, errDatabase
	}
	return out, nil
}
func (s *Store) BindApplication(ctx context.Context, actor Actor, in BindApplicationInput) (ApplicationView, error) {
	if actor.Role != "admin" {
		return ApplicationView{}, ErrForbidden
	}
	if !validUUID(in.ProjectID) || len(in.AppIdentifier) > 255 || !publishApplication.MatchString(in.AppIdentifier) || !slices.Contains([]string{"google_play", "app_store", "custom"}, in.Store) || (in.Store != "custom" || in.CredentialRef != "") && !secretReference(in.CredentialRef) {
		return ApplicationView{}, ErrInvalid
	}
	if in.Store == "google_play" && !validDigest(in.UploadCertificateSHA256) || in.Store == "app_store" && in.UploadCertificateSHA256 != "" {
		return ApplicationView{}, ErrInvalid
	}
	if in.Store == "custom" {
		if !validCustomBinding(in.Custom) || in.UploadCertificateSHA256 != "" || len(in.AllowedTracks) != 0 {
			return ApplicationView{}, ErrInvalid
		}
	} else if in.Custom != nil {
		return ApplicationView{}, ErrInvalid
	}
	tracks := slices.Clone(in.AllowedTracks)
	if len(tracks) == 0 && in.Store != "custom" {
		tracks = []string{"internal"}
	}
	slices.Sort(tracks)
	if len(tracks) > 32 {
		return ApplicationView{}, ErrInvalid
	}
	for i, t := range tracks {
		if !publishTrack.MatchString(t) || i > 0 && t == tracks[i-1] {
			return ApplicationView{}, ErrInvalid
		}
	}
	var out ApplicationView
	err := s.write(ctx, func(tx *gorm.DB) error {
		if err := authorize(tx, actor, "admin"); err != nil {
			return err
		}
		var project projectRecord
		if err := tx.First(&project, "id = ?", in.ProjectID).Error; err != nil {
			return err
		}
		var node nodeRecord
		if err := tx.First(&node, "(id = ? OR name = ?) AND state <> ?", in.NodeID, in.NodeID, "deleted").Error; err != nil {
			return err
		}
		var nodes []string
		if json.Unmarshal([]byte(project.NodesJSON), &nodes) != nil {
			return errDatabase
		}
		if !slices.Contains(nodes, node.Name) {
			return ErrForbidden
		}
		var old applicationRecord
		err := tx.First(&old, "store = ? AND app_identifier = ?", in.Store, in.AppIdentifier).Error
		text, _ := encode(tracks)
		evidence := ""
		if in.Custom != nil {
			evidence, _ = encode(in.Custom)
		}
		if err == nil {
			if old.ProjectID != in.ProjectID || old.NodeID != node.ID || old.CredentialRef != in.CredentialRef || old.UploadCertificateSHA256 != in.UploadCertificateSHA256 || old.TracksJSON != text || old.CustomEvidenceJSON != evidence {
				return ErrConflict
			}
			out, err = applicationView(old)
			return err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		row := applicationRecord{ID: uuid.NewString(), ProjectID: in.ProjectID, NodeID: node.ID, Store: in.Store, AppIdentifier: in.AppIdentifier, CredentialRef: in.CredentialRef, UploadCertificateSHA256: in.UploadCertificateSHA256, TracksJSON: text, Status: "pending"}
		if in.Store == "custom" {
			now := time.Now().UTC()
			row.Status = "verified"
			row.VerifiedAt = &now
			row.VerificationSource = "manual_attested"
			row.CustomEvidenceJSON = evidence
		}
		if err = tx.Create(&row).Error; err != nil {
			return err
		}
		if in.Store == "custom" {
			if err = audit(tx, actor, "custom_binding_attested", row.ID, "", ""); err != nil {
				return err
			}
		} else if err = createPublishQuery(tx, actor, row, nil, "doctor"); err != nil {
			return err
		}
		out, err = applicationView(row)
		return err
	})
	return out, err
}
func (s *Store) ListApplications(ctx context.Context, projectID string) ([]ApplicationView, error) {
	if !validUUID(projectID) {
		return nil, ErrInvalid
	}
	if err := s.CheckLock(ctx); err != nil {
		return nil, err
	}
	rows := []applicationRecord{}
	if err := s.db.WithContext(ctx).Where("project_id = ?", projectID).Order("id").Limit(100).Find(&rows).Error; err != nil {
		return nil, safeError(err)
	}
	out := []ApplicationView{}
	for _, row := range rows {
		v, e := applicationView(row)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, nil
}

// 只供原执行节点取得明确绑定的预检信息，不含材料内容，也不授商店写权限。
type NodeApplicationMaterial struct {
	BindingID               string `json:"binding_id"`
	CredentialRef           string `json:"credential_ref"`
	UploadCertificateSHA256 string `json:"upload_certificate_sha256,omitempty"`
}

func (s *Store) FindNodeApplication(ctx context.Context, actor NodeActor, ref protocol.LeaseRef, target, app string) (NodeApplicationMaterial, error) {
	var out NodeApplicationMaterial
	err := s.write(ctx, func(tx *gorm.DB) error {
		row, e := s.currentExecution(tx, actor, ref)
		if e != nil {
			return e
		}
		if row.HistoryState != "live" {
			return ErrRetentionRetired
		}
		var binding applicationRecord
		if e = tx.First(&binding, "project_id = ? AND node_id = ? AND store = ? AND app_identifier = ? AND status = ?", row.ProjectID, actor.ID, target, app, "verified").Error; e != nil {
			return e
		}
		out = NodeApplicationMaterial{binding.ID, binding.CredentialRef, binding.UploadCertificateSHA256}
		return checkBoundary(tx, actor, ref, *row.LeaseExpiresAt)
	})
	return out, err
}
func renderPublishValue(value string, task protocol.TaskSnapshot) (string, error) {
	facts := map[string]string{}
	for k, v := range task.Facts {
		facts[k] = v
	}
	// 仅从Store派生的快照身份建立远程签名输出，不采信传入动态事实。
	delete(facts, "ios.output_dir")
	if task.Definition.IOSSigning != nil {
		id := task.Facts["build.id"]
		if !validUUID(id) {
			return "", ErrInvalid
		}
		facts["ios.output_dir"] = ".mybuilds-ios-" + id
	}
	facts["build.number"] = fmt.Sprint(task.Number)
	rendered, missing, err := config.RenderField(value, "publish", task.Parameters, facts, false, false)
	if err != nil || missing || strings.ContainsAny(rendered, "{}\r\n\x00") {
		return "", ErrInvalid
	}
	return rendered, nil
}
func publishReports(tx *gorm.DB, row buildRecord, in protocol.PublishAuthorization) error {
	cfg, err := configuredReport(row)
	if err != nil {
		return err
	}
	if in.ReportIDs == nil || !validateArtifactIDs(in.ReportIDs) {
		return ErrInvalid
	}
	if cfg == nil {
		if in.ReportSealDigest != "" || len(in.ReportIDs) != 0 {
			return ErrConflict
		}
		return nil
	}
	evidence, err := storedReports(row)
	if err != nil {
		return err
	}
	if evidence == nil || !row.ReportFinal || !evidence.Sealed || evidence.Outcome != "passed" || row.ReportSealDigest == "" || in.ReportSealDigest != row.ReportSealDigest {
		return ErrConflict
	}
	ids := []string{}
	for _, f := range evidence.Files {
		ids = append(ids, f.ArtifactID)
	}
	slices.Sort(ids)
	copyIDs := slices.Clone(in.ReportIDs)
	slices.Sort(copyIDs)
	if !slices.Equal(ids, copyIDs) {
		return ErrConflict
	}
	return nil
}
func (s *Store) AuthorizePublish(ctx context.Context, actor NodeActor, in protocol.PublishAuthorization) (protocol.PublishGrant, error) {
	if !validUUID(in.IntentID) || !validRef(in.Ref) || in.Index < 1 || !validName(in.StepName) || !validUUID(in.ArtifactID) || !validDigest(in.ArtifactSHA256) || in.ArtifactSize <= 0 || in.ArtifactSize > 1<<30 || in.VersionCode < 1 || in.VersionCode > 2100000000 || len(in.VersionName) > 128 || !publishVersion.MatchString(in.VersionName) {
		return protocol.PublishGrant{}, ErrInvalid
	}
	var grant protocol.PublishGrant
	err := s.write(ctx, func(tx *gorm.DB) error {
		row, err := s.currentExecution(tx, actor, in.Ref)
		if err != nil {
			return err
		}
		if row.CancelRequested || row.HistoryState != "live" {
			return ErrConflict
		}
		if row.RemainingBudgetNS != nil && *row.RemainingBudgetNS <= 0 {
			return ErrBudgetInvalid
		}
		task, err := taskSnapshot(tx, row)
		if err != nil {
			return err
		}
		if in.Index > len(task.Definition.Steps) {
			return ErrInvalid
		}
		step := task.Definition.Steps[in.Index-1]
		if step.Kind != "upload" || step.Name != in.StepName || !slices.Contains([]string{"google_play", "app_store", "custom"}, step.Target) {
			return ErrInvalid
		}
		var slot stepRecord
		if err = tx.First(&slot, "build_id = ? AND phase = ? AND \"index\" = ?", row.ID, "ordinary", in.Index).Error; err != nil {
			return err
		}
		if !slot.Intent || !slices.Contains([]string{"intent", "started"}, slot.Status) {
			return ErrConflict
		}
		var prior []stepRecord
		if err = tx.Where("build_id = ? AND phase = ? AND \"index\" < ?", row.ID, "ordinary", in.Index).Find(&prior).Error; err != nil {
			return err
		}
		for _, p := range prior {
			if p.Status != "succeeded" && p.Status != "skipped" || p.CleanupFailed || !p.StopConfirmed {
				return ErrConflict
			}
		}
		var batch batchRecord
		if err = tx.First(&batch, "id = ?", row.BatchID).Error; err != nil {
			return err
		}
		if !batch.AllowUpload {
			return ErrForbidden
		}
		app, err := renderPublishValue(step.AppIdentifier, task)
		if err != nil || !publishApplication.MatchString(app) {
			return ErrInvalid
		}
		version := task.Parameters["version"]
		if version != in.VersionName || task.Number != in.VersionCode {
			return ErrConflict
		}
		var binding applicationRecord
		if err = tx.First(&binding, "project_id = ? AND node_id = ? AND store = ? AND app_identifier = ? AND status = ?", row.ProjectID, actor.ID, step.Target, app, "verified").Error; err != nil {
			return err
		}
		if binding.CredentialRef != step.Credentials {
			return ErrConflict
		}
		if err = validatePublishApprovals(tx, row, in.Index, in); err != nil {
			return err
		}
		if err = publishReports(tx, row, in); err != nil {
			return err
		}
		var artifact artifactRecord
		if err = tx.First(&artifact, "id = ? AND build_id = ? AND attempt_id = ? AND phase = ?", in.ArtifactID, row.ID, in.Ref.AttemptID, "ordinary").Error; err != nil {
			return err
		}
		if artifact.Index >= in.Index || artifact.Purpose != "" && artifact.Purpose != "artifact" || artifact.Size != in.ArtifactSize || artifact.SHA256 != in.ArtifactSHA256 {
			return ErrArtifactConflict
		}
		extension := ".aab"
		if step.Target == "app_store" {
			extension = ".ipa"
		}
		if step.Target != "custom" && !strings.HasSuffix(artifact.Name, extension) {
			return ErrArtifactConflict
		}
		if err = publicationArtifact(tx, row, task, step, in.Index, artifact); err != nil {
			return err
		}
		var count int64
		if err = tx.Model(&publishIntentRecord{}).Where("id = ?", in.IntentID).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return ErrConflict
		}
		action := "upload"
		track := in.Track
		releaseStatus := step.ReleaseStatus
		if releaseStatus == "" {
			releaseStatus = "completed"
		}
		if step.Target == "google_play" {
			if in.Apple != nil || in.Custom != nil {
				return ErrInvalid
			}
			configuredTrack, err := renderPublishValue(step.Track, task)
			if err != nil {
				return err
			}
			if configuredTrack == "" {
				configuredTrack = "internal"
			}
			var tracks []string
			if json.Unmarshal([]byte(binding.TracksJSON), &tracks) != nil {
				return errDatabase
			}
			if track != configuredTrack || !slices.Contains(tracks, track) || track == "production" && !in.ExplicitProduction || !slices.Contains([]string{"draft", "completed"}, releaseStatus) {
				return ErrConflict
			}
		} else if step.Target == "custom" {
			commandDigest, e := protocol.CustomCommandDigest(step.Argv, step.QueryArgv, step.WorkingDir, step.ResultFile, app, version, in.ArtifactID, in.ArtifactSHA256, in.VersionCode, in.ArtifactSize)
			if e != nil || in.Custom == nil || in.Custom.ResultSchemaVersion != 1 || in.Custom.CommandDigest != commandDigest || in.Apple != nil || in.Track != "" || in.ExplicitProduction {
				return ErrInvalid
			}
			action = "custom_upload"
			releaseStatus = ""
		} else {
			if in.Custom != nil || in.Apple == nil || !slices.Contains(appleActions, in.Apple.Action) || !validDigest(in.Apple.RequestSHA256) || track != "" {
				return ErrInvalid
			}
			action = in.Apple.Action
			submit := step.SubmitForReview != nil && *step.SubmitForReview
			automatic := step.AutomaticRelease != nil && *step.AutomaticRelease
			if in.Apple.SubmitForReview != submit || in.Apple.AutomaticRelease != automatic || automatic && !submit {
				return ErrConflict
			}
			position := slices.Index(appleActions, action)
			if position == 0 {
				if in.Apple.PreviousIntentID != "" {
					return ErrConflict
				}
			} else {
				if !submit || !validUUID(in.Apple.PreviousIntentID) {
					return ErrConflict
				}
				var previous publishIntentRecord
				if err = tx.First(&previous, "id = ? AND binding_id = ? AND attempt_id = ? AND step_index = ?", in.Apple.PreviousIntentID, binding.ID, in.Ref.AttemptID, in.Index).Error; err != nil {
					return err
				}
				if previous.Status == "unknown" || previous.Status == "failed" || slices.Index(appleActions, previous.Action) >= position {
					return ErrConflict
				}
			}
		}
		var guard applicationGuardRecord
		err = tx.First(&guard, "binding_id = ?", binding.ID).Error
		if err == nil {
			var held publishIntentRecord
			if e := tx.First(&held, "id = ?", guard.IntentID).Error; e != nil {
				return e
			}
			if step.Target != "app_store" || held.AttemptID != in.Ref.AttemptID || held.StepIndex != in.Index {
				return ErrConflict
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		grant = protocol.PublishGrant{IntentID: in.IntentID, Ref: in.Ref, Action: action, AppIdentifier: app, Track: track, ReleaseName: "mybuilds-" + in.IntentID, ReleaseStatus: releaseStatus, VersionName: version, VersionCode: in.VersionCode, ArtifactID: in.ArtifactID, ArtifactSize: in.ArtifactSize, ArtifactSHA256: in.ArtifactSHA256, ReportSealDigest: in.ReportSealDigest, ReportIDs: slices.Clone(in.ReportIDs), Apple: in.Apple, Custom: in.Custom}
		grant.AuthorizationDigest, err = protocol.PublishGrantDigest(grant)
		if err != nil {
			return ErrInvalid
		}
		requestDigest, _ := protocol.PublishAuthorizationDigest(in)
		text, err := encode(grant)
		if err != nil {
			return err
		}
		intent := publishIntentRecord{ID: in.IntentID, BindingID: binding.ID, BuildID: row.ID, AttemptID: in.Ref.AttemptID, StepIndex: in.Index, Action: action, ArtifactID: &artifact.ID, RequestDigest: requestDigest, GrantJSON: text, Status: "unknown", RemoteJSON: "{}"}
		if err = tx.Create(&intent).Error; err != nil {
			return err
		}
		if guard.BindingID == "" {
			if err = tx.Create(&applicationGuardRecord{BindingID: binding.ID, IntentID: intent.ID}).Error; err != nil {
				return err
			}
		}
		return checkBoundary(tx, actor, in.Ref, *row.LeaseExpiresAt)
	})
	return grant, err
}
func boundedPublishEvidence(remote protocol.PublishRemoteEvidence) bool {
	if remote.Custom != nil && (!validCustomEvidence(remote.Custom) || remote.Apple != nil) {
		return false
	}
	data, err := json.Marshal(remote)
	if err != nil || len(data) > 8192 {
		return false
	}
	for _, v := range []string{remote.EditID, remote.ReleaseName, remote.Track, remote.Lifecycle} {
		if len(v) > 128 || strings.ContainsAny(v, "/\\\x00\r\n") {
			return false
		}
		for _, r := range v {
			if unicode.IsControl(r) {
				return false
			}
		}
	}
	if remote.BundleSHA256 != "" && !validDigest(remote.BundleSHA256) {
		return false
	}
	if a := remote.Apple; a != nil {
		for _, v := range []string{a.AppID, a.BuildID, a.AppStoreVersionID, a.ReviewSubmissionID, a.ReviewItemID, a.ProcessingState, a.VersionState, a.ReviewState, a.ReleaseType, a.TransportID} {
			if len(v) > 128 || strings.ContainsAny(v, "/\\\x00\r\n") {
				return false
			}
			for _, r := range v {
				if unicode.IsControl(r) {
					return false
				}
			}
		}
		if a.RequestSHA256 != "" && !validDigest(a.RequestSHA256) || a.ResponseSHA256 != "" && !validDigest(a.ResponseSHA256) {
			return false
		}
	}
	return true
}
func (s *Store) RecordPublish(ctx context.Context, actor NodeActor, in protocol.PublishReceipt) (PublishView, error) {
	digest, err := protocol.PublishReceiptDigest(in)
	if err != nil || !validUUID(in.IntentID) || !validRef(in.Ref) || digest != in.Digest || !validDigest(in.AuthorizationDigest) || !slices.Contains([]string{"unknown", "uploaded", "processing", "submitted", "published", "failed", "confirmed"}, in.Status) || !boundedPublishEvidence(in.Remote) || !slices.Contains(publishEvidenceCodes, in.EvidenceCode) || !slices.Contains(publishMutationStages, in.MutationStage) || in.CleanupFailed && in.StopConfirmed {
		return PublishView{}, ErrInvalid
	}
	var out PublishView
	err = s.write(ctx, func(tx *gorm.DB) error {
		row, err := s.currentExecution(tx, actor, in.Ref)
		if err != nil {
			return err
		}
		var intent publishIntentRecord
		if err = tx.First(&intent, "id = ? AND build_id = ? AND attempt_id = ?", in.IntentID, row.ID, in.Ref.AttemptID).Error; err != nil {
			return err
		}
		var grant protocol.PublishGrant
		if json.Unmarshal([]byte(intent.GrantJSON), &grant) != nil {
			return errDatabase
		}
		if grant.Ref != in.Ref || grant.AuthorizationDigest != in.AuthorizationDigest {
			return ErrConflict
		}
		if intent.Status != "unknown" && intent.ReceiptDigest == "" {
			return ErrConflict
		}
		if intent.ReceiptDigest != "" {
			if intent.ReceiptDigest != in.Digest {
				return ErrConflict
			}
			out, err = publishView(tx, intent)
			return err
		}
		if grant.Custom != nil {
			if in.Remote.Apple != nil || in.Remote.Custom != nil && !validCustomEvidence(in.Remote.Custom) {
				return ErrInvalid
			}
			if in.Status != "unknown" && (!in.Started || !in.StopConfirmed || in.CleanupFailed || in.MutationStage != "custom_upload" || in.Remote.Custom == nil || in.Remote.Custom.Lifecycle != in.Status) {
				return ErrConflict
			}
			if in.Status != "unknown" && in.Status != "failed" && (!in.Remote.Custom.ActionConfirmed || in.Remote.Custom.RemoteID == "" || !slices.Contains([]string{"remote_receipt", "remote_state"}, in.EvidenceCode)) {
				return ErrConflict
			}
			if in.Status == "failed" && (in.Remote.Custom.ActionConfirmed || !slices.Contains([]string{"confirmed_not_sent", "remote_rejected"}, in.EvidenceCode)) {
				return ErrConflict
			}
		} else if in.Remote.Custom != nil {
			return ErrInvalid
		}
		if in.Status != "unknown" {
			if !in.StopConfirmed || in.CleanupFailed {
				return ErrConflict
			}
			if grant.Custom != nil {
				// 上面的具体custom回执已核完整原授权。
			} else if in.Status == "failed" {
				if in.Started || in.EvidenceCode != "confirmed_not_sent" || in.MutationStage != "not_started" {
					return ErrConflict
				}
			} else if grant.Apple == nil {
				if in.Status != "uploaded" || in.EvidenceCode != "remote_receipt" || !in.Started || !in.Remote.BundleAccepted || !in.Remote.TrackAccepted || !in.Remote.CommitAccepted || in.Remote.BundleSHA256 != grant.ArtifactSHA256 || in.Remote.VersionCode != grant.VersionCode || in.Remote.Track != grant.Track || in.Remote.ReleaseName != grant.ReleaseName {
					return ErrConflict
				}
			} else {
				a := in.Remote.Apple
				if !validAppleActionEvidence(grant, a) || in.Status != "confirmed" || !in.Started || in.EvidenceCode != "remote_receipt" || in.MutationStage != grant.Action {
					return ErrConflict
				}

			}
		}
		text, _ := encode(in)
		remote, _ := encode(in.Remote)
		intent.Status = in.Status
		intent.ReceiptDigest = in.Digest
		intent.ReceiptJSON = text
		intent.RemoteJSON = remote
		intent.EvidenceCode = in.EvidenceCode
		if err = tx.Model(&intent).Updates(map[string]any{"status": intent.Status, "receipt_digest": in.Digest, "receipt_json": text, "remote_json": remote, "evidence_code": in.EvidenceCode}).Error; err != nil {
			return err
		}
		// 中间回执绝不删除guard：只有实际step关闭、完整回执和停止证据一并通过才解除。
		out, err = publishView(tx, intent)
		if err != nil {
			return err
		}
		return checkBoundary(tx, actor, in.Ref, *row.LeaseExpiresAt)
	})
	return out, err
}
func publishView(tx *gorm.DB, intent publishIntentRecord) (PublishView, error) {
	var grant protocol.PublishGrant
	var binding applicationRecord
	var row buildRecord
	var batch batchRecord
	if json.Unmarshal([]byte(intent.GrantJSON), &grant) != nil {
		return PublishView{}, errDatabase
	}
	if err := tx.First(&binding, "id = ?", intent.BindingID).Error; err != nil {
		return PublishView{}, err
	}
	if err := tx.First(&row, "id = ?", intent.BuildID).Error; err != nil {
		return PublishView{}, err
	}
	if err := tx.First(&batch, "id = ?", row.BatchID).Error; err != nil {
		return PublishView{}, err
	}
	var remote protocol.PublishRemoteEvidence
	if json.Unmarshal([]byte(intent.RemoteJSON), &remote) != nil {
		return PublishView{}, errDatabase
	}
	var guards int64
	if err := tx.Model(&applicationGuardRecord{}).Where("binding_id = ?", binding.ID).Count(&guards).Error; err != nil {
		return PublishView{}, err
	}
	digest := publishIntentDigest(intent)
	return PublishView{ID: intent.ID, ProjectID: row.ProjectID, BuildID: row.ID, BuildName: row.Name, Number: grant.VersionCode, SHA: batch.SHA, AttemptID: intent.AttemptID, NodeID: grant.Ref.NodeID, Store: binding.Store, AppIdentifier: binding.AppIdentifier, Action: intent.Action, Track: grant.Track, VersionName: grant.VersionName, VersionCode: grant.VersionCode, ArtifactID: grant.ArtifactID, ArtifactSHA256: grant.ArtifactSHA256, ArtifactSize: grant.ArtifactSize, ReportSealDigest: grant.ReportSealDigest, ReportIDs: grant.ReportIDs, Status: intent.Status, EvidenceCode: intent.EvidenceCode, Remote: remote, ApplicationProtected: guards > 0, IntentDigest: digest, GrantedAt: intent.CreatedAt.UTC(), UpdatedAt: intent.UpdatedAt.UTC()}, nil
}
func publishIntentDigest(in publishIntentRecord) string {
	digest, _ := protocol.PublishReceiptDigest(protocol.PublishReceipt{IntentID: in.ID, AuthorizationDigest: in.RequestDigest, Status: in.Status, EvidenceCode: in.ReceiptDigest, MutationStage: in.RemoteJSON})
	return digest
}
func (s *Store) GetPublish(ctx context.Context, id string) (PublishView, error) {
	if !validUUID(id) {
		return PublishView{}, ErrInvalid
	}
	if err := s.CheckLock(ctx); err != nil {
		return PublishView{}, err
	}
	var row publishIntentRecord
	db := s.db.WithContext(ctx)
	if err := db.First(&row, "id = ?", id).Error; err != nil {
		return PublishView{}, safeError(err)
	}
	out, err := publishView(db, row)
	return out, safeError(err)
}
func (s *Store) ListPublishes(ctx context.Context, projectID string, limit int, after string) ([]PublishView, error) {
	if projectID != "" && !validUUID(projectID) || limit < 1 || limit > 100 || after != "" && !validUUID(after) {
		return nil, ErrInvalid
	}
	if err := s.CheckLock(ctx); err != nil {
		return nil, err
	}
	db := s.db.WithContext(ctx).Model(&publishIntentRecord{}).Joins("JOIN builds ON builds.id = publish_intents.build_id")
	if projectID != "" {
		db = db.Where("builds.project_id = ?", projectID)
	}
	if after != "" {
		db = db.Where("publish_intents.id > ?", after)
	}
	rows := []publishIntentRecord{}
	if err := db.Select("publish_intents.*").Order("publish_intents.id").Limit(limit).Find(&rows).Error; err != nil {
		return nil, safeError(err)
	}
	out := []PublishView{}
	for _, r := range rows {
		v, e := publishView(s.db.WithContext(ctx), r)
		if e != nil {
			return nil, safeError(e)
		}
		out = append(out, v)
	}
	return out, nil
}
func (s *Store) FindNodePublish(ctx context.Context, actor NodeActor, in protocol.PublishLookup) (NodePublishState, error) {
	if !validUUID(in.IntentID) || !validRef(in.Ref) || in.Index < 1 {
		return NodePublishState{}, ErrInvalid
	}
	var out NodePublishState
	err := s.write(ctx, func(tx *gorm.DB) error {
		if err := retentionNodeActor(tx, actor); err != nil {
			return err
		}
		var row buildRecord
		if err := tx.First(&row, "id = ?", in.Ref.BuildID).Error; err != nil {
			return err
		}
		if actor.ID != in.Ref.NodeID {
			return ErrForbidden
		}
		if buildRef(row) != in.Ref {
			allowed, err := approvalHistoricalRef(tx, row, in.Ref)
			if err != nil {
				return err
			}
			if !allowed {
				return ErrForbidden
			}
		}
		var step stepRecord
		if err := tx.First(&step, "build_id = ? AND phase = ? AND \"index\" = ? AND kind = ?", row.ID, "ordinary", in.Index, "upload").Error; err != nil {
			return err
		}
		out = NodePublishState{IntentID: in.IntentID, Ref: in.Ref, StepClosed: terminalStep(step.Status)}
		var intent publishIntentRecord
		err := tx.First(&intent, "id = ?", in.IntentID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if intent.BuildID != row.ID || intent.AttemptID != in.Ref.AttemptID || intent.StepIndex != in.Index {
			return ErrForbidden
		}
		var original protocol.PublishGrant
		if json.Unmarshal([]byte(intent.GrantJSON), &original) != nil || original.Ref != in.Ref || original.IntentID != in.IntentID {
			return ErrForbidden
		}
		d, err := protocol.PublishGrantDigest(original)
		if err != nil || d != original.AuthorizationDigest {
			return ErrConflict
		}
		out.Authorized = true
		out.Status = intent.Status
		out.ReceiptDigest = intent.ReceiptDigest
		return nil
	})
	return out, err
}

// 实际upload步骤终结时调用，同一事务关闭slot并核验所有已授动作。
func closePublishStep(tx *gorm.DB, row buildRecord, step stepRecord) error {
	if step.Kind != "upload" || !terminalStep(step.Status) {
		return nil
	}
	intents := []publishIntentRecord{}
	if err := tx.Where("build_id = ? AND attempt_id = ? AND step_index = ?", row.ID, row.AttemptID, step.Index).Limit(7).Find(&intents).Error; err != nil {
		return err
	}
	if len(intents) > 6 {
		return ErrConflict
	}
	if !step.StopConfirmed || step.CleanupFailed {
		return nil
	}
	for _, intent := range intents {
		if intent.Status == "unknown" {
			return nil
		}
		if intent.ReceiptDigest == "" {
			var decisions int64
			if err := tx.Model(&publishDecisionRecord{}).Where("intent_id = ?", intent.ID).Count(&decisions).Error; err != nil {
				return err
			}
			if decisions == 0 {
				return nil
			}
			continue
		}
		var receipt protocol.PublishReceipt
		if json.Unmarshal([]byte(intent.ReceiptJSON), &receipt) != nil || !receipt.StopConfirmed || receipt.CleanupFailed {
			return nil
		}
	}
	for _, intent := range intents {
		if err := tx.Where("binding_id = ?", intent.BindingID).Delete(&applicationGuardRecord{}).Error; err != nil {
			return err
		}
	}
	return nil
}
func validatePublishManifest(tx *gorm.DB, row buildRecord, p protocol.ExecutionProgress) error {
	if p.Kind != "build_finished" {
		if len(p.PublishIntents) > 0 {
			return ErrEventConflict
		}
		return nil
	}
	intents := []publishIntentRecord{}
	if err := tx.Where("build_id = ? AND attempt_id = ?", row.ID, row.AttemptID).Limit(385).Find(&intents).Error; err != nil {
		return err
	}
	if len(intents) > 384 || len(p.PublishIntents) != len(intents) {
		return ErrEventConflict
	}
	seen := map[string]bool{}
	for _, item := range p.PublishIntents {
		if seen[item.IntentID] {
			return ErrEventConflict
		}
		seen[item.IntentID] = true
		found := false
		for _, row := range intents {
			if row.ID == item.IntentID {
				found = true
				if row.Status != item.Status || row.ReceiptDigest != item.ReceiptDigest {
					return ErrEventConflict
				}
				break
			}
		}
		if !found {
			return ErrEventConflict
		}
	}
	return nil
}
