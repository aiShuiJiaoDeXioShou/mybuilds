package store

import (
	"context"
	"encoding/json"
	"gorm.io/gorm"
	"mybuilds/internal/protocol"
	"slices"
)

func (s *Store) Status(ctx context.Context) (QueueStatus, error) {
	if err := s.CheckLock(ctx); err != nil {
		return QueueStatus{}, err
	}
	var result QueueStatus
	db := s.db.WithContext(ctx)
	if err := db.Model(&projectRecord{}).Count(&result.Projects).Error; err != nil {
		return QueueStatus{}, safeError(err)
	}
	if err := db.Model(&buildRecord{}).Where("status = ?", "queued").Count(&result.Queued).Error; err != nil {
		return QueueStatus{}, safeError(err)
	}
	if err := db.Model(&buildRecord{}).Where("status = ?", "skipped").Count(&result.Skipped).Error; err != nil {
		return QueueStatus{}, safeError(err)
	}
	for _, pair := range []struct {
		status string
		dest   *int64
	}{{"running", &result.Running}, {"interrupted", &result.Interrupted}} {
		if err := db.Model(&buildRecord{}).Where("status = ?", pair.status).Count(pair.dest).Error; err != nil {
			return QueueStatus{}, safeError(err)
		}
	}
	var nodes []nodeRecord
	if err := db.Where("state <> ?", "deleted").Find(&nodes).Error; err != nil {
		return QueueStatus{}, safeError(err)
	}
	result.Nodes = int64(len(nodes))
	for _, node := range nodes {
		view, err := nodeView(db, node)
		if err != nil {
			return QueueStatus{}, safeError(err)
		}
		if view.State == "enabled" && view.Healthy && !view.Quarantined {
			result.HealthyNodes++
		}
	}
	return result, nil
}

func batchView(db *gorm.DB, id string) (BatchResult, error) {
	var batch batchRecord
	if err := db.First(&batch, "id = ?", id).Error; err != nil {
		return BatchResult{}, err
	}
	var rows []buildRecord
	if err := db.Where("batch_id = ?", id).Order("position ASC").Find(&rows).Error; err != nil {
		return BatchResult{}, err
	}
	result := BatchResult{ID: batch.ID, SHA: batch.SHA, Builds: make([]BuildView, 0, len(rows))}
	for _, row := range rows {
		view, err := buildView(db, row)
		if err != nil {
			return BatchResult{}, err
		}
		result.Builds = append(result.Builds, view)
	}
	return result, nil
}
func buildView(db *gorm.DB, row buildRecord) (BuildView, error) {
	var project projectRecord
	if err := db.Preload("Group").First(&project, "id = ?", row.ProjectID).Error; err != nil {
		return BuildView{}, err
	}
	var batch batchRecord
	if err := db.First(&batch, "id = ?", row.BatchID).Error; err != nil {
		return BuildView{}, err
	}
	result := BuildView{HistoryState: row.HistoryState, TerminalAt: row.TerminalAt, CleanedAt: row.CleanedAt, ID: row.ID, Project: project.Name, Group: project.Group.Name, BatchID: row.BatchID, Name: row.Name, Number: row.Number, Status: row.Status, Reason: row.Reason, SHA: batch.SHA, Branch: batch.Branch, Source: batch.Source, File: batch.File, SourceDigest: batch.SourceDigest, Condition: row.Condition, InitialBudgetNS: row.InitialBudgetNS, RemainingBudgetNS: row.RemainingBudgetNS, PostBudgetNS: row.PostBudgetNS, CreatedAt: row.CreatedAt.UTC(), Steps: []StepProgress{}, Post: []StepProgress{}}
	// 只关联已经持久化的发布意图，未授权构建不生成假记录。
	if err := db.Model(&publishIntentRecord{}).Where("build_id = ?", row.ID).Order("created_at,id").Pluck("id", &result.PublishIDs).Error; err != nil {
		return BuildView{}, err
	}
	// 完整清理后仅投影仍存在的身份与结果，不再解释已删除的大JSON或报告。
	if row.HistoryState == "cleaned" {
		minimal := BuildView{HistoryState: row.HistoryState, TerminalAt: row.TerminalAt, CleanedAt: row.CleanedAt, ID: row.ID, Project: project.Name, Group: project.Group.Name, BatchID: row.BatchID, Name: row.Name, Number: row.Number, Status: row.Status, Reason: row.Reason, SHA: batch.SHA, CreatedAt: row.CreatedAt.UTC(), ParameterKeys: []string{}, Reasons: []string{}, Steps: []StepProgress{}, Post: []StepProgress{}}
		minimal.PublishIDs = result.PublishIDs
		if row.RetryOf != nil {
			minimal.RetryOf = *row.RetryOf
		}
		return minimal, nil
	}
	var snapshot BuildSnapshot
	if json.Unmarshal([]byte(row.SnapshotJSON), &snapshot) != nil || !validateOrigin(snapshot.Origin, snapshot.Definition) || !originBatchMatches(snapshot.Origin, batch.SHA, batch.Source, batch.File) {
		return BuildView{}, errDatabase
	}
	result.Origin = snapshot.Origin
	sealed, err := sealedReports(row)
	if err != nil {
		return BuildView{}, err
	}
	if sealed != nil {
		result.Reports, result.ReportSealDigest = sealed, row.ReportSealDigest
	}
	if row.RetryOf != nil {
		result.RetryOf = *row.RetryOf
	}
	if row.CurrentApprovalID != nil {
		var a approvalRecord
		if err := db.First(&a, "id = ?", *row.CurrentApprovalID).Error; err != nil {
			return BuildView{}, err
		}
		result.CurrentApprovalID = a.ID
		result.ApprovalRevision = a.Revision
	}
	result.ResumeReason = row.ResumeReason
	result.NodeID = buildRef(row).NodeID
	result.SessionID = buildRef(row).SessionID
	result.AttemptID = buildRef(row).AttemptID
	result.LeaseID = buildRef(row).LeaseID
	result.LeaseEpoch = row.LeaseEpoch
	result.CancelRequested = row.CancelRequested
	result.StopUnconfirmed = row.StopUnconfirmed
	result.RemainingPostBudgetNS = row.RemainingPostBudgetNS
	result.PostPhase = row.PostPhase
	if row.Status == "running" && row.CancelRequested {
		result.Status = "cancel_requested"
	}
	if row.NodeID != nil {
		var node nodeRecord
		if err := db.First(&node, "id = ?", *row.NodeID).Error; err != nil {
			return BuildView{}, err
		}
		result.NodeName = node.Name
	}
	if err := json.Unmarshal([]byte(row.ParameterKeysJSON), &result.ParameterKeys); err != nil {
		return BuildView{}, errDatabase
	}
	if err := json.Unmarshal([]byte(row.ReasonsJSON), &result.Reasons); err != nil {
		return BuildView{}, errDatabase
	}
	if result.Reasons == nil {
		result.Reasons = []string{}
	}
	var steps []stepRecord
	if err := db.Where("build_id = ?", row.ID).Order("CASE phase WHEN 'ordinary' THEN 0 WHEN 'success' THEN 1 WHEN 'failure' THEN 2 ELSE 3 END ASC").Order("\"index\" ASC").Find(&steps).Error; err != nil {
		return BuildView{}, err
	}
	for _, step := range steps {
		progress := StepProgress{Phase: step.Phase, Index: step.Index, Name: step.Name, Kind: step.Kind, Condition: step.Condition, Status: step.Status, ElapsedNS: step.ElapsedNS, Reasons: []string{}, Intent: step.Intent, Started: step.Started, StopConfirmed: step.StopConfirmed, CleanupFailed: step.CleanupFailed, Reason: step.Reason, ExitCode: step.ExitCode}
		if err := json.Unmarshal([]byte(step.ReasonsJSON), &progress.Reasons); err != nil {
			return BuildView{}, errDatabase
		}
		if progress.Reasons == nil {
			progress.Reasons = []string{}
		}
		if step.Phase == "ordinary" {
			result.Steps = append(result.Steps, progress)
		} else {
			result.Post = append(result.Post, progress)
		}
	}
	return result, nil
}

func sealedReports(row buildRecord) (*protocol.ReportEvidence, error) {
	if row.ReportSealDigest == "" {
		return nil, nil
	}
	evidence, err := storedReports(row)
	if err != nil {
		return nil, err
	}
	if evidence == nil || !row.ReportFinal || !evidence.Sealed {
		return nil, errDatabase
	}
	data, err := encode(*evidence)
	if err != nil || reportKey(data) != row.ReportSealDigest {
		return nil, errDatabase
	}
	return evidence, nil
}
func (s *Store) GetBuild(ctx context.Context, id string) (BuildView, error) {
	if !validID(id) {
		return BuildView{}, ErrInvalid
	}
	if err := s.CheckLock(ctx); err != nil {
		return BuildView{}, err
	}
	db := s.db.WithContext(ctx)
	var row buildRecord
	if err := db.First(&row, "id = ?", id).Error; err != nil {
		return BuildView{}, safeError(err)
	}
	view, err := buildView(db, row)
	return view, safeError(err)
}
func (s *Store) ListBuilds(ctx context.Context, filter BuildFilter) ([]BuildView, error) {
	page, err := normalizePage(filter.Page)
	if err != nil {
		return nil, err
	}
	for _, name := range []string{filter.Project, filter.Group, filter.BuildName} {
		if name != "" && !validName(name) {
			return nil, ErrInvalid
		}
	}
	if filter.BatchID != "" && !validID(filter.BatchID) {
		return nil, ErrInvalid
	}
	if filter.Status != "" && !slices.Contains([]string{"queued", "skipped", "running", "cancel_requested", "succeeded", "failed", "cancelled", "interrupted"}, filter.Status) {
		return nil, ErrInvalid
	}
	if err = s.CheckLock(ctx); err != nil {
		return nil, err
	}
	db := s.db.WithContext(ctx).Model(&buildRecord{}).Select("builds.*").Joins("JOIN projects ON projects.id = builds.project_id").Joins("JOIN project_groups ON project_groups.id = projects.group_id")
	for _, item := range []struct{ column, value string }{{"projects.name", filter.Project}, {"project_groups.name", filter.Group}, {"builds.name", filter.BuildName}, {"builds.batch_id", filter.BatchID}} {
		if item.value != "" {
			db = db.Where(item.column+" = ?", item.value)
		}
	}
	if filter.Status == "cancel_requested" {
		db = db.Where("builds.status = ? AND builds.cancel_requested = ?", "running", true)
	} else if filter.Status != "" {
		db = db.Where("builds.status = ?", filter.Status)
	}
	var rows []buildRecord
	if err = db.Order("builds.created_at DESC, builds.id DESC").Limit(page.Limit).Offset(page.Offset).Find(&rows).Error; err != nil {
		return nil, safeError(err)
	}
	result := make([]BuildView, 0, len(rows))
	for _, row := range rows {
		view, err := buildView(s.db.WithContext(ctx), row)
		if err != nil {
			return nil, safeError(err)
		}
		result = append(result, view)
	}
	return result, nil
}
