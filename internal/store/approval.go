package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"gorm.io/gorm"
	"mybuilds/internal/protocol"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// 原checkpoint固定原Ref和ledger；恢复只添加新的Ref证明，不能覆盖历史。
func applyApprovalCheckpoint(tx *gorm.DB, row *buildRecord, in protocol.ExecutionEvent) error {
	p := in.Progress
	a := p.Approval
	digest, err := protocol.ApprovalCheckpointDigest(in.Ref, in.Seq, p)
	if err != nil || a == nil || digest != a.CheckpointDigest || row.PostPhase != "" || row.CancelRequested || row.Reason != "" || p.LastLogSeq != row.LastLogSeq || p.LastLogOffset != row.LastLogOffset || p.LastArtifactSeq != row.LastArtifactSeq {
		return ErrEventConflict
	}
	task, err := taskSnapshot(tx, *row)
	if err != nil {
		return err
	}
	snapshot, err := protocol.ApprovalSnapshotDigest(task)
	if err != nil || snapshot != a.SnapshotDigest {
		return ErrEventConflict
	}
	steps, err := loadSteps(tx, row.ID)
	if err != nil {
		return err
	}
	var target *stepRecord
	ledger := make([]protocol.ApprovalStepLedger, 0, len(steps))
	for i := range steps {
		step := &steps[i]
		if step.CleanupFailed || step.Status == "intent" || step.Status == "started" {
			return ErrEventConflict
		}
		if step.Phase == "ordinary" {
			if step.Index < p.Index && (!terminalStep(step.Status) || !step.StopConfirmed || step.Status == "failed" || step.Status == "cancelled") {
				return ErrEventConflict
			}
			if step.Index == p.Index {
				target = step
			}
			if step.Index > p.Index && (step.Intent || step.Started || step.StopConfirmed) {
				return ErrEventConflict
			}
		} else if step.Intent || step.Started || step.StopConfirmed {
			return ErrEventConflict
		}
		reasons := []string{}
		if json.Unmarshal([]byte(step.ReasonsJSON), &reasons) != nil {
			return errDatabase
		}
		ledger = append(ledger, protocol.ApprovalStepLedger{Phase: step.Phase, Index: step.Index, Name: step.Name, Kind: step.Kind, Condition: step.Condition, Status: step.Status, Reasons: reasons, ElapsedNS: step.ElapsedNS, Intent: step.Intent, Started: step.Started, StopConfirmed: step.StopConfirmed, CleanupFailed: step.CleanupFailed, Reason: step.Reason, ExitCode: step.ExitCode})
	}
	if target == nil || target.Kind != "approval" || target.Name != p.Name || target.Status != "pending" || target.Intent || target.Started || target.StopConfirmed {
		return ErrEventConflict
	}
	if err = approvalFiles(tx, *row, p, steps); err != nil {
		return err
	}
	evidence, err := storedReports(*row)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(evidence, a.Reports) {
		return ErrEventConflict
	}
	hasUpload := false
	for _, step := range steps {
		hasUpload = hasUpload || step.Kind == "upload"
	}
	if hasUpload && task.Definition.Reports != nil && (evidence == nil || !row.ReportFinal || !evidence.Sealed || evidence.Outcome != "passed") {
		return ErrEventConflict
	}
	if evidence != nil {
		if evidence.Reason != "" || evidence.Outcome == "failed" {
			return ErrEventConflict
		}
		if evidence.Sealed {
			if a.ReportManifest == nil || a.ReportManifest.SealDigest != row.ReportSealDigest {
				return ErrEventConflict
			}
		} else if a.ReportManifest != nil {
			return ErrEventConflict
		}
	} else if a.ReportManifest != nil {
		return ErrEventConflict
	}
	required, err := requiresIOSCleanup(*row)
	if err != nil {
		return err
	}
	if required {
		started := false
		for _, step := range steps {
			started = started || step.Phase == "ordinary" && step.Kind == "run" && step.Started
		}
		if started && !validDigest(a.IOSResourceDigest) {
			return ErrEventConflict
		}
	} else if a.IOSResourceDigest != "" {
		return ErrEventConflict
	}
	if a.ResourceID != "" || a.OwnershipDigest != "" {
		if !validUUID(a.ResourceID) || !validDigest(a.OwnershipDigest) {
			return ErrEventConflict
		}
		var resource nodeResourceRecord
		if err = tx.First(&resource, "id = ? AND attempt_id = ?", a.ResourceID, in.Ref.AttemptID).Error; err != nil {
			return ErrEventConflict
		}
		if resourceRecordRef(resource) != in.Ref || resource.OwnershipDigest != a.OwnershipDigest || !resource.HasWorkspace || !resource.HasResults {
			return ErrEventConflict
		}
	}
	var count int64
	if err = tx.Model(&approvalRecord{}).Where("build_id = ?", row.ID).Count(&count).Error; err != nil {
		return err
	}
	if count >= 128 || a.Revision != count+1 {
		return ErrEventConflict
	}
	checkpoint, err := encode(p)
	if err != nil {
		return err
	}
	oldRef, err := encode(in.Ref)
	if err != nil {
		return err
	}
	storedLedger, err := encode(ledger)
	if err != nil {
		return err
	}
	record := approvalRecord{ID: a.ID, BuildID: row.ID, AttemptID: in.Ref.AttemptID, NodeID: in.Ref.NodeID, OrdinaryIndex: p.Index, Revision: a.Revision, StepName: p.Name, CheckpointRefJSON: oldRef, EventSeq: in.Seq, EventDigest: in.Digest, CheckpointDigest: a.CheckpointDigest, SnapshotDigest: a.SnapshotDigest, WorkspaceID: a.WorkspaceID, ResultID: a.ResultID, ResourceID: a.ResourceID, OwnershipDigest: a.OwnershipDigest, NextOrdinaryIndex: a.NextOrdinaryIndex, RemainingBudgetNS: p.RemainingBudgetNS, RemainingPostBudgetNS: p.RemainingPostBudgetNS, LastLogSeq: p.LastLogSeq, LastLogOffset: p.LastLogOffset, LastArtifactSeq: p.LastArtifactSeq, CheckpointJSON: checkpoint, LedgerJSON: storedLedger, StopConfirmed: true, SystemResourcesClosed: true, State: "pending", CreatedAt: time.Now().UTC()}
	if err = tx.Create(&record).Error; err != nil {
		return err
	}
	if err = tx.Model(target).Updates(map[string]any{"intent": true, "status": "waiting_approval", "stop_confirmed": true, "exit_code": -1}).Error; err != nil {
		return err
	}
	if err = tx.Model(&nodeResourceRecord{}).Where("attempt_id = ? AND node_id = ? AND session_id = ? AND lease_id = ? AND epoch = ?", in.Ref.AttemptID, in.Ref.NodeID, in.Ref.SessionID, in.Ref.LeaseID, in.Ref.Epoch).Updates(map[string]any{"terminal_seq": in.Seq, "terminal_digest": in.Digest}).Error; err != nil {
		return err
	}
	row.CurrentApprovalID = &record.ID
	row.Status = "waiting_approval"
	row.StopUnconfirmed = false
	return tx.Model(row).Update("current_approval_id", record.ID).Error
}
func approvalFiles(tx *gorm.DB, row buildRecord, p protocol.ExecutionProgress, steps []stepRecord) error {
	if !reflect.DeepEqual(p.ArtifactSteps, p.Approval.Artifacts) {
		return ErrEventConflict
	}
	// 普通产物沿原完整manifest核验；未final报告的已确认文件单独核对，不能伪造seal。
	var files []artifactRecord
	if err := tx.Where("attempt_id = ?", row.AttemptID).Limit(129).Find(&files).Error; err != nil {
		return err
	}
	if len(files) > 128 {
		return ErrEventConflict
	}
	expected := map[string]stepRecord{}
	for _, s := range steps {
		if s.Kind == "artifact" && s.ArtifactIDsJSON != "" && s.ArtifactIDsJSON != "[]" && s.ArtifactIDsJSON != "null" {
			expected[s.Phase+"/"+strconv.Itoa(s.Index)] = s
		}
	}
	if len(expected) != len(p.ArtifactSteps) {
		return ErrEventConflict
	}
	seen := map[string]bool{}
	for _, e := range p.ArtifactSteps {
		step, ok := expected[e.Phase+"/"+strconv.Itoa(e.Index)]
		if !ok || step.Index != e.Index || e.Count != len(e.IDs) || !validateArtifactIDs(e.IDs) {
			return ErrEventConflict
		}
		var ids []string
		if json.Unmarshal([]byte(step.ArtifactIDsJSON), &ids) != nil || !reflect.DeepEqual(ids, e.IDs) {
			return ErrEventConflict
		}
		for _, id := range e.IDs {
			if seen[id] {
				return ErrEventConflict
			}
			seen[id] = true
		}
	}
	reportIDs := map[string]bool{}
	if p.Approval.Reports != nil {
		for _, f := range p.Approval.Reports.Files {
			if !validUUID(f.ArtifactID) || reportIDs[f.ArtifactID] {
				return ErrEventConflict
			}
			reportIDs[f.ArtifactID] = true
		}
	}
	for _, f := range files {
		if f.Purpose == "junit" {
			if !reportIDs[f.ID] && f.ReportRevision >= row.ReportRevision {
				return ErrEventConflict
			}
			delete(reportIDs, f.ID)
		} else {
			if !seen[f.ID] {
				return ErrEventConflict
			}
			delete(seen, f.ID)
		}
	}
	if len(seen) != 0 || len(reportIDs) != 0 {
		return ErrEventConflict
	}
	return nil
}
func approvalView(tx *gorm.DB, a approvalRecord) (ApprovalView, error) {
	var b buildRecord
	var project projectRecord
	var node nodeRecord
	if err := tx.First(&b, "id = ?", a.BuildID).Error; err != nil {
		return ApprovalView{}, err
	}
	if err := tx.First(&project, "id = ?", b.ProjectID).Error; err != nil {
		return ApprovalView{}, err
	}
	if err := tx.First(&node, "id = ?", a.NodeID).Error; err != nil {
		return ApprovalView{}, err
	}
	var p protocol.ExecutionProgress
	if json.Unmarshal([]byte(a.CheckpointJSON), &p) != nil || p.Approval == nil {
		return ApprovalView{}, errDatabase
	}
	ids := []string{}
	for _, e := range p.Approval.Artifacts {
		ids = append(ids, e.IDs...)
	}
	number := int64(0)
	if b.Number != nil {
		number = *b.Number
	}
	out := ApprovalView{ID: a.ID, BuildID: b.ID, ProjectID: project.ID, Project: project.Name, BuildName: b.Name, Number: number, Step: a.StepName, Index: a.OrdinaryIndex, Revision: a.Revision, CheckpointDigest: a.CheckpointDigest, State: a.State, NodeName: node.Name, ArtifactIDs: ids, Reports: p.Approval.Reports, RemainingBudgetNS: a.RemainingBudgetNS, RemainingPostBudgetNS: a.RemainingPostBudgetNS, Decision: a.Decision, DecisionActor: a.DecisionActor, DecidedAt: a.DecidedAt, CreatedAt: a.CreatedAt}
	if a.Note != "" {
		note := sha256.Sum256([]byte(a.Note))
		out.NoteDigest = hex.EncodeToString(note[:])
	}
	if p.Approval.ReportManifest != nil {
		out.ReportSealDigest = p.Approval.ReportManifest.SealDigest
	}
	return out, nil
}
func (s *Store) ListApprovals(ctx context.Context, actor Actor, in ApprovalFilter) ([]ApprovalView, error) {
	page, err := normalizePage(in.Page)
	if err != nil || page.Limit > 100 || in.State != "" && !slices.Contains([]string{"pending", "approved", "resumed", "rejected", "cancelled"}, in.State) {
		return nil, ErrInvalid
	}
	result := []ApprovalView{}
	err = s.write(ctx, func(tx *gorm.DB) error {
		if err := authorize(tx, actor, "admin", "approver"); err != nil {
			return err
		}
		query := tx.Model(&approvalRecord{})
		if in.ProjectID != "" {
			if !validUUID(in.ProjectID) {
				return ErrInvalid
			}
			query = query.Where("build_id IN (SELECT id FROM builds WHERE project_id = ?)", in.ProjectID)
		}
		if in.State != "" {
			query = query.Where("state = ?", in.State)
		}
		var rows []approvalRecord
		if err := query.Order("created_at DESC,id DESC").Limit(page.Limit).Offset(page.Offset).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			v, err := approvalView(tx, row)
			if err != nil {
				return err
			}
			result = append(result, v)
		}
		return nil
	})
	return result, err
}
func (s *Store) GetApproval(ctx context.Context, actor Actor, id string) (ApprovalView, error) {
	if !validUUID(id) {
		return ApprovalView{}, ErrInvalid
	}
	var out ApprovalView
	err := s.write(ctx, func(tx *gorm.DB) error {
		if err := authorize(tx, actor, "admin", "approver"); err != nil {
			return err
		}
		var row approvalRecord
		if err := tx.First(&row, "id = ?", id).Error; err != nil {
			return err
		}
		var err error
		out, err = approvalView(tx, row)
		return err
	})
	return out, err
}
func validApprovalNote(note string) bool {
	lower := strings.ToLower(note)
	return len(note) <= 1024 && utf8.ValidString(note) && !strings.ContainsFunc(note, unicode.IsControl) && !strings.Contains(note, "${") && !strings.Contains(lower, "token=") && !strings.Contains(lower, "token:") && !strings.Contains(lower, "password=") && !strings.Contains(lower, "password:") && !strings.Contains(lower, "-----begin")
}
func (s *Store) DecideApproval(ctx context.Context, actor Actor, buildID string, in ApprovalDecision) (ApprovalView, error) {
	if !validUUID(buildID) || !validUUID(in.ApprovalID) || in.Revision < 1 || !validDigest(in.CheckpointDigest) || !slices.Contains([]string{"approve", "reject"}, in.Decision) || !validApprovalNote(in.Note) {
		return ApprovalView{}, ErrInvalid
	}
	var out ApprovalView
	err := s.write(ctx, func(tx *gorm.DB) error {
		if err := authorize(tx, actor, "admin", "approver"); err != nil {
			return err
		}
		var a approvalRecord
		var row buildRecord
		if err := tx.First(&a, "id = ? AND build_id = ?", in.ApprovalID, buildID).Error; err != nil {
			return err
		}
		if a.Revision != in.Revision || a.CheckpointDigest != in.CheckpointDigest {
			return ErrConflict
		}
		if err := tx.First(&row, "id = ?", buildID).Error; err != nil {
			return err
		}
		if a.Decision != "" {
			if a.Decision != in.Decision || a.Note != in.Note || a.DecisionActor != actor.ID {
				return ErrConflict
			}
			var err error
			out, err = approvalView(tx, a)
			return err
		}
		if a.State != "pending" || row.Status != "waiting_approval" || row.CurrentApprovalID == nil || *row.CurrentApprovalID != a.ID || row.StopUnconfirmed {
			return ErrConflict
		}
		decisionDigest, err := encode(in)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		state := "approved"
		buildState := "approved"
		reason := ""
		if in.Decision == "reject" {
			state = "rejected"
			buildState = "cancelled"
			reason = "approval_rejected"
			row.TerminalAt = &now
			if err = closeApprovalSteps(tx, row, a.OrdinaryIndex, reason); err != nil {
				return err
			}
			row.PostPhase = "none"
		}
		var identity *string
		if actor.ID != "local-admin" {
			id := actor.ID
			identity = &id
		}
		if err = tx.Model(&a).Updates(map[string]any{"state": state, "decision": in.Decision, "decision_actor": actor.ID, "decision_actor_id": identity, "note": in.Note, "decision_digest": decisionDigest, "decided_at": now}).Error; err != nil {
			return err
		}
		if err = tx.Model(&row).Updates(map[string]any{"status": buildState, "reason": reason, "terminal_at": row.TerminalAt, "post_phase": row.PostPhase}).Error; err != nil {
			return err
		}
		if err = audit(tx, actor, "approval_"+in.Decision, a.ID, "pending", state); err != nil {
			return err
		}
		a.State = state
		a.Decision = in.Decision
		a.DecisionActor = actor.ID
		a.Note = in.Note
		a.DecidedAt = &now
		out, err = approvalView(tx, a)
		return err
	})
	return out, err
}
func (s *Store) ReadApprovalCheckpoint(ctx context.Context, actor NodeActor, in protocol.ApprovalCheckpointLookup) (protocol.ApprovalCheckpointReceipt, error) {
	if !validRef(in.Ref) || !validUUID(in.ApprovalID) || in.Revision < 1 || in.Seq < 1 || !validDigest(in.Digest) || !validDigest(in.CheckpointDigest) {
		return protocol.ApprovalCheckpointReceipt{}, ErrInvalid
	}
	var out protocol.ApprovalCheckpointReceipt
	err := s.write(ctx, func(tx *gorm.DB) error {
		node, _, err := authorizeNode(tx, actor)
		if err != nil {
			return err
		}
		if node.State == "disabled" || node.ID != in.Ref.NodeID {
			return ErrNodeUnauthorized
		}
		var a approvalRecord
		if err = tx.First(&a, "id = ? AND build_id = ? AND attempt_id = ?", in.ApprovalID, in.Ref.BuildID, in.Ref.AttemptID).Error; err != nil {
			return err
		}
		var ref protocol.LeaseRef
		if json.Unmarshal([]byte(a.CheckpointRefJSON), &ref) != nil || ref != in.Ref || a.Revision != in.Revision || a.EventSeq != in.Seq || a.EventDigest != in.Digest || a.CheckpointDigest != in.CheckpointDigest || !a.StopConfirmed || a.CleanupFailed || !a.SystemResourcesClosed {
			return ErrConflict
		}
		var receipt executionReceiptRecord
		if err = tx.First(&receipt, "build_id = ? AND attempt_id = ? AND seq = ?", in.Ref.BuildID, in.Ref.AttemptID, in.Seq).Error; err != nil {
			return err
		}
		if receipt.Kind != "approval_checkpoint" || receipt.Digest != in.Digest || !receipt.StopKnown {
			return ErrConflict
		}
		out = protocol.ApprovalCheckpointReceipt{Ref: ref, ApprovalID: a.ID, Revision: a.Revision, Seq: a.EventSeq, Digest: a.EventDigest, CheckpointDigest: a.CheckpointDigest, State: a.State, NodeName: node.Name, StopConfirmed: true, SystemResourcesClosed: true}
		return nil
	})
	return out, err
}

// 已确认的原pause可证明安全拒绝/取消，但不能确认已恢复的新epoch停止。
func approvalStopped(tx *gorm.DB, row buildRecord) (bool, error) {
	if row.CurrentApprovalID == nil || row.AttemptID == nil {
		return false, nil
	}
	var a approvalRecord
	if err := tx.First(&a, "id = ? AND build_id = ?", *row.CurrentApprovalID, row.ID).Error; err != nil {
		return false, err
	}
	if !slices.Contains([]string{"rejected", "cancelled"}, a.State) || a.ResumeRefJSON != "" || a.EventSeq != row.LastEventSeq || !a.StopConfirmed || a.CleanupFailed || !a.SystemResourcesClosed {
		return false, nil
	}
	var ref protocol.LeaseRef
	var p protocol.ExecutionProgress
	if json.Unmarshal([]byte(a.CheckpointRefJSON), &ref) != nil || ref != buildRef(row) || json.Unmarshal([]byte(a.CheckpointJSON), &p) != nil {
		return false, ErrConflict
	}
	d, err := protocol.ApprovalCheckpointDigest(ref, a.EventSeq, p)
	if err != nil || d != a.CheckpointDigest || progressDigest(p) != a.EventDigest {
		return false, ErrConflict
	}
	var receipt executionReceiptRecord
	if err = tx.First(&receipt, "build_id = ? AND attempt_id = ? AND seq = ?", row.ID, *row.AttemptID, a.EventSeq).Error; err != nil {
		return false, err
	}
	if receipt.Kind != "approval_checkpoint" || !receipt.StopKnown || receipt.Digest != a.EventDigest || a.LastLogSeq != row.LastLogSeq || a.LastLogOffset != row.LastLogOffset || a.LastArtifactSeq != row.LastArtifactSeq {
		return false, nil
	}
	evidence, err := storedReports(row)
	if err != nil {
		return false, err
	}
	if !reflect.DeepEqual(evidence, p.Approval.Reports) {
		return false, ErrConflict
	}
	steps, err := loadSteps(tx, row.ID)
	if err != nil {
		return false, err
	}
	if err = approvalFiles(tx, row, p, steps); err != nil {
		return false, err
	}
	return true, nil
}
func closeApprovalSteps(tx *gorm.DB, row buildRecord, index int, reason string) error {
	var active int64
	if err := tx.Model(&stepRecord{}).Where("build_id = ? AND (status IN ? OR cleanup_failed = ?)", row.ID, []string{"intent", "started"}, true).Count(&active).Error; err != nil {
		return err
	}
	if active != 0 {
		return ErrConflict
	}
	if err := tx.Model(&stepRecord{}).Where("build_id = ? AND phase = ? AND \"index\" = ? AND status = ?", row.ID, "ordinary", index, "waiting_approval").Updates(map[string]any{"status": "cancelled", "reason": reason, "stop_confirmed": true}).Error; err != nil {
		return err
	}
	return tx.Model(&stepRecord{}).Where("build_id = ? AND status = ?", row.ID, "pending").Updates(map[string]any{"status": "skipped", "reason": "not_started", "stop_confirmed": true, "exit_code": -1}).Error
}

func approvalVisibleReportIDs(tx *gorm.DB, row buildRecord) ([]string, error) {
	ids := []string{}
	sealed, err := sealedReports(row)
	if err != nil {
		return nil, err
	}
	if sealed != nil {
		for _, f := range sealed.Files {
			ids = append(ids, f.ArtifactID)
		}
		return ids, nil
	}
	if row.CurrentApprovalID == nil || !slices.Contains([]string{"waiting_approval", "approved", "rejected", "cancelled"}, row.Status) {
		return ids, nil
	}
	if err = validateApprovalRecovery(tx, row); err != nil {
		return nil, err
	}
	var a approvalRecord
	if err = tx.First(&a, "id = ?", *row.CurrentApprovalID).Error; err != nil {
		return nil, err
	}
	var p protocol.ExecutionProgress
	if json.Unmarshal([]byte(a.CheckpointJSON), &p) != nil || p.Approval == nil {
		return nil, ErrConflict
	}
	if p.Approval.Reports != nil {
		for _, f := range p.Approval.Reports.Files {
			ids = append(ids, f.ArtifactID)
		}
	}
	return ids, nil
}
