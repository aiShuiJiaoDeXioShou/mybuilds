package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"mybuilds/internal/protocol"
)

var progressReasons = []string{"", "exit", "start_error", "directory_error", "artifact_error", "timeout", "cancelled", "log_error", "cleanup_error", "progress_error", "condition", "not_started", "budget_exhausted", "not_selected", "authority_lost", "persistence_error", "precheck_error", "checkout_error", "post_error", "report_failed", "report_invalid", "report_missing", "report_secret", "report_error", "publish_precheck", "publish_unknown"}

func progressDigest(p protocol.ExecutionProgress) string {
	b, err := json.Marshal(p)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(b)
	return hex.EncodeToString(digest[:])
}
func validProgress(p protocol.ExecutionProgress) bool {
	_, offset := p.At.Zone()
	reportEvent := p.Kind == "reports_checked" || p.Kind == "reports_sealed"
	if (p.Approval != nil) != (p.Kind == "approval_checkpoint") {
		return false
	}
	if p.IOSResourceDigest != "" && (!p.IOSCleanupConfirmed || !validHex(p.IOSResourceDigest, 64)) {
		return false
	}
	if p.IOSCleanupConfirmed && p.Kind != "build_finished" || len(p.PublishIntents) != 0 && p.Kind != "build_finished" {
		return false
	}
	if len(p.LocalReports) != 0 || (p.Reports != nil) != reportEvent || p.ReportManifest != nil && p.Kind != "build_finished" || len(p.PublishIntents) != 0 && p.Kind != "build_finished" {
		return false
	}
	return !p.At.IsZero() && offset == 0 && p.ElapsedNS >= 0 && p.RemainingPostBudgetNS >= 0 && slices.Contains(progressReasons, p.Reason) && slices.Contains([]string{"intent", "started", "finished", "post_selected", "skipped", "build_finished", "reports_checked", "reports_sealed", "approval_checkpoint"}, p.Kind) && p.PID == 0 && p.PGID == 0 && p.LocalResultDir == "" && len(p.LocalArtifacts) == 0
}
func monotoneBudget(previous, next *int64) bool {
	if previous == nil {
		return next == nil
	}
	return next != nil && *next >= 0 && *next <= *previous
}
func terminalStep(status string) bool {
	return slices.Contains([]string{"succeeded", "failed", "cancelled", "skipped"}, status)
}
func loadSteps(db *gorm.DB, id string) ([]stepRecord, error) {
	var steps []stepRecord
	err := db.Where("build_id = ?", id).Find(&steps).Error
	return steps, err
}
func ordinarySummary(steps []stepRecord) (complete, started, failed, cancelled bool) {
	complete = true
	for _, step := range steps {
		if step.Phase != "ordinary" {
			continue
		}
		complete = complete && terminalStep(step.Status)
		started = started || step.Started
		failed = failed || step.Status == "failed"
		cancelled = cancelled || step.Status == "cancelled"
	}
	return
}
func validateArtifactIDs(ids []string, maxFiles int) bool {
	if len(ids) > maxFiles {
		return false
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if !validUUID(id) || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}
func applyStep(db *gorm.DB, row *buildRecord, p protocol.ExecutionProgress) error {
	if !slices.Contains([]string{"ordinary", "success", "failure", "always"}, p.Phase) || p.Index < 1 || p.PostPhase != "" {
		return ErrEventConflict
	}
	if err := validateReportIntent(db, row, p); err != nil {
		return err
	}
	steps, err := loadSteps(db, row.ID)
	if err != nil {
		return err
	}
	var step *stepRecord
	for i := range steps {
		if steps[i].Phase == p.Phase && steps[i].Index == p.Index {
			step = &steps[i]
			break
		}
	}
	if step == nil || step.Name != p.Name || step.Kind != p.StepKind {
		return ErrEventConflict
	}
	if p.Phase != "ordinary" {
		complete, started, _, _ := ordinarySummary(steps)
		if !complete || row.PostPhase == "" {
			return ErrEventConflict
		}
		selected := p.Phase == row.PostPhase || p.Phase == "always" && started
		if !selected {
			if p.Kind != "skipped" || p.Reason != "not_selected" && p.Reason != "precheck_error" {
				return ErrEventConflict
			}
		}
	}
	if p.Kind == "intent" || p.Kind == "started" {
		for _, other := range steps {
			if other.CleanupFailed {
				return ErrEventConflict
			}
			if other.ID == step.ID {
				continue
			}
			if other.Status == "intent" || other.Status == "started" {
				return ErrEventConflict
			}
			if other.Phase == p.Phase && other.Index < p.Index && !terminalStep(other.Status) {
				return ErrEventConflict
			}
		}
	}
	switch p.Kind {
	case "intent":
		if step.Status != "pending" || step.Intent || p.Started || p.StopConfirmed || p.CleanupFailed || p.Status != "" || p.Reason != "" || len(p.ArtifactIDs) != 0 {
			return ErrEventConflict
		}
		step.Intent = true
		step.Status = "intent"
	case "started":
		if step.Status != "intent" || !step.Intent || step.Started || !p.Started || p.StopConfirmed || p.CleanupFailed || p.Status != "" || p.Reason != "" || len(p.ArtifactIDs) != 0 {
			return ErrEventConflict
		}
		step.Started = true
		step.Status = "started"
	case "finished":
		if !step.Intent || !slices.Contains([]string{"intent", "started"}, step.Status) || !slices.Contains([]string{"succeeded", "failed", "cancelled"}, p.Status) || p.Started != step.Started || !p.StopConfirmed && !p.CleanupFailed || p.CleanupFailed && p.StopConfirmed {
			return ErrEventConflict
		}
		if p.Status == "succeeded" && (!p.Started || p.CleanupFailed || p.Reason != "") {
			return ErrEventConflict
		}
		if p.Status != "succeeded" && p.Reason == "" {
			return ErrEventConflict
		}
		if p.StepKind != "artifact" && len(p.ArtifactIDs) != 0 || !validateArtifactIDs(p.ArtifactIDs, 128) {
			return ErrEventConflict
		}
		if p.StepKind == "artifact" {
			if len(p.ArtifactIDs) != 0 && (!p.Started || !p.StopConfirmed || p.CleanupFailed) {
				return ErrEventConflict
			}
			if p.Status == "succeeded" && len(p.ArtifactIDs) == 0 {
				return ErrEventConflict
			}
		}
		step.Status = p.Status
		step.StopConfirmed = p.StopConfirmed
		step.CleanupFailed = p.CleanupFailed
		step.Reason = p.Reason
		step.ExitCode = p.ExitCode
		step.ElapsedNS = p.ElapsedNS
		step.ArtifactIDsJSON, err = encode(p.ArtifactIDs)
		if err != nil {
			return err
		}
		if p.Phase == "ordinary" && p.Status != "succeeded" && row.Reason == "" {
			row.Reason = p.Reason
		}
	case "skipped":
		if step.Intent || step.Started || step.StopConfirmed || !slices.Contains([]string{"pending", "skipped"}, step.Status) || p.Status != "skipped" || p.Started || !p.StopConfirmed || p.CleanupFailed || p.Reason == "" || len(p.ArtifactIDs) != 0 {
			return ErrEventConflict
		}
		step.Status = "skipped"
		step.StopConfirmed = true
		step.Reason = p.Reason
		step.ExitCode = p.ExitCode
	default:
		return ErrEventConflict
	}
	if err := db.Model(step).Updates(map[string]any{"status": step.Status, "intent": step.Intent, "started": step.Started, "stop_confirmed": step.StopConfirmed, "cleanup_failed": step.CleanupFailed, "reason": step.Reason, "exit_code": step.ExitCode, "elapsed_ns": step.ElapsedNS, "artifact_ids_json": step.ArtifactIDsJSON}).Error; err != nil {
		return err
	}
	return closePublishStep(db, *row, *step)
}
func applyPostSelection(db *gorm.DB, row *buildRecord, p protocol.ExecutionProgress) error {
	if p.Phase != "" || p.Index != 0 || p.Name != "" || p.StepKind != "" || p.Status != "" || row.PostPhase != "" || !slices.Contains([]string{"success", "failure", "none"}, p.PostPhase) || len(p.ArtifactIDs) != 0 {
		return ErrEventConflict
	}
	if err := validateReportPost(db, row, p); err != nil {
		return err
	}
	steps, err := loadSteps(db, row.ID)
	if err != nil {
		return err
	}
	complete, started, failed, cancelled := ordinarySummary(steps)
	if !complete {
		return ErrEventConflict
	}
	expected := "success"
	if failed || strings.HasPrefix(row.Reason, "report_") {
		expected = "failure"
	}
	if cancelled || row.CancelRequested || !started {
		expected = "none"
	}
	// 最后普通步骤成功后，日志与完整快照确认仍可能耗尽普通预算。
	// 保留已经确认的步骤，只在本次选择保存整段timeout原因。
	exhausted := expected == "success" && p.RemainingBudgetNS != nil && *p.RemainingBudgetNS == 0
	if exhausted {
		expected = "failure"
		if p.Reason != "timeout" {
			return ErrEventConflict
		}
	} else if p.Reason != "" {
		return ErrEventConflict
	}
	if p.PostPhase != expected {
		return ErrEventConflict
	}
	if exhausted && row.Reason == "" {
		row.Reason = "timeout"
	}
	row.PostPhase = p.PostPhase
	return nil
}
func verifyManifest(db *gorm.DB, row buildRecord, p protocol.ExecutionProgress, steps []stepRecord) error {
	if p.LastLogSeq != row.LastLogSeq || p.LastLogOffset != row.LastLogOffset || p.LastArtifactSeq != row.LastArtifactSeq || p.ArtifactSteps == nil || len(p.ArtifactSteps) > 128 {
		return ErrEventConflict
	}
	var artifacts []artifactRecord
	if err := db.Where("attempt_id = ?", *row.AttemptID).Find(&artifacts).Error; err != nil {
		return err
	}
	byID := map[string]artifactRecord{}
	for _, file := range artifacts {
		if file.Purpose != "junit" {
			byID[file.ID] = file
		}
	}
	expectedSteps := map[string]stepRecord{}
	for _, step := range steps {
		if step.Kind == "artifact" && step.ArtifactIDsJSON != "" && step.ArtifactIDsJSON != "null" && step.ArtifactIDsJSON != "[]" {
			expectedSteps[step.Phase+"/"+strconv.Itoa(step.Index)] = step
		}
	}
	if len(expectedSteps) != len(p.ArtifactSteps) {
		return ErrEventConflict
	}
	seen := map[string]bool{}
	seenSteps := map[string]bool{}
	for _, expectation := range p.ArtifactSteps {
		key := expectation.Phase + "/" + strconv.Itoa(expectation.Index)
		step, ok := expectedSteps[key]
		if !ok || seenSteps[key] || expectation.Count != len(expectation.IDs) || !validateArtifactIDs(expectation.IDs, 128) {
			return ErrEventConflict
		}
		seenSteps[key] = true
		var ids []string
		if json.Unmarshal([]byte(step.ArtifactIDsJSON), &ids) != nil {
			return errDatabase
		}
		if len(ids) != expectation.Count {
			return ErrEventConflict
		}
		for _, id := range expectation.IDs {
			file, ok := byID[id]
			if !ok || seen[id] || !slices.Contains(ids, id) || file.Phase != step.Phase || file.Index != step.Index || file.Step != step.Name {
				return ErrEventConflict
			}
			seen[id] = true
		}
	}
	if len(seen) != len(byID) {
		return ErrEventConflict
	}
	return validateReportManifest(db, row, p)
}
func applyTerminal(db *gorm.DB, row *buildRecord, p protocol.ExecutionProgress) error {
	if err := validatePublishManifest(db, *row, p); err != nil {
		return err
	}
	if p.Phase != "" || p.Index != 0 || p.Name != "" || p.StepKind != "" || !slices.Contains([]string{"succeeded", "failed", "cancelled", "skipped"}, p.Status) || len(p.ArtifactIDs) != 0 {
		return ErrEventConflict
	}
	required, err := requiresIOSCleanup(*row)
	if err != nil {
		return err
	}
	if !required && (p.IOSCleanupConfirmed || p.IOSResourceDigest != "") || required && !p.IOSCleanupConfirmed && !p.CleanupFailed {
		return ErrEventConflict
	}
	steps, err := loadSteps(db, row.ID)
	if err != nil {
		return err
	}
	precheck := p.Status == "failed" && (p.Reason == "precheck_error" || p.Reason == "checkout_error") && !p.Started && p.StopConfirmed && !p.CleanupFailed && p.PostPhase == "none" && row.PostPhase == ""
	if precheck {
		for _, step := range steps {
			if step.Intent || step.Started || step.StopConfirmed || step.CleanupFailed {
				return ErrEventConflict
			}
		}
		if err = db.Model(&stepRecord{}).Where("build_id = ?", row.ID).Updates(map[string]any{"status": "skipped", "reason": p.Reason, "stop_confirmed": true, "exit_code": -1}).Error; err != nil {
			return err
		}
		for i := range steps {
			steps[i].Status = "skipped"
			steps[i].Reason = p.Reason
			steps[i].StopConfirmed = true
			steps[i].ExitCode = -1
		}
		row.PostPhase = "none"
	} else if p.PostPhase != "" {
		return ErrEventConflict
	}
	started, cleanup := false, false
	for _, step := range steps {
		if !terminalStep(step.Status) || !step.StopConfirmed && !step.CleanupFailed {
			return ErrEventConflict
		}
		if required && p.IOSCleanupConfirmed && step.Phase == "ordinary" && step.Kind == "run" && step.Started && p.IOSResourceDigest == "" {
			return ErrEventConflict
		}
		started = started || step.Started
		cleanup = cleanup || step.CleanupFailed
	}
	nativeUnknown := required && !p.IOSCleanupConfirmed
	cleanup = cleanup || nativeUnknown
	if p.Started != started || p.CleanupFailed != cleanup || p.StopConfirmed == cleanup {
		return ErrEventConflict
	}
	if err = verifyManifest(db, *row, p, steps); err != nil {
		return err
	}
	_, ordinaryStarted, ordinaryFailed, ordinaryCancelled := ordinarySummary(steps)
	allOrdinaryConditionsSkipped := true
	for _, step := range steps {
		if step.Phase == "ordinary" && (step.Status != "skipped" || step.Reason != "condition") {
			allOrdinaryConditionsSkipped = false
		}
	}
	if allOrdinaryConditionsSkipped && p.Status != "skipped" {
		return ErrEventConflict
	}
	if p.Status == "skipped" && !allOrdinaryConditionsSkipped {
		return ErrEventConflict
	}
	postFailed := false
	for _, step := range steps {
		if step.Phase != "ordinary" && (step.Status == "failed" || step.Status == "cancelled") {
			postFailed = true
		}
	}
	if row.PostPhase == "" {
		return ErrEventConflict
	}
	if row.Reason == "timeout" && p.Status != "failed" {
		return ErrEventConflict
	}
	if p.Status == "succeeded" && (ordinaryFailed || ordinaryCancelled || postFailed || row.CancelRequested || p.Reason != "") {
		return ErrEventConflict
	}
	if p.Status == "cancelled" && (!row.CancelRequested && !ordinaryCancelled || ordinaryFailed) {
		return ErrEventConflict
	}
	if p.Status == "skipped" && (ordinaryStarted || ordinaryFailed || p.Reason != "condition") {
		return ErrEventConflict
	}
	if p.Status == "failed" && p.Reason == "" {
		return ErrEventConflict
	}
	if row.Reason != "" && row.Reason != p.Reason {
		return ErrEventConflict
	}
	row.Status = p.Status
	if row.Reason == "" {
		row.Reason = p.Reason
	}
	if cleanup {
		row.Status = "interrupted"
		row.StopUnconfirmed = true
		if row.Reason == "" {
			row.Reason = "cleanup_error"
		}
	}
	return nil
}
func (s *Store) ApplyEvent(ctx context.Context, actor NodeActor, in protocol.ExecutionEvent) (protocol.EventAck, error) {
	if !validRef(in.Ref) || in.Seq < 1 || !validHex(in.Digest, 64) || in.Digest != progressDigest(in.Progress) || !validProgress(in.Progress) {
		return protocol.EventAck{}, ErrInvalid
	}
	var ack protocol.EventAck
	err := s.write(ctx, func(tx *gorm.DB) error {
		row, err := s.currentExecution(tx, actor, in.Ref)
		if err != nil {
			return err
		}
		expires := *row.LeaseExpiresAt
		if in.Seq <= row.LastEventSeq {
			var receipt executionReceiptRecord
			if err = tx.First(&receipt, "build_id = ? AND attempt_id = ? AND seq = ?", row.ID, in.Ref.AttemptID, in.Seq).Error; err != nil {
				return err
			}
			if receipt.Digest != in.Digest {
				return ErrEventConflict
			}
			ack = protocol.EventAck{Seq: receipt.Seq, Digest: receipt.Digest}
			return checkBoundary(tx, actor, in.Ref, expires)
		}
		if row.LastEventSeq == math.MaxInt64 || in.Seq != row.LastEventSeq+1 {
			return ErrSequenceInvalid
		}
		p := in.Progress
		budgetBefore := row
		if !monotoneBudget(row.RemainingBudgetNS, p.RemainingBudgetNS) || p.RemainingPostBudgetNS > row.RemainingPostBudgetNS {
			return ErrBudgetInvalid
		}
		switch p.Kind {
		case "approval_checkpoint":
			err = applyApprovalCheckpoint(tx, &row, in)
		case "reports_checked":
			err = applyReportsChecked(tx, &row, p)
		case "reports_sealed":
			err = applyReportsSealed(tx, &row, p)
		case "post_selected":
			err = applyPostSelection(tx, &row, p)
		case "build_finished":
			err = applyTerminal(tx, &row, p)
		default:
			err = applyStep(tx, &row, p)
		}
		if err != nil {
			return err
		}
		row.RemainingBudgetNS = p.RemainingBudgetNS
		row.RemainingPostBudgetNS = p.RemainingPostBudgetNS
		row.LastEventSeq = in.Seq
		if p.Kind == "build_finished" && row.TerminalAt == nil {
			now := time.Now().UTC()
			row.TerminalAt = &now
		}
		if err = tx.Model(&row).Updates(map[string]any{"status": row.Status, "reason": row.Reason, "post_phase": row.PostPhase, "stop_unconfirmed": row.StopUnconfirmed, "remaining_budget_ns": row.RemainingBudgetNS, "remaining_post_budget_ns": row.RemainingPostBudgetNS, "last_event_seq": row.LastEventSeq, "terminal_at": row.TerminalAt}).Error; err != nil {
			return err
		}
		if err = tx.Create(&executionReceiptRecord{ID: uuid.NewString(), BuildID: row.ID, AttemptID: in.Ref.AttemptID, Seq: in.Seq, Digest: in.Digest, Kind: p.Kind, StopKnown: (p.Kind == "build_finished" || p.Kind == "approval_checkpoint") && p.StopConfirmed && !p.CleanupFailed && !row.StopUnconfirmed, CreatedAt: time.Now().UTC()}).Error; err != nil {
			return err
		}
		if p.Kind == "build_finished" {
			// 同原终态事务仅记录已登记完整归属的精确回执；不补猜旧资源或停止事实。
			if err = tx.Model(&nodeResourceRecord{}).Where("build_id = ? AND attempt_id = ? AND node_id = ? AND session_id = ? AND lease_id = ? AND epoch = ?", row.ID, in.Ref.AttemptID, in.Ref.NodeID, in.Ref.SessionID, in.Ref.LeaseID, in.Ref.Epoch).Updates(map[string]any{"terminal_seq": in.Seq, "terminal_digest": in.Digest}).Error; err != nil {
				return err
			}
		}
		ack = protocol.EventAck{Seq: in.Seq, Digest: in.Digest}
		if p.Kind == "reports_sealed" {
			if _, err = s.captureReportBudget(tx, budgetBefore); err != nil {
				return err
			}
		}
		return checkBoundary(tx, actor, in.Ref, expires)
	})
	if err != nil {
		return protocol.EventAck{}, err
	}
	return ack, nil
}
