package store

import (
	"context"
	"encoding/json"
	"math"
	"slices"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"mybuilds/internal/protocol"
)

func validRef(ref protocol.LeaseRef) bool {
	return validUUID(ref.NodeID) && validUUID(ref.SessionID) && validUUID(ref.BuildID) && validUUID(ref.AttemptID) && validUUID(ref.LeaseID) && ref.Epoch > 0
}
func buildRef(row buildRecord) protocol.LeaseRef {
	ref := protocol.LeaseRef{BuildID: row.ID, Epoch: row.LeaseEpoch}
	if row.NodeID != nil {
		ref.NodeID = *row.NodeID
	}
	if row.SessionID != nil {
		ref.SessionID = *row.SessionID
	}
	if row.AttemptID != nil {
		ref.AttemptID = *row.AttemptID
	}
	if row.LeaseID != nil {
		ref.LeaseID = *row.LeaseID
	}
	return ref
}

// 原到期值由调用方捕获；Renew写入新到期后仍检查原值，不能用自己延长的权限提交。
func checkBoundary(db *gorm.DB, actor NodeActor, ref protocol.LeaseRef, oldExpires time.Time) error {
	node, _, err := authorizeNode(db, actor)
	if err != nil {
		return err
	}
	if actor.ID != ref.NodeID || node.State == "disabled" {
		return ErrNodeUnauthorized
	}
	if node.SessionID == nil || *node.SessionID != ref.SessionID {
		return ErrLeaseInvalid
	}
	var attempt attemptRecord
	if err = db.First(&attempt, "id = ? AND build_id = ?", ref.AttemptID, ref.BuildID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return ErrLeaseInvalid
		}
		return err
	}
	if attempt.NodeID != ref.NodeID || attempt.SessionID != ref.SessionID || attempt.LeaseID != ref.LeaseID || attempt.Epoch != ref.Epoch || attempt.CredentialID != actor.CredentialID {
		return ErrLeaseInvalid
	}
	var row buildRecord
	if err = db.First(&row, "id = ?", ref.BuildID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return ErrLeaseInvalid
		}
		return err
	}
	if buildRef(row) != ref {
		return ErrLeaseInvalid
	}
	if !time.Now().UTC().Before(oldExpires) {
		return ErrLeaseExpired
	}
	return nil
}
func (s *Store) currentExecution(db *gorm.DB, actor NodeActor, ref protocol.LeaseRef) (buildRecord, error) {
	var row buildRecord
	if !validRef(ref) {
		return row, ErrInvalid
	}
	if err := db.First(&row, "id = ?", ref.BuildID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return row, ErrLeaseInvalid
		}
		return row, err
	}
	if row.LeaseExpiresAt == nil || buildRef(row) != ref {
		return row, ErrLeaseInvalid
	}
	if err := checkBoundary(db, actor, ref, *row.LeaseExpiresAt); err != nil {
		return row, err
	}
	if row.Status != "running" || row.StopUnconfirmed {
		return row, ErrLeaseInvalid
	}
	expiry := *row.LeaseExpiresAt
	s.transactionExpiry = &expiry
	return row, nil
}
func taskSnapshot(db *gorm.DB, row buildRecord) (protocol.TaskSnapshot, error) {
	var snapshot BuildSnapshot
	var project projectRecord
	var batch batchRecord
	if json.Unmarshal([]byte(row.SnapshotJSON), &snapshot) != nil {
		return protocol.TaskSnapshot{}, errDatabase
	}
	if err := db.First(&project, "id = ?", row.ProjectID).Error; err != nil {
		return protocol.TaskSnapshot{}, err
	}
	if err := db.First(&batch, "id = ?", row.BatchID).Error; err != nil {
		return protocol.TaskSnapshot{}, err
	}
	if row.Number == nil {
		return protocol.TaskSnapshot{}, errDatabase
	}
	return protocol.TaskSnapshot{Project: project.Name, BuildName: row.Name, Number: *row.Number, Repository: project.Repository, Branch: batch.Branch, SHA: batch.SHA, SourceDigest: batch.SourceDigest, Definition: snapshot.Definition, Parameters: snapshot.Params, Facts: snapshot.Facts}, nil
}
func grantFor(db *gorm.DB, row buildRecord, includeTask bool) (protocol.LeaseGrant, error) {
	if row.LeaseExpiresAt == nil {
		return protocol.LeaseGrant{}, ErrLeaseInvalid
	}
	grant := protocol.LeaseGrant{Ref: buildRef(row), TTLNS: int64(time.Until(*row.LeaseExpiresAt)), CancelRequested: row.CancelRequested, RemainingBudgetNS: row.RemainingBudgetNS, RemainingPostBudgetNS: row.RemainingPostBudgetNS}
	if grant.TTLNS <= 0 {
		return protocol.LeaseGrant{}, ErrLeaseExpired
	}
	if includeTask {
		task, err := taskSnapshot(db, row)
		if err != nil {
			return protocol.LeaseGrant{}, err
		}
		resume, err := approvalResumeEvidence(db, row)
		if err != nil {
			return protocol.LeaseGrant{}, err
		}
		task.Resume = resume
		grant.Task = &task
	}
	return grant, nil
}
func candidateReason(db *gorm.DB, row buildRecord, node nodeRecord, session nodeSessionRecord, policy LeasePolicy) (string, error) {
	var snapshot BuildSnapshot
	var project projectRecord
	var allowed, labels []string
	if json.Unmarshal([]byte(row.SnapshotJSON), &snapshot) != nil || json.Unmarshal([]byte(node.LabelsJSON), &labels) != nil {
		return "", errDatabase
	}
	if err := db.First(&project, "id = ?", row.ProjectID).Error; err != nil {
		return "", err
	}
	if json.Unmarshal([]byte(project.NodesJSON), &allowed) != nil {
		return "", errDatabase
	}
	if !slices.Contains(snapshot.AllowedNodes, node.Name) || !slices.Contains(allowed, node.Name) {
		return "node_unauthorized", nil
	}
	if snapshot.Definition.Runner == nil {
		if snapshot.DefaultNode == "" {
			return "default_node_missing", nil
		}
		if snapshot.DefaultNode != node.Name {
			return "node_unavailable", nil
		}
	}
	if node.State != "enabled" || !time.Now().UTC().Before(session.LastHeartbeat.Add(3*policy.Heartbeat)) {
		return "node_unavailable", nil
	}
	var guard int64
	if err := db.Model(&buildRecord{}).Where("node_id = ? AND stop_unconfirmed = ?", node.ID, true).Count(&guard).Error; err != nil {
		return "", err
	}
	if guard > 0 {
		return "node_unavailable", nil
	}
	tools, err := decodeTools(session.ToolsJSON)
	if err != nil {
		return "", err
	}
	passed := map[string]bool{}
	for _, tool := range tools {
		passed[tool.Name] = tool.Status == "passed"
	}
	for _, required := range []string{"shell", "git", "node_journal"} {
		if !passed[required] {
			return "capability_mismatch", nil
		}
	}
	if runner := snapshot.Definition.Runner; runner != nil {
		requiredTools := []string{}
		switch runner.Platform {
		case "android":
			requiredTools = []string{"java", "android_aapt2", "android_apksigner"}
		case "ios":
			if session.OS != "darwin" {
				return "capability_mismatch", nil
			}
			requiredTools = []string{"xcode", "ios_signing"}
		default:
			return "capability_mismatch", nil
		}
		if runner.Framework == "flutter" {
			requiredTools = append(requiredTools, "flutter", "dart")
			if runner.Platform == "ios" {
				requiredTools = append(requiredTools, "cocoapods")
			}
		}
		for _, required := range requiredTools {
			if !passed[required] {
				return "capability_mismatch", nil
			}
		}
		for _, label := range runner.Labels {
			if !slices.Contains(labels, label) {
				return "capability_mismatch", nil
			}
		}
	}
	if snapshot.Definition.IOSSigning != nil && (session.OS != "darwin" || !passed["xcode"] || !passed["ios_signing"]) {
		return "capability_mismatch", nil
	}
	var nameOccupied, global, nodeOccupied int64
	if err := db.Model(&buildRecord{}).Where("id <> ? AND project_id = ? AND name = ? AND (status IN ? OR stop_unconfirmed = ?)", row.ID, row.ProjectID, row.Name, []string{"running", "waiting_approval", "approved"}, true).Count(&nameOccupied).Error; err != nil {
		return "", err
	}
	if nameOccupied > 0 {
		return "build_name_locked", nil
	}
	if err := db.Model(&buildRecord{}).Where("status = ? OR stop_unconfirmed = ?", "running", true).Count(&global).Error; err != nil {
		return "", err
	}
	if err := db.Model(&buildRecord{}).Where("node_id = ? AND (status = ? OR stop_unconfirmed = ?)", node.ID, "running", true).Count(&nodeOccupied).Error; err != nil {
		return "", err
	}
	if global >= int64(policy.Concurrency) || nodeOccupied >= int64(min(node.MaxCapacity, session.Capacity)) {
		return "capacity_wait", nil
	}
	return "", nil
}
func (s *Store) Claim(ctx context.Context, actor NodeActor, in protocol.ClaimRequest, policy LeasePolicy) (*protocol.LeaseGrant, error) {
	if !validUUID(in.SessionID) || !validUUID(in.ClaimKey) || !validLeasePolicy(policy) {
		return nil, ErrInvalid
	}
	var grant *protocol.LeaseGrant
	err := s.write(ctx, func(tx *gorm.DB) error {
		node, _, err := authorizeNode(tx, actor)
		if err != nil {
			return err
		}
		session, err := currentSession(tx, node, actor, in.SessionID, policy)
		if err != nil {
			return err
		}
		var attempt attemptRecord
		err = tx.First(&attempt, "session_id = ? AND claim_key = ?", in.SessionID, in.ClaimKey).Error
		if err == nil {
			ref := protocol.LeaseRef{NodeID: attempt.NodeID, SessionID: attempt.SessionID, BuildID: attempt.BuildID, AttemptID: attempt.ID, LeaseID: attempt.LeaseID, Epoch: attempt.Epoch}
			row, e := s.currentExecution(tx, actor, ref)
			if e != nil {
				return e
			}
			value, e := grantFor(tx, row, true)
			if e != nil {
				return e
			}
			grant = &value
			return checkBoundary(tx, actor, ref, *row.LeaseExpiresAt)
		}
		if err != gorm.ErrRecordNotFound {
			return err
		}
		resumed, e := s.claimApproval(tx, actor, node, session, in.ClaimKey, policy)
		if e != nil {
			return e
		}
		if resumed != nil {
			grant = resumed
			return nil
		}
		var rows []buildRecord
		if err = tx.Where("status = ?", "queued").Order("created_at ASC, id ASC").Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			reason, e := candidateReason(tx, row, node, session, policy)
			if e != nil {
				return e
			}
			if reason != "" {
				if row.Reason != reason {
					if e = tx.Model(&row).Update("reason", reason).Error; e != nil {
						return e
					}
				}
				continue
			}
			if row.LeaseEpoch == math.MaxInt64 {
				return ErrConflict
			}
			now := time.Now().UTC()
			expires := now.Add(policy.Duration)
			attempt = attemptRecord{ID: uuid.NewString(), BuildID: row.ID, NodeID: node.ID, SessionID: session.ID, CredentialID: actor.CredentialID, ClaimKey: in.ClaimKey, LeaseID: uuid.NewString(), Epoch: row.LeaseEpoch + 1, LeaseExpiresAt: expires, CreatedAt: now}
			if e = tx.Create(&attempt).Error; e != nil {
				return e
			}
			row.NodeID = &node.ID
			row.SessionID = &session.ID
			row.AttemptID = &attempt.ID
			row.LeaseID = &attempt.LeaseID
			row.LeaseEpoch = attempt.Epoch
			row.LeaseExpiresAt = &expires
			row.Status = "running"
			row.Reason = ""
			row.RemainingPostBudgetNS = row.PostBudgetNS
			result := tx.Model(&buildRecord{}).Where("id = ? AND status = ?", row.ID, "queued").Updates(map[string]any{"node_id": node.ID, "session_id": session.ID, "attempt_id": attempt.ID, "lease_id": attempt.LeaseID, "lease_epoch": attempt.Epoch, "lease_expires_at": expires, "status": "running", "reason": "", "remaining_post_budget_ns": row.RemainingPostBudgetNS})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrConflict
			}
			s.transactionExpiry = &expires
			value, e := grantFor(tx, row, true)
			if e != nil {
				return e
			}
			grant = &value
			return checkBoundary(tx, actor, value.Ref, expires)
		}
		_, _, err = authorizeNode(tx, actor)
		return err
	})
	if err != nil {
		return nil, err
	}
	return grant, nil
}
func (s *Store) Renew(ctx context.Context, actor NodeActor, ref protocol.LeaseRef, policy LeasePolicy) (protocol.LeaseGrant, error) {
	if !validRef(ref) || !validLeasePolicy(policy) {
		return protocol.LeaseGrant{}, ErrInvalid
	}
	var grant protocol.LeaseGrant
	err := s.write(ctx, func(tx *gorm.DB) error {
		row, err := s.currentExecution(tx, actor, ref)
		if err != nil {
			return err
		}
		oldExpires := *row.LeaseExpiresAt
		var session nodeSessionRecord
		if err = tx.First(&session, "id = ?", ref.SessionID).Error; err != nil {
			return err
		}
		if session.HeartbeatNS != int64(policy.Heartbeat) || session.LeaseNS != int64(policy.Duration) {
			return ErrSessionConflict
		}
		expires := time.Now().UTC().Add(policy.Duration)
		if err = tx.Model(&row).Update("lease_expires_at", expires).Error; err != nil {
			return err
		}
		if err = tx.Model(&attemptRecord{}).Where("id = ?", ref.AttemptID).Update("lease_expires_at", expires).Error; err != nil {
			return err
		}
		row.LeaseExpiresAt = &expires
		grant, err = grantFor(tx, row, false)
		if err != nil {
			return err
		}
		return checkBoundary(tx, actor, ref, oldExpires)
	})
	if err != nil {
		return protocol.LeaseGrant{}, err
	}
	return grant, nil
}
func (s *Store) CheckExecution(ctx context.Context, actor NodeActor, ref protocol.LeaseRef) error {
	if !validRef(ref) {
		return ErrInvalid
	}
	return s.write(ctx, func(tx *gorm.DB) error {
		row, err := s.currentExecution(tx, actor, ref)
		if err != nil {
			return err
		}
		return checkBoundary(tx, actor, ref, *row.LeaseExpiresAt)
	})
}
