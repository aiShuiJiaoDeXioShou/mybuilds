package store

import (
	"context"
	"encoding/json"
	"gorm.io/gorm"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"slices"
	"sort"
	"time"
)

// ComparisonKey不包含当前SHA或动态执行身份，使可信成功证据可跨提交比较。
func ComparisonKey(project, branch, name string, selected []string, snapshot BuildSnapshot) string {
	names := slices.Clone(selected)
	sort.Strings(names)
	var origin *PipelineOrigin
	if snapshot.Origin != nil {
		copy := *snapshot.Origin
		copy.SHA = ""
		origin = &copy
	}
	return hookHash(struct {
		Origin                *PipelineOrigin
		Project, Branch, Name string
		Selected              []string
		Definition            config.Build
		Params                map[string]string
	}{origin, project, branch, name, names, snapshot.Definition, snapshot.Params})
}
func automaticSemantic(snapshot BuildSnapshot, sha string) string {
	if snapshot.AutomaticWindowID == "" {
		return ""
	}
	return hookHash(struct{ Key, SHA string }{snapshot.ComparisonKey, sha})
}
func findBaseline(tx *gorm.DB, key string) (*WebhookBaseline, error) {
	var rows []buildRecord
	if e := tx.Where("comparison_key = ? AND status = ? AND stop_unconfirmed = ?", key, "succeeded", false).Find(&rows).Error; e != nil {
		return nil, e
	}
	var best *WebhookBaseline
	for _, b := range rows {
		if b.AttemptID == nil || b.LastEventSeq <= 0 {
			continue
		}
		var r executionReceiptRecord
		e := tx.First(&r, "build_id = ? AND attempt_id = ? AND seq = ? AND kind = ? AND stop_known = ?", b.ID, *b.AttemptID, b.LastEventSeq, "build_finished", true).Error
		if e == gorm.ErrRecordNotFound {
			continue
		}
		if e != nil {
			return nil, e
		}
		if !validHex(r.Digest, 64) {
			return nil, errDatabase
		}
		var batch batchRecord
		if e = tx.First(&batch, "id = ?", b.BatchID).Error; e != nil {
			return nil, e
		}
		candidate := &WebhookBaseline{BuildID: b.ID, SHA: batch.SHA, AttemptID: *b.AttemptID, Seq: r.Seq, ReceiptDigest: r.Digest, ConfirmedAt: r.CreatedAt.UTC()}
		if best == nil || candidate.ConfirmedAt.After(best.ConfirmedAt) || candidate.ConfirmedAt.Equal(best.ConfirmedAt) && candidate.BuildID < best.BuildID {
			best = candidate
		}
	}
	return best, nil
}
func (s *Store) FindWebhookBaseline(ctx context.Context, key string) (*WebhookBaseline, error) {
	if !validHex(key, 64) {
		return nil, ErrInvalid
	}
	if e := s.CheckLock(ctx); e != nil {
		return nil, e
	}
	v, e := findBaseline(s.db.WithContext(ctx), key)
	return v, safeError(e)
}
func windowResult(row webhookWindowRecord, replayed bool) (WebhookWindowResult, error) {
	w, e := windowView(row)
	if e != nil {
		return WebhookWindowResult{}, e
	}
	return WebhookWindowResult{WindowID: w.ID, State: w.State, Reason: w.Reason, SHA: w.FinalSHA, BatchID: w.BatchID, BuildIDs: w.BuildIDs, ReusedBuildIDs: w.ReusedBuildIDs, Replayed: replayed}, nil
}
func (s *Store) FailWebhookWindow(ctx context.Context, ref WebhookCloseRef, reason string) (out WebhookWindowResult, err error) {
	if !slices.Contains([]string{"hook_policy_changed", "hook_pipeline_invalid", "hook_source_error", "hook_limit", "hook_timeout", "hook_forbidden"}, reason) {
		return out, ErrInvalid
	}
	err = s.write(ctx, func(tx *gorm.DB) error {
		var row webhookWindowRecord
		if e := tx.First(&row, "id = ?", ref.WindowID).Error; e != nil {
			return e
		}
		if row.State != "pending" {
			var e error
			out, e = windowResult(row, true)
			return e
		}
		if row.Revision != ref.Revision || row.PolicyVersion != ref.PolicyVersion {
			return ErrConflict
		}
		row.State = "failed"
		row.Reason = reason
		if e := tx.Model(&row).Updates(map[string]any{"state": row.State, "reason": row.Reason}).Error; e != nil {
			return e
		}
		var e error
		out, e = windowResult(row, false)
		return e
	})
	return out, err
}
func (s *Store) CloseWebhookWindow(ctx context.Context, in WebhookCloseInput) (out WebhookWindowResult, err error) {
	if !validID(in.Ref.WindowID) || !validHex(in.SHA, 40, 64) || len(in.Builds) == 0 || len(in.Builds) != len(in.SelectedBuilds) || len(in.Comparisons) != len(in.Builds) {
		return out, ErrInvalid
	}
	err = s.write(ctx, func(tx *gorm.DB) error {
		var row webhookWindowRecord
		if e := tx.First(&row, "id = ?", in.Ref.WindowID).Error; e != nil {
			return e
		}
		if row.State != "pending" {
			var e error
			out, e = windowResult(row, true)
			return e
		}
		if row.Revision != in.Ref.Revision || row.PolicyVersion != in.Ref.PolicyVersion || time.Now().UTC().Before(row.Deadline) {
			return ErrConflict
		}
		policy, project, e := hookActor(tx, in.Actor)
		if e != nil {
			return e
		}
		if project.ID != row.ProjectID || policy.PolicyVersion != row.PolicyVersion {
			return ErrForbidden
		}
		w, e := windowView(row)
		if e != nil {
			return e
		}
		names := slices.Clone(in.SelectedBuilds)
		sort.Strings(names)
		expected := slices.Clone(policy.BuildNames)
		sort.Strings(expected)
		if !slices.Equal(names, expected) {
			return ErrInvalid
		}
		fresh := []PreparedBuild{}
		ids := map[string]string{}
		reused := []string{}
		for i, b := range in.Builds {
			if !slices.Contains(names, b.Name) || ids[b.Name] != "" {
				return ErrInvalid
			}
			validated, upload, e := validatePrepared(b)
			if e != nil {
				return e
			}
			if upload && !policy.AllowUpload {
				return ErrForbidden
			}
			comparison := in.Comparisons[i]
			key := ComparisonKey(project.ID, row.Branch, b.Name, names, validated.Snapshot)
			if comparison.Name != b.Name || comparison.Key != key || validated.Snapshot.ComparisonKey != key || !protocol.ValidateChanges(&comparison.Changes, in.SHA) || validated.Snapshot.Changes == nil || hookHash(validated.Snapshot.Changes) != hookHash(comparison.Changes) {
				return ErrInvalid
			}
			latest, e := findBaseline(tx, key)
			if e != nil {
				return e
			}
			if hookHash(latest) != hookHash(comparison.Baseline) {
				return ErrConflict
			}
			if latest == nil {
				if comparison.Changes.Mode != "full" || comparison.Changes.Reason != "baseline_missing" || comparison.Changes.BaselineBuildID != "" || comparison.Changes.BaselineSHA != "" {
					return ErrInvalid
				}
			} else if comparison.Changes.BaselineBuildID != latest.BuildID || comparison.Changes.BaselineSHA != latest.SHA {
				return ErrInvalid
			}
			validated.Snapshot.AutomaticWindowID = row.ID
			semantic := automaticSemantic(validated.Snapshot, in.SHA)
			var prior buildRecord
			e = tx.Where("semantic_key = ? AND (status IN ? OR stop_unconfirmed = ?)", semantic, []string{"queued", "running", "cancel_requested", "waiting_approval", "approved", "succeeded"}, true).Order("created_at ASC, id ASC").First(&prior).Error
			if e == nil {
				ids[b.Name] = prior.ID
				reused = append(reused, prior.ID)
			} else if e == gorm.ErrRecordNotFound {
				fresh = append(fresh, validated)
				ids[b.Name] = "fresh"
			} else {
				return e
			}
		}
		if len(fresh) > 0 {
			batch, e := enqueuePreparedTx(tx, project, EnqueueInput{ProjectID: project.ID, ProjectVersion: project.PolicyVersion, Actor: Actor{ID: "hook:" + policy.CredentialID}, SHA: in.SHA, Branch: w.Branch, Source: in.Source, File: in.File, SourceDigest: in.SourceDigest, Builds: fresh, SelectedBuilds: names, HasUpload: in.HasUpload, AllowUpload: policy.AllowUpload}, policy.AllowUpload)
			if e != nil {
				return e
			}
			row.BatchID = batch.ID
			for _, b := range batch.Builds {
				ids[b.Name] = b.ID
			}
		}
		all := make([]string, 0, len(names))
		for _, name := range names {
			if ids[name] == "" || ids[name] == "fresh" {
				return errDatabase
			}
			all = append(all, ids[name])
		}
		row.State = "closed"
		row.FinalSHA = in.SHA
		row.BuildIDsJSON, _ = encode(all)
		row.ReusedBuildIDsJSON, _ = encode(reused)
		if _, _, e = hookActor(tx, in.Actor); e != nil {
			return e
		}
		if e = tx.Model(&row).Updates(map[string]any{"state": row.State, "final_sha": row.FinalSHA, "batch_id": row.BatchID, "build_ids_json": row.BuildIDsJSON, "reused_build_ids_json": row.ReusedBuildIDsJSON}).Error; e != nil {
			return e
		}
		out, e = windowResult(row, false)
		return e
	})
	return out, err
}

// 避免从字符串正文推断新的证据结构。
func decodeHookSnapshot(raw string) (BuildSnapshot, error) {
	var s BuildSnapshot
	if json.Unmarshal([]byte(raw), &s) != nil {
		return s, errDatabase
	}
	return s, nil
}
