package store

import (
	"encoding/json"
	"fmt"

	"gorm.io/gorm"

	"mybuilds/internal/protocol"

	"slices"

	"unicode"
)

func validCustomBinding(in *protocol.CustomBindingEvidence) bool {
	if in == nil || in.Source != "manual_attested" || in.EvidenceCode != "ownership_attested" || len(in.Note) < 1 || len(in.Note) > 2048 || !validDigest(in.EvidenceSHA256) {
		return false
	}
	for _, r := range in.Note {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

var customLifecycles = []string{"unknown", "uploaded", "processing", "submitted", "published", "failed"}

func validCustomEvidence(in *protocol.CustomPublishEvidence) bool {
	if in == nil || !slices.Contains(customLifecycles, in.Lifecycle) || len(in.RemoteID) > 128 || (in.RequestSHA256 != "" && !validDigest(in.RequestSHA256)) || (in.ResponseSHA256 != "" && !validDigest(in.ResponseSHA256)) {
		return false
	}
	for _, r := range in.RemoteID {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func customQueryContext(tx *gorm.DB, intent publishIntentRecord, grant protocol.PublishGrant) (*protocol.CustomQueryContext, error) {
	var row buildRecord
	if e := tx.First(&row, "id = ?", intent.BuildID).Error; e != nil {
		return nil, e
	}
	task, e := taskSnapshot(tx, row)
	if e != nil {
		return nil, e
	}
	if intent.StepIndex < 1 || intent.StepIndex > len(task.Definition.Steps) {
		return nil, ErrInvalid
	}
	step := task.Definition.Steps[intent.StepIndex-1]
	if step.Target != "custom" || len(step.QueryArgv) == 0 {
		return nil, ErrInvalid
	}
	env := map[string]string{}
	for k, v := range task.Definition.Env {
		env[k] = v
	}
	for k, v := range step.Env {
		env[k] = v
	}
	facts := map[string]string{"project": task.Project, "build.name": task.BuildName, "git.branch": task.Branch, "step.name": step.Name, "git.sha": task.SHA, "build.id": grant.Ref.BuildID, "build.number": fmt.Sprint(grant.VersionCode)}
	c := &protocol.CustomQueryContext{Repository: task.Repository, SHA: task.SHA, WorkingDir: step.WorkingDir, ResultFile: step.ResultFile, QueryArgv: slices.Clone(step.QueryArgv), Environment: env, Params: task.Parameters, Facts: facts, OriginalRef: grant.Ref, AuthorizationDigest: grant.AuthorizationDigest, ArtifactID: grant.ArtifactID, ArtifactSHA256: grant.ArtifactSHA256, ArtifactSize: grant.ArtifactSize, VersionName: grant.VersionName, Number: grant.VersionCode, ReportSealDigest: grant.ReportSealDigest, ReportIDs: slices.Clone(grant.ReportIDs)}
	data, e := json.Marshal(c)
	if e != nil || len(data) > 64<<10 {
		return nil, ErrInvalid
	}
	return c, nil
}
func exactCustomQuery(in *protocol.CustomQueryEvidence, g protocol.PublishGrant) bool {
	if in == nil || g.Custom == nil || in.IntentID != g.IntentID || in.AuthorizationDigest != g.AuthorizationDigest || in.ArtifactID != g.ArtifactID || in.ArtifactSHA256 != g.ArtifactSHA256 || in.VersionName != g.VersionName || in.VersionCode != g.VersionCode || !validCustomEvidence(&in.Remote) {
		return false
	}
	if in.Remote.Lifecycle == "failed" {
		return !in.Remote.ActionConfirmed && slices.Contains([]string{"confirmed_not_sent", "remote_rejected"}, in.EvidenceCode)
	}
	return in.Remote.Lifecycle != "unknown" && in.Remote.ActionConfirmed && in.Remote.RemoteID != "" && slices.Contains([]string{"remote_receipt", "remote_state"}, in.EvidenceCode)
}
