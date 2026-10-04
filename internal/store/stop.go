package store

import (
	"context"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"mybuilds/internal/protocol"
)

func (s *Store) Cancel(ctx context.Context, actor Actor, buildID string) (BuildView, error) {
	if !validUUID(buildID) {
		return BuildView{}, ErrInvalid
	}
	var view BuildView
	err := s.write(ctx, func(tx *gorm.DB) error {
		if err := authorize(tx, actor, "admin"); err != nil {
			return err
		}
		var build buildRecord
		if err := tx.First(&build, "id = ?", buildID).Error; err != nil {
			return err
		}
		switch build.Status {
		case "queued":
			build.Status = "cancelled"
			build.Reason = "cancelled"
			build.CancelRequested = true
		case "running":
			build.CancelRequested = true
		case "cancelled":
		default:
			return ErrConflict
		}
		if err := tx.Model(&build).Updates(map[string]any{"status": build.Status, "reason": build.Reason, "cancel_requested": build.CancelRequested}).Error; err != nil {
			return err
		}
		if err := audit(tx, actor, "build_cancel", buildID, "", build.Status); err != nil {
			return err
		}
		var err error
		view, err = buildView(tx, build)
		return err
	})
	if err != nil {
		return BuildView{}, err
	}
	return view, nil
}
func (s *Store) ExpireLeases(ctx context.Context) error {
	return s.write(ctx, func(tx *gorm.DB) error {
		var builds []buildRecord
		if err := tx.Where("status = ? AND lease_expires_at <= ?", "running", time.Now().UTC()).Find(&builds).Error; err != nil {
			return err
		}
		for _, build := range builds {
			reason := build.Reason
			if reason == "" {
				reason = "lease_expired"
			}
			if err := tx.Model(&build).Where("status = ? AND lease_expires_at <= ?", "running", time.Now().UTC()).Updates(map[string]any{"status": "interrupted", "reason": reason, "stop_unconfirmed": true}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
func validStopConfirmation(in protocol.StopConfirmation, code string) bool {
	if !validRef(in.Ref) || in.EvidenceCode != code || len(in.Note) < 1 || len(in.Note) > 1024 || !utf8.ValidString(in.Note) {
		return false
	}
	for _, c := range in.Note {
		if unicode.IsControl(c) {
			return false
		}
	}
	return true
}
func confirmStopped(db *gorm.DB, in protocol.StopConfirmation, actorID string) error {
	var build buildRecord
	if err := db.First(&build, "id = ?", in.Ref.BuildID).Error; err != nil {
		return err
	}
	if buildRef(build) != in.Ref {
		return ErrLeaseInvalid
	}
	if build.Status != "interrupted" {
		return ErrStopUnconfirmed
	}
	var attempt attemptRecord
	if err := db.First(&attempt, "id = ? AND build_id = ?", in.Ref.AttemptID, in.Ref.BuildID).Error; err != nil {
		return err
	}
	if attempt.NodeID != in.Ref.NodeID || attempt.SessionID != in.Ref.SessionID || attempt.LeaseID != in.Ref.LeaseID || attempt.Epoch != in.Ref.Epoch {
		return ErrLeaseInvalid
	}
	if !build.StopUnconfirmed {
		var existing stopConfirmationRecord
		if err := db.First(&existing, "attempt_id = ?", in.Ref.AttemptID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return ErrStopUnconfirmed
			}
			return err
		}
		if existing.EvidenceCode != in.EvidenceCode || existing.Note != in.Note {
			return ErrConflict
		}
		return nil
	}
	record := stopConfirmationRecord{ID: uuid.NewString(), BuildID: in.Ref.BuildID, AttemptID: in.Ref.AttemptID, NodeID: in.Ref.NodeID, SessionID: in.Ref.SessionID, LeaseID: in.Ref.LeaseID, Epoch: in.Ref.Epoch, ActorID: actorID, EvidenceCode: in.EvidenceCode, Note: in.Note, CreatedAt: time.Now().UTC()}
	if err := db.Create(&record).Error; err != nil {
		return err
	}
	result := db.Model(&build).Where("status = ? AND stop_unconfirmed = ?", "interrupted", true).Update("stop_unconfirmed", false)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrStopUnconfirmed
	}
	return nil
}
func (s *Store) ConfirmStopped(ctx context.Context, actor Actor, in protocol.StopConfirmation) error {
	if !validStopConfirmation(in, "admin_observed_stopped") {
		return ErrInvalid
	}
	return s.write(ctx, func(tx *gorm.DB) error {
		if err := authorize(tx, actor, "admin"); err != nil {
			return err
		}
		if err := confirmStopped(tx, in, actor.ID); err != nil {
			return err
		}
		return authorize(tx, actor, "admin")
	})
}
func (s *Store) ConfirmNodeStopped(ctx context.Context, actor NodeActor, in protocol.StopConfirmation) error {
	if !validStopConfirmation(in, "process_group_reaped") {
		return ErrInvalid
	}
	return s.write(ctx, func(tx *gorm.DB) error {
		if _, _, err := authorizeNode(tx, actor); err != nil {
			return err
		}
		if actor.ID != in.Ref.NodeID {
			return ErrNodeUnauthorized
		}
		if err := confirmStopped(tx, in, actor.CredentialID); err != nil {
			return err
		}
		_, _, err := authorizeNode(tx, actor)
		return err
	})
}
