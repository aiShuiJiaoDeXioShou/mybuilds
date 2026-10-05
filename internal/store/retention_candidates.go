package store

import (
	"context"
	"errors"
	"slices"
	"time"

	"gorm.io/gorm"
)

const retentionRelationLimit = 100000

// EvaluateRetention只评估；候选条件与保护独立，不以旧查询授权任何删除。
func (s *Store) EvaluateRetention(ctx context.Context, actor Actor, projectID string, page Page) (RetentionPage, error) {
	page, err := normalizePage(page)
	if err != nil || !validUUID(projectID) {
		return RetentionPage{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var result RetentionPage
	err = s.write(ctx, func(tx *gorm.DB) error {
		if err := authorize(tx, actor, "admin"); err != nil {
			return err
		}
		var err error
		result, err = evaluateRetention(tx, projectID, time.Now().UTC(), page)
		return err
	})
	if err != nil {
		return RetentionPage{}, retentionPolicyError(ctx, err)
	}
	return result, nil
}

func retentionTerminal(status string) bool {
	return slices.Contains([]string{"succeeded", "failed", "cancelled", "skipped", "interrupted"}, status)
}

func evaluateRetention(tx *gorm.DB, projectID string, at time.Time, page Page) (RetentionPage, error) {
	page, err := normalizePage(page)
	if err != nil {
		return RetentionPage{}, err
	}
	policy, err := effectiveRetention(tx, projectID)
	if err != nil {
		return RetentionPage{}, err
	}
	var project projectRecord
	if err = tx.First(&project, "id = ?", projectID).Error; err != nil {
		return RetentionPage{}, err
	}
	rows := []buildRecord{}
	if err = tx.Where("project_id = ?", projectID).Order("CASE WHEN terminal_at IS NULL THEN 1 ELSE 0 END, terminal_at DESC, id DESC").Limit(page.Limit).Offset(page.Offset).Find(&rows).Error; err != nil {
		return RetentionPage{}, err
	}
	dependencies, err := retentionRetryProtection(tx, projectID)
	if err != nil {
		return RetentionPage{}, err
	}
	result := RetentionPage{Items: make([]RetentionEntry, 0, len(rows)), Limit: page.Limit, Offset: page.Offset}
	cutoff := at.Add(-time.Duration(policy.Days) * 24 * time.Hour)
	for _, row := range rows {
		if err = tx.Statement.Context.Err(); err != nil {
			return RetentionPage{}, err
		}
		entry := RetentionEntry{BuildID: row.ID, Project: project.Name, BuildName: row.Name, Number: row.Number, TerminalAt: row.TerminalAt, HistoryState: row.HistoryState, CleanedAt: row.CleanedAt, EvaluatedAt: &at}
		if retentionTerminal(row.Status) && row.TerminalAt != nil && !row.TerminalAt.IsZero() && row.HistoryState != "cleaned" {
			var newer int64
			if err = tx.Model(&buildRecord{}).Where("project_id = ? AND history_state <> ? AND status IN ? AND terminal_at IS NOT NULL AND (terminal_at > ? OR (terminal_at = ? AND id > ?))", projectID, "cleaned", []string{"succeeded", "failed", "cancelled", "skipped", "interrupted"}, *row.TerminalAt, *row.TerminalAt, row.ID).Count(&newer).Error; err != nil {
				return RetentionPage{}, err
			}
			entry.Candidate = newer >= policy.Builds || row.TerminalAt.Before(cutoff)
		}
		entry.ProtectReasons, err = retentionProtectionWithRetries(tx, row, dependencies[row.ID] || dependencies[""])
		if err != nil {
			return RetentionPage{}, err
		}
		var job retentionJobRecord
		if err = tx.First(&job, "build_id = ?", row.ID).Error; err == nil {
			entry.JobID = job.ID
			entry.Reason = job.Reason
			entry.RetiredAt = job.RetiredAt
			entry.CentralCompletedAt = job.CentralCompletedAt
			entry.NodeCompletedAt = job.NodeCompletedAt
			jobEntry, jobErr := retentionJobEntry(tx, row, job)
			if jobErr != nil {
				return RetentionPage{}, jobErr
			}
			entry.CentralState, entry.NodeState = jobEntry.CentralState, jobEntry.NodeState

		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return RetentionPage{}, err
		}
		result.Items = append(result.Items, entry)
	}
	return result, nil
}

// 只沿已存在RetryOf关系计算；未知、跨项目、环或过限时保守保护本项目。
func retentionRetryProtection(tx *gorm.DB, projectID string) (map[string]bool, error) {
	type relation struct {
		ID, ProjectID, HistoryState string
		RetryOf                     *string
	}
	rows := []relation{}
	err := tx.Model(&buildRecord{}).Select("id, project_id, history_state, retry_of").Where("retry_of IS NOT NULL AND (project_id = ? OR retry_of IN (SELECT id FROM builds WHERE project_id = ?))", projectID, projectID).Limit(retentionRelationLimit + 1).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	protected := map[string]bool{}
	if len(rows) > retentionRelationLimit {
		protected[""] = true
		return protected, nil
	}
	links := map[string]relation{}
	for _, row := range rows {
		links[row.ID] = row
	}
	done := map[string]bool{}
	visits := 0
	for _, child := range rows {
		if tx.Statement.Context.Err() != nil {
			return nil, tx.Statement.Context.Err()
		}
		if child.HistoryState == "cleaned" || done[child.ID] {
			continue
		}
		seen := map[string]bool{}
		chain := []string{}
		current := child
		for current.RetryOf != nil {
			if err = tx.Statement.Context.Err(); err != nil {
				return nil, err
			}
			visits++
			if visits > retentionRelationLimit || seen[current.ID] || current.ProjectID != projectID || !validUUID(*current.RetryOf) {
				protected[""] = true
				break
			}
			seen[current.ID] = true
			chain = append(chain, current.ID)
			parentID := *current.RetryOf
			protected[parentID] = true
			if done[parentID] {
				break
			}
			parent, ok := links[parentID]
			if !ok {
				if err = tx.Model(&buildRecord{}).Select("id, project_id, history_state, retry_of").First(&parent, "id = ?", parentID).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						protected[""] = true
						break
					}
					return nil, err
				}
				links[parentID] = parent
			}
			if parent.ProjectID != projectID {
				protected[""] = true
				break
			}
			current = parent
		}
		if protected[""] {
			return protected, nil
		}
		for _, id := range chain {
			done[id] = true
		}
	}
	return protected, nil
}
