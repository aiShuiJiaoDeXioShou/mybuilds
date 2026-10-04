package pipeline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"mybuilds/internal/protocol"
)

func remoteOptions(t *testing.T) *RemoteOptions {
	t.Helper()
	parent := t.TempDir()
	if err := os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	return &RemoteOptions{AuthorityContext: context.Background(), ResultParent: parent, RemainingPostBudgetNS: int64(time.Second), Progress: func(context.Context, protocol.ExecutionProgress) error { return nil }, Log: func(context.Context, protocol.LogRecord) error { return nil }}
}
func TestRemoteRequiresSingleSelectionAndDoesNotReadHostSecrets(t *testing.T) {
	root := t.TempDir()
	d := localDocument(t, `version: 1
env: {SECRET: "${REMOTE_SECRET}"}
steps:
 - kind: run
   run: printf '%s' "$SECRET" > value
`)
	t.Setenv("REMOTE_SECRET", "host-secret")
	remote := remoteOptions(t)
	if result, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: root, Remote: remote}); err == nil || result != nil {
		t.Fatal(result, err)
	}
	requireAbsent(t, root, "value")
	remote.Secrets = map[string]string{"REMOTE_SECRET": "task-secret"}
	for _, preview := range []PreviewOptions{{All: true}, {Step: "step1"}} {
		if _, err := runWithCleanup(t, context.Background(), d, RunOptions{PreviewOptions: preview, Workspace: root, Remote: remote}); err == nil {
			t.Fatal("remote shortcut accepted")
		}
	}
	if _, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: root, Remote: remote}); err != nil {
		t.Fatal(err)
	}
	requireFile(t, root, "value", "task-secret")
}
func TestRemotePersistenceAndAuthorityCloseAllPost(t *testing.T) {
	for _, failureKind := range []string{"intent", "started", "finished", "log", "authority"} {
		t.Run(failureKind, func(t *testing.T) {
			root := t.TempDir()
			d := localDocument(t, `version: 1
steps:
 - kind: run
   name: action
   run: printf hello; sleep 0.1
post:
 always:
  - kind: run
    run: touch forbidden-always
`)
			authority, cancel := context.WithCancel(context.Background())
			defer cancel()
			remote := remoteOptions(t)
			remote.AuthorityContext = authority
			remote.Progress = func(_ context.Context, p protocol.ExecutionProgress) error {
				if p.Kind == failureKind {
					return errors.New("private-persistence")
				}
				if failureKind == "authority" && p.Kind == "started" {
					cancel()
				}
				return nil
			}
			if failureKind == "log" {
				remote.Log = func(context.Context, protocol.LogRecord) error { return errors.New("private-log") }
			}
			if _, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: root, Remote: remote}); err == nil {
				t.Fatal("failure accepted")
			}
			requireAbsent(t, root, "forbidden-always")
		})
	}
}
func TestRemoteUserCancelKeepsAuthorityForAlways(t *testing.T) {
	root := t.TempDir()
	d := localDocument(t, `version: 1
steps:
 - kind: run
   run: sleep 60
post:
 always:
  - kind: run
    run: printf done > always
`)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	remote := remoteOptions(t)
	remote.Progress = func(_ context.Context, p protocol.ExecutionProgress) error {
		if p.Kind == "started" && p.Phase == "ordinary" {
			cancel()
		}
		return nil
	}
	result, err := runWithCleanup(t, ctx, d, RunOptions{Workspace: root, Remote: remote})
	if err == nil || result == nil {
		t.Fatal(result, err)
	}
	requireFile(t, root, "always", "done")
}
func TestRemoteNanosecondBudgetAndActualStructuredEvidence(t *testing.T) {
	root := t.TempDir()
	d := localDocument(t, `version: 1
steps:
 - kind: run
   name: create
   run: printf private-task-secret; printf artifact > result.apk
 - kind: artifact
   name: snapshot
   paths: [result.apk]
post:
 failure:
  - kind: run
    name: unselected
    run: touch forbidden
`)
	remote := remoteOptions(t)
	remote.Secrets = map[string]string{}
	var mu sync.Mutex
	var events []protocol.ExecutionProgress
	var logs []protocol.LogRecord
	remote.Progress = func(_ context.Context, p protocol.ExecutionProgress) error {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, p)
		return nil
	}
	remote.Log = func(_ context.Context, l protocol.LogRecord) error {
		mu.Lock()
		defer mu.Unlock()
		logs = append(logs, l)
		return nil
	}
	result, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: root, Remote: remote})
	if err != nil {
		t.Fatal(err)
	}
	realParent, parentErr := filepath.EvalSymlinks(remote.ResultParent)
	if parentErr != nil || result.ResultDir == "" || !strings.HasPrefix(result.ResultDir, realParent+string(filepath.Separator)) {
		t.Fatal(result)
	}
	var snapshot, selected, unselected, finished bool
	for _, p := range events {
		if p.Kind == "finished" && p.Phase == "ordinary" && p.Index == 2 {
			snapshot = true
			if len(p.LocalArtifacts) != 1 || p.LocalArtifacts[0].Name != "result.apk" || p.LocalArtifacts[0].Size != 8 || p.ElapsedNS <= 0 || !p.Started || !p.StopConfirmed {
				t.Fatal(p)
			}
		}
		if p.Kind == "post_selected" && p.PostPhase == "success" {
			selected = true
		}
		if p.Kind == "skipped" && p.Phase == "failure" && p.Index == 1 && p.Reason == "not_selected" {
			unselected = true
		}
		if p.Kind == "build_finished" && p.Status == "succeeded" {
			finished = true
		}
	}
	if !snapshot || !selected || !unselected || !finished || len(logs) == 0 {
		t.Fatal(events, logs)
	}
	for _, log := range logs {
		if log.Index < 1 || log.Phase == "" || log.UTC.Location() != time.UTC {
			t.Fatal(log)
		}
	}
	// intent持久化延迟真实扣除纳秒预算，不能按process毫秒统计漏掉。
	root = t.TempDir()
	zero := int64(10 * time.Millisecond)
	remote = remoteOptions(t)
	remote.RemainingBudgetNS = &zero
	remote.Progress = func(_ context.Context, p protocol.ExecutionProgress) error {
		if p.Kind == "intent" {
			time.Sleep(15 * time.Millisecond)
		}
		return nil
	}
	result, err = runWithCleanup(t, context.Background(), d, RunOptions{Workspace: root, Remote: remote})
	if err == nil {
		t.Fatal(result)
	}
	requireAbsent(t, root, "result.apk")
}

func TestRemoteZeroBudgetAndAllSkippedHaveNoResultDirectory(t *testing.T) {
	for _, allSkipped := range []bool{false, true} {
		t.Run(map[bool]string{false: "zero", true: "conditions"}[allSkipped], func(t *testing.T) {
			root := t.TempDir()
			source := `version: 1
steps:
 - kind: run
   name: action
   run: touch forbidden
post:
 always:
  - kind: run
    name: cleanup
    run: touch forbidden-post
`
			if allSkipped {
				source = strings.Replace(source, "   run: touch forbidden\n", "   when: {params: {enabled: yes}}\n   run: touch forbidden\n", 1)
				source = strings.Replace(source, "version: 1", "version: 1\nparams: {enabled: no}", 1)
			}
			d := localDocument(t, source)
			remote := remoteOptions(t)
			var last protocol.ExecutionProgress
			var post bool
			remote.Progress = func(_ context.Context, p protocol.ExecutionProgress) error {
				if p.Kind == "build_finished" {
					last = p
				}
				if p.Kind == "post_selected" && p.PostPhase == "none" {
					post = true
				}
				return nil
			}
			if !allSkipped {
				zero := int64(0)
				remote.RemainingBudgetNS = &zero
			}
			result, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: root, Remote: remote})
			if result == nil || result.ResultDir != "" || last.Started || !last.StopConfirmed || !post {
				t.Fatal(result, last, post, err)
			}
			if allSkipped && (err != nil || last.Status != "skipped" || last.Reason != "condition") {
				t.Fatal(last, err)
			}
			if !allSkipped && (err == nil || last.Status != "failed" || last.Reason != "timeout" || last.RemainingBudgetNS == nil || *last.RemainingBudgetNS != 0) {
				t.Fatal(last, err)
			}
			requireAbsent(t, root, "forbidden")
			requireAbsent(t, root, "forbidden-post")
		})
	}
}

func TestRemoteStructuredLogsReuseActualSecretRedaction(t *testing.T) {
	root := t.TempDir()
	d := localDocument(t, `version: 1
env: {SECRET: "${TASK_SECRET}"}
steps:
 - kind: run
   name: output
   run: printf '%s\n' "$SECRET"; printf '%s\n' "$SECRET" >&2
post:
 always:
  - kind: run
    name: cleanup
    run: printf '%s\n' "$SECRET"
`)
	secret := strings.Repeat("真实密钥", 1500)
	remote := remoteOptions(t)
	remote.Secrets = map[string]string{"TASK_SECRET": secret}
	var mu sync.Mutex
	var records []protocol.LogRecord
	remote.Log = func(_ context.Context, r protocol.LogRecord) error {
		mu.Lock()
		defer mu.Unlock()
		records = append(records, r)
		return nil
	}
	result, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: root, Remote: remote})
	if err != nil {
		t.Fatal(err)
	}
	var ordinary, always bool
	for _, record := range records {
		if strings.Contains(record.Text, "真实密钥") {
			t.Fatal("secret in structured log")
		}
		if record.Phase == "ordinary" && record.Index == 1 && record.Step == "output" && record.Text == "[REDACTED]" {
			ordinary = true
		}
		if record.Phase == "always" && record.Index == 1 && record.Step == "cleanup" && record.Text == "[REDACTED]" {
			always = true
		}
	}
	if !ordinary || !always {
		t.Fatal(records)
	}
	for _, step := range append(result.Builds[0].Steps, result.Builds[0].Post...) {
		if step.LogPath != "" {
			data, err := os.ReadFile(filepath.Join(result.ResultDir, step.LogPath))
			if err != nil || strings.Contains(string(data), "真实密钥") {
				t.Fatal(err)
			}
		}
	}
}

func TestRemoteLastFinishedConfirmationConsumesBudgetWithoutRewritingStep(t *testing.T) {
	root := t.TempDir()
	d := localDocument(t, `version: 1
timeout: 100ms
steps:
 - kind: run
   name: action
   run: printf done > action
post:
 failure:
  - kind: run
    name: failure
    run: printf failure > failure
 always:
  - kind: run
    name: always
    run: printf always > always
`)
	budget := int64(100 * time.Millisecond)
	remote := remoteOptions(t)
	remote.RemainingBudgetNS = &budget
	var selected protocol.ExecutionProgress
	remote.Progress = func(_ context.Context, p protocol.ExecutionProgress) error {
		if p.Kind == "finished" && p.Phase == "ordinary" {
			if p.Status != "succeeded" {
				t.Fatal(p)
			}
			time.Sleep(150 * time.Millisecond)
		}
		if p.Kind == "post_selected" {
			selected = p
		}
		return nil
	}
	result, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: root, Remote: remote})
	if result == nil || err == nil || result.Builds[0].Status != "failed" || result.Builds[0].Reason != "timeout" || result.Builds[0].Steps[0].Status != "succeeded" {
		t.Fatal(result, err)
	}
	if selected.PostPhase != "failure" || selected.Reason != "timeout" || selected.RemainingBudgetNS == nil || *selected.RemainingBudgetNS != 0 {
		t.Fatal(selected)
	}
	requireFile(t, root, "failure", "failure")
	requireFile(t, root, "always", "always")
}

func TestRemoteLastPostConfirmationConsumesPostBudgetWithoutRewritingStep(t *testing.T) {
	root := t.TempDir()
	d := localDocument(t, `version: 1
steps:
 - kind: run
   name: action
   run: printf action > action
post:
 always:
  - kind: run
    name: cleanup
    run: printf cleanup > cleanup
`)
	remote := remoteOptions(t)
	remote.RemainingPostBudgetNS = int64(100 * time.Millisecond)
	var last protocol.ExecutionProgress
	remote.Progress = func(_ context.Context, p protocol.ExecutionProgress) error {
		if p.Kind == "finished" && p.Phase == "always" {
			if p.Status != "succeeded" {
				t.Fatal(p)
			}
			time.Sleep(150 * time.Millisecond)
		}
		if p.Kind == "build_finished" {
			last = p
		}
		return nil
	}
	result, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: root, Remote: remote})
	if err == nil || result.Builds[0].Status != "failed" || result.Builds[0].Reason != "post_error" || result.Builds[0].Post[0].Status != "succeeded" || last.Status != "failed" || last.Reason != "post_error" || last.RemainingPostBudgetNS != 0 {
		t.Fatal(result, last, err)
	}
}
