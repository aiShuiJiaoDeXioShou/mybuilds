//go:build darwin || linux

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"golang.org/x/sys/unix"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"mybuilds/internal/server"
	"mybuilds/internal/store"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// 夹具仅执行自有Git/HTTP/文件动作，故障发生在真实中央确认之后。
func actualFaultControl(t *testing.T, source, fault string) (*store.Store, store.Actor, config.AgentConfig, *httptest.Server, string) {
	t.Helper()
	st, admin, cfg, handler := agentControl(t)
	repo := t.TempDir()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command(git, args...)
		cmd.Dir = repo
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + repo, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null"}
		if _, err := cmd.CombinedOutput(); err != nil {
			t.Fatal("own git fixture", err)
		}
	}
	runGit("init", "--initial-branch=main", "--template=")
	if err := os.WriteFile(filepath.Join(repo, "mybuilds.yml"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	runGit("add", "mybuilds.yml")
	runGit("-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "-m", "own")
	if _, err := st.CreateProject(context.Background(), admin, store.ProjectInput{Name: "fault", Repository: repo, Branches: []string{"main"}, AllowedNodes: []string{cfg.Node}, DefaultNode: cfg.Node}); err != nil {
		t.Fatal(err)
	}
	var changed atomic.Bool
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r)
		if fault == "journal_permissions" && r.URL.Path == "/api/agent/logs" && changed.CompareAndSwap(false, true) {
			if err := os.Chmod(filepath.Join(cfg.DataDir, "journal"), 0500); err != nil {
				t.Error(err)
			}
		}
	}))
	t.Cleanup(control.Close)
	cfg.Server = control.URL
	data, _ := json.Marshal(server.TriggerRequest{Branch: "main", All: true})
	req, _ := http.NewRequest("POST", control.URL+"/api/projects/fault/builds", bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "own-fault")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var batch server.BatchView
	err = json.NewDecoder(response.Body).Decode(&batch)
	response.Body.Close()
	if err != nil || response.StatusCode != 201 || len(batch.Builds) == 0 {
		t.Fatal("trigger", response.StatusCode, err)
	}
	t.Setenv("JAVA_HOME", filepath.Join(t.TempDir(), "missing"))
	t.Setenv("ANDROID_HOME", filepath.Join(t.TempDir(), "missing"))
	t.Setenv("ANDROID_SDK_ROOT", "")
	return st, admin, cfg, control, batch.Builds[0].ID
}
func TestActualAuthorityLossDuringAlwaysReapsOwnGroupOnly(t *testing.T) {
	source := `version: 1
steps:
 - kind: run
   name: ordinary
   run: printf ordinary > ordinary
post:
 always:
  - kind: run
    name: cleanup
    run: printf 'begin\n'; sleep 8; printf forbidden > after-cleanup
`
	st, admin, cfg, _, id := actualFaultControl(t, source, "")
	unrelated := exec.Command("sleep", "30")
	if err := unrelated.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { unrelated.Process.Kill(); unrelated.Wait() })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, cfg) }()
	deadline := time.Now().Add(12 * time.Second)
	pgid := 0
	disabled := false
	for time.Now().Before(deadline) {
		view, err := st.GetBuild(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if !disabled && len(view.Post) > 0 && view.Post[0].Started {
			files, _ := filepath.Glob(filepath.Join(cfg.DataDir, "journal", "*.json"))
			for _, file := range files {
				data, e := os.ReadFile(file)
				var state journalState
				if e == nil && json.Unmarshal(data, &state) == nil && state.Ref != nil && state.Ref.BuildID == id {
					pgid = state.PGID
				}
			}
			if err := st.SetNodeState(context.Background(), admin, cfg.Node, "disabled"); err != nil {
				t.Fatal(err)
			}
			disabled = true
		}
		select {
		case err := <-done:
			if !disabled || err == nil || pgid <= 0 || unix.Kill(-pgid, 0) == nil || unix.Kill(unrelated.Process.Pid, 0) != nil {
				t.Fatal("wrong physical authority cleanup", err)
			}
			paths, _ := filepath.Glob(filepath.Join(cfg.DataDir, "scm", "checkout-*", "workspace", "after-cleanup"))
			if len(paths) != 0 {
				t.Fatal("always continued after authority loss")
			}
			final, e := st.GetBuild(context.Background(), id)
			if e != nil || final.Status != "interrupted" || !final.Post[0].Started || final.Steps[0].Status != "succeeded" {
				t.Fatal("missing actual post selection", e)
			}
			return
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Fatal("always authority loss did not stop process")
}
func TestActualJournalPermissionFailureAfterLogACKStopsRunAndAlways(t *testing.T) {
	source := `version: 1
steps:
 - kind: run
   name: action
   run: printf 'begin\n'; sleep 8; printf forbidden > after-action
post:
 always:
  - kind: run
    name: cleanup
    run: printf forbidden > after-cleanup
`
	st, admin, cfg, _, id := actualFaultControl(t, source, "journal_permissions")
	t.Cleanup(func() { os.Chmod(filepath.Join(cfg.DataDir, "journal"), 0700) })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, cfg) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("private save failure accepted")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("save failure did not cancel process")
	}
	for _, name := range []string{"after-action", "after-cleanup"} {
		paths, _ := filepath.Glob(filepath.Join(cfg.DataDir, "scm", "checkout-*", "workspace", name))
		if len(paths) != 0 {
			t.Fatal("action continued after save failure", name)
		}
	}
	files, _ := filepath.Glob(filepath.Join(cfg.DataDir, "journal", "*.json"))
	found := false
	for _, file := range files {
		data, e := os.ReadFile(file)
		var state journalState
		if e == nil && json.Unmarshal(data, &state) == nil && state.Ref != nil && state.Ref.BuildID == id {
			found = true
			if state.PGID > 0 && unix.Kill(-state.PGID, 0) == nil {
				t.Fatal("group not reaped")
			}
			if state.PendingLog == nil || state.LastLogSeq != 0 {
				t.Fatal("ACK released without durable cursor")
			}
		}
	}
	if !found {
		t.Fatal("lost fence journal")
	}
	chunks, err := st.ListLogChunks(context.Background(), admin, id, 0, store.Page{})
	if err != nil || len(chunks) != 1 {
		t.Fatal("missing actual central log ACK", err)
	}
	if err := Serve(context.Background(), cfg); err == nil {
		t.Fatal("restart bypassed damaged journal")
	}
}

func TestActualRevokedCredentialKeepsStoppedFenceUntilAdminConfirmation(t *testing.T) {
	source := `version: 1
steps:
 - kind: run
   name: ordinary
   run: printf 'begin\n'; sleep 8; printf forbidden > after-action
post:
 always:
  - kind: run
    name: cleanup
    run: printf forbidden > after-cleanup
`
	st, admin, cfg, _, id := actualFaultControl(t, source, "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, cfg) }()
	revoked := false
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		view, err := st.GetBuild(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if !revoked && view.Steps[0].Started {
			if err := st.RevokeNodeToken(context.Background(), admin, cfg.Node); err != nil {
				t.Fatal(err)
			}
			revoked = true
		}
		select {
		case err := <-done:
			if !revoked || err == nil {
				t.Fatal("revoke did not close authority", err)
			}
			files, _ := filepath.Glob(filepath.Join(cfg.DataDir, "journal", "*.json"))
			var proof journalState
			found := false
			for _, file := range files {
				data, e := os.ReadFile(file)
				var state journalState
				if e == nil && json.Unmarshal(data, &state) == nil && state.Ref != nil && state.Ref.BuildID == id {
					proof = state
					found = true
				}
			}
			if !found || !proof.StopConfirmed || proof.CleanupFailed || proof.PGID <= 0 || unix.Kill(-proof.PGID, 0) == nil {
				t.Fatal("missing actual stopped proof")
			}
			before, e := st.GetBuild(context.Background(), id)
			if e != nil || before.Status != "interrupted" || !before.StopUnconfirmed {
				t.Fatal("revoked credential improperly cleared guard", e)
			}
			for _, name := range []string{"after-action", "after-cleanup"} {
				paths, _ := filepath.Glob(filepath.Join(cfg.DataDir, "scm", "checkout-*", "workspace", name))
				if len(paths) != 0 {
					t.Fatal("revoke continued action")
				}
			}
			confirmation := protocol.StopConfirmation{Ref: *proof.Ref, EvidenceCode: "admin_observed_stopped", Note: "自有测试进程组已实际回收核对"}
			if err := st.ConfirmStopped(context.Background(), admin, confirmation); err != nil {
				t.Fatal(err)
			}
			after, e := st.GetBuild(context.Background(), id)
			if e != nil || after.StopUnconfirmed || after.Status != before.Status || after.Reason != before.Reason {
				t.Fatal("confirmation changed execution result", e)
			}
			if err := Serve(context.Background(), cfg); err == nil || err.Error() != "agent_journal_unconfirmed" {
				t.Fatal("007 replayed old journal after manual confirmation", err)
			}
			return
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Fatal("revoked process did not stop")
}
