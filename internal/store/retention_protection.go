package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"path"
	"reflect"
	"slices"

	"gorm.io/gorm"
	"mybuilds/internal/protocol"
	"mybuilds/internal/reports"
)

var retentionProtectionOrder = []string{"active", "terminal_time_unknown", "stop_unconfirmed", "execution_unconfirmed", "resource_unconfirmed", "logs_unconfirmed", "artifacts_unconfirmed", "reports_unconfirmed", "retry_dependency", "readers_active", "ownership_unknown", "state_unknown"}

// 保护只核已持久的具体证据；文件读写、PID解释与停止确认不属于此事务。
func retentionProtection(tx *gorm.DB, row buildRecord) ([]string, error) {
	dependencies, err := retentionRetryProtection(tx, row.ProjectID)
	if err != nil {
		return nil, err
	}
	return retentionProtectionWithRetries(tx, row, dependencies[row.ID] || dependencies[""])
}

func retentionProtectionWithRetries(tx *gorm.DB, row buildRecord, retryDependency bool) ([]string, error) {
	guard := map[string]bool{}
	if row.Status == "queued" || row.Status == "running" {
		guard["active"] = true
	} else if !retentionTerminal(row.Status) {
		guard["state_unknown"] = true
	}
	if !slices.Contains([]string{"live", "retiring", "partial", "cleaned"}, row.HistoryState) {
		guard["state_unknown"] = true
	}
	if row.HistoryState == "cleaned" {
		return []string{}, nil
	}
	if retentionTerminal(row.Status) && (row.TerminalAt == nil || row.TerminalAt.IsZero()) {
		guard["terminal_time_unknown"] = true
	}
	if row.StopUnconfirmed {
		guard["stop_unconfirmed"] = true
	}
	guard["retry_dependency"] = retryDependency
	steps := []stepRecord{}
	if err := tx.Where("build_id = ?", row.ID).Limit(retentionRelationLimit + 1).Find(&steps).Error; err != nil {
		return nil, err
	}
	if len(steps) > retentionRelationLimit {
		guard["execution_unconfirmed"] = true
		return orderedRetentionReasons(guard), nil
	}
	if err := validateRecoveryBuild(tx, row); err != nil {
		if !errors.Is(err, errDatabase) {
			return nil, err
		}
		guard["ownership_unknown"] = true
		guard["execution_unconfirmed"] = true
	}
	var terminal *executionReceiptRecord
	independentStop := false
	stopCode := ""
	if row.AttemptID != nil {
		var receipt executionReceiptRecord
		if err := tx.First(&receipt, "build_id = ? AND attempt_id = ? AND seq = ?", row.ID, *row.AttemptID, row.LastEventSeq).Error; err == nil {
			if receipt.Kind == "build_finished" && receipt.StopKnown && validDigest(receipt.Digest) {
				terminal = &receipt
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		if row.Status == "interrupted" && !row.StopUnconfirmed {
			var stop stopConfirmationRecord
			err := tx.First(&stop, "attempt_id = ?", *row.AttemptID).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			}
			ref := buildRef(row)
			stopCode = stop.EvidenceCode
			independentStop = err == nil && stop.BuildID == ref.BuildID && stop.AttemptID == ref.AttemptID && stop.NodeID == ref.NodeID && stop.SessionID == ref.SessionID && stop.LeaseID == ref.LeaseID && stop.Epoch == ref.Epoch && slices.Contains([]string{"admin_observed_stopped", "process_group_reaped"}, stop.EvidenceCode)
		}
		if terminal == nil && !independentStop {
			guard["execution_unconfirmed"] = true
		}
		if !independentStop {
			for _, step := range steps {
				if !terminalStep(step.Status) || !step.StopConfirmed || step.CleanupFailed {
					guard["execution_unconfirmed"] = true
				}
			}
		}

		var receipts struct{ Count, First, Last, Invalid int64 }
		if err := tx.Model(&executionReceiptRecord{}).Select("COUNT(*) AS count, COALESCE(MIN(seq),0) AS first, COALESCE(MAX(seq),0) AS last, COALESCE(SUM(CASE WHEN attempt_id <> ? OR kind NOT IN ? OR length(digest) <> 64 THEN 1 ELSE 0 END),0) AS invalid", *row.AttemptID, []string{"intent", "started", "finished", "skipped", "post_selected", "build_finished", "reports_checked", "reports_sealed"}).Where("build_id = ?", row.ID).Scan(&receipts).Error; err != nil {
			return nil, err
		}
		if row.LastEventSeq < 0 || receipts.Count != row.LastEventSeq || receipts.Last != row.LastEventSeq || receipts.Count > 0 && receipts.First != 1 || receipts.Invalid != 0 {
			guard["execution_unconfirmed"] = true
		}
		var resource nodeResourceRecord
		err := tx.First(&resource, "attempt_id = ?", *row.AttemptID).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		ref := buildRef(row)
		if err != nil || resource.BuildID != row.ID || resource.NodeID != ref.NodeID || resource.SessionID != ref.SessionID || resource.LeaseID != ref.LeaseID || resource.Epoch != ref.Epoch || !validDigest(resource.OwnershipDigest) || !resource.HasWorkspace {
			guard["resource_unconfirmed"] = true
			guard["ownership_unknown"] = true
		} else {
			started := false
			for _, step := range steps {
				started = started || step.Started || step.Intent
			}
			if started && !resource.HasResults {
				guard["resource_unconfirmed"] = true
			}
			if terminal != nil {
				if resource.TerminalSeq != terminal.Seq || resource.TerminalDigest != terminal.Digest {
					guard["resource_unconfirmed"] = true
				}
			} else if !independentStop || !retentionResourceCompleted(row, resource, stopCode) {
				// 独立Stop只证明物理停止；中央必须另有精确游标的真实资源完成确认。
				guard["resource_unconfirmed"] = true
			}
		}
	} else {
		for _, step := range steps {
			if step.Intent || step.Started || step.CleanupFailed {
				guard["execution_unconfirmed"] = true
			}
		}
		if !(row.Status == "queued" || row.Status == "skipped" || row.Status == "cancelled" && row.CancelRequested) {
			guard["execution_unconfirmed"] = true
		}
	}
	logs := []logChunkRecord{}
	if err := tx.Where("build_id = ?", row.ID).Order("seq").Limit(retentionRelationLimit + 1).Find(&logs).Error; err != nil {
		return nil, err
	}
	offset := int64(0)
	if len(logs) > retentionRelationLimit || int64(len(logs)) != row.LastLogSeq {
		guard["logs_unconfirmed"] = true
	}
	for i, log := range logs {
		if row.AttemptID == nil || log.AttemptID != *row.AttemptID || log.Seq != int64(i+1) || log.Offset != offset || log.Size < 1 || log.Size > 64*1024 || offset > math.MaxInt64-log.Size || !validDigest(log.Digest) || !validUUID(log.StorageID) {
			guard["logs_unconfirmed"] = true
			break
		}
		offset += log.Size
	}
	if offset != row.LastLogOffset {
		guard["logs_unconfirmed"] = true
	}
	files := []artifactRecord{}
	if err := tx.Where("build_id = ?", row.ID).Order("seq").Limit(retentionRelationLimit + 1).Find(&files).Error; err != nil {
		return nil, err
	}
	if len(files) > retentionRelationLimit || int64(len(files)) != row.LastArtifactSeq {
		guard["artifacts_unconfirmed"] = true
	}
	byID := map[string]artifactRecord{}
	for i, file := range files {
		if row.AttemptID == nil || file.AttemptID != *row.AttemptID || file.Seq != int64(i+1) || !validUUID(file.ID) || !validUUID(file.StorageID) || !validDigest(file.SHA256) || file.Size < 0 || !slices.Contains([]string{"", "artifact", "junit"}, file.Purpose) {
			guard["artifacts_unconfirmed"] = true
		}
		byID[file.ID] = file
	}
	expected := map[string]bool{}
	for _, step := range steps {
		if step.Kind != "artifact" {
			continue
		}
		ids := []string{}
		if step.ArtifactIDsJSON != "" && json.Unmarshal([]byte(step.ArtifactIDsJSON), &ids) != nil {
			guard["artifacts_unconfirmed"] = true
			continue
		}
		if !validateArtifactIDs(ids) {
			guard["artifacts_unconfirmed"] = true
			continue
		}
		for _, id := range ids {
			file, ok := byID[id]
			if !ok || expected[id] || file.Purpose == "junit" || file.Phase != step.Phase || file.Index != step.Index || file.Step != step.Name {
				guard["artifacts_unconfirmed"] = true
			}
			expected[id] = true
		}
	}
	for _, file := range files {
		if file.Purpose != "junit" && !expected[file.ID] {
			guard["artifacts_unconfirmed"] = true
		}
	}
	if !retentionReportsConfirmed(row, steps, files) {
		guard["reports_unconfirmed"] = true
	}
	var readers int64
	if err := tx.Model(&evidenceReadRecord{}).Where("build_id = ? AND state <> ?", row.ID, "closed").Count(&readers).Error; err != nil {
		return nil, err
	}
	guard["readers_active"] = readers > 0
	return orderedRetentionReasons(guard), nil
}

func orderedRetentionReasons(guards map[string]bool) []string {
	out := []string{}
	for _, reason := range retentionProtectionOrder {
		if guards[reason] {
			out = append(out, reason)
		}
	}
	return out
}

func retentionReportsConfirmed(row buildRecord, steps []stepRecord, files []artifactRecord) bool {
	if len(row.SnapshotJSON) > 1<<20 {
		return false
	}
	cfg, err := configuredReport(row)
	if err != nil {
		return false
	}
	ordinaryStarted := false
	for _, step := range steps {
		ordinaryStarted = ordinaryStarted || step.Phase == "ordinary" && step.Started
	}
	// 合法零动作不需要虚构seal；artifact普通动作可封存缺失结果，但不伪造XML来源。
	if cfg == nil || !ordinaryStarted {
		if row.ReportRevision != 0 || row.ReportFinal || row.ReportsJSON != "" || row.ReportSealDigest != "" {
			return false
		}
		for _, f := range files {
			if f.Purpose == "junit" {
				return false
			}
		}
		return true
	}
	evidence, err := sealedReports(row)
	if err != nil || evidence == nil || evidence.Outcome == "pending" || evidence.Reason == "report_error" || evidence.Reason == "timeout" || !validReportEvidence(evidence, cfg.Required == nil || *cfg.Required) || !reportSources(steps, *evidence) {
		return false
	}
	byID := map[string]artifactRecord{}
	for _, f := range files {
		if f.Purpose == "junit" {
			byID[f.ID] = f
		}
	}
	if len(byID) != len(evidence.Files) {
		return false
	}
	inputs := make([]protocol.JUnitResult, 0, len(evidence.Files))
	for _, f := range evidence.Files {
		file, ok := byID[f.ArtifactID]
		if !ok || file.ReportRevision != row.ReportRevision || file.ReportKey != f.Key || file.Phase != "ordinary" || file.Index != f.SourceIndex || file.Step != f.SourceStep || file.Name != path.Base(f.Path) || file.Size != f.Size || file.SHA256 != f.SHA256 {
			return false
		}
		var parsed protocol.JUnitResult
		if json.Unmarshal([]byte(file.VerifiedJUnitJSON), &parsed) != nil || parsed.Diagnostics == nil || parsed.Counts != f.Counts {
			return false
		}
		for i := range parsed.Diagnostics {
			if parsed.Diagnostics[i].PathKey != "" {
				return false
			}
			parsed.Diagnostics[i].PathKey = f.Key
		}
		inputs = append(inputs, parsed)
	}
	merged, err := reports.MergeJUnit(inputs)
	return err == nil && merged.Counts == evidence.Counts && reflect.DeepEqual(merged.Diagnostics, evidence.Diagnostics)
}

// 完成事实只包含固定游标和reaped代码；拒绝缺失、重复、未知、null及非整数值。
func retentionResourceCompleted(row buildRecord, resource nodeResourceRecord, stopCode string) bool {
	if resource.CompletedAt == nil || resource.CompletedAt.IsZero() || stopCode != "process_group_reaped" || len(resource.CompletionJSON) > 1024 {
		return false
	}
	decoder := json.NewDecoder(bytes.NewBufferString(resource.CompletionJSON))
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return false
	}
	seen := map[string]bool{}
	var completion protocol.NodeResourceCompletion
	for decoder.More() {
		token, err = decoder.Token()
		field, ok := token.(string)
		if err != nil || !ok || seen[field] {
			return false
		}
		seen[field] = true
		token, err = decoder.Token()
		if err != nil {
			return false
		}
		if field == "stop_code" {
			value, ok := token.(string)
			if !ok || value != "process_group_reaped" {
				return false
			}
			completion.StopCode = value
			continue
		}
		value, ok := token.(json.Number)
		if !ok {
			return false
		}
		number, err := value.Int64()
		if err != nil || number < 0 {
			return false
		}
		switch field {
		case "last_event_seq":
			completion.LastEventSeq = number
		case "last_log_seq":
			completion.LastLogSeq = number
		case "last_log_offset":
			completion.LastLogOffset = number
		case "last_artifact_seq":
			completion.LastArtifactSeq = number
		default:
			return false
		}
	}
	token, err = decoder.Token()
	if err != nil || token != json.Delim('}') || len(seen) != 5 {
		return false
	}
	if _, err = decoder.Token(); err != io.EOF {
		return false
	}
	return completion.StopCode == stopCode && completion.LastEventSeq == row.LastEventSeq && completion.LastLogSeq == row.LastLogSeq && completion.LastLogOffset == row.LastLogOffset && completion.LastArtifactSeq == row.LastArtifactSeq
}
