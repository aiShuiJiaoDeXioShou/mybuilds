package store

import (
	"context"
	"encoding/json"
	"gorm.io/gorm"
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
	result := BuildView{ID: row.ID, Project: project.Name, Group: project.Group.Name, BatchID: row.BatchID, Name: row.Name, Number: row.Number, Status: row.Status, Reason: row.Reason, SHA: batch.SHA, Branch: batch.Branch, Source: batch.Source, File: batch.File, SourceDigest: batch.SourceDigest, Condition: row.Condition, InitialBudgetNS: row.InitialBudgetNS, RemainingBudgetNS: row.RemainingBudgetNS, PostBudgetNS: row.PostBudgetNS, CreatedAt: row.CreatedAt.UTC(), Steps: []StepProgress{}, Post: []StepProgress{}}
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
		progress := StepProgress{Phase: step.Phase, Index: step.Index, Name: step.Name, Kind: step.Kind, Condition: step.Condition, Status: step.Status, ElapsedNS: step.ElapsedNS, Reasons: []string{}}
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
	if filter.Status != "" && filter.Status != "queued" && filter.Status != "skipped" {
		return nil, ErrInvalid
	}
	if err = s.CheckLock(ctx); err != nil {
		return nil, err
	}
	db := s.db.WithContext(ctx).Model(&buildRecord{}).Select("builds.*").Joins("JOIN projects ON projects.id = builds.project_id").Joins("JOIN project_groups ON project_groups.id = projects.group_id")
	for _, item := range []struct{ column, value string }{{"projects.name", filter.Project}, {"project_groups.name", filter.Group}, {"builds.name", filter.BuildName}, {"builds.batch_id", filter.BatchID}, {"builds.status", filter.Status}} {
		if item.value != "" {
			db = db.Where(item.column+" = ?", item.value)
		}
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
