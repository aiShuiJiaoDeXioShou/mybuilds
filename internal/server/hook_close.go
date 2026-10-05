package server

import (
	"context"
	"encoding/json"
	"errors"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"mybuilds/internal/scm"
	"mybuilds/internal/store"
	"time"
)

func (s *Server) closeDueWebhookWindows(ctx context.Context) error {
	rows, e := s.store.DueWebhookWindows(ctx, store.Page{Limit: 100})
	if e != nil {
		return e
	}
	for _, w := range rows {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if e = s.closeWebhookWindow(ctx, w); errors.Is(e, store.ErrLockLost) {
			return e
		}
	}
	return nil
}
func (s *Server) closeWebhookWindow(parent context.Context, w store.WebhookWindow) error {
	ctx, cancel := context.WithTimeout(parent, 45*time.Second)
	defer cancel()
	ref := store.WebhookCloseRef{WindowID: w.ID, Revision: w.Revision, PolicyVersion: w.PolicyVersion}
	fail := func(reason string) error {
		save, stop := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
		defer stop()
		_, e := s.store.FailWebhookWindow(save, ref, reason)
		return e
	}
	for ctx.Err() == nil {
		current, p, e := s.store.ReadWebhookWindow(ctx, w.ID)
		if e != nil {
			return e
		}
		if current.State != "pending" {
			return nil
		}
		w = current
		ref.Revision = w.Revision
		policy, e := s.store.ReadWebhookPolicy(ctx, p.Name)
		if e != nil || !policy.Enabled || policy.PolicyVersion != w.PolicyVersion {
			return fail("hook_policy_changed")
		}
		if _, e = s.hookSecret(policy); e != nil {
			return fail("hook_policy_changed")
		}
		source, e := s.resolvePipeline(ctx, p, w.Branch, "")
		if e != nil {
			if ctx.Err() != nil {
				return fail("hook_timeout")
			}
			return fail("hook_source_error")
		}
		request := TriggerRequest{Branch: w.Branch, Ref: source.SHA, BuildNames: policy.BuildNames, AllowUpload: policy.AllowUpload}
		// 先验证全部选择，之后按每个已验证定义读取独立基线并重新同一准备体。
		initial, names, _, e := s.prepareTrigger(ctx, p, source, request, true, nil)
		if e != nil {
			if errors.Is(e, store.ErrForbidden) {
				return fail("hook_forbidden")
			}
			return fail("hook_pipeline_invalid")
		}
		facts := map[string]*protocol.ChangeFacts{}
		comparisons := []store.WebhookComparison{}
		for _, b := range initial {
			base, e := s.store.FindWebhookBaseline(ctx, b.Snapshot.ComparisonKey)
			if e != nil {
				return e
			}
			baselineSHA := ""
			if base != nil {
				baselineSHA = base.SHA
			}
			delta, e := scm.ReadChanges(ctx, scm.ChangeOptions{Source: scm.Options{DataDir: s.config.DataDir, Repository: p.Repository, Branch: w.Branch, Ref: source.SHA, SecretsFile: s.config.SecretsFile}, TargetSHA: source.SHA, BaselineSHA: baselineSHA})
			if e != nil {
				if ctx.Err() != nil {
					return fail("hook_timeout")
				}
				return fail("hook_source_error")
			}
			changes := &protocol.ChangeFacts{Mode: "diff", TargetSHA: source.SHA, Paths: delta.Paths, Digest: delta.Digest}
			if base == nil {
				changes.Mode = "full"
				changes.Reason = "baseline_missing"
			} else {
				changes.BaselineBuildID = base.BuildID
				changes.BaselineSHA = base.SHA
				if !delta.BaselineAvailable {
					changes.Mode = "full"
					changes.Reason = "baseline_unavailable"
				}
			}
			data, _ := json.Marshal(changes)
			if len(data) > config.MaxConfigBytes {
				return fail("hook_limit")
			}
			facts[b.Name] = changes
			comparisons = append(comparisons, store.WebhookComparison{Name: b.Name, Key: b.Snapshot.ComparisonKey, Baseline: base, Changes: *changes})
		}
		prepared, _, hasUpload, e := s.prepareTrigger(ctx, p, source, request, true, facts)
		if e != nil {
			return fail("hook_pipeline_invalid")
		}
		_, e = s.store.CloseWebhookWindow(ctx, store.WebhookCloseInput{Ref: ref, Actor: store.WebhookActor{ProjectID: p.ID, CredentialID: policy.CredentialID, SecretFingerprint: policy.SecretFingerprint, PolicyVersion: policy.PolicyVersion}, SHA: source.SHA, Source: source.Mode, File: source.File, SourceDigest: source.SourceDigest, SelectedBuilds: names, Builds: prepared, Comparisons: comparisons, HasUpload: hasUpload})
		if errors.Is(e, store.ErrConflict) {
			continue
		}
		if errors.Is(e, store.ErrForbidden) {
			return fail("hook_policy_changed")
		}
		if errors.Is(e, store.ErrInvalid) {
			return fail("hook_pipeline_invalid")
		}
		return e
	}
	return fail("hook_timeout")
}
