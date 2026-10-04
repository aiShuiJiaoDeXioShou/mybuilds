package store

import (
	"context"
	"encoding/json"
	"math"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"gorm.io/gorm"
	"mybuilds/internal/protocol"
)

func validArtifactName(name string) bool {
	if name == "" || len(name) > 255 || name == "." || name == ".." || !utf8.ValidString(name) || strings.TrimSpace(name) != name || strings.ContainsAny(name, "/\\") {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func validArtifactDeclaration(in protocol.ArtifactDeclaration) bool {
	return validRef(in.Ref) && validUUID(in.ID) && in.Seq > 0 && slices.Contains([]string{"ordinary", "success", "failure", "always"}, in.Phase) && in.Index > 0 && validName(in.Step) && validArtifactName(in.Name) && in.Size >= 0 && in.Size <= 1<<30 && validDigest(in.SHA256)
}
func artifactView(build buildRecord, file artifactRecord) protocol.ArtifactView {
	return protocol.ArtifactView{ID: file.ID, BuildID: file.BuildID, AttemptID: file.AttemptID, BuildName: build.Name, Phase: file.Phase, Step: file.Step, Name: file.Name, Index: file.Index, Size: file.Size, SHA256: file.SHA256, CompletedAt: file.CreatedAt.UTC()}
}
func sameArtifact(file artifactRecord, in protocol.ArtifactDeclaration) bool {
	return file.ID == in.ID && file.BuildID == in.Ref.BuildID && file.AttemptID == in.Ref.AttemptID && file.Seq == in.Seq && file.Phase == in.Phase && file.Step == in.Step && file.Index == in.Index && file.Name == in.Name && file.Size == in.Size && file.SHA256 == in.SHA256
}
func (s *Store) CommitArtifact(ctx context.Context, actor NodeActor, in ArtifactCommit) (ArtifactCommitted, error) {
	if !validArtifactDeclaration(in.Declaration) || !validUUID(in.StorageID) {
		return ArtifactCommitted{}, ErrInvalid
	}
	var result ArtifactCommitted
	err := s.write(ctx, func(tx *gorm.DB) error {
		d := in.Declaration
		build, err := s.currentExecution(tx, actor, d.Ref)
		if err != nil {
			return err
		}
		expires := *build.LeaseExpiresAt
		var existing artifactRecord
		err = tx.First(&existing, "id = ?", d.ID).Error
		if err == nil {
			if !sameArtifact(existing, d) {
				return ErrArtifactConflict
			}
			result = ArtifactCommitted{View: artifactView(build, existing), StorageID: existing.StorageID}
			return checkBoundary(tx, actor, d.Ref, expires)
		}
		if err != gorm.ErrRecordNotFound {
			return err
		}
		var step stepRecord
		if err = tx.First(&step, "build_id = ? AND phase = ? AND \"index\" = ?", build.ID, d.Phase, d.Index).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return ErrArtifactConflict
			}
			return err
		}
		var ids []string
		if step.Kind != "artifact" || !terminalStep(step.Status) || !step.Started || !step.StopConfirmed || step.CleanupFailed || step.Name != d.Step {
			return ErrArtifactConflict
		}
		if json.Unmarshal([]byte(step.ArtifactIDsJSON), &ids) != nil {
			return errDatabase
		}
		if !slices.Contains(ids, d.ID) {
			return ErrArtifactConflict
		}
		if build.LastArtifactSeq == math.MaxInt64 || d.Seq != build.LastArtifactSeq+1 {
			return ErrSequenceInvalid
		}
		var totals struct{ Count, Size int64 }
		if err = tx.Model(&artifactRecord{}).Select("COUNT(*) AS count, COALESCE(SUM(size),0) AS size").Where("attempt_id = ?", d.Ref.AttemptID).Scan(&totals).Error; err != nil {
			return err
		}
		if totals.Count >= 128 || totals.Size > (4<<30)-d.Size {
			return ErrArtifactConflict
		}
		file := artifactRecord{ID: d.ID, BuildID: build.ID, AttemptID: d.Ref.AttemptID, Seq: d.Seq, Phase: d.Phase, Step: d.Step, Name: d.Name, Index: d.Index, Size: d.Size, SHA256: d.SHA256, StorageID: in.StorageID, CreatedAt: time.Now().UTC()}
		if err = tx.Create(&file).Error; err != nil {
			return err
		}
		if err = tx.Model(&build).Update("last_artifact_seq", d.Seq).Error; err != nil {
			return err
		}
		result = ArtifactCommitted{View: artifactView(build, file), StorageID: in.StorageID, Created: true}
		return checkBoundary(tx, actor, d.Ref, expires)
	})
	if err != nil {
		return ArtifactCommitted{}, err
	}
	return result, nil
}
func (s *Store) FindNodeArtifact(ctx context.Context, actor NodeActor, ref protocol.LeaseRef, id string) (protocol.ArtifactView, error) {
	if !validRef(ref) || !validUUID(id) {
		return protocol.ArtifactView{}, ErrInvalid
	}
	var result protocol.ArtifactView
	err := s.write(ctx, func(tx *gorm.DB) error {
		build, err := s.currentExecution(tx, actor, ref)
		if err != nil {
			return err
		}
		var file artifactRecord
		if err = tx.First(&file, "id = ? AND build_id = ? AND attempt_id = ?", id, ref.BuildID, ref.AttemptID).Error; err != nil {
			return err
		}
		result = artifactView(build, file)
		return checkBoundary(tx, actor, ref, *build.LeaseExpiresAt)
	})
	if err != nil {
		return protocol.ArtifactView{}, err
	}
	return result, nil
}
func (s *Store) ListArtifacts(ctx context.Context, actor Actor, buildID string, page Page) ([]protocol.ArtifactView, error) {
	page, err := normalizePage(page)
	if err != nil || !validUUID(buildID) {
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
	var files []artifactRecord
	if err = db.Where("build_id = ?", buildID).Order("seq ASC").Limit(page.Limit).Offset(page.Offset).Find(&files).Error; err != nil {
		return nil, safeError(err)
	}
	result := make([]protocol.ArtifactView, 0, len(files))
	for _, file := range files {
		result = append(result, artifactView(build, file))
	}
	return result, nil
}
func (s *Store) GetArtifact(ctx context.Context, actor Actor, id string) (ArtifactStored, error) {
	if !validUUID(id) {
		return ArtifactStored{}, ErrInvalid
	}
	if err := s.CheckLock(ctx); err != nil {
		return ArtifactStored{}, err
	}
	db := s.db.WithContext(ctx)
	if err := authorize(db, actor, "admin", "approver"); err != nil {
		return ArtifactStored{}, safeError(err)
	}
	var file artifactRecord
	if err := db.First(&file, "id = ?", id).Error; err != nil {
		return ArtifactStored{}, safeError(err)
	}
	var build buildRecord
	if err := db.First(&build, "id = ?", file.BuildID).Error; err != nil {
		return ArtifactStored{}, safeError(err)
	}
	return ArtifactStored{View: artifactView(build, file), StorageID: file.StorageID}, nil
}
