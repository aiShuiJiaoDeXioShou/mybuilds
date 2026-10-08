package store

import (
	"context"
	"encoding/json"
	"math"
	"path"
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
	if in.SourcePath != "" && (!validSourcePath(in.SourcePath) || path.Base(in.SourcePath) != in.Name || in.Purpose == "junit") {
		return false
	}
	switch in.Purpose {
	case "", "artifact":
		if in.ReportRevision != 0 || in.ReportKey != "" {
			return false
		}
	case "junit":
		if in.Phase != "ordinary" || in.Size < 1 || in.Size > 8<<20 || in.ReportRevision < 1 || !validDigest(in.ReportKey) {
			return false
		}
	default:
		return false
	}
	return validRef(in.Ref) && validUUID(in.ID) && in.Seq > 0 && slices.Contains([]string{"ordinary", "success", "failure", "always"}, in.Phase) && in.Index > 0 && validName(in.Step) && validArtifactName(in.Name) && in.Size >= 0 && in.Size <= 1<<30 && validDigest(in.SHA256)
}
func artifactView(build buildRecord, file artifactRecord) protocol.ArtifactView {
	return protocol.ArtifactView{ID: file.ID, BuildID: file.BuildID, AttemptID: file.AttemptID, BuildName: build.Name, Phase: file.Phase, Step: file.Step, Name: file.Name, Index: file.Index, Size: file.Size, SHA256: file.SHA256, Purpose: file.Purpose, ReportRevision: file.ReportRevision, ReportKey: file.ReportKey, CompletedAt: file.CreatedAt.UTC()}
}
func sameArtifact(file artifactRecord, in protocol.ArtifactDeclaration) bool {
	return file.ID == in.ID && file.BuildID == in.Ref.BuildID && file.AttemptID == in.Ref.AttemptID && file.Seq == in.Seq && file.Phase == in.Phase && file.Step == in.Step && file.Index == in.Index && file.Name == in.Name && file.Size == in.Size && file.SHA256 == in.SHA256 && file.Purpose == in.Purpose && file.ReportRevision == in.ReportRevision && file.ReportKey == in.ReportKey && file.SourcePath == in.SourcePath
}
func (s *Store) CommitArtifact(ctx context.Context, actor NodeActor, in ArtifactCommit) (ArtifactCommitted, error) {
	if !validArtifactDeclaration(in.Declaration) || !validUUID(in.StorageID) {
		return ArtifactCommitted{}, ErrInvalid
	}
	if in.Declaration.Purpose == "junit" && (in.VerifiedJUnit == nil || in.VerifiedJUnit.Diagnostics == nil) || in.Declaration.Purpose != "junit" && in.VerifiedJUnit != nil {
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
		verifiedJSON := ""
		if d.Purpose == "junit" {
			verifiedJSON, err = validateJUnitArtifact(tx, build, in)
			if err != nil {
				return err
			}
		} else {
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
		}
		if build.LastArtifactSeq == math.MaxInt64 || d.Seq != build.LastArtifactSeq+1 {
			return ErrSequenceInvalid
		}
		var totals struct{ OrdinaryCount, Size int64 }
		if err = tx.Model(&artifactRecord{}).Select("COUNT(CASE WHEN purpose <> 'junit' THEN 1 END) AS ordinary_count, COALESCE(SUM(size),0) AS size").Where("attempt_id = ?", d.Ref.AttemptID).Scan(&totals).Error; err != nil {
			return err
		}
		if d.Purpose != "junit" && totals.OrdinaryCount >= 128 || totals.Size > (4<<30)-d.Size {
			return ErrArtifactConflict
		}
		file := artifactRecord{SourcePath: d.SourcePath, ID: d.ID, BuildID: build.ID, AttemptID: d.Ref.AttemptID, Seq: d.Seq, Phase: d.Phase, Step: d.Step, Name: d.Name, Index: d.Index, Size: d.Size, SHA256: d.SHA256, StorageID: in.StorageID, Purpose: d.Purpose, ReportRevision: d.ReportRevision, ReportKey: d.ReportKey, VerifiedJUnitJSON: verifiedJSON, CreatedAt: time.Now().UTC()}
		if err = tx.Create(&file).Error; err != nil {
			return err
		}
		if err = tx.Model(&build).Update("last_artifact_seq", d.Seq).Error; err != nil {
			return err
		}
		result = ArtifactCommitted{View: artifactView(build, file), StorageID: in.StorageID, Created: true}
		if d.Purpose == "junit" {
			if _, err = s.captureReportBudget(tx, build); err != nil {
				return err
			}
		}
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
		current := err == nil
		if err != nil {
			if e := retentionNodeActor(tx, actor); e != nil {
				return e
			}
			if e := tx.First(&build, "id = ?", ref.BuildID).Error; e != nil {
				return e
			}
			if actor.ID != ref.NodeID || build.CurrentApprovalID == nil {
				return err
			}
			known, e := approvalHistoricalRef(tx, build, ref)
			if e != nil {
				return e
			}
			if !known {
				return err
			}
		}

		var file artifactRecord
		if err = tx.First(&file, "id = ? AND build_id = ? AND attempt_id = ?", id, ref.BuildID, ref.AttemptID).Error; err != nil {
			return err
		}
		result = artifactView(build, file)
		if !current {
			return nil
		}
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
	if err = evidenceReadBuildState(build); err != nil {
		return nil, err
	}
	var files []artifactRecord
	query := db.Where("build_id = ?", buildID)
	ids, err := approvalVisibleReportIDs(db, build)
	if err != nil {
		return nil, safeError(err)
	}
	query = query.Where("purpose <> ? OR id IN ?", "junit", ids)

	if err = query.Order("seq ASC").Limit(page.Limit).Offset(page.Offset).Find(&files).Error; err != nil {
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
	// 原文件metadata完整清理后，保留事项中的固定对象身份仍能解释410。
	var retired int64
	if err := db.Model(&retentionObjectRecord{}).Joins("JOIN retention_jobs ON retention_jobs.id = retention_objects.job_id").Joins("JOIN builds ON builds.id = retention_jobs.build_id").Where("retention_objects.object_id = ? AND retention_objects.kind IN ? AND builds.history_state <> 'live'", id, []string{"artifact", "junit"}).Count(&retired).Error; err != nil {
		return ArtifactStored{}, safeError(err)
	}
	if retired > 0 {
		return ArtifactStored{}, ErrRetentionRetired
	}
	var file artifactRecord
	if err := db.First(&file, "id = ?", id).Error; err != nil {
		return ArtifactStored{}, safeError(err)
	}
	var build buildRecord
	if err := db.First(&build, "id = ?", file.BuildID).Error; err != nil {
		return ArtifactStored{}, safeError(err)
	}
	if err := evidenceReadBuildState(build); err != nil {
		return ArtifactStored{}, err
	}
	if file.Purpose == "junit" {
		ids, err := approvalVisibleReportIDs(db, build)
		if err != nil {
			return ArtifactStored{}, safeError(err)
		}
		if !slices.Contains(ids, file.ID) {
			return ArtifactStored{}, ErrNotFound
		}
	}
	return ArtifactStored{View: artifactView(build, file), StorageID: file.StorageID}, nil
}

func validSourcePath(value string) bool {
	return len(value) <= 4096 && !strings.HasPrefix(value, "/") && value != ".." && !strings.HasPrefix(value, "../") && path.Clean(value) == value && !strings.ContainsAny(value, "\\\x00\r\n")
}
