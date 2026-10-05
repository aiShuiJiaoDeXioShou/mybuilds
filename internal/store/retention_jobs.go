package store

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func retentionRound(limit int) (int, error) {
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 100 {
		return 0, ErrRetentionLimit
	}
	return limit, nil
}

// ScheduleRetention只登记退役意图与现有元数据，不做文件IO或改写构建终态。
func (s *Store) ScheduleRetention(ctx context.Context, actor Actor, projectID string, limit int) (RetentionPage, error) {
	limit, err := retentionRound(limit)
	if err != nil {
		return RetentionPage{}, err
	}
	if !validUUID(projectID) {
		return RetentionPage{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var out RetentionPage
	err = s.write(ctx, func(tx *gorm.DB) error {
		if err := authorize(tx, actor, "admin"); err != nil {
			return err
		}
		if err := scheduleRetention(tx, projectID, &limit, time.Now().UTC()); err != nil {
			return err
		}
		var err error
		out, err = evaluateRetention(tx, projectID, time.Now().UTC(), Page{Limit: 100})
		return err
	})
	if err != nil {
		return RetentionPage{}, retentionPolicyError(ctx, err)
	}
	return out, nil
}
func (s *Store) ListRetention(ctx context.Context, actor Actor, projectID string, page Page) (RetentionPage, error) {
	page, err := normalizePage(page)
	if err != nil || !validUUID(projectID) {
		return RetentionPage{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out := RetentionPage{Items: []RetentionEntry{}, Limit: page.Limit, Offset: page.Offset}
	err = s.write(ctx, func(tx *gorm.DB) error {
		if err := authorize(tx, actor, "admin"); err != nil {
			return err
		}
		if _, err := effectiveRetention(tx, projectID); err != nil {
			return err
		}
		var jobs []retentionJobRecord
		if err := tx.Where("project_id = ?", projectID).Order("created_at DESC, id DESC").Limit(page.Limit).Offset(page.Offset).Find(&jobs).Error; err != nil {
			return err
		}
		for _, job := range jobs {
			var row buildRecord
			if err := tx.First(&row, "id = ?", job.BuildID).Error; err != nil {
				return err
			}
			entry, err := retentionJobEntry(tx, row, job)
			if err != nil {
				return err
			}
			entry.Candidate, _, err = retentionCurrentCandidate(tx, row, time.Now().UTC())
			if err != nil {
				return err
			}
			entry.ProtectReasons, err = retentionProtection(tx, row)
			if err != nil {
				return err
			}
			out.Items = append(out.Items, entry)
		}
		return nil
	})
	if err != nil {
		return RetentionPage{}, retentionPolicyError(ctx, err)
	}
	return out, nil
}

func retentionJobEntry(tx *gorm.DB, row buildRecord, job retentionJobRecord) (RetentionEntry, error) {
	var p projectRecord
	if err := tx.First(&p, "id = ?", row.ProjectID).Error; err != nil {
		return RetentionEntry{}, err
	}
	entry := RetentionEntry{BuildID: row.ID, Project: p.Name, BuildName: row.Name, Number: row.Number, TerminalAt: row.TerminalAt, HistoryState: row.HistoryState, CleanedAt: row.CleanedAt, JobID: job.ID, Reason: job.Reason, EvaluatedAt: &job.EvaluatedAt, RetiredAt: job.RetiredAt, CentralCompletedAt: job.CentralCompletedAt, NodeCompletedAt: job.NodeCompletedAt, CentralState: job.State, NodeState: "not_applicable"}
	var err error
	if job.CentralCompletedAt != nil {
		entry.CentralState = "completed"
	}
	var count int64
	if err = tx.Model(&nodeDeletionRecord{}).Where("job_id = ?", job.ID).Count(&count).Error; err != nil {
		return entry, err
	}
	if count > 0 {
		entry.NodeState = "pending"
	}
	if job.NodeCompletedAt != nil {
		entry.NodeState = "completed"
	}
	return entry, nil
}
func retentionCurrentCandidate(tx *gorm.DB, row buildRecord, at time.Time) (bool, EffectiveRetention, error) {
	p, err := effectiveRetention(tx, row.ProjectID)
	if err != nil {
		return false, p, err
	}
	if row.HistoryState == "cleaned" || !retentionTerminal(row.Status) || row.TerminalAt == nil || row.TerminalAt.IsZero() {
		return false, p, nil
	}
	var newer int64
	err = tx.Model(&buildRecord{}).Where("project_id = ? AND history_state <> 'cleaned' AND status IN ? AND terminal_at IS NOT NULL AND (terminal_at > ? OR (terminal_at = ? AND id > ?))", row.ProjectID, []string{"succeeded", "failed", "cancelled", "skipped", "interrupted"}, *row.TerminalAt, *row.TerminalAt, row.ID).Count(&newer).Error
	return newer >= p.Builds || row.TerminalAt.Before(at.Add(-time.Duration(p.Days)*24*time.Hour)), p, err
}
func scheduleRetention(tx *gorm.DB, projectID string, remaining *int, at time.Time) error {
	policy, err := effectiveRetention(tx, projectID)
	if err != nil {
		return err
	}
	rows, err := retentionCandidateRows(tx, projectID, policy, at, *remaining)
	if err != nil {
		return err
	}
	for _, row := range rows {
		*remaining--
		if err := tx.Model(&projectRecord{}).Where("id = ?", projectID).UpdateColumn("retention_candidate_cursor", row.ID).Error; err != nil {
			return err
		}
		if err := tx.Statement.Context.Err(); err != nil {
			return err
		}
		reasons, err := retentionProtection(tx, row)
		if err != nil {
			return err
		}
		if !retentionBusinessClear(reasons) {
			continue
		}
		var job retentionJobRecord
		err = tx.First(&job, "build_id = ?", row.ID).Error
		if err == nil {
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if row.HistoryState != "live" {
			return ErrRetentionObjectInvalid
		}
		job = retentionJobRecord{ID: uuid.NewString(), ProjectID: row.ProjectID, BuildID: row.ID, State: "pending", GlobalVersion: policy.GlobalVersion, ProjectVersion: policy.ProjectVersion, EvaluatedAt: at, RetiredAt: &at, CreatedAt: at, UpdatedAt: at}
		if len(reasons) > 0 {
			job.State = "waiting"
			job.Reason = "readers_active"
		}
		if err = tx.Create(&job).Error; err != nil {
			return err
		}
		if err = registerRetentionObjects(tx, job, row, at); err != nil {
			return err
		}
		if err = tx.Model(&row).Update("history_state", "retiring").Error; err != nil {
			return err
		}
		if err = retentionCentralProgress(tx, &job, at); err != nil {
			return err
		}
	}
	return nil
}
func retentionBusinessClear(reasons []string) bool {
	for _, r := range reasons {
		if r != "readers_active" {
			return false
		}
	}
	return true
}
func registerRetentionObjects(tx *gorm.DB, job retentionJobRecord, row buildRecord, at time.Time) error {
	var logs []logChunkRecord
	if err := tx.Where("build_id = ?", row.ID).Limit(retentionRelationLimit + 1).Find(&logs).Error; err != nil {
		return err
	}
	var files []artifactRecord
	if err := tx.Where("build_id = ?", row.ID).Limit(retentionRelationLimit + 1).Find(&files).Error; err != nil {
		return err
	}
	if len(logs)+len(files) > retentionRelationLimit {
		return ErrRetentionLimit
	}
	add := func(kind, id, storage, sha string, size int64) error {
		if !validUUID(id) || !validUUID(storage) || !validDigest(sha) || size < 0 {
			return ErrRetentionObjectInvalid
		}
		o := retentionObjectRecord{ID: uuid.NewString(), JobID: job.ID, Kind: kind, ObjectID: id, StorageID: storage, SHA256: sha, Size: size, State: "retired", CreatedAt: at, UpdatedAt: at}
		return tx.Create(&o).Error
	}
	for _, l := range logs {
		if err := add("log", l.ID, l.StorageID, l.Digest, l.Size); err != nil {
			return err
		}
	}
	for _, f := range files {
		kind := "artifact"
		if f.Purpose == "junit" {
			kind = "junit"
		} else if f.Purpose != "" && f.Purpose != "artifact" {
			return ErrRetentionObjectInvalid
		}
		if err := add(kind, f.ID, f.StorageID, f.SHA256, f.Size); err != nil {
			return err
		}
	}
	var resources []nodeResourceRecord
	if err := tx.Where("build_id = ?", row.ID).Limit(retentionRelationLimit + 1).Find(&resources).Error; err != nil {
		return err
	}
	if len(resources) > retentionRelationLimit {
		return ErrRetentionLimit
	}
	for _, r := range resources {
		d := nodeDeletionRecord{ID: uuid.NewString(), JobID: job.ID, ResourceID: r.ID, NodeID: r.NodeID, BuildID: row.ID, AttemptID: r.AttemptID, OwnershipDigest: r.OwnershipDigest, State: "pending", CreatedAt: at, UpdatedAt: at}
		if err := tx.Create(&d).Error; err != nil {
			return err
		}
	}
	return nil
}
func retentionCentralProgress(tx *gorm.DB, job *retentionJobRecord, at time.Time) error {
	var count int64
	if err := tx.Model(&retentionObjectRecord{}).Where("job_id = ? AND state <> 'deleted'", job.ID).Count(&count).Error; err != nil {
		return err
	}
	if count != 0 || job.CentralCompletedAt != nil {
		return nil
	}
	job.CentralCompletedAt = &at
	return tx.Model(job).Update("central_completed_at", at).Error
}

// AdvanceRetention只返回当前重新授权可消费的中央事项；空项目仅受信后台使用。
func (s *Store) AdvanceRetention(ctx context.Context, projectID string, limit int) ([]RetentionObject, error) {
	limit, err := retentionRound(limit)
	if err != nil {
		return nil, err
	}
	if projectID != "" && !validUUID(projectID) {
		return nil, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out := []RetentionObject{}
	err = s.write(ctx, func(tx *gorm.DB) error {
		var ids []string
		if projectID == "" {
			var err error
			ids, err = retentionProjectWindow(tx)
			if err != nil {
				return err
			}
		} else {
			var p projectRecord
			if err := tx.First(&p, "id = ?", projectID).Error; err != nil {
				return err
			}
			ids = []string{projectID}
		}
		remaining := limit
		for _, id := range ids {
			if remaining == 0 {
				break
			}
			if err := scheduleRetention(tx, id, &remaining, time.Now().UTC()); err != nil {
				return err
			}
			if projectID == "" {
				if err := tx.Model(&retentionPolicyRecord{}).Where("id = ?", 1).UpdateColumn("project_cursor", id).Error; err != nil {
					return err
				}
			}
		}
		objects, err := retentionObjectWindow(tx, projectID, limit)
		if err != nil {
			return err
		}
		for _, o := range objects {
			if projectID == "" {
				if err := tx.Model(&retentionPolicyRecord{}).Where("id = ?", 1).UpdateColumn("object_cursor", o.ID).Error; err != nil {
					return err
				}
			} else {
				if err := tx.Model(&projectRecord{}).Where("id = ?", projectID).UpdateColumn("retention_object_cursor", o.ID).Error; err != nil {
					return err
				}
			}
			row, job, err := retentionObjectContext(tx, o)
			if err != nil {
				return err
			}
			if err = retentionDeletionAllowed(tx, row); err != nil && err != ErrRetentionReadersActive {
				if err != ErrRetentionProtected && err != ErrRetentionReadersActive {
					return err
				}
				if err = tx.Model(&job).Updates(map[string]any{"state": "paused", "reason": "retention_protected"}).Error; err != nil {
					return err
				}
				continue
			}
			out = append(out, retentionObjectView(o, row.ID))
		}
		return nil
	})
	if err != nil {
		return nil, retentionPolicyError(ctx, err)
	}
	return out, nil
}
func retentionObjectContext(tx *gorm.DB, o retentionObjectRecord) (buildRecord, retentionJobRecord, error) {
	var j retentionJobRecord
	var b buildRecord
	err := tx.First(&j, "id = ?", o.JobID).Error
	if err == nil {
		err = tx.First(&b, "id = ?", j.BuildID).Error
	}
	if err == nil && (j.ProjectID != b.ProjectID || !slices.Contains([]string{"retiring", "partial"}, b.HistoryState)) {
		err = ErrRetentionObjectInvalid
	}
	return b, j, err
}
func retentionDeletionAllowed(tx *gorm.DB, row buildRecord) error {
	eligible, _, err := retentionCurrentCandidate(tx, row, time.Now().UTC())
	if err != nil {
		return err
	}
	if !eligible {
		return ErrRetentionProtected
	}
	reasons, err := retentionProtection(tx, row)
	if err != nil {
		return err
	}
	if !retentionBusinessClear(reasons) {
		return ErrRetentionProtected
	}
	if slices.Contains(reasons, "readers_active") {
		return ErrRetentionReadersActive
	}
	return nil
}
func retentionObjectView(o retentionObjectRecord, build string) RetentionObject {
	return RetentionObject{ID: o.ID, JobID: o.JobID, BuildID: build, Kind: o.Kind, ObjectID: o.ObjectID, StorageID: o.StorageID, Size: o.Size, SHA256: o.SHA256, Identity: o.Identity, QuarantineSlot: o.QuarantineSlot, State: o.State, Reason: o.Reason}
}
