package store

import (
	"context"
	"encoding/json"
	"io"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

// Recover只核对已有持久证据，成功前不改变任何可调度身份；不会恢复旧动作。
func (s *Store) Recover(ctx context.Context) error {
	err := s.write(ctx, func(tx *gorm.DB) error {
		if err := validatePublishRecovery(tx); err != nil {
			return err
		}
		cursor := ""
		for {
			var rows []buildRecord
			if err := tx.Where("id > ?", cursor).Order("id").Limit(100).Find(&rows).Error; err != nil {
				return errDatabase
			}
			for _, row := range rows {
				if err := validateRecoveryBuild(tx, row); err != nil {
					return errDatabase
				}
				cursor = row.ID
			}
			if len(rows) < 100 {
				return nil
			}
		}
	})
	if err != nil {
		return err
	}
	// 全部核对完成才沿既有过期规则保护执行，当前有效期限/身份/预算不写回。
	return s.ExpireLeases(ctx)
}
func validateRecoveryBuild(db *gorm.DB, row buildRecord) error {
	if !validUUID(row.ID) || !validUUID(row.BatchID) || !validUUID(row.ProjectID) || !slices.Contains([]string{"queued", "skipped", "running", "succeeded", "failed", "cancelled", "interrupted", "waiting_approval", "approved"}, row.Status) || row.StopUnconfirmed && row.Status != "interrupted" {
		return errDatabase
	}
	hasRef := row.NodeID != nil || row.SessionID != nil || row.AttemptID != nil || row.LeaseID != nil || row.LeaseExpiresAt != nil || row.LeaseEpoch != 0
	if hasRef {
		if !validRef(buildRef(row)) || row.LeaseExpiresAt == nil || row.LeaseExpiresAt.IsZero() || row.Status == "queued" {
			return errDatabase
		}
		var attempt attemptRecord
		if err := db.First(&attempt, "id = ? AND build_id = ?", *row.AttemptID, row.ID).Error; err != nil {
			return errDatabase
		}
		if attempt.NodeID != *row.NodeID || attempt.SessionID != *row.SessionID || attempt.LeaseID != *row.LeaseID || attempt.Epoch != row.LeaseEpoch || !attempt.LeaseExpiresAt.Equal(*row.LeaseExpiresAt) {
			return errDatabase
		}
		var session nodeSessionRecord
		if err := db.First(&session, "id = ? AND node_id = ?", attempt.SessionID, attempt.NodeID).Error; err != nil {
			return errDatabase
		}
		var node nodeRecord
		if db.First(&node, "id = ?", attempt.NodeID).Error != nil {
			return errDatabase
		}
		var credential nodeCredentialRecord
		if err := db.First(&credential, "id = ? AND node_id = ?", attempt.CredentialID, attempt.NodeID).Error; err != nil {
			return errDatabase
		}
		if row.Status == "running" && (node.SessionID == nil || *node.SessionID != attempt.SessionID || node.State == "disabled" || node.State == "deleted" || credential.RevokedAt != nil) {
			return errDatabase
		}
		if session.CredentialID != attempt.CredentialID {
			return errDatabase
		}
	} else if row.Status == "running" || row.Status == "interrupted" || row.StopUnconfirmed {
		return errDatabase
	}
	if row.Status == "queued" && (row.CancelRequested || row.PostPhase != "" || row.LastEventSeq != 0 || row.LastLogSeq != 0 || row.LastLogOffset != 0 || row.LastArtifactSeq != 0) {
		return errDatabase
	}
	if row.CurrentApprovalID != nil {
		if err := validateApprovalRecovery(db, row); err != nil {
			return err
		}
	}
	if row.Status != "queued" && row.Status != "running" && row.Status != "waiting_approval" && row.Status != "approved" {
		return nil
	} // 合法终态沿原判定，不重新计算条件或结果。
	_, _, err := frozenBuild(db, row)
	return err
}

// frozenBuild对原快照和原步骤作结构核对，不从当前设置/环境重新判断when。
func frozenBuild(db *gorm.DB, row buildRecord) (BuildSnapshot, []stepRecord, error) {
	var snapshot BuildSnapshot
	if len(row.SnapshotJSON) > 1<<20 {
		return snapshot, nil, errDatabase
	}
	decoder := json.NewDecoder(strings.NewReader(row.SnapshotJSON))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&snapshot) != nil || decoder.Decode(new(any)) != io.EOF {
		return snapshot, nil, errDatabase
	}
	var project projectRecord
	var batch batchRecord
	if db.First(&project, "id = ?", row.ProjectID).Error != nil || db.First(&batch, "id = ?", row.BatchID).Error != nil || batch.ProjectID != row.ProjectID {
		return snapshot, nil, errDatabase
	}
	if !validHex(batch.SHA, 40, 64) || !validHex(batch.SourceDigest, 64) || !sourceFileValid(batch.File) || !slices.Contains([]string{"auto", "repo", "profile"}, batch.Source) || !validBranchPattern(batch.Branch) || strings.ContainsAny(batch.Branch, "*?[") {
		return snapshot, nil, errDatabase
	}
	if !validateOrigin(snapshot.Origin, snapshot.Definition) || !originBatchMatches(snapshot.Origin, batch.SHA, batch.Source, batch.File) {
		return snapshot, nil, errDatabase
	}
	if !validName(row.Name) || !validList(snapshot.AllowedNodes, 128, validName) || snapshot.DefaultNode != "" && !slices.Contains(snapshot.AllowedNodes, snapshot.DefaultNode) {
		return snapshot, nil, errDatabase
	}
	if snapshot.Facts["project"] != project.Name || snapshot.Facts["build.name"] != row.Name || snapshot.Facts["build.id"] != row.ID || snapshot.Facts["git.sha"] != batch.SHA || snapshot.Facts["git.branch"] != batch.Branch || row.Number == nil || *row.Number <= 0 || snapshot.Facts["build.number"] != strconv.FormatInt(*row.Number, 10) {
		return snapshot, nil, errDatabase
	}
	if snapshot.Condition != row.Condition || row.Condition != "ready" {
		return snapshot, nil, errDatabase
	}
	if !monotoneBudget(row.InitialBudgetNS, row.RemainingBudgetNS) || row.PostBudgetNS <= 0 || row.RemainingPostBudgetNS < 0 || row.RemainingPostBudgetNS > row.PostBudgetNS || !slices.Contains([]string{"", "success", "failure", "none"}, row.PostPhase) || row.LastEventSeq < 0 || row.LastLogSeq < 0 || row.LastLogOffset < 0 || row.LastArtifactSeq < 0 {
		return snapshot, nil, errDatabase
	}
	var reasons []string
	var keys []string
	if json.Unmarshal([]byte(row.ReasonsJSON), &reasons) != nil || !reflect.DeepEqual(reasons, snapshot.Reasons) || json.Unmarshal([]byte(row.ParameterKeysJSON), &keys) != nil || len(keys) != len(snapshot.Params) {
		return snapshot, nil, errDatabase
	}
	seen := map[string]bool{}
	for _, key := range keys {
		if _, ok := snapshot.Params[key]; !ok || seen[key] {
			return snapshot, nil, errDatabase
		}
		seen[key] = true
	}
	steps, err := loadSteps(db, row.ID)
	if err != nil {
		return snapshot, nil, errDatabase
	}
	prepared := PreparedBuild{Name: row.Name, Status: "queued", Snapshot: snapshot, InitialBudgetNS: row.InitialBudgetNS, PostBudgetNS: row.PostBudgetNS}
	for _, step := range steps {
		var why []string
		if json.Unmarshal([]byte(step.ReasonsJSON), &why) != nil || step.ElapsedNS < 0 || !slices.Contains([]string{"pending", "intent", "started", "succeeded", "failed", "cancelled", "skipped", "waiting_approval"}, step.Status) || step.Started && !step.Intent || step.CleanupFailed && step.StopConfirmed {
			return snapshot, nil, errDatabase
		}
		if (step.Status == "intent" || step.Status == "started") && (step.StopConfirmed || step.CleanupFailed) {
			return snapshot, nil, errDatabase
		}
		if step.Status == "waiting_approval" && (step.Kind != "approval" || !step.Intent || step.Started || !step.StopConfirmed || step.CleanupFailed || row.CurrentApprovalID == nil) {
			return snapshot, nil, errDatabase
		}
		if step.Status == "intent" && (!step.Intent || step.Started) || step.Status == "started" && (!step.Intent || !step.Started) || step.Status == "pending" && (step.Intent || step.Started || step.StopConfirmed || step.CleanupFailed) || step.Status == "succeeded" && ((!step.Started && step.Kind != "approval") || !step.StopConfirmed || step.CleanupFailed) {
			return snapshot, nil, errDatabase
		}
		status := "pending"
		if step.Phase == "ordinary" && step.Condition == "skipped" {
			status = "skipped"
		}
		prepared.Steps = append(prepared.Steps, StepProgress{Phase: step.Phase, Index: step.Index, Name: step.Name, Kind: step.Kind, Condition: step.Condition, Status: status, Reasons: why})
	}
	valid, _, err := validatePrepared(prepared)
	if err != nil || !reflect.DeepEqual(valid.Snapshot.Params, snapshot.Params) {
		return snapshot, nil, errDatabase
	}
	return snapshot, steps, nil
}
