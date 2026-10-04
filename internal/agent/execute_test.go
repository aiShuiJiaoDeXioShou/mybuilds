//go:build darwin || linux

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"golang.org/x/sys/unix"
	"io"
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

func TestServeActualClaimRunProgressAndCentralLog(t *testing.T)      { serveActualTask(t, false) }
func TestServeActualArtifactSnapshotAndCentralMetadata(t *testing.T) { serveActualTask(t, true) }
func TestServeRenewsDuringActualLongProcess(t *testing.T)            { serveActualTask(t, false, "renew") }
func TestServeActualUserCancelStillRunsAlways(t *testing.T)          { serveActualTask(t, false, "cancel") }
func TestServeActualLostEventReceiptRetriesSameExecution(t *testing.T) {
	serveActualTask(t, false, "lost_ack")
}
func TestServeActualDisabledNodeStopsWithoutAlways(t *testing.T) {
	serveActualTask(t, false, "disable")
}
func TestServeActualInvalidPrivateSecretsHasZeroActionTerminal(t *testing.T) {
	serveActualTask(t, false, "secrets_invalid")
}
func TestServeActualForbiddenTokenReferenceHasZeroActionTerminal(t *testing.T) {
	serveActualTask(t, false, "secret_token")
}
func TestServeActualLostTerminalReceiptRetainsJournalAndRejectsRestart(t *testing.T) {
	serveActualTask(t, false, "terminal_lost_ack")
}
func TestServeActualRenewLossConfirmsStoppedOnlyAfterExpiry(t *testing.T) {
	serveActualTask(t, false, "renew_network_failure")
}
func serveActualTask(t *testing.T, artifact bool, modes ...string) {
	mode := ""
	if len(modes) > 0 {
		mode = modes[0]
	}
	st, admin, cfg, handler := agentControl(t)
	var dropped atomic.Bool
	if mode == "renew_network_failure" {
		original := handler
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			original.ServeHTTP(w, r)
			if r.URL.Path == "/api/agent/renew" {
				time.Sleep(2 * time.Second)
			}
		})
	}
	if mode == "lost_ack" || mode == "terminal_lost_ack" {
		original := handler
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			terminal := false
			if mode == "terminal_lost_ack" && r.URL.Path == "/api/agent/events" {
				data, e := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
				if e != nil {
					t.Fatal(e)
				}
				r.Body = io.NopCloser(bytes.NewReader(data))
				var event protocol.ExecutionEvent
				if json.Unmarshal(data, &event) == nil {
					terminal = event.Progress.Kind == "build_finished"
				}
			}
			original.ServeHTTP(w, r)
			if (mode == "lost_ack" || terminal) && r.URL.Path == "/api/agent/events" && dropped.CompareAndSwap(false, true) {
				conn, _, e := w.(http.Hijacker).Hijack()
				if e == nil {
					conn.Close()
				}
			}
		})
	}
	control := httptest.NewServer(handler)
	defer control.Close()
	cfg.Server = control.URL
	repo := t.TempDir()
	git, _ := exec.LookPath("git")
	fixture := func(args ...string) {
		t.Helper()
		cmd := exec.Command(git, args...)
		cmd.Dir = repo
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + repo, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_AUTHOR_NAME=fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid", "GIT_COMMITTER_NAME=fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid"}
		if _, err := cmd.CombinedOutput(); err != nil {
			t.Fatal("fixture git", err)
		}
	}
	fixture("init", "--initial-branch=main", "--template=")
	source := `version: 1
steps:
 - kind: run
   name: actual
   env:
    ACTUAL_NODE: "{{node.name}}"
   run: test "$ACTUAL_NODE" = actual-node; printf actual; printf once >> "$MYBUILDS_WORKSPACE/count"; printf snapshot > output.apk
post:
 always:
  - kind: run
    name: cleanup
    run: printf cleanup; printf cleanup > "$MYBUILDS_WORKSPACE/cleanup"
`
	if mode == "renew" {
		source = strings.Replace(source, "printf actual;", "printf actual; sleep 9;", 1)
	}
	if mode == "cancel" || mode == "disable" || mode == "renew_network_failure" {
		source = strings.Replace(source, "printf actual;", "printf actual; sleep 8;", 1)
	}
	if mode == "secrets_invalid" || mode == "secret_token" {
		cfg.SecretsFile = filepath.Join(t.TempDir(), "secrets.env")
		content := "invalid private-test-marker syntax"
		if mode == "secret_token" {
			cfg.TokenEnv = "OWN_NODE_TOKEN"
			content = "OWN_NODE_TOKEN=private-test-marker\n"
			source = strings.Replace(source, "steps:", "env:\n LEAK: ${OWN_NODE_TOKEN}\nsteps:", 1)
		}
		if err := os.WriteFile(cfg.SecretsFile, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if artifact {
		source = strings.Replace(source, "post:", " - kind: artifact\n   name: collect\n   paths: [output.apk]\npost:", 1)
	}
	if err := os.WriteFile(filepath.Join(repo, "mybuilds.yml"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	fixture("add", "mybuilds.yml")
	fixture("commit", "-m", "own")
	if _, err := st.CreateProject(context.Background(), admin, store.ProjectInput{Name: "actual", Repository: repo, Branches: []string{"main"}, AllowedNodes: []string{cfg.Node}, DefaultNode: cfg.Node}); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(server.TriggerRequest{Branch: "main"})
	request, _ := http.NewRequest("POST", control.URL+"/api/projects/actual/builds", bytes.NewReader(data))
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "actual-claim")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var batch server.BatchView
	if err = json.NewDecoder(response.Body).Decode(&batch); err != nil || response.StatusCode != 201 || len(batch.Builds) != 1 {
		t.Fatal("trigger", response.StatusCode, batch, err)
	}
	t.Setenv("JAVA_HOME", filepath.Join(t.TempDir(), "missing"))
	t.Setenv("ANDROID_HOME", filepath.Join(t.TempDir(), "missing"))
	t.Setenv("ANDROID_SDK_ROOT", "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, cfg) }()
	expireCtx, stopExpire := context.WithCancel(context.Background())
	defer stopExpire()
	if mode == "renew_network_failure" {
		go func() {
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-expireCtx.Done():
					return
				case <-ticker.C:
					st.ExpireLeases(expireCtx)
				}
			}
		}()
	}
	deadline := time.Now().Add(20 * time.Second)
	changed := false
	startedPGID := 0
	for time.Now().Before(deadline) {
		view, err := st.GetBuild(context.Background(), batch.Builds[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		if !changed && (mode == "cancel" || mode == "disable") && len(view.Steps) > 0 && view.Steps[0].Started {
			changed = true
			files, _ := filepath.Glob(filepath.Join(cfg.DataDir, "journal", "*.json"))
			for _, file := range files {
				data, e := os.ReadFile(file)
				var state journalState
				if e == nil && json.Unmarshal(data, &state) == nil && state.Ref != nil {
					startedPGID = state.PGID
				}
			}
			if mode == "cancel" {
				_, err = st.Cancel(context.Background(), admin, view.ID)
			} else {
				err = st.SetNodeState(context.Background(), admin, cfg.Node, "disabled")
			}
			if err != nil {
				t.Fatal(err)
			}
		}
		if (mode == "secrets_invalid" || mode == "secret_token") && view.Status == "failed" {
			if view.Reason != "precheck_error" || view.Steps[0].Started || view.Steps[0].Intent || view.Post[0].Started || view.StopUnconfirmed {
				t.Fatal("invalid zero action", view)
			}
			paths, _ := filepath.Glob(filepath.Join(cfg.DataDir, "scm", "checkout-*"))
			if mode == "secrets_invalid" && len(paths) != 0 {
				t.Fatal("private precheck performed checkout")
			}
			waitAttemptJournalRemoved(t, cfg.DataDir, view.ID)
			cancel()
			if err := <-done; err != nil {
				t.Fatal(err)
			}

			return
		}
		if mode == "renew_network_failure" && view.Status == "interrupted" && !view.StopUnconfirmed {
			if view.Reason != "lease_expired" || view.Post[0].Started {
				t.Fatal("expired receipt fabricated terminal/post", view)
			}
			paths, _ := filepath.Glob(filepath.Join(cfg.DataDir, "scm", "checkout-*", "workspace", "cleanup"))
			if len(paths) != 0 {
				t.Fatal("always after renew loss")
			}
			waitAttemptJournalRemoved(t, cfg.DataDir, view.ID)
			cancel()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			return
		}
		if mode == "terminal_lost_ack" && view.Status == "succeeded" {
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("lost terminal receipt reported local success")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("terminal receipt failure did not stop Serve")
			}
			if !dropped.Load() || view.StopUnconfirmed {
				t.Fatal("terminal loss invented central guard")
			}
			files, _ := filepath.Glob(filepath.Join(cfg.DataDir, "journal", "*.json"))
			found := false
			for _, file := range files {
				data, e := os.ReadFile(file)
				var state journalState
				if e != nil || json.Unmarshal(data, &state) != nil {
					t.Fatal(e)
				}
				if state.Ref != nil && state.Ref.BuildID == view.ID {
					found = true
					if state.PendingEvent == nil || state.PendingEvent.Progress.Kind != "build_finished" || !state.StopConfirmed || state.CleanupFailed {
						t.Fatal("missing pending terminal proof")
					}
				}
			}
			if !found {
				t.Fatal("terminal ACK loss dropped journal")
			}
			node, e := st.GetNode(context.Background(), admin, cfg.Node)
			if e != nil {
				t.Fatal(e)
			}
			if err := Serve(context.Background(), cfg); err == nil || err.Error() != "agent_journal_unconfirmed" {
				t.Fatal("restart bypassed journal", err)
			}
			after, e := st.GetNode(context.Background(), admin, cfg.Node)
			if e != nil || after.LastHeartbeat == nil || node.LastHeartbeat == nil || !after.LastHeartbeat.Equal(*node.LastHeartbeat) {
				t.Fatal("restart changed session", e)
			}
			paths, _ := filepath.Glob(filepath.Join(cfg.DataDir, "scm", "checkout-*", "workspace", "count"))
			if len(paths) != 1 {
				t.Fatal("duplicate execution")
			}
			count, e := os.ReadFile(paths[0])
			if e != nil || string(count) != "once" {
				t.Fatal("duplicate execution", e)
			}
			return
		}
		if view.Status == "succeeded" || mode == "cancel" && view.Status == "cancelled" {
			if view.NodeName != cfg.Node || view.AttemptID == "" || len(view.Steps) != 1+btoi(artifact) || !view.Steps[0].Started || !view.Steps[0].StopConfirmed || view.Steps[0].ElapsedNS <= 0 || len(view.Post) != 1 || !view.Post[0].Started {
				t.Fatal(view)
			}
			logs, err := st.ListLogChunks(context.Background(), admin, view.ID, 0, store.Page{})
			if err != nil || len(logs) == 0 {
				t.Fatal("no central logs", err)
			}
			if artifact {
				files, e := st.ListArtifacts(context.Background(), admin, view.ID, store.Page{})
				if e != nil || len(files) != 1 || files[0].Size != 8 {
					t.Fatal("central artifact", files, e)
				}
				req, _ := http.NewRequest("GET", control.URL+"/api/artifacts/"+files[0].ID+"/"+files[0].Name, nil)
				req.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
				reply, e := http.DefaultClient.Do(req)
				if e != nil {
					t.Fatal(e)
				}
				body, e := io.ReadAll(reply.Body)
				reply.Body.Close()
				if e != nil || reply.StatusCode != 200 || string(body) != "snapshot" {
					t.Fatal("snapshot download", reply.StatusCode, string(body), e)
				}
			}
			waitAttemptJournalRemoved(t, cfg.DataDir, view.ID)
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("serve shutdown blocked")
			}
			paths, err := filepath.Glob(filepath.Join(cfg.DataDir, "scm", "checkout-*", "workspace", "count"))
			if mode == "cancel" {
				paths, err = filepath.Glob(filepath.Join(cfg.DataDir, "scm", "checkout-*", "workspace", "cleanup"))
			}
			if err != nil || len(paths) != 1 {
				t.Fatal("duplicate/no execution", paths, err)
			}
			count, err := os.ReadFile(paths[0])
			expected := "once"
			if mode == "cancel" {
				expected = "cleanup"
			}
			if err != nil || string(count) != expected {
				t.Fatal(string(count), err)
			}

			return
		}
		select {
		case err := <-done:
			if mode == "disable" && changed && err != nil {
				paths, _ := filepath.Glob(filepath.Join(cfg.DataDir, "scm", "checkout-*", "workspace", "cleanup"))
				if len(paths) != 0 {
					t.Fatal("always after authority loss")
				}
				journals, _ := filepath.Glob(filepath.Join(cfg.DataDir, "journal", "*.json"))
				var state journalState
				found := false
				for _, file := range journals {
					data, e := os.ReadFile(file)
					var candidate journalState
					if e != nil || json.Unmarshal(data, &candidate) != nil {
						t.Fatal(e)
					}
					if candidate.Ref != nil {
						state = candidate
						found = true
					}
				}
				view, e := st.GetBuild(context.Background(), view.ID)
				if e != nil {
					t.Fatal(e)
				}
				if found && (!state.StopConfirmed || state.CleanupFailed) || !found && view.StopUnconfirmed {
					t.Fatalf("missing actual group cleanup found=%v stop=%v cleanupFailed=%v started=%v pid=%d", found, state.StopConfirmed, state.CleanupFailed, state.Started, state.PID)
				}
				if startedPGID <= 0 || unix.Kill(-startedPGID, 0) == nil {
					t.Fatal("process group survived")
				}
				return
			}
			t.Fatalf("Serve stopped: %v (%s/%s)", err, view.Status, view.Reason)
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Fatal("no executed terminal")
}

func btoi(value bool) int {
	if value {
		return 1
	}
	return 0
}

func waitAttemptJournalRemoved(t *testing.T, directory, buildID string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		files, _ := filepath.Glob(filepath.Join(directory, "journal", "*.json"))
		active := false
		for _, file := range files {
			data, e := os.ReadFile(file)
			var state journalState
			if e == nil && json.Unmarshal(data, &state) == nil && state.Ref != nil && state.Ref.BuildID == buildID {
				active = true
			}
		}
		if !active {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("confirmed attempt journal was not removed before shutdown")
}
