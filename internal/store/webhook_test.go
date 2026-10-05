package store

import (
	"errors"
	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"strings"
	"sync"
	"testing"
	"time"
)

func hookFixture(t *testing.T, s *Store, quiet time.Duration) (Project, WebhookPolicy, WebhookActor) {
	t.Helper()
	p, e := s.CreateProject(testContext, localAdmin, ProjectInput{Name: "hooked", Repository: "/tmp/own-trusted-repo", Provider: "github", AllowedNodes: []string{"mac"}, DefaultNode: "mac"})
	if e != nil {
		t.Fatal(e)
	}
	settings := config.ProjectSettings{Hook: &config.HookSettings{Enabled: true, RepositoryKey: "42"}, Triggers: &config.TriggerSettings{Builds: []string{"default"}, QuietPeriod: quiet.String()}}
	policy, e := s.ConfigureWebhook(testContext, localAdmin, WebhookPolicyInput{Policy: WebhookPolicy{ProjectID: p.ID, Provider: "github", Enabled: true, RepositoryKey: "42", CredentialID: "own-credential", SecretFingerprint: strings.Repeat("a", 64), BuildNames: []string{"default"}, QuietPeriod: quiet}, Settings: settings, ExpectedPolicyVersion: p.PolicyVersion})
	if e != nil {
		t.Fatal(e)
	}
	return p, policy, WebhookActor{ProjectID: p.ID, CredentialID: policy.CredentialID, SecretFingerprint: policy.SecretFingerprint, PolicyVersion: policy.PolicyVersion}
}
func hookEvent(delivery, body string) protocol.WebhookEvent {
	e := protocol.WebhookEvent{Provider: "github", Kind: "push", DeliveryID: delivery, RepositoryKey: "42", Branch: "main", After: strings.Repeat("a", 40), BodyDigest: body}
	e.ReceiptDigest = protocol.WebhookEventDigest(e)
	return e
}
func TestWebhookReceiveAtomicReplayFixedWindowAndRotation(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		_, p, a := hookFixture(t, s, time.Minute)
		e := hookEvent("one", strings.Repeat("b", 64))
		r, err := s.ReceiveWebhook(testContext, a, e)
		if err != nil || r.WindowID == "" {
			t.Fatalf("receive: %+v %v", r, err)
		}
		again, err := s.ReceiveWebhook(testContext, a, e)
		if err != nil || !again.Replayed || again.EventID != r.EventID {
			t.Fatalf("replay: %+v %v", again, err)
		}
		alias := e
		alias.DeliveryID = "two"
		alias.ReceiptDigest = protocol.WebhookEventDigest(alias)
		again, err = s.ReceiveWebhook(testContext, a, alias)
		if err != nil || !again.Replayed || again.EventID != r.EventID {
			t.Fatal("换delivery重复执行", again, err)
		}
		changed := hookEvent("one", strings.Repeat("c", 64))
		if _, err = s.ReceiveWebhook(testContext, a, changed); !errors.Is(err, ErrConflict) {
			t.Fatal("delivery异内容未拒", err)
		}
		w, _, err := s.ReadWebhookWindow(testContext, r.WindowID)
		if err != nil || w.Revision != 1 || w.Deadline.Sub(w.OpenedAt) != p.QuietPeriod {
			t.Fatal("窗口被重投延长", w, err)
		}
		next := hookEvent("three", strings.Repeat("d", 64))
		next.After = strings.Repeat("c", 40)
		next.ReceiptDigest = protocol.WebhookEventDigest(next)
		merged, err := s.ReceiveWebhook(testContext, a, next)
		if err != nil || merged.WindowID != r.WindowID {
			t.Fatal("未合并", merged, err)
		}
		after, _, _ := s.ReadWebhookWindow(testContext, r.WindowID)
		if after.Revision != 2 || !after.Deadline.Equal(w.Deadline) {
			t.Fatal("合并改变固定截止", after)
		}
		a.PolicyVersion++
		if _, err = s.ReceiveWebhook(testContext, a, next); !errors.Is(err, ErrForbidden) {
			t.Fatal("伪新版本授接收权", err)
		}
		status, err := s.Status(testContext)
		if err != nil || status.Queued != 0 {
			t.Fatal("接收阶段分号", status, err)
		}
	})
}
func TestWebhookCloseCASAndAutomaticReuseNoNumberLoss(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		p, policy, a := hookFixture(t, s, 0)
		p, _ = s.GetProject(testContext, p.Name)
		prepare := func(id string) WebhookCloseInput {
			w, _, e := s.ReadWebhookWindow(testContext, id)
			if e != nil {
				t.Fatal(e)
			}
			in := enqueueInput(p, "unused")
			b := in.Builds[0]
			b.Name = "default"
			b.Snapshot.ComparisonKey = ComparisonKey(p.ID, "main", b.Name, []string{"default"}, b.Snapshot)
			facts := protocol.ChangeFacts{Mode: "full", Reason: "baseline_missing", TargetSHA: in.SHA, Paths: []string{}, Digest: protocol.ChangesDigest([]string{})}
			b.Snapshot.Changes = &facts
			return WebhookCloseInput{Ref: WebhookCloseRef{WindowID: id, Revision: w.Revision, PolicyVersion: policy.PolicyVersion}, Actor: a, SHA: in.SHA, Source: in.Source, File: in.File, SourceDigest: in.SourceDigest, SelectedBuilds: []string{"default"}, Builds: []PreparedBuild{b}, Comparisons: []WebhookComparison{{Name: "default", Key: b.Snapshot.ComparisonKey, Changes: facts}}}
		}
		e := hookEvent("first", strings.Repeat("1", 64))
		r, err := s.ReceiveWebhook(testContext, a, e)
		if err != nil {
			t.Fatal(err)
		}
		in := prepare(r.WindowID)
		wrong := in
		wrong.Ref.Revision++
		if _, err = s.CloseWebhookWindow(testContext, wrong); !errors.Is(err, ErrConflict) {
			t.Fatal("CAS未拒", err)
		}
		first, err := s.CloseWebhookWindow(testContext, in)
		if err != nil || first.State != "closed" || len(first.BuildIDs) != 1 {
			t.Fatal(first, err)
		}
		repeat, err := s.CloseWebhookWindow(testContext, in)
		if err != nil || !repeat.Replayed || repeat.BatchID != first.BatchID {
			t.Fatal("关闭重投", repeat, err)
		}
		e = hookEvent("next", strings.Repeat("2", 64))
		r, err = s.ReceiveWebhook(testContext, a, e)
		if err != nil {
			t.Fatal(err)
		}
		again, err := s.CloseWebhookWindow(testContext, prepare(r.WindowID))
		if err != nil || again.BatchID != "" || len(again.ReusedBuildIDs) != 1 || again.BuildIDs[0] != first.BuildIDs[0] {
			t.Fatal("sameSHA自动再执行", again, err)
		}
		p, _ = s.GetProject(testContext, p.Name)
		if p.NextNumber != 2 {
			t.Fatal("复用分配新编号", p.NextNumber)
		}
	})
}
func TestWebhookBaselineUsesExactKnownTerminalReceiptAndRetryFreezesChanges(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		a, session, p, policy := leaseFixture(t, s)
		in := enqueueInput(p, "manual-baseline")
		b := &in.Builds[0]
		b.Snapshot.ComparisonKey = ComparisonKey(p.ID, in.Branch, b.Name, []string{b.Name}, b.Snapshot)
		b.Snapshot.Changes = &protocol.ChangeFacts{Mode: "full", Reason: "baseline_missing", TargetSHA: in.SHA, Paths: []string{}, Digest: protocol.ChangesDigest([]string{})}
		out, e := s.Enqueue(testContext, in)
		if e != nil {
			t.Fatal(e)
		}
		base, e := s.FindWebhookBaseline(testContext, b.Snapshot.ComparisonKey)
		if e != nil || base != nil {
			t.Fatal("queued被猜成功基线", base, e)
		}
		grant, e := s.Claim(testContext, a, protocol.ClaimRequest{SessionID: session.SessionID, ClaimKey: uuid.NewString()}, policy)
		if e != nil || grant == nil || grant.Task.Changes == nil {
			t.Fatal("节点未收到冻结事实", e)
		}
		seq := completeRunEvents(t, s, a, *grant, false)
		terminal := event(grant.Ref, seq+1, protocol.ExecutionProgress{Kind: "build_finished", Status: "succeeded", Started: true, StopConfirmed: true, RemainingPostBudgetNS: int64(2*time.Minute) - 100, ArtifactSteps: []protocol.ArtifactExpectation{}})
		accept(t, s, a, terminal)
		base, e = s.FindWebhookBaseline(testContext, b.Snapshot.ComparisonKey)
		if e != nil || base == nil || base.BuildID != out.Builds[0].ID || base.ReceiptDigest != terminal.Digest || base.Seq != terminal.Seq {
			t.Fatal("成功不是精确中央终态", base, e)
		}
		retried, e := s.Retry(testContext, localAdmin, RetryInput{BuildID: out.Builds[0].ID, Key: "retry-preserves"})
		if e != nil {
			t.Fatal(e)
		}
		var row buildRecord
		if e = s.db.First(&row, "id = ?", retried.Builds[0].ID).Error; e != nil {
			t.Fatal(e)
		}
		frozen, e := decodeHookSnapshot(row.SnapshotJSON)
		if e != nil || frozen.Changes == nil || frozen.ComparisonKey != b.Snapshot.ComparisonKey || frozen.AutomaticWindowID != "" {
			t.Fatal("retry重计算事实", frozen, e)
		}
	})
}
func TestWebhookWholeBatchRealSQLRollbackAndTwentyReceivers(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		p, policy, a := hookFixture(t, s, 0)
		p, _ = s.GetProject(testContext, p.Name)
		settings := p.Settings
		settings.Triggers.Builds = []string{"first", "second"}
		policy.BuildNames = settings.Triggers.Builds
		var e error
		policy, e = s.ConfigureWebhook(testContext, localAdmin, WebhookPolicyInput{Policy: policy, Settings: settings, ExpectedPolicyVersion: p.PolicyVersion})
		if e != nil {
			t.Fatal(e)
		}
		a.PolicyVersion = policy.PolicyVersion
		p, _ = s.GetProject(testContext, p.Name)
		receipts := make(chan WebhookReceipt, 20)
		errs := make(chan error, 20)
		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, e := s.ReceiveWebhook(testContext, a, hookEvent("concurrent", strings.Repeat("e", 64)))
				receipts <- r
				errs <- e
			}()
		}
		wg.Wait()
		close(receipts)
		close(errs)
		for e := range errs {
			if e != nil {
				t.Fatal(e)
			}
		}
		id := ""
		first := 0
		for r := range receipts {
			if id == "" {
				id = r.WindowID
			}
			if id != r.WindowID {
				t.Fatal("竞争多窗口")
			}
			if !r.Replayed {
				first++
			}
		}
		if first != 1 {
			t.Fatal("首次接收不唯一", first)
		}
		w, _, e := s.ReadWebhookWindow(testContext, id)
		if e != nil {
			t.Fatal(e)
		}
		template := enqueueInput(p, "unused")
		facts := protocol.ChangeFacts{Mode: "full", Reason: "baseline_missing", TargetSHA: template.SHA, Paths: []string{}, Digest: protocol.ChangesDigest([]string{})}
		in := WebhookCloseInput{Ref: WebhookCloseRef{WindowID: id, Revision: w.Revision, PolicyVersion: w.PolicyVersion}, Actor: a, SHA: template.SHA, Source: template.Source, File: template.File, SourceDigest: template.SourceDigest, SelectedBuilds: policy.BuildNames}
		for _, name := range policy.BuildNames {
			b := template.Builds[0]
			b.Name = name
			b.Snapshot.Changes = &facts
			b.Snapshot.ComparisonKey = ComparisonKey(p.ID, "main", name, policy.BuildNames, b.Snapshot)
			in.Builds = append(in.Builds, b)
			in.Comparisons = append(in.Comparisons, WebhookComparison{Name: name, Key: b.Snapshot.ComparisonKey, Changes: facts})
		}
		if e = s.db.Exec("CREATE UNIQUE INDEX only_own_hook_fixture_project ON builds(project_id)").Error; e != nil {
			t.Fatal(e)
		}
		if _, e = s.CloseWebhookWindow(testContext, in); e == nil {
			t.Fatal("真实第二INSERT失败被吞")
		}
		after, _, e := s.ReadWebhookWindow(testContext, id)
		if e != nil || after.State != "pending" || after.BatchID != "" {
			t.Fatal("失败部分关闭", after, e)
		}
		current, _ := s.GetProject(testContext, p.Name)
		if current.NextNumber != 1 {
			t.Fatal("回滚失号", current.NextNumber)
		}
		status, _ := s.Status(testContext)
		if status.Queued != 0 {
			t.Fatal("部分构建残留", status)
		}
		if e = s.db.Exec("DROP INDEX only_own_hook_fixture_project").Error; e != nil {
			t.Fatal(e)
		}
		result, e := s.CloseWebhookWindow(testContext, in)
		if e != nil || len(result.BuildIDs) != 2 {
			t.Fatal("重新真实提交", result, e)
		}
		current, _ = s.GetProject(testContext, p.Name)
		if current.NextNumber != 3 {
			t.Fatal("重新提交分号错误", current.NextNumber)
		}
	})
}
