package store

import (
	"fmt"
	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"strings"
	"testing"
	"time"
)

func TestWebhookFailedWindowAndPolicyAllowNewBody(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		p, policy, actor := hookFixture(t, s, 0)
		event := hookEvent("", strings.Repeat("f", 64))
		first, err := s.ReceiveWebhook(testContext, actor, event)
		if err != nil {
			t.Fatal(err)
		}
		w, _, err := s.ReadWebhookWindow(testContext, first.WindowID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.FailWebhookWindow(testContext, WebhookCloseRef{WindowID: w.ID, Revision: w.Revision, PolicyVersion: w.PolicyVersion}, "hook_source_error"); err != nil {
			t.Fatal(err)
		}
		next, err := s.ReceiveWebhook(testContext, actor, event)
		if err != nil || next.Replayed || next.WindowID == first.WindowID {
			t.Fatal("失败窗口永久拒绝无delivery请求", next, err)
		}
		p, err = s.GetProject(testContext, p.Name)
		if err != nil {
			t.Fatal(err)
		}
		settings := p.Settings
		settings.Triggers.QuietPeriod = "1s"
		policy.QuietPeriod = 1e9
		policy, err = s.ConfigureWebhook(testContext, localAdmin, WebhookPolicyInput{Policy: policy, Settings: settings, ExpectedPolicyVersion: p.PolicyVersion})
		if err != nil {
			t.Fatal(err)
		}
		actor.PolicyVersion = policy.PolicyVersion
		changed, err := s.ReceiveWebhook(testContext, actor, event)
		if err != nil || changed.Replayed || changed.WindowID == next.WindowID {
			t.Fatal("body跨策略错误合并", changed, err)
		}
	})
}

func TestWebhookPausedAndApprovedReuseOriginalBuild(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		node, session, p, leasePolicy := leaseFixture(t, s)
		settings := config.ProjectSettings{Hook: &config.HookSettings{Enabled: true, RepositoryKey: "42"}, Triggers: &config.TriggerSettings{Builds: []string{"android"}, QuietPeriod: "0s"}}
		policy, err := s.ConfigureWebhook(testContext, localAdmin, WebhookPolicyInput{Policy: WebhookPolicy{ProjectID: p.ID, Provider: p.Provider, Enabled: true, RepositoryKey: "42", CredentialID: "own-credential", SecretFingerprint: strings.Repeat("a", 64), BuildNames: []string{"android"}}, Settings: settings, ExpectedPolicyVersion: p.PolicyVersion})
		if err != nil {
			t.Fatal(err)
		}
		actor := WebhookActor{ProjectID: p.ID, CredentialID: policy.CredentialID, SecretFingerprint: policy.SecretFingerprint, PolicyVersion: policy.PolicyVersion}
		p, err = s.GetProject(testContext, p.Name)
		if err != nil {
			t.Fatal(err)
		}
		raw := enqueueInput(p, "unused")
		build := raw.Builds[0]
		build.Snapshot.Definition.Steps = append(build.Snapshot.Definition.Steps, config.Step{Kind: "approval", Name: "release"}, config.Step{Kind: "run", Name: "after", Run: "true"})
		build.Steps = append(build.Steps, StepProgress{Phase: "ordinary", Index: 2, Name: "release", Kind: "approval", Condition: "ready", Status: "pending"}, StepProgress{Phase: "ordinary", Index: 3, Name: "after", Kind: "run", Condition: "ready", Status: "pending"})
		build.Snapshot.ComparisonKey = ComparisonKey(p.ID, "main", build.Name, []string{build.Name}, build.Snapshot)
		facts := protocol.ChangeFacts{Mode: "full", Reason: "baseline_missing", TargetSHA: raw.SHA, Paths: []string{}, Digest: protocol.ChangesDigest([]string{})}
		build.Snapshot.Changes = &facts
		closeEvent := func(i int) WebhookWindowResult {
			ev := hookEvent("", fmt.Sprintf("%064x", i))
			ev.Provider = p.Provider
			ev.After = raw.SHA
			ev.ReceiptDigest = protocol.WebhookEventDigest(ev)
			receipt, err := s.ReceiveWebhook(testContext, actor, ev)
			if err != nil {
				t.Fatal(err)
			}
			w, _, err := s.ReadWebhookWindow(testContext, receipt.WindowID)
			if err != nil {
				t.Fatal(err)
			}
			out, err := s.CloseWebhookWindow(testContext, WebhookCloseInput{Ref: WebhookCloseRef{WindowID: w.ID, Revision: w.Revision, PolicyVersion: policy.PolicyVersion}, Actor: actor, SHA: raw.SHA, Source: raw.Source, File: raw.File, SourceDigest: raw.SourceDigest, SelectedBuilds: []string{build.Name}, Builds: []PreparedBuild{build}, Comparisons: []WebhookComparison{{Name: build.Name, Key: build.Snapshot.ComparisonKey, Changes: facts}}})
			if err != nil {
				t.Fatal(err)
			}
			return out
		}
		first := closeEvent(1)
		grant, err := s.Claim(testContext, node, protocol.ClaimRequest{SessionID: session.SessionID, ClaimKey: uuid.NewString()}, leasePolicy)
		if err != nil || grant == nil {
			t.Fatal("真实领取", err)
		}
		for i, kind := range []string{"intent", "started", "finished"} {
			progress := stepProgress(kind, "", "ordinary", "compile", 1)
			if i > 0 {
				progress.Started = true
			}
			if i == 2 {
				progress.Status = "succeeded"
				progress.StopConfirmed = true
			}
			accept(t, s, node, event(grant.Ref, int64(i+1), progress))
		}
		checkpoint := approvalCheckpoint(t, *grant)
		accept(t, s, node, checkpoint)
		check := func(i int) {
			again := closeEvent(i)
			if len(again.ReusedBuildIDs) != 1 || again.BuildIDs[0] != first.BuildIDs[0] || again.BatchID != "" {
				t.Fatal("审批期间新建同SHA构建", again)
			}
			current, err := s.GetProject(testContext, p.Name)
			if err != nil || current.NextNumber != 2 {
				t.Fatal("审批复投占号", err)
			}
		}
		check(2)
		_, err = s.DecideApproval(testContext, localAdmin, grant.Ref.BuildID, ApprovalDecision{ApprovalID: checkpoint.Progress.Approval.ID, Revision: 1, CheckpointDigest: checkpoint.Progress.Approval.CheckpointDigest, Decision: "approve"})
		if err != nil {
			t.Fatal(err)
		}
		check(3)
	})
}

func TestWebhookBodyDoesNotReopenExpiredWindow(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		_, _, actor := hookFixture(t, s, 20*time.Millisecond)
		event := hookEvent("", strings.Repeat("e", 64))
		first, err := s.ReceiveWebhook(testContext, actor, event)
		if err != nil {
			t.Fatal(err)
		}
		w, _, err := s.ReadWebhookWindow(testContext, first.WindowID)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Until(w.Deadline) + 5*time.Millisecond)
		next, err := s.ReceiveWebhook(testContext, actor, event)
		if err != nil || next.Replayed || next.WindowID == first.WindowID {
			t.Fatal("截止后无ID请求错误复用旧窗口", next, err)
		}
		before, _, err := s.ReadWebhookWindow(testContext, first.WindowID)
		if err != nil || !before.Deadline.Equal(w.Deadline) {
			t.Fatal("原截止改变", err)
		}
	})
}
