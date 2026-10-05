package store

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"mybuilds/internal/protocol"
)

// FinalizeRetention只由已持控制独占的中央消费者收敛真实完成证据，无文件IO。
func (s *Store) FinalizeRetention(ctx context.Context, projectID string, limit int) (RetentionPage, error) {
	limit, err := retentionRound(limit)
	if err != nil {
		return RetentionPage{}, err
	}
	if projectID != "" && !validUUID(projectID) {
		return RetentionPage{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out := RetentionPage{Items: []RetentionEntry{}, Limit: limit}
	err = s.write(ctx, func(tx *gorm.DB) error {
		if projectID != "" {
			var project projectRecord
			if err := tx.First(&project, "id = ?", projectID).Error; err != nil {
				return err
			}
		}
		jobs, err := retentionFinalizeWindow(tx, projectID, limit)
		if err != nil {
			return err
		}
		for _, job := range jobs {
			if projectID == "" {
				if err := tx.Model(&retentionPolicyRecord{}).Where("id = ?", 1).UpdateColumn("finalize_cursor", job.ID).Error; err != nil {
					return err
				}
			} else {
				if err := tx.Model(&projectRecord{}).Where("id = ?", projectID).UpdateColumn("retention_finalize_cursor", job.ID).Error; err != nil {
					return err
				}
			}
			if err := tx.Statement.Context.Err(); err != nil {
				return err
			}
			var row buildRecord
			if err := tx.First(&row, "id = ?", job.BuildID).Error; err != nil {
				return err
			}
			if row.ProjectID != job.ProjectID || job.RetiredAt == nil || job.RetiredAt.IsZero() {
				return ErrRetentionObjectInvalid
			}
			if err := retentionDeletionAllowed(tx, row); err != nil {
				if err != ErrRetentionProtected && err != ErrRetentionReadersActive {
					return err
				}
				job.State = "paused"
				job.Reason = "retention_protected"
				if err = tx.Model(&job).Updates(map[string]any{"state": job.State, "reason": job.Reason}).Error; err != nil {
					return err
				}
			} else {
				central, err := retentionHistoryCentral(tx, row, job)
				if err != nil {
					return err
				}
				node, hasNode, err := retentionHistoryNodes(tx, row, job)
				if err != nil {
					return err
				}
				at := time.Now().UTC()
				updates := map[string]any{}
				if central && job.CentralCompletedAt == nil {
					job.CentralCompletedAt = &at
					updates["central_completed_at"] = at
				}
				if node && hasNode && job.NodeCompletedAt == nil {
					job.NodeCompletedAt = &at
					updates["node_completed_at"] = at
				}
				if central && node {
					if err = retentionCleanHistory(tx, row, at); err != nil {
						return err
					}
					row.HistoryState = "cleaned"
					row.CleanedAt = &at
					job.State = "completed"
					job.Reason = ""
					updates["state"] = "completed"
					updates["reason"] = ""
				} else {
					job.State, job.Reason, err = retentionHistoryPending(tx, job, node)
					if err != nil {
						return err
					}
					updates["state"] = job.State
					updates["reason"] = job.Reason
				}
				if err = tx.Model(&job).Updates(updates).Error; err != nil {
					return err
				}
			}
			entry, err := retentionJobEntry(tx, row, job)
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

// 完成时间不是删除证明；每个原对象须仍与固定事项及真实确认一一对应。
func retentionHistoryCentral(tx *gorm.DB, row buildRecord, job retentionJobRecord) (bool, error) {
	var logs []logChunkRecord
	var files []artifactRecord
	var objects []retentionObjectRecord
	if err := tx.Where("build_id = ?", row.ID).Limit(retentionRelationLimit + 1).Find(&logs).Error; err != nil {
		return false, err
	}
	if err := tx.Where("build_id = ?", row.ID).Limit(retentionRelationLimit + 1).Find(&files).Error; err != nil {
		return false, err
	}
	if err := tx.Where("job_id = ?", job.ID).Limit(retentionRelationLimit + 1).Find(&objects).Error; err != nil {
		return false, err
	}
	if len(logs)+len(files)+len(objects) > retentionRelationLimit {
		return false, ErrRetentionLimit
	}
	expected := map[string]bool{}
	for _, l := range logs {
		expected["log:"+l.ID] = true
	}
	for _, f := range files {
		kind := "artifact"
		if f.Purpose == "junit" {
			kind = "junit"
		}
		expected[kind+":"+f.ID] = true
	}
	if len(expected) != len(objects) {
		return false, nil
	}
	for _, o := range objects {
		if err := tx.Statement.Context.Err(); err != nil {
			return false, err
		}
		key := o.Kind + ":" + o.ObjectID
		if !expected[key] {
			return false, nil
		}
		delete(expected, key)
		if o.State != "deleted" || o.Reason != "" || !validRetentionIdentity(o.Identity) || o.QuarantineSlot != fmt.Sprintf("retention/quarantine/%s/%s", job.ID, o.ObjectID) || o.QuarantinedAt == nil || o.QuarantinedAt.IsZero() || o.ConfirmedAt == nil || o.ConfirmedAt.Before(*o.QuarantinedAt) {
			return false, nil
		}
		if err := retentionObjectOriginal(tx, o, row); err != nil {
			return false, err
		}
	}
	return len(expected) == 0, nil
}
func retentionHistoryNodes(tx *gorm.DB, row buildRecord, job retentionJobRecord) (bool, bool, error) {
	var resources []nodeResourceRecord
	var deletions []nodeDeletionRecord
	if err := tx.Where("build_id = ?", row.ID).Limit(retentionRelationLimit + 1).Find(&resources).Error; err != nil {
		return false, false, err
	}
	if err := tx.Where("job_id = ?", job.ID).Limit(retentionRelationLimit + 1).Find(&deletions).Error; err != nil {
		return false, false, err
	}
	if len(resources)+len(deletions) > retentionRelationLimit {
		return false, false, ErrRetentionLimit
	}
	if len(resources) == 0 {
		if buildRef(row).NodeID != "" || len(deletions) != 0 {
			return false, false, nil
		}
		return true, false, nil
	}
	if len(resources) != len(deletions) {
		return false, true, nil
	}
	expected := map[string]bool{}
	for _, r := range resources {
		expected[r.ID] = true
	}
	for _, d := range deletions {
		if !expected[d.ResourceID] {
			return false, true, nil
		}
		delete(expected, d.ResourceID)
		b, j, r, err := nodeDeletionContext(tx, d)
		if err != nil {
			return false, true, err
		}
		if b.ID != row.ID || j.ID != job.ID || d.State != "completed" || d.CompletedAt == nil || d.CompletedAt.IsZero() || d.Reason != "" || d.Seq < 1 || d.Seq > retentionRelationLimit {
			return false, true, nil
		}
		receipts := []nodeDeletionReceiptRecord{}
		if err = tx.Where("delete_id = ?", d.ID).Order("seq ASC").Limit(retentionRelationLimit + 1).Find(&receipts).Error; err != nil {
			return false, true, err
		}
		if int64(len(receipts)) != d.Seq {
			return false, true, nil
		}
		previousWorkspace, previousResults := "", ""
		for i, receipt := range receipts {
			if err = tx.Statement.Context.Err(); err != nil {
				return false, true, err
			}
			in := protocol.NodeDeletionConfirmation{ID: receipt.DeleteID, ResourceID: receipt.ResourceID, OwnershipDigest: receipt.OwnershipDigest, Nonce: receipt.Nonce, Seq: receipt.Seq, Digest: receipt.Digest, WorkspaceState: receipt.WorkspaceState, ResultsState: receipt.ResultsState, Reason: receipt.Reason}
			digest, digestErr := protocol.NodeDeletionDigest(in)
			if digestErr != nil || digest != receipt.Digest || receipt.Seq != int64(i+1) || !validUUID(receipt.Nonce) || receipt.ResourceID != r.ID || receipt.OwnershipDigest != r.OwnershipDigest || !retentionDeletionSlots(in, r) || receipt.CreatedAt.IsZero() {
				return false, true, nil
			}
			if previousWorkspace == "deleted" && receipt.WorkspaceState != "deleted" || previousResults == "deleted" && receipt.ResultsState != "deleted" {
				return false, true, nil
			}
			previousWorkspace, previousResults = receipt.WorkspaceState, receipt.ResultsState
		}
		last := receipts[len(receipts)-1]
		if last.Seq != d.Seq || last.Digest != d.Digest || last.Nonce != d.Nonce || last.WorkspaceState != d.WorkspaceState || last.ResultsState != d.ResultsState || last.Reason != d.Reason {
			return false, true, nil
		}
		if r.HasWorkspace && last.WorkspaceState != "deleted" || !r.HasWorkspace && last.WorkspaceState != "not_applicable" || r.HasResults && last.ResultsState != "deleted" || !r.HasResults && last.ResultsState != "not_applicable" {
			return false, true, nil
		}
	}
	return len(expected) == 0, true, nil
}
func retentionCleanHistory(tx *gorm.DB, row buildRecord, at time.Time) error {
	// 最后一次保护检查仍读取完整正文与收据；SQL删除后不重算或重写旧终态摘要。
	if err := retentionDeletionAllowed(tx, row); err != nil {
		return err
	}
	if err := retirePublishArtifacts(tx, row.ID); err != nil {
		return err
	}
	for _, model := range []any{&stepRecord{}, &artifactRecord{}, &logChunkRecord{}} {
		if err := tx.Where("build_id = ?", row.ID).Delete(model).Error; err != nil {
			return err
		}
	}
	if err := tx.Where("build_id = ? AND state = 'closed'", row.ID).Delete(&evidenceReadRecord{}).Error; err != nil {
		return err
	}
	if err := tx.Where("build_id = ? AND seq <> ? AND kind <> ?", row.ID, row.LastEventSeq, "approval_checkpoint").Delete(&executionReceiptRecord{}).Error; err != nil {
		return err
	}
	if err := tx.Model(&row).Updates(map[string]any{"history_state": "cleaned", "cleaned_at": at, "snapshot_json": "", "parameter_keys_json": "", "reasons_json": "", "reports_json": "", "report_revision": 0, "report_final": false, "report_seal_digest": "", "report_checked_index": 0}).Error; err != nil {
		return err
	}
	return audit(tx, Actor{}, "retention_completed", row.ID, row.HistoryState, "cleaned")
}

// 未完成事项从当前真实对象和节点结果聚合，不能把失败或读等待抹成空partial。
func retentionHistoryPending(tx *gorm.DB, job retentionJobRecord, nodeConfirmed bool) (string, string, error) {
	var objects []retentionObjectRecord
	var nodes []nodeDeletionRecord
	if err := tx.Where("job_id = ?", job.ID).Order("created_at ASC, id ASC").Limit(retentionRelationLimit + 1).Find(&objects).Error; err != nil {
		return "", "", err
	}
	if err := tx.Where("job_id = ?", job.ID).Order("created_at ASC, id ASC").Limit(retentionRelationLimit + 1).Find(&nodes).Error; err != nil {
		return "", "", err
	}
	if len(objects)+len(nodes) > retentionRelationLimit {
		return "", "", ErrRetentionLimit
	}
	state, reason, priority := "pending", "", 0
	promote := func(next, code string, rank int) {
		if rank > priority || rank == priority && reason == "" && code != "" {
			state, reason, priority = next, code, rank
		}
	}
	for _, o := range objects {
		if err := tx.Statement.Context.Err(); err != nil {
			return "", "", err
		}
		switch o.State {
		case "failed":
			promote("failed", o.Reason, 4)
		case "paused":
			promote("paused", o.Reason, 3)
		case "quarantined", "deleted":
			promote("partial", "", 1)
		case "pending", "retired":
			if o.Reason == "retention_readers_active" {
				promote("waiting", "readers_active", 2)
			}
		default:
			return "", "", ErrRetentionObjectInvalid
		}
	}
	for _, n := range nodes {
		if err := tx.Statement.Context.Err(); err != nil {
			return "", "", err
		}
		switch n.State {
		case "failed":
			promote("failed", n.Reason, 4)
		case "partial":
			promote("partial", n.Reason, 1)
		case "completed":
			if nodeConfirmed {
				promote("partial", "", 1)
			} else {
				promote("paused", "resource_unconfirmed", 3)
			}
		case "pending", "authorized":
		default:
			return "", "", ErrRetentionObjectInvalid
		}
	}
	return state, reason, nil
}
