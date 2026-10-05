package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"path"
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type RetryInput struct {
	BuildID     string
	Key         string
	AllowUpload bool
}

func (s *Store) Retry(ctx context.Context, actor Actor, in RetryInput) (BatchResult, error) {
	if !validUUID(in.BuildID) || !validKey(in.Key) {
		return BatchResult{}, ErrInvalid
	}
	canonical, _ := json.Marshal(struct {
		Operation   string `json:"operation"`
		BuildID     string `json:"build_id"`
		AllowUpload bool   `json:"allow_upload"`
	}{"retry", in.BuildID, in.AllowUpload})
	hash := sha256.Sum256(canonical)
	digest := hex.EncodeToString(hash[:])
	var result BatchResult
	err := s.write(ctx, func(tx *gorm.DB) error {
		if err := authorize(tx, actor, "admin", "trigger"); err != nil {
			return err
		}
		var original buildRecord
		if err := tx.First(&original, "id = ?", in.BuildID).Error; err != nil {
			return err
		}
		var project projectRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&project, "id = ?", original.ProjectID).Error; err != nil {
			return err
		}
		if original.HistoryState != "live" {
			// 已退役正文不能再解码；原已确认请求只读返回保留的批次关系。
			replay, err := findRequest(tx, actor, in.Key, digest)
			if err != nil {
				return err
			}
			if replay != nil {
				result = *replay
				return authorize(tx, actor, "admin", "trigger")
			}
			return ErrRetentionRetired
		}
		snapshot, steps, err := frozenBuild(tx, original)
		if err != nil {
			return err
		}
		var batch batchRecord
		if err = tx.First(&batch, "id = ?", original.BatchID).Error; err != nil {
			return err
		}
		allowed, err := retryAuthorization(project, snapshot, batch.Branch, steps, actor, in.AllowUpload)
		if err != nil {
			return err
		}
		replay, err := findRequest(tx, actor, in.Key, digest)
		if err != nil {
			return err
		}
		if replay != nil {
			result = *replay
			return authorize(tx, actor, "admin", "trigger")
		}
		if original.Number == nil || original.StopUnconfirmed || !slices.Contains([]string{"succeeded", "failed", "cancelled", "interrupted"}, original.Status) {
			return ErrConflict
		}
		if err = validateRecoveryBuild(tx, original); err != nil {
			return err
		}
		if err = retryStopped(tx, original, steps); err != nil {
			return err
		}
		for _, step := range steps {
			if step.Condition != "skipped" && step.Kind != "run" && step.Kind != "artifact" && step.Kind != "upload" {
				return ErrInvalid
			}
		}
		if project.NextNumber <= 0 || project.NextNumber == math.MaxInt64 {
			return ErrConflict
		}
		// frozenBuild已深解码原快照；新执行只改身份事实和收窄授权范围。
		id := uuid.NewString()
		number := project.NextNumber
		snapshot.Facts["build.id"] = id
		snapshot.Facts["build.number"] = strconv.FormatInt(number, 10)
		snapshot.AllowedNodes = allowed
		encoded, err := encode(snapshot)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		newBatch := batchRecord{AllowUpload: in.AllowUpload && actor.Role == "admin", ID: uuid.NewString(), ProjectID: project.ID, IdentityID: actor.ID, SHA: batch.SHA, Branch: batch.Branch, Source: batch.Source, File: batch.File, SourceDigest: batch.SourceDigest, CreatedAt: now}
		if err = tx.Create(&newBatch).Error; err != nil {
			return err
		}
		keys := make([]string, 0, len(snapshot.Params))
		for key := range snapshot.Params {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		encodedKeys, _ := encode(keys)
		row := buildRecord{ID: id, RetryOf: &original.ID, BatchID: newBatch.ID, ProjectID: project.ID, Number: &number, Name: original.Name, Status: "queued", SnapshotJSON: encoded, ParameterKeysJSON: encodedKeys, Condition: original.Condition, ReasonsJSON: original.ReasonsJSON, InitialBudgetNS: original.InitialBudgetNS, RemainingBudgetNS: original.InitialBudgetNS, PostBudgetNS: original.PostBudgetNS, CreatedAt: now}
		if err = tx.Create(&row).Error; err != nil {
			return err
		}
		for _, old := range steps {
			status := "pending"
			if old.Phase == "ordinary" && old.Condition == "skipped" {
				status = "skipped"
			}
			step := stepRecord{ID: uuid.NewString(), BuildID: id, Phase: old.Phase, Index: old.Index, Name: old.Name, Kind: old.Kind, Condition: old.Condition, Status: status, ReasonsJSON: old.ReasonsJSON}
			if err = tx.Create(&step).Error; err != nil {
				return err
			}
		}
		changed := tx.Model(&projectRecord{}).Where("id = ? AND next_number = ? AND policy_version = ?", project.ID, number, project.PolicyVersion).Update("next_number", number+1)
		if changed.Error != nil {
			return changed.Error
		}
		if changed.RowsAffected != 1 {
			return ErrConflict
		}
		if err = tx.Create(&requestRecord{ID: uuid.NewString(), IdentityID: actor.ID, Key: in.Key, RequestDigest: digest, BatchID: newBatch.ID, CreatedAt: now}).Error; err != nil {
			return err
		}
		if err = audit(tx, actor, "build_retry", id, original.ID, id); err != nil {
			return err
		}
		var currentProject projectRecord
		var currentOriginal buildRecord
		if tx.First(&currentProject, "id = ?", project.ID).Error != nil || tx.First(&currentOriginal, "id = ?", original.ID).Error != nil {
			return errDatabase
		}
		if currentOriginal.StopUnconfirmed || currentOriginal.Status != original.Status {
			return ErrConflict
		}
		if currentOriginal.HistoryState != "live" {
			return ErrRetentionRetired
		}
		if _, err = retryAuthorization(currentProject, snapshot, batch.Branch, steps, actor, in.AllowUpload); err != nil {
			return err
		}
		if err = authorize(tx, actor, "admin", "trigger"); err != nil {
			return err
		}
		result, err = batchView(tx, newBatch.ID)
		return err
	})
	if err != nil {
		return BatchResult{}, err
	}
	return result, nil
}
func retryAuthorization(project projectRecord, snapshot BuildSnapshot, branch string, steps []stepRecord, actor Actor, allowUpload bool) ([]string, error) {
	var current, branches []string
	if json.Unmarshal([]byte(project.NodesJSON), &current) != nil || !validList(current, 128, validName) || json.Unmarshal([]byte(project.BranchesJSON), &branches) != nil || !validList(branches, 128, validBranchPattern) {
		return nil, errDatabase
	}
	permitted := false
	for _, pattern := range branches {
		match, _ := path.Match(pattern, branch)
		permitted = permitted || match
	}
	if !permitted {
		return nil, ErrForbidden
	}
	allowed := []string{}
	for _, node := range snapshot.AllowedNodes {
		if slices.Contains(current, node) {
			allowed = append(allowed, node)
		}
	}
	if len(allowed) == 0 || snapshot.Definition.Runner == nil && (snapshot.DefaultNode == "" || !slices.Contains(allowed, snapshot.DefaultNode)) {
		return nil, ErrForbidden
	}
	for _, step := range steps {
		if step.Kind == "upload" && (actor.Role != "admin" || !allowUpload) {
			return nil, ErrForbidden
		}
	}
	return allowed, nil
}
func retryStopped(db *gorm.DB, row buildRecord, steps []stepRecord) error {
	if row.AttemptID == nil {
		if row.Status != "cancelled" || !row.CancelRequested {
			return ErrConflict
		}
		for _, step := range steps {
			if step.Intent || step.Started || step.StopConfirmed || step.CleanupFailed {
				return ErrConflict
			}
		}
		return nil
	}
	unknown := row.Status == "interrupted"
	for _, step := range steps {
		unknown = unknown || !terminalStep(step.Status) || !step.StopConfirmed || step.CleanupFailed
	}
	if !unknown {
		return nil
	}
	var confirmation stopConfirmationRecord
	if err := db.First(&confirmation, "attempt_id = ?", *row.AttemptID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return ErrStopUnconfirmed
		}
		return err
	}
	ref := buildRef(row)
	if confirmation.BuildID != row.ID || confirmation.NodeID != ref.NodeID || confirmation.SessionID != ref.SessionID || confirmation.LeaseID != ref.LeaseID || confirmation.Epoch != ref.Epoch {
		return ErrStopUnconfirmed
	}
	return nil
}
