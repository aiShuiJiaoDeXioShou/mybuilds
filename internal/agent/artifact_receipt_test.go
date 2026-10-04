//go:build darwin || linux

package agent

import (
	"bytes"
	"context"
	"encoding/json"
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

// 中央已经提交文件后真实断开连接，节点必须查询同一ID而非重新分配文件。
func TestActualArtifactLostPUTReceiptUsesConfirmedMetadata(t *testing.T) {
	st, admin, cfg, handler := agentControl(t)
	var dropped atomic.Bool
	var metadataReads atomic.Int32
	var actualID atomic.Value
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/agent/artifacts/") {
			metadataReads.Add(1)
		}
		if r.Method == "PUT" {
			actualID.Store(strings.TrimPrefix(r.URL.Path, "/api/agent/artifacts/"))
		}
		handler.ServeHTTP(w, r)
		if r.Method == "PUT" && strings.HasPrefix(r.URL.Path, "/api/agent/artifacts/") && dropped.CompareAndSwap(false, true) {
			connection, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				connection.Close()
			}
		}
	}))
	defer control.Close()
	cfg.Server = control.URL
	repo := t.TempDir()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "--initial-branch=main", "--template="}} {
		cmd := exec.Command(git, args...)
		cmd.Dir = repo
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + repo, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null"}
		if _, err := cmd.CombinedOutput(); err != nil {
			t.Fatal("own git fixture", err)
		}
	}
	source := `version: 1
steps:
 - kind: run
   name: create
   run: printf actual-snapshot > output.apk
 - kind: artifact
   name: collect
   paths: [output.apk]
`
	if err := os.WriteFile(filepath.Join(repo, "mybuilds.yml"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "mybuilds.yml"}, {"-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "-m", "own"}} {
		cmd := exec.Command(git, args...)
		cmd.Dir = repo
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + repo, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null"}
		if _, err := cmd.CombinedOutput(); err != nil {
			t.Fatal("own git fixture", err)
		}
	}
	if _, err := st.CreateProject(context.Background(), admin, store.ProjectInput{Name: "receipt", Repository: repo, Branches: []string{"main"}, AllowedNodes: []string{cfg.Node}, DefaultNode: cfg.Node}); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(server.TriggerRequest{Branch: "main"})
	request, _ := http.NewRequest("POST", control.URL+"/api/projects/receipt/builds", bytes.NewReader(data))
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "lost-artifact-receipt")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var batch server.BatchView
	err = json.NewDecoder(response.Body).Decode(&batch)
	response.Body.Close()
	if err != nil || response.StatusCode != 201 || len(batch.Builds) != 1 {
		t.Fatal("trigger", response.StatusCode, err)
	}
	t.Setenv("JAVA_HOME", filepath.Join(t.TempDir(), "missing"))
	t.Setenv("ANDROID_HOME", filepath.Join(t.TempDir(), "missing"))
	t.Setenv("ANDROID_SDK_ROOT", "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, cfg) }()
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		view, err := st.GetBuild(context.Background(), batch.Builds[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		if view.Status == "succeeded" {
			if !dropped.Load() || metadataReads.Load() != 1 {
				t.Fatal("missing actual lost receipt metadata recovery")
			}
			files, err := st.ListArtifacts(context.Background(), admin, view.ID, store.Page{})
			if err != nil || len(files) != 1 || files[0].Size != 15 || files[0].ID != actualID.Load() {
				t.Fatal("duplicate/nonmatching artifact", err)
			}
			waitAttemptJournalRemoved(t, cfg.DataDir, view.ID)
			cancel()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			return
		}
		select {
		case err := <-done:
			t.Fatalf("Serve exited before actual confirmation: %v (%s/%s)", err, view.Status, view.Reason)
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Fatal("no terminal")
}
