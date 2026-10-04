package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"path"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/bmatcuk/doublestar/v4"
	"gorm.io/gorm"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"mybuilds/internal/reports"
)

func configuredReport(row buildRecord) (*config.JUnitReport, error) {
	var snapshot BuildSnapshot
	if json.Unmarshal([]byte(row.SnapshotJSON), &snapshot) != nil {
		return nil, errDatabase
	}
	if snapshot.Definition.Reports == nil {
		return nil, nil
	}
	if snapshot.Definition.Reports.JUnit == nil {
		return nil, errDatabase
	}
	return snapshot.Definition.Reports.JUnit, nil
}
func reportsPatterns(tx *gorm.DB, row buildRecord, cfg *config.JUnitReport) ([]string, error) {
	task, err := taskSnapshot(tx, row)
	if err != nil {
		return nil, err
	}
	var node nodeRecord
	if row.NodeID == nil || tx.First(&node, "id = ?", *row.NodeID).Error != nil {
		return nil, errDatabase
	}
	facts := map[string]string{"project": task.Project, "build.name": row.Name, "build.id": row.ID, "build.number": strconv.FormatInt(task.Number, 10), "git.sha": task.SHA, "git.branch": task.Branch, "node.name": node.Name}
	result := make([]string, 0, len(cfg.Paths))
	for _, pattern := range cfg.Paths {
		value, missing, e := config.RenderField(pattern, "build.reports.junit.paths", task.Parameters, facts, false, false)
		if e != nil || missing {
			return nil, ErrEventConflict
		}
		result = append(result, value)
	}
	return result, nil
}
func storedReports(row buildRecord) (*protocol.ReportEvidence, error) {
	if row.ReportRevision == 0 {
		if row.ReportsJSON != "" || row.ReportFinal || row.ReportSealDigest != "" {
			return nil, errDatabase
		}
		return nil, nil
	}
	var e protocol.ReportEvidence
	if json.Unmarshal([]byte(row.ReportsJSON), &e) != nil || e.Revision != row.ReportRevision || e.Files == nil || e.Diagnostics == nil {
		return nil, errDatabase
	}
	return &e, nil
}
func reportPath(value string) bool {
	if value == "" || len(value) > 1024 || !utf8.ValidString(value) || path.IsAbs(value) || strings.ContainsAny(value, "\\") || len(value) > 1 && value[1] == ':' || strings.ContainsFunc(value, unicode.IsControl) || path.Clean(value) != value || !validArtifactName(path.Base(value)) {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == ".." {
			return false
		}
	}
	return true
}
func reportKey(value string) string {
	h := sha256.Sum256([]byte(value))
	return hex.EncodeToString(h[:])
}
func reportCounts(c protocol.JUnitCounts) bool {
	_, err := reports.MergeJUnit([]protocol.JUnitResult{{Counts: c, Diagnostics: []protocol.JUnitDiagnostic{}}})
	return err == nil
}
func validReportEvidence(e *protocol.ReportEvidence, required bool) bool {
	if e == nil || e.Files == nil || e.Diagnostics == nil || e.Revision < 1 || e.Required != required || len(e.Files) > 64 || len(e.Diagnostics) > 20 || !slices.Contains([]string{"pending", "passed", "failed", "missing"}, e.Outcome) || !slices.Contains([]string{"", "report_failed", "report_invalid", "report_missing", "report_secret", "report_error", "timeout"}, e.Reason) || (e.Outcome == "failed") != (e.Reason != "") {
		return false
	}
	if !reportCounts(e.Counts) {
		return false
	}
	ids := map[string]bool{}
	keys := map[string]bool{}
	counts := []protocol.JUnitResult{}
	var size int64
	last := ""
	for _, file := range e.Files {
		if !reportPath(file.Path) || file.Path <= last || file.Key != reportKey(file.Path) || !validUUID(file.ArtifactID) || ids[file.ArtifactID] || keys[file.Key] || !validDigest(file.SHA256) || file.Size < 1 || file.Size > 8<<20 || file.SourceIndex < 1 || !validName(file.SourceStep) || !reportCounts(file.Counts) {
			return false
		}
		size += file.Size
		if size > 64<<20 {
			return false
		}
		last = file.Path
		ids[file.ArtifactID] = true
		keys[file.Key] = true
		counts = append(counts, protocol.JUnitResult{Counts: file.Counts, Diagnostics: []protocol.JUnitDiagnostic{}})
	}
	merged, err := reports.MergeJUnit(counts)
	if err != nil || merged.Counts != e.Counts {
		return false
	}
	used := 0
	for _, d := range e.Diagnostics {
		if !keys[d.PathKey] {
			return false
		}
		used += len(d.PathKey) + len(d.Case) + len(d.Outcome) + len(d.Message)
	}
	if used > 20<<10 {
		return false
	}
	if _, err = reports.MergeJUnit([]protocol.JUnitResult{{Counts: e.Counts, Diagnostics: e.Diagnostics}}); err != nil {
		return false
	}
	if int64(len(e.Diagnostics)) > e.Counts.Failures+e.Counts.Errors {
		return false
	}
	return true
}
func reportEventShape(p protocol.ExecutionProgress) bool {
	return p.Reports != nil && p.ReportManifest == nil && len(p.LocalReports) == 0 && !p.Started && !p.StopConfirmed && !p.CleanupFailed && p.Status == "" && p.Reason == "" && p.PostPhase == "" && len(p.ArtifactIDs) == 0 && p.LastLogSeq == 0 && p.LastLogOffset == 0 && p.LastArtifactSeq == 0 && len(p.ArtifactSteps) == 0 && p.ExitCode == -1
}
func reportSources(steps []stepRecord, e protocol.ReportEvidence) bool {
	for _, f := range e.Files {
		found := false
		for _, step := range steps {
			if step.Phase == "ordinary" && step.Index == f.SourceIndex && step.Name == f.SourceStep && step.Kind == "run" && terminalStep(step.Status) && step.Started && step.StopConfirmed && !step.CleanupFailed {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func reportOutcome(e protocol.ReportEvidence, final bool, patterns []string) bool {
	for _, f := range e.Files {
		matched := false
		for _, pattern := range patterns {
			ok, err := doublestar.Match(pattern, f.Path)
			if err != nil {
				return false
			}
			matched = matched || ok
		}
		if !matched {
			return false
		}
	}
	missing := false
	for _, pattern := range patterns {
		matched := false
		for _, f := range e.Files {
			ok, err := doublestar.Match(pattern, f.Path)
			if err != nil {
				return false
			}
			matched = matched || ok
		}
		missing = missing || !matched
	}
	// 文件错误可能保留其余合法诊断；这些固定失败码不能伪装成功。
	if e.Reason == "report_invalid" || e.Reason == "report_secret" || e.Reason == "report_error" || e.Reason == "timeout" {
		return e.Outcome == "failed"
	}
	if e.Counts.Failures+e.Counts.Errors > 0 {
		return e.Outcome == "failed" && e.Reason == "report_failed"
	}
	if final && missing && e.Required {
		return e.Outcome == "failed" && e.Reason == "report_missing"
	}
	if final && !e.Required && len(e.Files) == 0 {
		return e.Outcome == "missing" && e.Reason == ""
	}
	if !final {
		return e.Outcome == "pending" && e.Reason == ""
	}
	return e.Outcome == "passed" && e.Reason == ""
}
func saveReports(tx *gorm.DB, row *buildRecord, e protocol.ReportEvidence) error {
	encoded, err := encode(e)
	if err != nil {
		return err
	}
	row.ReportsJSON = encoded
	row.ReportRevision = e.Revision
	return tx.Model(row).Updates(map[string]any{"report_revision": row.ReportRevision, "report_final": row.ReportFinal, "reports_json": row.ReportsJSON, "report_seal_digest": row.ReportSealDigest, "report_checked_index": row.ReportCheckedIndex}).Error
}
func applyReportsChecked(tx *gorm.DB, row *buildRecord, p protocol.ExecutionProgress) error {
	cfg, err := configuredReport(*row)
	if err != nil {
		return err
	}
	if cfg == nil || !reportEventShape(p) || row.ReportFinal || row.PostPhase != "" || row.ReportRevision == math.MaxInt64 {
		return ErrEventConflict
	}
	required := cfg.Required == nil || *cfg.Required
	if !validReportEvidence(p.Reports, required) || p.Reports.Sealed || p.Reports.Revision != row.ReportRevision+1 {
		return ErrEventConflict
	}
	steps, err := loadSteps(tx, row.ID)
	if err != nil {
		return err
	}
	final := p.Index == 0
	if final {
		if p.Phase != "" || p.Name != "" || p.StepKind != "" {
			return ErrEventConflict
		}
		complete, started, _, _ := ordinarySummary(steps)
		if !complete || !started {
			return ErrEventConflict
		}
		for _, step := range steps {
			if step.Phase == "ordinary" && (!step.StopConfirmed || step.CleanupFailed) {
				return ErrEventConflict
			}
			if step.Phase == "ordinary" && step.Kind == "run" && step.Started && step.Index > row.ReportCheckedIndex {
				return ErrEventConflict
			}
		}
	} else {
		if p.Phase != "ordinary" || p.StepKind != "run" || p.Index <= row.ReportCheckedIndex {
			return ErrEventConflict
		}
		found := false
		for _, step := range steps {
			if step.Phase != "ordinary" {
				continue
			}
			if step.Index == p.Index {
				found = step.Name == p.Name && step.Kind == "run" && terminalStep(step.Status) && step.Started && step.StopConfirmed && !step.CleanupFailed
			}
			if step.Index > p.Index && (step.Intent || step.Started) {
				return ErrEventConflict
			}
			if step.Index < p.Index && (!terminalStep(step.Status) || !step.StopConfirmed || step.CleanupFailed || step.Kind == "run" && step.Started && step.Index > row.ReportCheckedIndex) {
				return ErrEventConflict
			}
		}
		if !found {
			return ErrEventConflict
		}
	}
	if !reportSources(steps, *p.Reports) {
		return ErrEventConflict
	}
	previous, err := storedReports(*row)
	if err != nil {
		return err
	}
	if previous != nil {
		oldIDs := map[string]protocol.ReportFile{}
		for _, f := range previous.Files {
			oldIDs[f.ArtifactID] = f
		}
		for _, f := range p.Reports.Files {
			if old, ok := oldIDs[f.ArtifactID]; ok && old != f {
				return ErrEventConflict
			}
		}
	}
	patterns, err := reportsPatterns(tx, *row, cfg)
	if err != nil {
		return err
	}
	if !reportOutcome(*p.Reports, final, patterns) {
		return ErrEventConflict
	}
	row.ReportFinal = final
	if !final {
		row.ReportCheckedIndex = p.Index
	}
	return saveReports(tx, row, *p.Reports)
}
func validateReportIntent(tx *gorm.DB, row *buildRecord, p protocol.ExecutionProgress) error {
	if p.Kind != "intent" || p.Phase != "ordinary" {
		return nil
	}
	cfg, err := configuredReport(*row)
	if err != nil || cfg == nil {
		return err
	}
	if row.ReportFinal {
		return ErrEventConflict
	}
	evidence, err := storedReports(*row)
	if err != nil {
		return err
	}
	if evidence != nil && evidence.Outcome == "failed" {
		return ErrEventConflict
	}
	steps, err := loadSteps(tx, row.ID)
	if err != nil {
		return err
	}
	for _, step := range steps {
		if step.Phase == "ordinary" && step.Index < p.Index && step.Kind == "run" && step.Started && step.StopConfirmed && !step.CleanupFailed && row.ReportCheckedIndex < step.Index {
			return ErrEventConflict
		}
	}
	return nil
}
func validateReportPost(tx *gorm.DB, row *buildRecord, p protocol.ExecutionProgress) error {
	cfg, err := configuredReport(*row)
	if err != nil || cfg == nil {
		return err
	}
	steps, err := loadSteps(tx, row.ID)
	if err != nil {
		return err
	}
	_, started, _, _ := ordinarySummary(steps)
	if !started {
		if row.ReportRevision != 0 || p.Reports != nil || p.ReportManifest != nil {
			return ErrEventConflict
		}
		return nil
	}
	evidence, err := storedReports(*row)
	if err != nil {
		return err
	}
	if evidence == nil || !row.ReportFinal || !evidence.Sealed || !validDigest(row.ReportSealDigest) || evidence.Reason == "report_error" || evidence.Reason == "timeout" {
		return ErrEventConflict
	}
	if evidence.Outcome == "failed" && row.Reason == "" {
		row.Reason = evidence.Reason
	}
	return nil
}

// reportBudgetDeadline只使用最后final检查的控制端receipt，不信节点At。
func reportBudgetDeadline(tx *gorm.DB, row buildRecord) (*time.Time, error) {
	evidence, err := storedReports(row)
	if err != nil {
		return nil, err
	}
	if evidence == nil || !row.ReportFinal || evidence.Sealed || row.ReportSealDigest != "" || row.AttemptID == nil {
		return nil, ErrEventConflict
	}
	var receipt executionReceiptRecord
	if err = tx.First(&receipt, "build_id = ? AND attempt_id = ? AND seq = ?", row.ID, *row.AttemptID, row.LastEventSeq).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrEventConflict
		}
		return nil, err
	}
	now := time.Now().UTC()
	if receipt.Kind != "reports_checked" || receipt.CreatedAt.IsZero() || receipt.CreatedAt.After(now) {
		return nil, ErrEventConflict
	}
	if row.RemainingBudgetNS == nil {
		return nil, nil
	}
	if *row.RemainingBudgetNS <= 0 {
		return nil, ErrBudgetInvalid
	}
	deadline := receipt.CreatedAt.Add(time.Duration(*row.RemainingBudgetNS))
	if !now.Before(deadline) {
		return nil, ErrBudgetInvalid
	}
	return &deadline, nil
}
func remainingReportBudget(tx *gorm.DB, row buildRecord) (*int64, error) {
	deadline, err := reportBudgetDeadline(tx, row)
	if err != nil || deadline == nil {
		return nil, err
	}
	remaining := time.Until(*deadline).Nanoseconds()
	if remaining <= 0 {
		return nil, ErrBudgetInvalid
	}
	return &remaining, nil
}

// captureReportBudget让最后控制锁查询也受同一个绝对普通deadline约束。
func (s *Store) captureReportBudget(tx *gorm.DB, row buildRecord) (*int64, error) {
	deadline, err := reportBudgetDeadline(tx, row)
	if err != nil || deadline == nil {
		return nil, err
	}
	if s.transactionReportDeadline == nil || deadline.Before(*s.transactionReportDeadline) {
		s.transactionReportDeadline = deadline
	}
	remaining := time.Until(*deadline).Nanoseconds()
	if remaining <= 0 {
		return nil, ErrBudgetInvalid
	}
	return &remaining, nil
}

// ReportUploadBudget只返回当前final检查的共同控制端时钟锚，不按文件刷新预算。
func (s *Store) ReportUploadBudget(ctx context.Context, actor NodeActor, ref protocol.LeaseRef) (*int64, error) {
	if !validRef(ref) {
		return nil, ErrInvalid
	}
	var deadline *time.Time
	err := s.write(ctx, func(tx *gorm.DB) error {
		row, err := s.currentExecution(tx, actor, ref)
		if err != nil {
			return err
		}
		if _, err = s.captureReportBudget(tx, row); err != nil {
			return err
		}
		if s.transactionReportDeadline != nil {
			copy := *s.transactionReportDeadline
			deadline = &copy
		}
		return checkBoundary(tx, actor, ref, *row.LeaseExpiresAt)
	})
	if err != nil || deadline == nil {
		return nil, err
	}
	remaining := time.Until(*deadline).Nanoseconds()
	if remaining <= 0 {
		return nil, ErrBudgetInvalid
	}
	return &remaining, nil
}
func validateJUnitArtifact(tx *gorm.DB, row buildRecord, in ArtifactCommit) (string, error) {
	d := in.Declaration
	evidence, err := storedReports(row)
	if err != nil {
		return "", err
	}
	if evidence == nil || !row.ReportFinal || evidence.Sealed || d.Purpose != "junit" || d.ReportRevision != row.ReportRevision || d.Phase != "ordinary" || in.VerifiedJUnit == nil || in.VerifiedJUnit.Diagnostics == nil || d.Size < 1 || d.Size > 8<<20 {
		return "", ErrArtifactConflict
	}
	var expected *protocol.ReportFile
	for i := range evidence.Files {
		if evidence.Files[i].ArtifactID == d.ID {
			expected = &evidence.Files[i]
			break
		}
	}
	if expected == nil || expected.Key != d.ReportKey || expected.SourceIndex != d.Index || expected.SourceStep != d.Step || path.Base(expected.Path) != d.Name || expected.Size != d.Size || expected.SHA256 != d.SHA256 || expected.Counts != in.VerifiedJUnit.Counts {
		return "", ErrArtifactConflict
	}
	for _, diag := range in.VerifiedJUnit.Diagnostics {
		if diag.PathKey != "" {
			return "", ErrArtifactConflict
		}
	}
	verified, err := reports.MergeJUnit([]protocol.JUnitResult{*in.VerifiedJUnit})
	if err != nil || !reflect.DeepEqual(verified, *in.VerifiedJUnit) {
		return "", ErrArtifactConflict
	}
	var totals struct{ Count, Size int64 }
	if err = tx.Model(&artifactRecord{}).Select("COUNT(*) AS count, COALESCE(SUM(size),0) AS size").Where("attempt_id = ? AND purpose = ?", *row.AttemptID, "junit").Scan(&totals).Error; err != nil {
		return "", err
	}
	if totals.Count >= 64 || totals.Size > (64<<20)-d.Size {
		return "", ErrArtifactConflict
	}
	if _, err = remainingReportBudget(tx, row); err != nil {
		return "", err
	}
	return encode(*in.VerifiedJUnit)
}
func applyReportsSealed(tx *gorm.DB, row *buildRecord, p protocol.ExecutionProgress) error {
	budgetBefore := *row
	if p.RemainingBudgetNS != nil && *p.RemainingBudgetNS == 0 {
		return ErrBudgetInvalid
	}
	if !reportEventShape(p) || p.Phase != "" || p.Index != 0 || p.Name != "" || p.StepKind != "" || row.PostPhase != "" || !row.ReportFinal || row.ReportSealDigest != "" {
		return ErrEventConflict
	}
	previous, err := storedReports(*row)
	if err != nil {
		return err
	}
	if previous == nil || previous.Sealed || !p.Reports.Sealed || p.Reports.Files == nil || p.Reports.Diagnostics == nil {
		return ErrEventConflict
	}
	candidate := *p.Reports
	candidate.Sealed = false
	if !reflect.DeepEqual(candidate, *previous) {
		return ErrEventConflict
	}
	files := []artifactRecord{}
	if err = tx.Where("build_id = ? AND attempt_id = ? AND purpose = ?", row.ID, *row.AttemptID, "junit").Find(&files).Error; err != nil {
		return err
	}
	if len(files) != len(previous.Files) {
		return ErrEventConflict
	}
	byID := map[string]artifactRecord{}
	for _, f := range files {
		byID[f.ID] = f
	}
	inputs := make([]protocol.JUnitResult, 0, len(previous.Files))
	for _, f := range previous.Files {
		file, ok := byID[f.ArtifactID]
		if !ok || file.ReportRevision != row.ReportRevision || file.ReportKey != f.Key || file.Phase != "ordinary" || file.Index != f.SourceIndex || file.Step != f.SourceStep || file.Name != path.Base(f.Path) || file.Size != f.Size || file.SHA256 != f.SHA256 {
			return ErrEventConflict
		}
		var parsed protocol.JUnitResult
		if json.Unmarshal([]byte(file.VerifiedJUnitJSON), &parsed) != nil || parsed.Diagnostics == nil || parsed.Counts != f.Counts {
			return ErrEventConflict
		}
		for i := range parsed.Diagnostics {
			if parsed.Diagnostics[i].PathKey != "" {
				return ErrEventConflict
			}
			parsed.Diagnostics[i].PathKey = f.Key
		}
		inputs = append(inputs, parsed)
	}
	merged, err := reports.MergeJUnit(inputs)
	if err != nil || merged.Counts != previous.Counts || !reflect.DeepEqual(merged.Diagnostics, previous.Diagnostics) {
		return ErrEventConflict
	}
	if _, err = remainingReportBudget(tx, *row); err != nil {
		return err
	}
	data, err := encode(*p.Reports)
	if err != nil {
		return err
	}
	row.ReportSealDigest = reportKey(data)
	if err = saveReports(tx, row, *p.Reports); err != nil {
		return err
	}
	_, err = remainingReportBudget(tx, budgetBefore)
	return err
}
func validateReportManifest(tx *gorm.DB, row buildRecord, p protocol.ExecutionProgress) error {
	cfg, err := configuredReport(row)
	if err != nil {
		return err
	}
	if cfg == nil {
		if p.ReportManifest != nil || p.Reports != nil {
			return ErrEventConflict
		}
		return nil
	}
	steps, err := loadSteps(tx, row.ID)
	if err != nil {
		return err
	}
	_, started, _, _ := ordinarySummary(steps)
	if !started {
		if row.ReportRevision != 0 || p.ReportManifest != nil || p.Reports != nil {
			return ErrEventConflict
		}
		return nil
	}
	evidence, err := storedReports(row)
	if err != nil {
		return err
	}
	m := p.ReportManifest
	if evidence == nil || !row.ReportFinal || !evidence.Sealed || p.Reports != nil || m == nil || m.IDs == nil || m.SealDigest != row.ReportSealDigest || !validDigest(m.SealDigest) || len(m.IDs) != len(evidence.Files) || !validateArtifactIDs(m.IDs) {
		return ErrEventConflict
	}
	var files []artifactRecord
	if err = tx.Where("attempt_id = ? AND purpose = ?", *row.AttemptID, "junit").Find(&files).Error; err != nil {
		return err
	}
	if len(files) != len(evidence.Files) {
		return ErrEventConflict
	}
	for _, f := range evidence.Files {
		if !slices.Contains(m.IDs, f.ArtifactID) {
			return ErrEventConflict
		}
	}
	return nil
}
