package store

import (
	"context"
	"errors"
	"math"
	"time"

	"gorm.io/gorm"
	"mybuilds/internal/config"
)

// SyncGlobalRetention只由持控制独占的serve启动消费；同值不刷新版本或审计。
func (s *Store) SyncGlobalRetention(ctx context.Context, policy config.Retention) error {
	if config.ValidateRetention(policy) != nil {
		return ErrRetentionInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := s.write(ctx, func(tx *gorm.DB) error {
		var row retentionPolicyRecord
		err := tx.First(&row, 1).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			row = retentionPolicyRecord{ID: 1, Builds: policy.Builds, Days: policy.Days, Version: 1, ChangedAt: time.Now().UTC()}
			if err = tx.Create(&row).Error; err != nil {
				return err
			}
		} else {
			if err != nil {
				return err
			}
			if config.ValidateRetention(config.Retention{Builds: row.Builds, Days: row.Days}) != nil || row.Version < 1 {
				return errDatabase
			}
			if row.Builds == policy.Builds && row.Days == policy.Days {
				return nil
			}
			if row.Version == math.MaxInt64 {
				return ErrConflict
			}
			if err = tx.Model(&row).Updates(map[string]any{"builds": policy.Builds, "days": policy.Days, "version": row.Version + 1, "changed_at": time.Now().UTC()}).Error; err != nil {
				return err
			}
		}
		// 无用户身份的实际控制启动审计，不伪造管理员token。
		return audit(tx, Actor{}, "retention_global", "global", "", "")
	})
	return retentionPolicyError(ctx, err)
}

// EffectiveRetention每次查询合并当前全局与项目覆盖，不把启动值缓存成永久策略。
func (s *Store) EffectiveRetention(ctx context.Context, actor Actor, projectID string) (EffectiveRetention, error) {
	if !validUUID(projectID) {
		return EffectiveRetention{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var view EffectiveRetention
	err := s.write(ctx, func(tx *gorm.DB) error {
		if err := authorize(tx, actor, "admin"); err != nil {
			return err
		}
		var err error
		view, err = effectiveRetention(tx, projectID)
		return err
	})
	if err != nil {
		return EffectiveRetention{}, retentionPolicyError(ctx, err)
	}
	return view, nil
}

func retentionPolicyError(ctx context.Context, err error) error {
	if err == nil || err == ErrLockLost || err == ErrLocked {
		return err
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ErrRetentionTimeout
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return ErrRetentionCancelled
	}
	return err
}

func effectiveRetention(tx *gorm.DB, projectID string) (EffectiveRetention, error) {
	var project projectRecord
	if err := tx.First(&project, "id = ?", projectID).Error; err != nil {
		return EffectiveRetention{}, err
	}
	var policy retentionPolicyRecord
	if err := tx.First(&policy, 1).Error; err != nil {
		return EffectiveRetention{}, err
	}
	if config.ValidateRetention(config.Retention{Builds: policy.Builds, Days: policy.Days}) != nil || policy.Version < 1 || project.PolicyVersion < 1 {
		return EffectiveRetention{}, errDatabase
	}
	settings, err := projectView(project)
	if err != nil {
		return EffectiveRetention{}, err
	}
	if config.ValidateRetentionOverride(settings.Settings.Retention) != nil {
		return EffectiveRetention{}, errDatabase
	}
	view := EffectiveRetention{Builds: policy.Builds, Days: policy.Days, BuildsSource: "global", DaysSource: "global", GlobalVersion: policy.Version, ProjectVersion: project.PolicyVersion}
	if override := settings.Settings.Retention; override != nil {
		if override.Builds != nil {
			view.Builds, view.BuildsSource = *override.Builds, "project"
		}
		if override.Days != nil {
			view.Days, view.DaysSource = *override.Days, "project"
		}
	}
	return view, nil
}
