package store

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// 每次最多两个不重叠窗口，扫描配额不因保护或既有事项而扩大。
func retentionCandidateRows(tx *gorm.DB, projectID string, policy EffectiveRetention, at time.Time, limit int) ([]buildRecord, error) {
	var project projectRecord
	if err := tx.First(&project, "id = ?", projectID).Error; err != nil {
		return nil, err
	}
	cursor := project.RetentionCandidateCursor
	var anchor buildRecord
	if cursor != "" {
		if !validUUID(cursor) {
			return nil, ErrRetentionObjectInvalid
		}
		if err := tx.First(&anchor, "id = ?", cursor).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			cursor = ""
		} else if err != nil {
			return nil, err
		}
		if cursor != "" && (anchor.ProjectID != projectID || anchor.TerminalAt == nil || anchor.TerminalAt.IsZero()) {
			return nil, ErrRetentionObjectInvalid
		}
	}
	query := func() *gorm.DB {
		return tx.Where("project_id = ? AND history_state <> 'cleaned' AND terminal_at IS NOT NULL AND status IN ? AND (terminal_at < ? OR (SELECT COUNT(*) FROM builds newer WHERE newer.project_id = builds.project_id AND newer.history_state <> 'cleaned' AND newer.terminal_at IS NOT NULL AND newer.status IN ? AND (newer.terminal_at > builds.terminal_at OR (newer.terminal_at = builds.terminal_at AND newer.id > builds.id))) >= ?)", projectID, []string{"succeeded", "failed", "cancelled", "skipped", "interrupted"}, at.Add(-time.Duration(policy.Days)*24*time.Hour), []string{"succeeded", "failed", "cancelled", "skipped", "interrupted"}, policy.Builds).Order("terminal_at ASC, id ASC")
	}
	q := query()
	if cursor != "" {
		q = q.Where("terminal_at > ? OR (terminal_at = ? AND id > ?)", *anchor.TerminalAt, *anchor.TerminalAt, cursor)
	}
	rows := []buildRecord{}
	if err := q.Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	if cursor != "" && len(rows) < limit {
		var before []buildRecord
		if err := query().Where("terminal_at < ? OR (terminal_at = ? AND id <= ?)", *anchor.TerminalAt, *anchor.TerminalAt, cursor).Limit(limit - len(rows)).Find(&before).Error; err != nil {
			return nil, err
		}
		rows = append(rows, before...)
	}
	return rows, nil
}

func retentionProjectWindow(tx *gorm.DB) ([]string, error) {
	var policy retentionPolicyRecord
	if err := tx.First(&policy, 1).Error; err != nil {
		return nil, err
	}
	cursor := policy.ProjectCursor
	if cursor != "" {
		if !validUUID(cursor) {
			return nil, ErrRetentionObjectInvalid
		}
	}
	q := tx.Model(&projectRecord{}).Order("id")
	if cursor != "" {
		q = q.Where("id > ?", cursor)
	}
	ids := []string{}
	if err := q.Limit(100).Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	if cursor != "" && len(ids) < 100 {
		var before []string
		if err := tx.Model(&projectRecord{}).Where("id <= ?", cursor).Order("id").Limit(100-len(ids)).Pluck("id", &before).Error; err != nil {
			return nil, err
		}
		ids = append(ids, before...)
	}
	return ids, nil
}

func retentionObjectWindow(tx *gorm.DB, projectID string, limit int) ([]retentionObjectRecord, error) {
	var cursor string
	if projectID == "" {
		var p retentionPolicyRecord
		if err := tx.First(&p, 1).Error; err != nil {
			return nil, err
		}
		cursor = p.ObjectCursor
	} else {
		var p projectRecord
		if err := tx.First(&p, "id = ?", projectID).Error; err != nil {
			return nil, err
		}
		cursor = p.RetentionObjectCursor
	}
	var anchor retentionObjectRecord
	if cursor != "" {
		if !validUUID(cursor) {
			return nil, ErrRetentionObjectInvalid
		}
		if err := tx.First(&anchor, "id = ?", cursor).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			cursor = ""
		} else if err != nil {
			return nil, err
		}
		if cursor != "" {
			var job retentionJobRecord
			if err := tx.First(&job, "id = ?", anchor.JobID).Error; err != nil {
				return nil, err
			}
			if (projectID != "" && job.ProjectID != projectID) || anchor.CreatedAt.IsZero() {
				return nil, ErrRetentionObjectInvalid
			}
		}
	}
	query := func() *gorm.DB {
		q := tx.Model(&retentionObjectRecord{}).Joins("JOIN retention_jobs ON retention_jobs.id = retention_objects.job_id").Where("retention_objects.state <> 'deleted'").Order("retention_objects.created_at ASC, retention_objects.id ASC")
		if projectID != "" {
			q = q.Where("retention_jobs.project_id = ?", projectID)
		}
		return q.Select("retention_objects.*")
	}
	q := query()
	if cursor != "" {
		q = q.Where("retention_objects.created_at > ? OR (retention_objects.created_at = ? AND retention_objects.id > ?)", anchor.CreatedAt, anchor.CreatedAt, cursor)
	}
	rows := []retentionObjectRecord{}
	if err := q.Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	if cursor != "" && len(rows) < limit {
		var before []retentionObjectRecord
		if err := query().Where("retention_objects.created_at < ? OR (retention_objects.created_at = ? AND retention_objects.id <= ?)", anchor.CreatedAt, anchor.CreatedAt, cursor).Limit(limit - len(rows)).Find(&before).Error; err != nil {
			return nil, err
		}
		rows = append(rows, before...)
	}
	return rows, nil
}

func retentionFinalizeWindow(tx *gorm.DB, projectID string, limit int) ([]retentionJobRecord, error) {
	var cursor string
	if projectID == "" {
		var p retentionPolicyRecord
		if err := tx.First(&p, 1).Error; err != nil {
			return nil, err
		}
		cursor = p.FinalizeCursor
	} else {
		var p projectRecord
		if err := tx.First(&p, "id = ?", projectID).Error; err != nil {
			return nil, err
		}
		cursor = p.RetentionFinalizeCursor
	}
	var anchor retentionJobRecord
	if cursor != "" {
		if !validUUID(cursor) {
			return nil, ErrRetentionObjectInvalid
		}
		if err := tx.First(&anchor, "id = ?", cursor).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			cursor = ""
		} else if err != nil {
			return nil, err
		}
		if cursor != "" && ((projectID != "" && anchor.ProjectID != projectID) || anchor.CreatedAt.IsZero()) {
			return nil, ErrRetentionObjectInvalid
		}
	}
	query := func() *gorm.DB {
		q := tx.Model(&retentionJobRecord{}).Joins("JOIN builds ON builds.id = retention_jobs.build_id").Where("builds.history_state <> 'cleaned'").Order("retention_jobs.created_at ASC, retention_jobs.id ASC")
		if projectID != "" {
			q = q.Where("retention_jobs.project_id = ?", projectID)
		}
		return q.Select("retention_jobs.*")
	}
	q := query()
	if cursor != "" {
		q = q.Where("retention_jobs.created_at > ? OR (retention_jobs.created_at = ? AND retention_jobs.id > ?)", anchor.CreatedAt, anchor.CreatedAt, cursor)
	}
	rows := []retentionJobRecord{}
	if err := q.Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	if cursor != "" && len(rows) < limit {
		var before []retentionJobRecord
		if err := query().Where("retention_jobs.created_at < ? OR (retention_jobs.created_at = ? AND retention_jobs.id <= ?)", anchor.CreatedAt, anchor.CreatedAt, cursor).Limit(limit - len(rows)).Find(&before).Error; err != nil {
			return nil, err
		}
		rows = append(rows, before...)
	}
	return rows, nil
}

func retentionDeletionWindow(tx *gorm.DB, nodeID string, limit int) ([]nodeDeletionRecord, error) {
	var node nodeRecord
	if err := tx.First(&node, "id = ?", nodeID).Error; err != nil {
		return nil, err
	}
	cursor := node.RetentionDeletionCursor
	var anchor nodeDeletionRecord
	if cursor != "" {
		if !validUUID(cursor) {
			return nil, ErrRetentionObjectInvalid
		}
		if err := tx.First(&anchor, "id = ?", cursor).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			cursor = ""
		} else if err != nil {
			return nil, err
		}
		if cursor != "" && (anchor.NodeID != nodeID || anchor.CreatedAt.IsZero()) {
			return nil, ErrRetentionObjectInvalid
		}
	}
	query := func() *gorm.DB {
		return tx.Where("node_id = ? AND state <> 'completed'", nodeID).Order("created_at ASC, id ASC")
	}
	q := query()
	if cursor != "" {
		q = q.Where("created_at > ? OR (created_at = ? AND id > ?)", anchor.CreatedAt, anchor.CreatedAt, cursor)
	}
	rows := []nodeDeletionRecord{}
	if err := q.Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	if cursor != "" && len(rows) < limit {
		var before []nodeDeletionRecord
		if err := query().Where("created_at < ? OR (created_at = ? AND id <= ?)", anchor.CreatedAt, anchor.CreatedAt, cursor).Limit(limit - len(rows)).Find(&before).Error; err != nil {
			return nil, err
		}
		rows = append(rows, before...)
	}
	return rows, nil
}
