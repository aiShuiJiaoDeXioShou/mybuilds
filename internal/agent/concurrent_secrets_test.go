//go:build darwin || linux

package agent

import (
	"context"
	"encoding/json"
	"io"
	"mybuilds/internal/server"
	"mybuilds/internal/store"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestActualConcurrentTasksHaveIsolatedSecretsWorkspacesAndLogs(t *testing.T) {
	source := `version: 1
builds:
 a:
  env: {SECRET: "${A_SECRET}"}
  steps:
   - kind: run
     name: action
     run: printf '%s' "$MYBUILDS_BUILD_NAME" > name; printf '%s' "$SECRET" > secret; printf '%s\n' "$SECRET"; sleep 2
 b:
  env: {SECRET: "${B_SECRET}"}
  steps:
   - kind: run
     name: action
     run: printf '%s' "$MYBUILDS_BUILD_NAME" > name; printf '%s' "$SECRET" > secret; printf '%s\n' "$SECRET"; sleep 2
`
	st, admin, cfg, control, id := actualFaultControl(t, source, "")
	cfg.SecretsFile = filepath.Join(t.TempDir(), "secrets.env")
	secretA, secretB := "private-alpha-test-marker", "private-beta-test-marker"
	if err := os.WriteFile(cfg.SecretsFile, []byte("A_SECRET="+secretA+"\nB_SECRET="+secretB+"\nOTHER_SECRET=unselected-private-marker\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("A_SECRET", "wrong-host-alpha")
	t.Setenv("B_SECRET", "wrong-host-beta")
	first, err := st.GetBuild(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, cfg) }()
	overlapped := false
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		builds, err := st.ListBuilds(context.Background(), store.BuildFilter{BatchID: first.BatchID})
		if err != nil || len(builds) != 2 {
			t.Fatal("batch", err)
		}
		if builds[0].Status == "running" && builds[1].Status == "running" && builds[0].Steps[0].Started && builds[1].Steps[0].Started {
			overlapped = true
		}
		if builds[0].Status == "succeeded" && builds[1].Status == "succeeded" {
			if !overlapped || builds[0].AttemptID == builds[1].AttemptID {
				t.Fatal("tasks not independently concurrent")
			}
			for _, build := range builds {
				chunks, err := st.ListLogChunks(context.Background(), admin, build.ID, 0, store.Page{})
				if err != nil || len(chunks) == 0 {
					t.Fatal("logs", err)
				}
				request, _ := http.NewRequest("GET", control.URL+"/api/builds/"+build.ID+"/log", nil)
				request.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
				response, e := http.DefaultClient.Do(request)
				if e != nil {
					t.Fatal(e)
				}
				data, e := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
				response.Body.Close()
				var page server.LogPage
				if e != nil || response.StatusCode != 200 || json.Unmarshal(data, &page) != nil || len(page.Records) == 0 || strings.Contains(string(data), secretA) || strings.Contains(string(data), secretB) || strings.Contains(string(data), "unselected-private-marker") {
					t.Fatal("central log secret isolation", e)
				}
				redacted := false
				for _, record := range page.Records {
					if record.Stream == "stdout" && record.Text == "[REDACTED]" {
						redacted = true
					}
				}
				if !redacted {
					t.Fatal("missing actual redacted stdout")
				}
				waitAttemptJournalRemoved(t, cfg.DataDir, build.ID)
			}
			cancel()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			workspaces, _ := filepath.Glob(filepath.Join(cfg.DataDir, "scm", "checkout-*", "workspace"))
			if len(workspaces) != 2 {
				t.Fatal("shared workspace")
			}
			for _, workspace := range workspaces {
				name, e := os.ReadFile(filepath.Join(workspace, "name"))
				if e != nil {
					t.Fatal(e)
				}
				value, e := os.ReadFile(filepath.Join(workspace, "secret"))
				want := secretA
				if string(name) == "b" {
					want = secretB
				}
				if e != nil || string(value) != want {
					t.Fatal("cross-task/host secret injection", e)
				}
			}
			journals, _ := filepath.Glob(filepath.Join(cfg.DataDir, "journal", "*.json"))
			for _, file := range journals {
				data, _ := os.ReadFile(file)
				if strings.Contains(string(data), secretA) || strings.Contains(string(data), secretB) {
					t.Fatal("secret journal leak")
				}
			}
			return
		}
		select {
		case err := <-done:
			t.Fatal("concurrent Serve failed", err)
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Fatal("parallel tasks did not finish")
}
