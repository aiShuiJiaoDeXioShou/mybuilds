package store

import (
	"context"
	"errors"
	"math"
	"slices"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"mybuilds/internal/protocol"
)

func retentionNodeActor(tx *gorm.DB, actor NodeActor) error {
	node, _, err := authorizeNode(tx, actor)
	if err != nil {
		return err
	}
	if node.State == "disabled" {
		return ErrNodeUnauthorized
	}
	return nil
}
func nodeDeletionContext(tx *gorm.DB, d nodeDeletionRecord) (buildRecord, retentionJobRecord, nodeResourceRecord, error) {
	var b buildRecord
	var j retentionJobRecord
	var r nodeResourceRecord
	for _, id := range []string{d.ID, d.JobID, d.ResourceID, d.NodeID, d.BuildID, d.AttemptID} {
		if !validUUID(id) {
			return b, j, r, ErrRetentionObjectInvalid
		}
	}
	if !validDigest(d.OwnershipDigest) {
		return b, j, r, ErrRetentionObjectInvalid
	}
	if err := tx.First(&b, "id = ?", d.BuildID).Error; err != nil {
		return b, j, r, err
	}
	if err := tx.First(&j, "id = ?", d.JobID).Error; err != nil {
		return b, j, r, err
	}
	if err := tx.First(&r, "id = ?", d.ResourceID).Error; err != nil {
		return b, j, r, err
	}
	if j.BuildID != b.ID || j.ProjectID != b.ProjectID || r.NodeID != d.NodeID || r.BuildID != d.BuildID || r.AttemptID != d.AttemptID || r.OwnershipDigest != d.OwnershipDigest || resourceRecordRef(r) != buildRef(b) || !r.HasWorkspace && !r.HasResults {
		return b, j, r, ErrRetentionObjectInvalid
	}
	if !slices.Contains([]string{"retiring", "partial"}, b.HistoryState) {
		return b, j, r, ErrRetentionRetired
	}
	return b, j, r, nil
}
func (s *Store) ClaimNodeDeletions(ctx context.Context, actor NodeActor, limit int) ([]protocol.NodeDeletion, error) {
	if limit == 0 {
		limit = 10
	}
	if limit < 1 || limit > 10 {
		return nil, ErrRetentionLimit
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out := []protocol.NodeDeletion{}
	err := s.write(ctx, func(tx *gorm.DB) error {
		if err := retentionNodeActor(tx, actor); err != nil {
			return err
		}
		rows, err := retentionDeletionWindow(tx, actor.ID, limit)
		if err != nil {
			return err
		}
		for _, d := range rows {
			if err := tx.Model(&nodeRecord{}).Where("id = ?", actor.ID).UpdateColumn("retention_deletion_cursor", d.ID).Error; err != nil {
				return err
			}
			b, _, r, err := nodeDeletionContext(tx, d)
			if err != nil {
				return err
			}
			if err = retentionDeletionAllowed(tx, b); err == ErrRetentionProtected || err == ErrRetentionReadersActive {
				continue
			} else if err != nil {
				return err
			}
			out = append(out, protocol.NodeDeletion{ID: d.ID, ResourceID: d.ResourceID, BuildID: d.BuildID, AttemptID: d.AttemptID, OwnershipDigest: d.OwnershipDigest, HasWorkspace: r.HasWorkspace, HasResults: r.HasResults})
		}
		return retentionNodeActor(tx, actor)
	})
	if err != nil {
		return nil, retentionPolicyError(ctx, err)
	}
	return out, nil
}
func (s *Store) AuthorizeNodeDeletion(ctx context.Context, actor NodeActor, id string) (protocol.DeletionAuthority, error) {
	if !validUUID(id) {
		return protocol.DeletionAuthority{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var out protocol.DeletionAuthority
	err := s.write(ctx, func(tx *gorm.DB) error {
		if err := retentionNodeActor(tx, actor); err != nil {
			return err
		}
		var d nodeDeletionRecord
		if err := tx.First(&d, "id = ?", id).Error; err != nil {
			return err
		}
		if d.NodeID != actor.ID {
			return ErrNodeUnauthorized
		}
		b, _, _, err := nodeDeletionContext(tx, d)
		if err != nil {
			return err
		}
		if d.State == "completed" {
			return ErrRetentionRetired
		}
		if err = retentionDeletionAllowed(tx, b); err != nil {
			return err
		}
		expires := time.Now().UTC().Add(5 * time.Second)
		nonce := uuid.NewString()
		if err = tx.Model(&d).Updates(map[string]any{"nonce": nonce, "expires_at": expires}).Error; err != nil {
			return err
		}
		out = protocol.DeletionAuthority{ID: d.ID, NodeID: d.NodeID, ResourceID: d.ResourceID, OwnershipDigest: d.OwnershipDigest, Nonce: nonce, ExpiresAt: expires}
		return retentionNodeActor(tx, actor)
	})
	if err != nil {
		return protocol.DeletionAuthority{}, retentionPolicyError(ctx, err)
	}
	return out, nil
}
func retentionDeletionSlots(in protocol.NodeDeletionConfirmation, r nodeResourceRecord) bool {
	for _, slot := range []struct {
		present bool
		state   string
	}{{r.HasWorkspace, in.WorkspaceState}, {r.HasResults, in.ResultsState}} {
		if slot.present {
			if !slices.Contains([]string{"deleted", "partial", "failed"}, slot.state) {
				return false
			}
		} else if slot.state != "not_applicable" {
			return false
		}
	}
	complete := (in.WorkspaceState == "deleted" || in.WorkspaceState == "not_applicable") && (in.ResultsState == "deleted" || in.ResultsState == "not_applicable")
	if complete {
		return in.Reason == ""
	}
	return slices.Contains([]string{"partial", "resource_unconfirmed", "identity_mismatch", "invalid_object", "permission_denied", "persistence_error", "authority_expired", "operation_timeout"}, in.Reason)
}
func sameDeletionReceipt(receipt nodeDeletionReceiptRecord, in protocol.NodeDeletionConfirmation) bool {
	return receipt.DeleteID == in.ID && receipt.Seq == in.Seq && receipt.ResourceID == in.ResourceID && receipt.OwnershipDigest == in.OwnershipDigest && receipt.Nonce == in.Nonce && receipt.Digest == in.Digest && receipt.WorkspaceState == in.WorkspaceState && receipt.ResultsState == in.ResultsState && receipt.Reason == in.Reason
}
func (s *Store) ConfirmNodeDeletion(ctx context.Context, actor NodeActor, in protocol.NodeDeletionConfirmation) error {
	if !validUUID(in.ID) || !validUUID(in.ResourceID) || !validDigest(in.OwnershipDigest) || !validUUID(in.Nonce) || in.Seq < 1 || !validDigest(in.Digest) {
		return ErrRetentionObjectInvalid
	}
	digest, err := protocol.NodeDeletionDigest(in)
	if err != nil || digest != in.Digest {
		return ErrRetentionReceiptConflict
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err = s.write(ctx, func(tx *gorm.DB) error {
		if err := retentionNodeActor(tx, actor); err != nil {
			return err
		}
		var d nodeDeletionRecord
		if err := tx.First(&d, "id = ?", in.ID).Error; err != nil {
			return err
		}
		if d.NodeID != actor.ID {
			return ErrNodeUnauthorized
		}
		if d.ResourceID != in.ResourceID || d.OwnershipDigest != in.OwnershipDigest {
			return ErrRetentionReceiptConflict
		}
		var old nodeDeletionReceiptRecord
		err := tx.First(&old, "delete_id = ? AND seq = ?", in.ID, in.Seq).Error
		if err == nil {
			if !sameDeletionReceipt(old, in) {
				return ErrRetentionReceiptConflict
			}
			return retentionNodeActor(tx, actor)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if d.Seq == math.MaxInt64 || in.Seq != d.Seq+1 || d.Nonce != in.Nonce || !validUUID(d.Nonce) || d.ExpiresAt == nil {
			return ErrRetentionReceiptConflict
		}
		b, j, r, err := nodeDeletionContext(tx, d)
		if err != nil {
			return err
		}
		if !retentionDeletionSlots(in, r) {
			return ErrRetentionObjectInvalid
		}
		if d.WorkspaceState == "deleted" && in.WorkspaceState != "deleted" || d.ResultsState == "deleted" && in.ResultsState != "deleted" {
			return ErrRetentionReceiptConflict
		}
		at := time.Now().UTC()
		receipt := nodeDeletionReceiptRecord{ID: uuid.NewString(), DeleteID: in.ID, Seq: in.Seq, ResourceID: in.ResourceID, OwnershipDigest: in.OwnershipDigest, Nonce: in.Nonce, Digest: in.Digest, WorkspaceState: in.WorkspaceState, ResultsState: in.ResultsState, Reason: in.Reason, CreatedAt: at}
		if err = tx.Create(&receipt).Error; err != nil {
			return err
		}
		complete := (in.WorkspaceState == "deleted" || in.WorkspaceState == "not_applicable") && (in.ResultsState == "deleted" || in.ResultsState == "not_applicable")
		state := "partial"
		if in.WorkspaceState == "failed" || in.ResultsState == "failed" {
			state = "failed"
		}
		updates := map[string]any{"seq": in.Seq, "digest": in.Digest, "workspace_state": in.WorkspaceState, "results_state": in.ResultsState, "reason": in.Reason, "state": state}
		if complete {
			updates["state"] = "completed"
			if d.CompletedAt == nil {
				updates["completed_at"] = at
			}
		}
		if err = tx.Model(&d).Updates(updates).Error; err != nil {
			return err
		}
		allowed := retentionDeletionAllowed(tx, b)
		if allowed != nil && allowed != ErrRetentionProtected && allowed != ErrRetentionReadersActive {
			return allowed
		}
		if allowed != nil {
			if err = tx.Model(&j).Updates(map[string]any{"state": "paused", "reason": "retention_protected"}).Error; err != nil {
				return err
			}
		} else if complete {
			var left int64
			if err = tx.Model(&nodeDeletionRecord{}).Where("job_id = ? AND state <> 'completed'", j.ID).Count(&left).Error; err != nil {
				return err
			}
			if left == 0 && j.NodeCompletedAt == nil {
				if err = tx.Model(&j).Update("node_completed_at", at).Error; err != nil {
					return err
				}
			}
		} else {
			if err = tx.Model(&j).Updates(map[string]any{"state": state, "reason": in.Reason}).Error; err != nil {
				return err
			}
		}
		return retentionNodeActor(tx, actor)
	})
	return retentionPolicyError(ctx, err)
}
