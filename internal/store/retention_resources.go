package store

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"gorm.io/gorm"
	"mybuilds/internal/protocol"
)

// RegisterNodeResource登记真实执行的固定归属；终态仅确认已存在的精确原请求。
func (s *Store) RegisterNodeResource(ctx context.Context, actor NodeActor, in protocol.NodeResourceRegistration) error {
	if !validRef(in.Ref) || !validUUID(in.ID) || !validDigest(in.OwnershipDigest) || !in.HasWorkspace && !in.HasResults {
		return ErrInvalid
	}
	if c := in.Completion; c != nil && (c.StopCode != "process_group_reaped" || c.LastEventSeq < 0 || c.LastLogSeq < 0 || c.LastLogOffset < 0 || c.LastArtifactSeq < 0 || c.LastLogSeq == 0 && c.LastLogOffset != 0) {
		return ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := s.write(ctx, func(tx *gorm.DB) error {
		node, _, err := authorizeNode(tx, actor)
		if err != nil {
			return err
		}
		if node.State == "disabled" || actor.ID != in.Ref.NodeID {
			return ErrNodeUnauthorized
		}
		var build buildRecord
		if err = tx.First(&build, "id = ?", in.Ref.BuildID).Error; err != nil {
			return err
		}
		var old nodeResourceRecord
		err = tx.First(&old, "attempt_id = ?", in.Ref.AttemptID).Error
		found := err == nil
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		same := found && resourceRecordRef(old) == in.Ref && old.ID == in.ID && old.OwnershipDigest == in.OwnershipDigest && old.HasWorkspace == in.HasWorkspace && old.HasResults == in.HasResults
		if slices.Contains([]string{"succeeded", "failed", "cancelled", "skipped", "interrupted"}, build.Status) {
			if buildRef(build) != in.Ref || !same {
				return ErrConflict
			}
			if in.Completion != nil {
				if err := completeNodeResource(tx, build, old, *in.Completion); err != nil {
					return err
				}
			}
			// 当前独立身份只读核对原登记，不恢复旧session/fence，不写停止或时间。
			current, _, err := authorizeNode(tx, actor)
			if err == nil && current.State == "disabled" {
				return ErrNodeUnauthorized
			}
			return err
		}
		if in.Completion != nil {
			return ErrConflict
		}
		current, err := s.currentExecution(tx, actor, in.Ref)
		if err != nil {
			return err
		}
		expires := *current.LeaseExpiresAt
		if found {
			if !same {
				rebound, err := approvalResourceRebind(tx, current, old, in)
				if err != nil {
					return err
				}
				if rebound {
					return checkBoundary(tx, actor, in.Ref, expires)
				}
				// 同一attempt只能增加一次结果槽；不能减槽、换ID或改已有槽摘要。
				if resourceRecordRef(old) != in.Ref || old.ID != in.ID || old.HasWorkspace != in.HasWorkspace || old.HasResults || !in.HasResults || old.OwnershipDigest == in.OwnershipDigest {
					return ErrConflict
				}
				if err = tx.Model(&old).Updates(map[string]any{"has_results": true, "ownership_digest": in.OwnershipDigest}).Error; err != nil {
					return err
				}
			}
		} else {
			row := nodeResourceRecord{ID: in.ID, BuildID: in.Ref.BuildID, AttemptID: in.Ref.AttemptID, NodeID: in.Ref.NodeID, SessionID: in.Ref.SessionID, LeaseID: in.Ref.LeaseID, Epoch: in.Ref.Epoch, OwnershipDigest: in.OwnershipDigest, HasWorkspace: in.HasWorkspace, HasResults: in.HasResults, RegisteredAt: time.Now().UTC()}
			if err = tx.Create(&row).Error; err != nil {
				return err
			}
		}
		return checkBoundary(tx, actor, in.Ref, expires)
	})
	return retentionPolicyError(ctx, err)
}

// 完成证明只能补充精确归属的独立停止执行，不能修订执行或停止事实。
func completeNodeResource(tx *gorm.DB, build buildRecord, resource nodeResourceRecord, completion protocol.NodeResourceCompletion) error {
	if build.Status != "interrupted" || build.StopUnconfirmed || completion.LastEventSeq != build.LastEventSeq || completion.LastLogSeq != build.LastLogSeq || completion.LastLogOffset != build.LastLogOffset || completion.LastArtifactSeq != build.LastArtifactSeq {
		return ErrConflict
	}
	var stop stopConfirmationRecord
	if err := tx.First(&stop, "attempt_id = ?", build.AttemptID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrConflict
		}
		return err
	}
	if stop.BuildID != resource.BuildID || stop.AttemptID != resource.AttemptID || stop.NodeID != resource.NodeID || stop.SessionID != resource.SessionID || stop.LeaseID != resource.LeaseID || stop.Epoch != resource.Epoch || stop.EvidenceCode != completion.StopCode {
		return ErrConflict
	}
	encoded, err := json.Marshal(completion)
	if err != nil {
		return err
	}
	if resource.CompletedAt != nil || resource.CompletionJSON != "" {
		if resource.CompletionJSON != string(encoded) || !retentionResourceCompleted(build, resource, stop.EvidenceCode) {
			return ErrConflict
		}
		return nil
	}
	return tx.Model(&resource).Updates(map[string]any{"completed_at": time.Now().UTC(), "completion_json": string(encoded)}).Error
}

func resourceRecordRef(row nodeResourceRecord) protocol.LeaseRef {
	return protocol.LeaseRef{BuildID: row.BuildID, AttemptID: row.AttemptID, NodeID: row.NodeID, SessionID: row.SessionID, LeaseID: row.LeaseID, Epoch: row.Epoch}
}
