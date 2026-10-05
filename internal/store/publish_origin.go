package store

import (
	"github.com/bmatcuk/doublestar/v4"
	"gorm.io/gorm"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"path"
	"strings"
)

// 中央只使用节点实际声明的原路径；旧证据仅在唯一字面producer路径时保守兼容。
func publicationArtifact(tx *gorm.DB, row buildRecord, task protocol.TaskSnapshot, upload config.Step, index int, wanted artifactRecord) error {
	pattern, e := renderPublishValue(upload.File, task)
	if e != nil {
		return e
	}
	var candidates []artifactRecord
	if e = tx.Where("build_id = ? AND attempt_id = ? AND phase = ? AND \"index\" < ?", row.ID, row.AttemptID, "ordinary", index).Limit(129).Find(&candidates).Error; e != nil {
		return e
	}
	matches := 0
	selected := false
	for _, a := range candidates {
		if a.Purpose != "" && a.Purpose != "artifact" {
			continue
		}
		if a.Index < 1 || a.Index > len(task.Definition.Steps) {
			return ErrArtifactConflict
		}
		producer := task.Definition.Steps[a.Index-1]
		if producer.Kind != "artifact" || producer.Name != a.Step {
			return ErrArtifactConflict
		}
		source := a.SourcePath
		if source == "" {
			if len(producer.Paths) != 1 {
				return ErrArtifactConflict
			}
			source, e = renderPublishValue(producer.Paths[0], task)
			if e != nil || strings.ContainsAny(source, "*?[") || path.Base(source) != a.Name {
				return ErrArtifactConflict
			}
		}
		if !validSourcePath(source) {
			return ErrArtifactConflict
		}
		declared := false
		for _, p := range producer.Paths {
			p, e = renderPublishValue(p, task)
			if e != nil {
				return e
			}
			ok, e := doublestar.PathMatch(p, source)
			if e != nil {
				return ErrArtifactConflict
			}
			declared = declared || ok
		}
		if !declared {
			return ErrArtifactConflict
		}
		ok, e := doublestar.PathMatch(pattern, source)
		if e != nil {
			return ErrArtifactConflict
		}
		if ok {
			matches++
			selected = selected || a.ID == wanted.ID
		}
	}
	if matches != 1 || !selected {
		return ErrArtifactConflict
	}
	return nil
}

var publishEvidenceCodes = []string{"", "remote_receipt", "remote_state", "confirmed_not_sent", "remote_rejected", "result_unconfirmed", "transport_unconfirmed", "query_only"}
var publishMutationStages = []string{"", "not_started", "upload", "edit", "bundle", "track", "commit", "upload_binary", "select_build", "set_release_policy", "create_review", "add_review_item", "submit_review"}
