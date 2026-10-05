package pipeline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mybuilds/internal/protocol"
)

func TestRemoteResultRegistrationBeforeIntent(t *testing.T) {
	workspace := t.TempDir()
	d := localDocument(t, `version: 1
steps:
 - kind: run
   run: printf registered > output
post:
 always:
  - kind: run
    run: printf stopped > cleanup
`)
	remote := remoteOptions(t)
	remaining := int64(time.Second)
	remote.RemainingBudgetNS = &remaining
	registered, intents := 0, 0
	created := ""
	remote.ResultCreated = func(ctx context.Context, dir string) error {
		registered++
		created = dir
		children, err := os.ReadDir(dir)
		parent, parentErr := filepath.EvalSymlinks(remote.ResultParent)
		if err != nil || parentErr != nil || len(children) != 0 || intents != 0 || filepath.Dir(dir) != parent {
			t.Fatal("登记须在任何日志/intent前且仅真实结果根", err)
		}
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > time.Second || time.Until(deadline) <= 0 {
			t.Fatal("登记沿原剩余预算")
		}
		return nil
	}
	remote.Progress = func(_ context.Context, p protocol.ExecutionProgress) error {
		if p.Kind == "intent" {
			intents++
			if registered != 1 || p.LocalResultDir != created {
				t.Fatal("已登记真实目录才可intent")
			}
		}
		return nil
	}
	result, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: workspace, Remote: remote})
	if err != nil || result == nil || registered != 1 || intents != 2 || result.ResultDir != created {
		t.Fatal("仅一个本次登记，普通与always共用", err, registered, intents)
	}
	requireFile(t, workspace, "output", "registered")
	requireFile(t, workspace, "cleanup", "stopped")
}

func TestRemoteResultRegistrationFailurePreservesAndStartsNoAction(t *testing.T) {
	for _, mode := range []string{"save_failure", "budget", "authority"} {
		t.Run(mode, func(t *testing.T) {
			workspace := t.TempDir()
			d := localDocument(t, `version: 1
steps:
 - kind: run
   run: touch forbidden
post:
 always:
  - kind: run
    run: touch forbidden-post
`)
			remote := remoteOptions(t)
			authority, cancel := context.WithCancel(context.Background())
			defer cancel()
			remote.AuthorityContext = authority
			remaining := int64(40 * time.Millisecond)
			remote.RemainingBudgetNS = &remaining
			created, registered, intents := "", 0, 0
			remote.ResultCreated = func(ctx context.Context, dir string) error {
				created, registered = dir, registered+1
				switch mode {
				case "budget":
					<-ctx.Done()
					return ctx.Err()
				case "authority":
					cancel()
					return ctx.Err()
				default:
					return errors.New("私有持久化失败")
				}
			}
			remote.Progress = func(_ context.Context, p protocol.ExecutionProgress) error {
				if p.Kind == "intent" || p.Kind == "started" {
					intents++
				}
				return nil
			}
			_, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: workspace, Remote: remote})
			if err == nil || registered != 1 || intents != 0 {
				t.Fatal("登记失败或失权不能执行普通/post", err, registered, intents)
			}
			if info, statErr := os.Lstat(created); statErr != nil || !info.IsDir() {
				t.Fatal("已创建而未确认目录必须保留", statErr)
			}
			requireAbsent(t, workspace, "forbidden")
			requireAbsent(t, workspace, "forbidden-post")
		})
	}
}

func TestRemoteAllSkippedDoesNotRegisterResult(t *testing.T) {
	workspace := t.TempDir()
	d := localDocument(t, `version: 1
params: {enabled: no}
steps:
 - kind: run
   when: {params: {enabled: yes}}
   run: touch forbidden
`)
	remote := remoteOptions(t)
	remote.Facts = map[string]string{"git.branch": "main"}
	called := 0
	remote.ResultCreated = func(context.Context, string) error { called++; return nil }
	result, err := runWithCleanup(t, context.Background(), d, RunOptions{Workspace: workspace, Remote: remote})
	if err != nil || called != 0 || result == nil || result.ResultDir != "" {
		t.Fatal("无动作分支不造结果根或登记", err, called)
	}
	requireAbsent(t, workspace, "forbidden")
}
