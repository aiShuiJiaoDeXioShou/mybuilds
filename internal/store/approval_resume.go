package store

import (
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"math"
	"mybuilds/internal/protocol"
	"time"
)

// 仅已确认审批续执行链允许旧归属的只读证据；任何写入仍走currentExecution。
func approvalHistoricalRef(tx *gorm.DB, row buildRecord, ref protocol.LeaseRef) (bool, error) {
	current := buildRef(row)
	if ref == current {
		return true, nil
	}
	if !validRef(ref) || ref.BuildID != current.BuildID || ref.AttemptID != current.AttemptID || ref.NodeID != current.NodeID || ref.Epoch >= current.Epoch {
		return false, nil
	}
	var records []approvalRecord
	if err := tx.Where("build_id = ? AND attempt_id = ? AND node_id = ? AND resume_ref_json <> ?", row.ID, ref.AttemptID, ref.NodeID, "").Order("revision ASC").Limit(129).Find(&records).Error; err != nil {
		return false, err
	}
	if len(records) > 128 {
		return false, ErrConflict
	}
	cursor := ref
	for _, a := range records {
		if err := tx.Statement.Context.Err(); err != nil {
			return false, err
		}
		var old, next protocol.LeaseRef
		if json.Unmarshal([]byte(a.CheckpointRefJSON), &old) != nil || json.Unmarshal([]byte(a.ResumeRefJSON), &next) != nil {
			return false, ErrConflict
		}
		if old != cursor {
			continue
		}
		if a.Decision != "approve" || a.DecidedAt == nil || a.State != "resumed" || !a.StopConfirmed || a.CleanupFailed || !a.SystemResourcesClosed || next.BuildID != old.BuildID || next.AttemptID != old.AttemptID || next.NodeID != old.NodeID || next.Epoch != old.Epoch+1 || !validRef(next) {
			return false, ErrConflict
		}
		var p protocol.ExecutionProgress
		if json.Unmarshal([]byte(a.CheckpointJSON), &p) != nil {
			return false, ErrConflict
		}
		digest, err := protocol.ApprovalCheckpointDigest(old, a.EventSeq, p)
		if err != nil || digest != a.CheckpointDigest || progressDigest(p) != a.EventDigest {
			return false, ErrConflict
		}
		var receipt executionReceiptRecord
		if err = tx.First(&receipt, "build_id = ? AND attempt_id = ? AND seq = ?", row.ID, ref.AttemptID, a.EventSeq).Error; err != nil {
			return false, err
		}
		if receipt.Kind != "approval_checkpoint" || !receipt.StopKnown || receipt.Digest != a.EventDigest {
			return false, ErrConflict
		}
		cursor = next
		if cursor == current {
			return true, nil
		}
	}
	return false, nil
}
func approvalResumeEvidence(tx *gorm.DB, row buildRecord) (*protocol.ApprovalResumeEvidence, error) {
	if row.CurrentApprovalID == nil {
		return nil, nil
	}
	var a approvalRecord
	if err := tx.First(&a, "id = ? AND build_id = ?", *row.CurrentApprovalID, row.ID).Error; err != nil {
		return nil, err
	}
	if a.State != "resumed" || a.Decision != "approve" {
		return nil, ErrConflict
	}
	var old, next protocol.LeaseRef
	var p protocol.ExecutionProgress
	var steps []protocol.ApprovalStepLedger
	if json.Unmarshal([]byte(a.CheckpointRefJSON), &old) != nil || json.Unmarshal([]byte(a.ResumeRefJSON), &next) != nil || next != buildRef(row) || json.Unmarshal([]byte(a.CheckpointJSON), &p) != nil || p.Approval == nil || json.Unmarshal([]byte(a.LedgerJSON), &steps) != nil {
		return nil, ErrConflict
	}
	for i := range steps {
		if steps[i].Phase == "ordinary" && steps[i].Index == a.OrdinaryIndex {
			steps[i].Status = "succeeded"
			steps[i].Intent = true
			steps[i].Started = false
			steps[i].StopConfirmed = true
			steps[i].ExitCode = -1
		}
	}
	return &protocol.ApprovalResumeEvidence{IOSResourceDigest: p.Approval.IOSResourceDigest, ResourceID: a.ResourceID, OwnershipDigest: a.OwnershipDigest, ApprovalID: a.ID, Revision: a.Revision, CheckpointDigest: a.CheckpointDigest, CheckpointRef: old, SnapshotDigest: a.SnapshotDigest, WorkspaceID: a.WorkspaceID, ResultID: a.ResultID, NextOrdinaryIndex: a.NextOrdinaryIndex, Steps: steps, EventSeq: a.EventSeq, LastLogSeq: a.LastLogSeq, LastLogOffset: a.LastLogOffset, LastArtifactSeq: a.LastArtifactSeq, Artifacts: p.Approval.Artifacts, Reports: p.Approval.Reports, ReportManifest: p.Approval.ReportManifest, PublishIntents: p.Approval.PublishIntents}, nil
}
func (s *Store) claimApproval(tx *gorm.DB, actor NodeActor, node nodeRecord, session nodeSessionRecord, key string, policy LeasePolicy) (*protocol.LeaseGrant, error) {
	var rows []buildRecord
	if err := tx.Where("status = ? AND node_id = ?", "approved", node.ID).Order("created_at,id").Limit(128).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		reason, err := candidateReason(tx, row, node, session, policy)
		if err != nil {
			return nil, err
		}
		if reason != "" {
			if err = tx.Model(&row).Update("resume_reason", reason).Error; err != nil {
				return nil, err
			}
			continue
		}
		if row.CurrentApprovalID == nil || row.AttemptID == nil || row.StopUnconfirmed || row.LeaseEpoch == math.MaxInt64 {
			return nil, ErrConflict
		}
		var approval approvalRecord
		if err = tx.First(&approval, "id = ? AND build_id = ?", *row.CurrentApprovalID, row.ID).Error; err != nil {
			return nil, err
		}
		var old protocol.LeaseRef
		if approval.State != "approved" || approval.Decision != "approve" || approval.DecidedAt == nil || json.Unmarshal([]byte(approval.CheckpointRefJSON), &old) != nil || old != buildRef(row) || !approval.StopConfirmed || approval.CleanupFailed || !approval.SystemResourcesClosed {
			return nil, ErrConflict
		}
		now := time.Now().UTC()
		expires := now.Add(policy.Duration)
		ref := protocol.LeaseRef{BuildID: row.ID, AttemptID: *row.AttemptID, NodeID: node.ID, SessionID: session.ID, LeaseID: uuid.NewString(), Epoch: row.LeaseEpoch + 1}
		encoded, err := encode(ref)
		if err != nil {
			return nil, err
		}
		result := tx.Model(&approval).Where("state = ? AND resume_ref_json = ?", "approved", "").Updates(map[string]any{"state": "resumed", "resume_ref_json": encoded})
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected != 1 {
			return nil, ErrConflict
		}
		result = tx.Model(&attemptRecord{}).Where("id = ? AND build_id = ?", ref.AttemptID, row.ID).Updates(map[string]any{"node_id": ref.NodeID, "session_id": ref.SessionID, "credential_id": actor.CredentialID, "claim_key": key, "lease_id": ref.LeaseID, "epoch": ref.Epoch, "lease_expires_at": expires})
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected != 1 {
			return nil, ErrConflict
		}
		if err = tx.Model(&stepRecord{}).Where("build_id = ? AND phase = ? AND \"index\" = ? AND kind = ? AND status = ?", row.ID, "ordinary", approval.OrdinaryIndex, "approval", "waiting_approval").Updates(map[string]any{"status": "succeeded", "intent": true, "stop_confirmed": true, "exit_code": -1}).Error; err != nil {
			return nil, err
		}
		row.SessionID = &ref.SessionID
		row.LeaseID = &ref.LeaseID
		row.LeaseEpoch = ref.Epoch
		row.LeaseExpiresAt = &expires
		row.Status = "running"
		row.ResumeReason = ""
		if err = tx.Model(&row).Updates(map[string]any{"session_id": ref.SessionID, "lease_id": ref.LeaseID, "lease_epoch": ref.Epoch, "lease_expires_at": expires, "status": "running", "resume_reason": ""}).Error; err != nil {
			return nil, err
		}
		s.transactionExpiry = &expires
		grant, err := grantFor(tx, row, true)
		if err != nil {
			return nil, err
		}
		if err = checkBoundary(tx, actor, ref, expires); err != nil {
			return nil, err
		}
		return &grant, nil
	}
	return nil, nil
}

// 只对中央已批准且绑定同一旧资源的真实恢复授权重新绑定当前Ref。
func approvalResourceRebind(tx *gorm.DB, row buildRecord, old nodeResourceRecord, in protocol.NodeResourceRegistration) (bool, error) {
	if row.CurrentApprovalID == nil || old.ID != in.ID || old.HasWorkspace != in.HasWorkspace || old.HasResults != in.HasResults {
		return false, nil
	}
	var a approvalRecord
	if err := tx.First(&a, "id = ?", *row.CurrentApprovalID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	var previous, next protocol.LeaseRef
	if a.State != "resumed" || a.ResourceID != old.ID || a.OwnershipDigest != old.OwnershipDigest || json.Unmarshal([]byte(a.CheckpointRefJSON), &previous) != nil || json.Unmarshal([]byte(a.ResumeRefJSON), &next) != nil || previous != resourceRecordRef(old) || next != in.Ref || next != buildRef(row) {
		return false, nil
	}
	if err := tx.Model(&old).Updates(map[string]any{"session_id": in.Ref.SessionID, "lease_id": in.Ref.LeaseID, "epoch": in.Ref.Epoch, "ownership_digest": in.OwnershipDigest, "terminal_seq": 0, "terminal_digest": "", "completed_at": nil, "completion_json": ""}).Error; err != nil {
		return false, err
	}
	return true, nil
}

func validateApprovalRecovery(tx *gorm.DB, row buildRecord) error {
	if row.CurrentApprovalID == nil {
		return nil
	}
	var a approvalRecord
	if err := tx.First(&a, "id = ? AND build_id = ? AND attempt_id = ?", *row.CurrentApprovalID, row.ID, row.AttemptID).Error; err != nil {
		return err
	}
	var old protocol.LeaseRef
	var p protocol.ExecutionProgress
	if json.Unmarshal([]byte(a.CheckpointRefJSON), &old) != nil || json.Unmarshal([]byte(a.CheckpointJSON), &p) != nil || p.Approval == nil || !a.StopConfirmed || a.CleanupFailed || !a.SystemResourcesClosed {
		return ErrConflict
	}
	digest, err := protocol.ApprovalCheckpointDigest(old, a.EventSeq, p)
	if err != nil || digest != a.CheckpointDigest || progressDigest(p) != a.EventDigest {
		return ErrConflict
	}
	var receipt executionReceiptRecord
	if err = tx.First(&receipt, "build_id = ? AND attempt_id = ? AND seq = ?", row.ID, old.AttemptID, a.EventSeq).Error; err != nil {
		return err
	}
	if receipt.Kind != "approval_checkpoint" || !receipt.StopKnown || receipt.Digest != a.EventDigest {
		return ErrConflict
	}
	if a.State == "resumed" {
		var ref protocol.LeaseRef
		if json.Unmarshal([]byte(a.ResumeRefJSON), &ref) != nil || ref != buildRef(row) {
			return ErrConflict
		}
	} else if old != buildRef(row) || a.ResumeRefJSON != "" || a.EventSeq != row.LastEventSeq {
		return ErrConflict
	}
	return nil
}

// 三种发布在同一实际授权事务检查前序所有审批，Node提供的skip或字段不能替代决定。
func validatePublishApprovals(tx *gorm.DB, row buildRecord, index int, in protocol.PublishAuthorization) error {
	steps, err := loadSteps(tx, row.ID)
	if err != nil {
		return err
	}
	for _, step := range steps {
		if step.Phase != "ordinary" || step.Kind != "approval" || step.Index >= index {
			continue
		}
		var a approvalRecord
		if err = tx.First(&a, "build_id = ? AND attempt_id = ? AND ordinary_index = ?", row.ID, row.AttemptID, step.Index).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrForbidden
			}
			return err
		}
		var ref protocol.LeaseRef
		if a.Decision != "approve" || a.State != "resumed" || a.DecidedAt == nil || step.Status != "succeeded" || step.Started || !step.Intent || !step.StopConfirmed || step.CleanupFailed || json.Unmarshal([]byte(a.CheckpointRefJSON), &ref) != nil {
			return ErrForbidden
		}
		allowed, err := approvalHistoricalRef(tx, row, ref)
		if err != nil {
			return err
		}
		if !allowed {
			return ErrForbidden
		}
	}
	return nil
}
