package store

import (
	"context"
	"encoding/json"
	"math"
	"net"
	"net/url"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"mybuilds/internal/config"
)

func validList(values []string, max int, check func(string) bool) bool {
	if len(values) == 0 || len(values) > max {
		return false
	}
	seen := make(map[string]bool, len(values))
	for _, v := range values {
		if seen[v] || !check(v) {
			return false
		}
		seen[v] = true
	}
	return true
}
func validBranchPattern(value string) bool {
	if value == "" || len(value) > 256 || strings.HasPrefix(value, "-") || strings.Contains(value, "..") || strings.Contains(value, "//") || strings.Contains(value, "@{") || strings.HasSuffix(value, ".lock") || strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") {
		return false
	}
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsControl(r) || strings.ContainsRune("\\~^:", r) {
			return false
		}
	}
	_, err := path.Match(value, "main")
	return err == nil
}
func validRepository(repo string) bool {
	if repo == "" || len(repo) > 4096 || strings.TrimSpace(repo) != repo || strings.HasPrefix(repo, "-") {
		return false
	}
	for _, r := range repo {
		if unicode.IsControl(r) {
			return false
		}
	}
	if filepath.IsAbs(repo) {
		return true
	}
	if !strings.Contains(repo, "://") {
		return strings.Contains(repo, "@") && strings.Contains(repo, ":") && !strings.ContainsAny(repo, " \t")
	}
	u, err := url.Parse(repo)
	if err != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	if u.User != nil {
		if _, has := u.User.Password(); has || u.Scheme != "ssh" {
			return false
		}
	}
	switch u.Scheme {
	case "https", "ssh":
		return u.Host != "" && u.Path != ""
	case "http":
		host := u.Hostname()
		ip := net.ParseIP(host)
		return (host == "localhost" || (ip != nil && ip.IsLoopback())) && u.Path != ""
	case "file":
		return (u.Host == "" || u.Host == "localhost") && filepath.IsAbs(u.Path)
	}
	return false
}
func normalizeProject(input ProjectInput) (ProjectInput, error) {
	if input.Group == "" {
		input.Group = "default"
	}
	if input.Provider == "" {
		input.Provider = "generic"
	}
	if len(input.Branches) == 0 {
		input.Branches = []string{"main"}
	}
	if input.BuildNumberStart == 0 {
		input.BuildNumberStart = 1
	}
	if !validName(input.Name) || !validName(input.Group) || !slices.Contains([]string{"generic", "github", "gitlab", "gitee"}, input.Provider) || !validRepository(input.Repository) || input.BuildNumberStart < 1 || !validList(input.AllowedNodes, 128, validName) || !validList(input.Branches, 128, validBranchPattern) {
		return input, ErrInvalid
	}
	if input.DefaultNode != "" && !slices.Contains(input.AllowedNodes, input.DefaultNode) {
		return input, ErrInvalid
	}
	if err := config.ValidateProjectSettings(input.Settings); err != nil {
		return input, ErrInvalid
	}
	return input, nil
}
func encode(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", ErrInvalid
	}
	return string(data), nil
}
func projectView(row projectRecord) (Project, error) {
	p := Project{ID: row.ID, Name: row.Name, GroupID: row.GroupID, GroupName: row.Group.Name, Repository: row.Repository, Provider: row.Provider, DefaultNode: row.DefaultNode, NextNumber: row.NextNumber, PolicyVersion: row.PolicyVersion, CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC()}
	for _, item := range []struct {
		text   string
		target any
	}{{row.BranchesJSON, &p.Branches}, {row.NodesJSON, &p.AllowedNodes}, {row.SettingsJSON, &p.Settings}} {
		if err := json.Unmarshal([]byte(item.text), item.target); err != nil {
			return Project{}, errDatabase
		}
	}
	return p, nil
}
func (s *Store) CreateProject(ctx context.Context, actor Actor, input ProjectInput) (Project, error) {
	input, err := normalizeProject(input)
	if err != nil {
		return Project{}, err
	}
	branches, _ := encode(input.Branches)
	nodes, _ := encode(input.AllowedNodes)
	settings, err := encode(input.Settings)
	if err != nil {
		return Project{}, err
	}
	row := projectRecord{ID: uuid.NewString(), Name: input.Name, Repository: input.Repository, Provider: input.Provider, DefaultNode: input.DefaultNode, BranchesJSON: branches, NodesJSON: nodes, SettingsJSON: settings, NextNumber: input.BuildNumberStart, PolicyVersion: 1}
	err = s.write(ctx, func(tx *gorm.DB) error {
		if err := authorize(tx, actor, "admin"); err != nil {
			return err
		}
		var group groupRecord
		if err := tx.First(&group, "name = ?", input.Group).Error; err != nil {
			return err
		}
		row.GroupID = group.ID
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		row.Group = group
		return audit(tx, actor, "project_create", row.ID, "", group.ID)
	})
	if err != nil {
		return Project{}, err
	}
	return projectView(row)
}
func (s *Store) GetProject(ctx context.Context, name string) (Project, error) {
	if !validName(name) {
		return Project{}, ErrInvalid
	}
	if err := s.CheckLock(ctx); err != nil {
		return Project{}, err
	}
	var row projectRecord
	if err := s.db.WithContext(ctx).Preload("Group").First(&row, "name = ?", name).Error; err != nil {
		return Project{}, safeError(err)
	}
	return projectView(row)
}
func (s *Store) ListProjects(ctx context.Context, filter ProjectFilter) ([]Project, error) {
	page, err := normalizePage(filter.Page)
	if err != nil {
		return nil, err
	}
	if filter.Group != "" && !validName(filter.Group) {
		return nil, ErrInvalid
	}
	if err = s.CheckLock(ctx); err != nil {
		return nil, err
	}
	db := s.db.WithContext(ctx).Model(&projectRecord{}).Preload("Group")
	if filter.Group != "" {
		db = db.Joins("JOIN project_groups AS current_group ON current_group.id = projects.group_id").Where("current_group.name = ?", filter.Group)
	}
	var rows []projectRecord
	if err = db.Order("projects.created_at DESC, projects.id DESC").Limit(page.Limit).Offset(page.Offset).Find(&rows).Error; err != nil {
		return nil, safeError(err)
	}
	result := make([]Project, 0, len(rows))
	for _, row := range rows {
		view, err := projectView(row)
		if err != nil {
			return nil, err
		}
		result = append(result, view)
	}
	return result, nil
}
func (s *Store) SetProjectSettings(ctx context.Context, actor Actor, name string, settings config.ProjectSettings) (Project, error) {
	if !validName(name) || config.ValidateProjectSettings(settings) != nil {
		return Project{}, ErrInvalid
	}
	var row projectRecord
	err := s.write(ctx, func(tx *gorm.DB) error {
		if err := authorize(tx, actor, "admin"); err != nil {
			return err
		}
		if err := tx.Preload("Group").First(&row, "name = ?", name).Error; err != nil {
			return err
		}
		if settings.Pipeline == nil || settings.Notifications == nil {
			var previous config.ProjectSettings
			if json.Unmarshal([]byte(row.SettingsJSON), &previous) != nil {
				return errDatabase
			}
			if settings.Pipeline == nil {
				settings.Pipeline = previous.Pipeline
			}
			if settings.Notifications == nil {
				settings.Notifications = previous.Notifications
			}
		}
		if config.ValidateProjectSettings(settings) != nil {
			return ErrInvalid
		}
		encoded, err := encode(settings)
		if err != nil {
			return err
		}
		if row.PolicyVersion == math.MaxInt64 {
			return ErrConflict
		}
		row.SettingsJSON = encoded
		row.PolicyVersion++
		row.UpdatedAt = time.Now().UTC()
		if err := tx.Model(&projectRecord{}).Where("id = ?", row.ID).Updates(map[string]any{"settings_json": encoded, "policy_version": row.PolicyVersion, "updated_at": row.UpdatedAt}).Error; err != nil {
			return err
		}
		return audit(tx, actor, "project_settings", row.ID, "", "")
	})
	if err != nil {
		return Project{}, err
	}
	return projectView(row)
}
func (s *Store) MoveProject(ctx context.Context, actor Actor, name, groupName string) (Project, error) {
	if !validName(name) || !validName(groupName) {
		return Project{}, ErrInvalid
	}
	var row projectRecord
	err := s.write(ctx, func(tx *gorm.DB) error {
		if err := authorize(tx, actor, "admin"); err != nil {
			return err
		}
		if err := tx.Preload("Group").First(&row, "name = ?", name).Error; err != nil {
			return err
		}
		var group groupRecord
		if err := tx.First(&group, "name = ?", groupName).Error; err != nil {
			return err
		}
		if row.GroupID == group.ID {
			return nil
		}
		if row.PolicyVersion == math.MaxInt64 {
			return ErrConflict
		}
		previous := row.GroupID
		row.GroupID = group.ID
		row.Group = group
		row.PolicyVersion++
		row.UpdatedAt = time.Now().UTC()
		if err := tx.Model(&projectRecord{}).Where("id = ?", row.ID).Updates(map[string]any{"group_id": row.GroupID, "policy_version": row.PolicyVersion, "updated_at": row.UpdatedAt}).Error; err != nil {
			return err
		}
		return audit(tx, actor, "project_move", row.ID, previous, group.ID)
	})
	if err != nil {
		return Project{}, err
	}
	return projectView(row)
}
func (s *Store) DeleteProject(ctx context.Context, actor Actor, name string) error {
	if !validName(name) {
		return ErrInvalid
	}
	return s.write(ctx, func(tx *gorm.DB) error {
		if err := authorize(tx, actor, "admin"); err != nil {
			return err
		}
		var row projectRecord
		if err := tx.First(&row, "name = ?", name).Error; err != nil {
			return err
		}
		if err := tx.Delete(&row).Error; err != nil {
			return err
		}
		return audit(tx, actor, "project_delete", row.ID, "", "")
	})
}
