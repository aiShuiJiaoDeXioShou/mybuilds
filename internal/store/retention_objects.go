package store

import (
	"context"
	"fmt"
	"slices"
	"time"

	"gorm.io/gorm"
)

func retentionObjectOriginal(tx *gorm.DB, o retentionObjectRecord, b buildRecord) error {
	if !validUUID(o.ID) || !validUUID(o.JobID) || !validUUID(o.ObjectID) || !validUUID(o.StorageID) || !validDigest(o.SHA256) || o.Size < 0 {
		return ErrRetentionObjectInvalid
	}
	if o.Kind == "log" {
		var l logChunkRecord
		if err := tx.First(&l, "id = ?", o.ObjectID).Error; err != nil {
			return err
		}
		if l.BuildID != b.ID || l.StorageID != o.StorageID || l.Size != o.Size || l.Digest != o.SHA256 {
			return ErrRetentionObjectInvalid
		}
	} else if o.Kind == "artifact" || o.Kind == "junit" {
		var f artifactRecord
		if err := tx.First(&f, "id = ?", o.ObjectID).Error; err != nil {
			return err
		}
		if f.BuildID != b.ID || f.StorageID != o.StorageID || f.Size != o.Size || f.SHA256 != o.SHA256 || (o.Kind == "junit") != (f.Purpose == "junit") {
			return ErrRetentionObjectInvalid
		}
	} else {
		return ErrRetentionObjectInvalid
	}
	return nil
}
func (s *Store) AuthorizeRetentionObject(ctx context.Context, id string, observed RetentionObjectObservation) (RetentionObject, error) {
	if !validUUID(id) || !validRetentionIdentity(observed.Identity) || observed.Size < 0 || !validDigest(observed.SHA256) {
		return RetentionObject{}, ErrRetentionObjectInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var out RetentionObject
	err := s.write(ctx, func(tx *gorm.DB) error {
		var o retentionObjectRecord
		if err := tx.First(&o, "id = ?", id).Error; err != nil {
			return err
		}
		b, _, err := retentionObjectContext(tx, o)
		if err != nil {
			return err
		}
		if err = retentionObjectOriginal(tx, o, b); err != nil {
			return err
		}
		if !slices.Contains([]string{"retired", "quarantined", "failed", "paused"}, o.State) {
			return ErrRetentionObjectInvalid
		}
		if observed.Size != o.Size || observed.SHA256 != o.SHA256 {
			return ErrRetentionOwnershipUnknown
		}
		slot := fmt.Sprintf("retention/quarantine/%s/%s", o.JobID, o.ObjectID)
		if o.Identity != "" && (o.Identity != observed.Identity || o.QuarantineSlot != slot) {
			return ErrRetentionOwnershipUnknown
		}
		if err = s.repairRetentionReads(tx, o, b, observed.Identity); err != nil {
			return err
		}
		if err = retentionDeletionAllowed(tx, b); err != nil {
			return err
		}
		if o.Identity == "" {
			if o.QuarantineSlot != "" {
				return ErrRetentionObjectInvalid
			}
			o.Identity = observed.Identity
			o.QuarantineSlot = slot
			if err = tx.Model(&o).Updates(map[string]any{"identity": o.Identity, "quarantine_slot": slot}).Error; err != nil {
				return err
			}
		}
		if o.State == "failed" || o.State == "paused" || o.Reason != "" {
			o.State = "retired"
			if o.QuarantinedAt != nil {
				o.State = "quarantined"
			}
			o.Reason = ""
			if err = tx.Model(&o).Updates(map[string]any{"state": o.State, "reason": ""}).Error; err != nil {
				return err
			}
		}
		out = retentionObjectView(o, b.ID)
		return nil
	})
	if err != nil {
		return RetentionObject{}, retentionPolicyError(ctx, err)
	}
	return out, nil
}
func (s *Store) ConfirmRetentionObject(ctx context.Context, id string, result RetentionObjectResult) error {
	if !validUUID(id) || !validRetentionIdentity(result.Identity) || !slices.Contains([]string{"quarantined", "deleted", "failed", "paused"}, result.State) {
		return ErrRetentionObjectInvalid
	}
	if result.State == "quarantined" || result.State == "deleted" {
		if result.Reason != "" {
			return ErrRetentionObjectInvalid
		}
	} else if !slices.Contains([]string{"retention_io_error", "retention_ownership_unknown", "retention_readers_active", "retention_protected", "retention_timeout", "retention_cancelled"}, result.Reason) {
		return ErrRetentionObjectInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := s.write(ctx, func(tx *gorm.DB) error {
		var o retentionObjectRecord
		if err := tx.First(&o, "id = ?", id).Error; err != nil {
			return err
		}
		b, j, err := retentionObjectContext(tx, o)
		if err != nil {
			return err
		}
		if o.Identity == "" || o.Identity != result.Identity || o.QuarantineSlot != "retention/quarantine/"+o.JobID+"/"+o.ObjectID {
			return ErrRetentionOwnershipUnknown
		}
		if o.State == result.State && o.Reason == result.Reason {
			return nil
		}
		if err = retentionDeletionAllowed(tx, b); err != nil {
			return err
		}
		if err = retentionObjectOriginal(tx, o, b); err != nil {
			return err
		}
		if o.State == "deleted" || result.State == "deleted" && o.QuarantinedAt == nil || result.State == "quarantined" && !slices.Contains([]string{"retired", "failed", "paused"}, o.State) {
			return ErrRetentionReceiptConflict
		}
		at := time.Now().UTC()
		updates := map[string]any{"state": result.State, "reason": result.Reason, "confirmed_at": at}
		if result.State == "quarantined" && o.QuarantinedAt == nil {
			updates["quarantined_at"] = at
		}
		if err = tx.Model(&o).Updates(updates).Error; err != nil {
			return err
		}
		state := "partial"
		if result.State == "failed" || result.State == "paused" {
			state = result.State
		}
		if err = tx.Model(&j).Updates(map[string]any{"state": state, "reason": result.Reason}).Error; err != nil {
			return err
		}
		if err = tx.Model(&b).Update("history_state", "partial").Error; err != nil {
			return err
		}
		return retentionCentralProgress(tx, &j, at)
	})
	return retentionPolicyError(ctx, err)
}

// 仅已持同fd EX/hash的中央消费者传入观察；不靠超时解除任何读取。
func (s *Store) repairRetentionReads(tx *gorm.DB, o retentionObjectRecord, b buildRecord, identity string) error {
	var reads []evidenceReadRecord
	kind := o.Kind
	if kind == "junit" {
		kind = "artifact"
	}
	if err := tx.Where("build_id = ? AND object_id = ? AND kind = ? AND state <> 'closed'", b.ID, o.ObjectID, kind).Limit(retentionRelationLimit + 1).Find(&reads).Error; err != nil {
		return err
	}
	if len(reads) == 0 {
		return nil
	}
	if len(reads) > retentionRelationLimit {
		return ErrRetentionLimit
	}
	if err := s.checkEvidenceReadOwner(tx, s.evidenceReadOwner); err != nil {
		return err
	}
	for _, read := range reads {
		if err := tx.Statement.Context.Err(); err != nil {
			return err
		}
		if !validUUID(read.Owner) || read.Owner == s.evidenceReadOwner || read.StorageID != o.StorageID || read.ClosedAt != nil {
			continue
		}
		if read.State == "pending" && read.Identity == "" || read.State == "active" && read.Identity == identity {
			if err := tx.Model(&read).Updates(map[string]any{"state": "closed", "closed_at": time.Now().UTC()}).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// 初次受限打开失败没有fd身份；只记录安全失败，不授隔离或删除权。
func (s *Store) RecordRetentionObjectFailure(ctx context.Context, id, reason string) error {
	if !validUUID(id) || !slices.Contains([]string{"retention_ownership_unknown", "retention_readers_active", "retention_io_error"}, reason) {
		return ErrRetentionObjectInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := s.write(ctx, func(tx *gorm.DB) error {
		var o retentionObjectRecord
		if err := tx.First(&o, "id = ?", id).Error; err != nil {
			return err
		}
		b, j, err := retentionObjectContext(tx, o)
		if err != nil {
			return err
		}
		if err = retentionObjectOriginal(tx, o, b); err != nil {
			return err
		}
		if o.Identity != "" || o.QuarantineSlot != "" || o.QuarantinedAt != nil || o.ConfirmedAt != nil || !slices.Contains([]string{"pending", "retired", "failed", "paused"}, o.State) {
			return ErrRetentionObjectInvalid
		}
		state, jobState, jobReason := "failed", "failed", reason
		if reason == "retention_readers_active" {
			state, jobState, jobReason = "retired", "waiting", "readers_active"
		}
		if o.State == state && o.Reason == reason && j.State == jobState && j.Reason == jobReason {
			return nil
		}
		if err = tx.Model(&o).Updates(map[string]any{"state": state, "reason": reason}).Error; err != nil {
			return err
		}
		return tx.Model(&j).Updates(map[string]any{"state": jobState, "reason": jobReason}).Error
	})
	return retentionPolicyError(ctx, err)
}
