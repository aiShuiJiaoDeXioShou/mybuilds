package store

import (
	"context"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"strings"
	"time"
	"unicode"
)

func validName(name string) bool {
	if name == "" || len(name) > 64 || name == "." || name == ".." || strings.TrimSpace(name) != name || strings.ContainsAny(name, "/\\") {
		return false
	}
	for _, c := range name {
		if unicode.IsControl(c) {
			return false
		}
	}
	return true
}
func validID(id string) bool {
	return id != "" && len(id) <= 128 && !strings.ContainsAny(id, "/\\\x00")
}
func normalizePage(page Page) (Page, error) {
	if page.Limit == 0 {
		page.Limit = 20
	}
	if page.Limit < 1 || page.Limit > 200 || page.Offset < 0 || page.Offset > 1000000 {
		return Page{}, ErrInvalid
	}
	return page, nil
}
func audit(tx *gorm.DB, actor Actor, action, object, from, to string) error {
	return tx.Create(&auditRecord{ID: uuid.NewString(), ActorID: actor.ID, Action: action, ObjectID: object, From: from, To: to, CreatedAt: time.Now().UTC()}).Error
}
func groupView(row groupRecord) Group {
	return Group{ID: row.ID, Name: row.Name, CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC()}
}
func (s *Store) CreateGroup(ctx context.Context, actor Actor, name string) (Group, error) {
	if !validName(name) {
		return Group{}, ErrInvalid
	}
	row := groupRecord{ID: uuid.NewString(), Name: name}
	err := s.write(ctx, func(tx *gorm.DB) error {
		if err := authorize(tx, actor, "admin"); err != nil {
			return err
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		return audit(tx, actor, "group_create", row.ID, "", name)
	})
	if err != nil {
		return Group{}, err
	}
	return groupView(row), nil
}
func (s *Store) ListGroups(ctx context.Context, page Page) ([]Group, error) {
	page, err := normalizePage(page)
	if err != nil {
		return nil, err
	}
	if err = s.CheckLock(ctx); err != nil {
		return nil, err
	}
	var rows []groupRecord
	if err = s.db.WithContext(ctx).Order("created_at DESC, id DESC").Limit(page.Limit).Offset(page.Offset).Find(&rows).Error; err != nil {
		return nil, safeError(err)
	}
	result := make([]Group, 0, len(rows))
	for _, row := range rows {
		result = append(result, groupView(row))
	}
	return result, nil
}
func (s *Store) RenameGroup(ctx context.Context, actor Actor, name, newName string) (Group, error) {
	if !validName(name) || !validName(newName) {
		return Group{}, ErrInvalid
	}
	var row groupRecord
	err := s.write(ctx, func(tx *gorm.DB) error {
		if err := authorize(tx, actor, "admin"); err != nil {
			return err
		}
		if name == "default" {
			return ErrForbidden
		}
		if err := tx.First(&row, "name = ?", name).Error; err != nil {
			return err
		}
		if name == newName {
			return nil
		}
		if err := tx.Model(&row).Updates(map[string]any{"name": newName, "updated_at": time.Now().UTC()}).Error; err != nil {
			return err
		}
		return audit(tx, actor, "group_rename", row.ID, name, newName)
	})
	if err != nil {
		return Group{}, err
	}
	return groupView(row), nil
}
func (s *Store) DeleteGroup(ctx context.Context, actor Actor, name string) error {
	if !validName(name) {
		return ErrInvalid
	}
	return s.write(ctx, func(tx *gorm.DB) error {
		if err := authorize(tx, actor, "admin"); err != nil {
			return err
		}
		if name == "default" {
			return ErrForbidden
		}
		var row groupRecord
		if err := tx.First(&row, "name = ?", name).Error; err != nil {
			return err
		}
		if err := tx.Delete(&row).Error; err != nil {
			return err
		}
		return audit(tx, actor, "group_delete", row.ID, name, "")
	})
}
