package store

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// 启动时绑定真实独占实例；同一Store不会因重复New或请求刷新owner。
func (s *Store) RegisterEvidenceReadOwner(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := s.write(ctx, func(tx *gorm.DB) error {
		var metadata metadataRecord
		if err := tx.First(&metadata, "id = ?", 1).Error; err != nil {
			return err
		}
		if !validUUID(s.evidenceReadOwner) {
			return ErrConflict
		}
		if metadata.EvidenceReadOwner == s.evidenceReadOwner {
			return nil
		}
		return tx.Model(&metadata).Update("evidence_read_owner", s.evidenceReadOwner).Error
	})
	if err != nil {
		return "", retentionPolicyError(ctx, err)
	}
	return s.evidenceReadOwner, nil
}

func (s *Store) checkEvidenceReadOwner(tx *gorm.DB, owner string) error {
	var metadata metadataRecord
	if err := tx.First(&metadata, "id = ?", 1).Error; err != nil {
		return err
	}
	if owner != s.evidenceReadOwner || metadata.EvidenceReadOwner != owner || !validUUID(owner) {
		return ErrConflict
	}
	return nil
}

func (s *Store) BeginEvidenceRead(ctx context.Context, actor Actor, kind, objectID, owner string) (EvidenceRead, error) {
	if !validUUID(objectID) || !validUUID(owner) || kind != "log" && kind != "artifact" {
		return EvidenceRead{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var out EvidenceRead
	err := s.write(ctx, func(tx *gorm.DB) error {
		if err := authorize(tx, actor, "admin", "approver"); err != nil {
			return err
		}
		if err := s.checkEvidenceReadOwner(tx, owner); err != nil {
			return err
		}
		out = EvidenceRead{ID: uuid.NewString(), Kind: kind, ObjectID: objectID}
		var artifact artifactRecord
		if kind == "log" {
			var log logChunkRecord
			if err := tx.First(&log, "id = ?", objectID).Error; err != nil {
				return err
			}
			out.BuildID, out.StorageID, out.Size, out.SHA256 = log.BuildID, log.StorageID, log.Size, log.Digest
		} else {
			if err := tx.First(&artifact, "id = ?", objectID).Error; err != nil {
				return err
			}
			out.BuildID, out.StorageID, out.Size, out.SHA256 = artifact.BuildID, artifact.StorageID, artifact.Size, artifact.SHA256
		}
		var build buildRecord
		if err := tx.First(&build, "id = ?", out.BuildID).Error; err != nil {
			return err
		}
		if err := evidenceReadBuildState(build); err != nil {
			return err
		}
		if kind == "artifact" && artifact.Purpose == "junit" {
			sealed, err := sealedReports(build)
			if err != nil {
				return err
			}
			if sealed == nil || artifact.ReportRevision != build.ReportRevision {
				return ErrNotFound
			}
		}
		if !validUUID(out.StorageID) || !validDigest(out.SHA256) || out.Size < 0 || kind == "log" && out.Size == 0 {
			return ErrRetentionOwnershipUnknown
		}
		row := evidenceReadRecord{ID: out.ID, BuildID: out.BuildID, Kind: kind, ObjectID: objectID, StorageID: out.StorageID, Owner: owner, State: "pending", CreatedAt: time.Now().UTC()}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		return authorize(tx, actor, "admin", "approver")
	})
	if err != nil {
		return EvidenceRead{}, retentionPolicyError(ctx, err)
	}
	return out, nil
}

func evidenceReadBuildState(build buildRecord) error {
	if build.HistoryState != "live" {
		return ErrRetentionRetired
	}
	return nil
}

func (s *Store) ActivateEvidenceRead(ctx context.Context, id, identity string) error {
	if !validUUID(id) || !validRetentionIdentity(identity) {
		return ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := s.write(ctx, func(tx *gorm.DB) error {
		var row evidenceReadRecord
		if err := tx.First(&row, "id = ?", id).Error; err != nil {
			return err
		}
		if err := s.checkEvidenceReadOwner(tx, row.Owner); err != nil {
			return err
		}
		var build buildRecord
		if err := tx.First(&build, "id = ?", row.BuildID).Error; err != nil {
			return err
		}
		if err := evidenceReadBuildState(build); err != nil {
			return err
		}
		if row.State == "active" && row.Identity == identity {
			return nil
		}
		if row.State != "pending" || row.Identity != "" || row.ClosedAt != nil {
			return ErrConflict
		}
		return tx.Model(&row).Updates(map[string]any{"state": "active", "identity": identity}).Error
	})
	return retentionPolicyError(ctx, err)
}

// 仅原实例关闭自己的登记；新实例须以真实EX同inode观察修复旧active。
func (s *Store) EndEvidenceRead(ctx context.Context, id string) error {
	if !validUUID(id) {
		return ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := s.write(ctx, func(tx *gorm.DB) error {
		var row evidenceReadRecord
		if err := tx.First(&row, "id = ?", id).Error; err != nil {
			return err
		}
		if err := s.checkEvidenceReadOwner(tx, row.Owner); err != nil {
			return err
		}
		if row.State == "closed" && row.ClosedAt != nil {
			return nil
		}
		if row.State != "pending" && row.State != "active" || row.ClosedAt != nil {
			return ErrConflict
		}
		return tx.Model(&row).Updates(map[string]any{"state": "closed", "closed_at": time.Now().UTC()}).Error
	})
	return retentionPolicyError(ctx, err)
}

func validRetentionIdentity(identity string) bool {
	parts := strings.Split(identity, ":")
	if len(identity) > 160 || len(parts) != 6 || parts[0] != "v1" || parts[1] != "darwin" && parts[1] != "linux" || !validHex(parts[2], 16) || !validHex(parts[3], 16) || parts[2] != strings.ToLower(parts[2]) || parts[3] != strings.ToLower(parts[3]) || len(parts[5]) != 9 {
		return false
	}
	seconds, err := strconv.ParseInt(parts[4], 10, 64)
	if err != nil || strconv.FormatInt(seconds, 10) != parts[4] {
		return false
	}
	for _, c := range parts[5] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
