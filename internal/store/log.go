package store

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"mybuilds/internal/protocol"
)

func validDigest(digest string) bool {
	return validHex(digest, 64) && digest == strings.ToLower(digest)
}
func logAck(row logChunkRecord) protocol.LogAck {
	return protocol.LogAck{Seq: row.Seq, NextOffset: row.Offset + row.Size, Digest: row.Digest}
}
func (s *Store) CommitLogChunk(ctx context.Context, actor NodeActor, in LogCommit) (LogCommitted, error) {
	if !validRef(in.Ref) || in.Seq < 1 || in.Offset < 0 || in.Size < 1 || in.Size > 64*1024 || in.Offset > math.MaxInt64-in.Size || in.RecordCount < 1 || in.RecordCount > 16 || !validDigest(in.Digest) || !validUUID(in.StorageID) {
		return LogCommitted{}, ErrInvalid
	}
	var committed LogCommitted
	err := s.write(ctx, func(tx *gorm.DB) error {
		build, err := s.currentExecution(tx, actor, in.Ref)
		if err != nil {
			return err
		}
		expires := *build.LeaseExpiresAt
		if in.Seq <= build.LastLogSeq {
			var original logChunkRecord
			if err = tx.First(&original, "build_id = ? AND attempt_id = ? AND seq = ?", build.ID, in.Ref.AttemptID, in.Seq).Error; err != nil {
				return err
			}
			if original.Digest != in.Digest || original.Offset != in.Offset || original.Size != in.Size || original.RecordCount != in.RecordCount {
				return ErrLogConflict
			}
			committed = LogCommitted{Ack: logAck(original), StorageID: original.StorageID}
			return checkBoundary(tx, actor, in.Ref, expires)
		}
		if build.LastLogSeq == math.MaxInt64 || in.Seq != build.LastLogSeq+1 || in.Offset != build.LastLogOffset {
			return ErrSequenceInvalid
		}
		row := logChunkRecord{ID: uuid.NewString(), BuildID: build.ID, AttemptID: in.Ref.AttemptID, Seq: in.Seq, Offset: in.Offset, Size: in.Size, Digest: in.Digest, StorageID: in.StorageID, RecordCount: in.RecordCount, CreatedAt: time.Now().UTC()}
		if err = tx.Create(&row).Error; err != nil {
			return err
		}
		if err = tx.Model(&build).Updates(map[string]any{"last_log_seq": in.Seq, "last_log_offset": in.Offset + in.Size}).Error; err != nil {
			return err
		}
		committed = LogCommitted{Ack: logAck(row), StorageID: row.StorageID, Created: true}
		return checkBoundary(tx, actor, in.Ref, expires)
	})
	if err != nil {
		return LogCommitted{}, err
	}
	return committed, nil
}
func (s *Store) ListLogChunks(ctx context.Context, actor Actor, buildID string, afterSeq int64, page Page) ([]LogStored, error) {
	page, err := normalizePage(page)
	if err != nil || !validUUID(buildID) || afterSeq < 0 {
		return nil, ErrInvalid
	}
	if err = s.CheckLock(ctx); err != nil {
		return nil, err
	}
	db := s.db.WithContext(ctx)
	if err = authorize(db, actor, "admin", "approver"); err != nil {
		return nil, safeError(err)
	}
	var build buildRecord
	if err = db.First(&build, "id = ?", buildID).Error; err != nil {
		return nil, safeError(err)
	}
	var rows []logChunkRecord
	if err = db.Where("build_id = ? AND seq > ?", buildID, afterSeq).Order("seq ASC").Limit(page.Limit).Offset(page.Offset).Find(&rows).Error; err != nil {
		return nil, safeError(err)
	}
	result := make([]LogStored, 0, len(rows))
	for _, row := range rows {
		result = append(result, LogStored{BuildID: row.BuildID, AttemptID: row.AttemptID, Seq: row.Seq, Offset: row.Offset, Size: row.Size, Digest: row.Digest, StorageID: row.StorageID, RecordCount: row.RecordCount, CreatedAt: row.CreatedAt.UTC()})
	}
	return result, nil
}
