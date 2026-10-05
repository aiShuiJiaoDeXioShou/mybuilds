//go:build darwin || linux

package agent

import (
	"bytes"
	"context"
	"encoding/json"
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

// 真实用户命令只通过本次输入协议消费原snapshot；不是executor替身。
func TestCustomAgentCommandHelper(t *testing.T) {
	path := os.Getenv("MYBUILDS_PUBLISH_INPUT")
	if path == "" {
		return
	}
	data, e := os.ReadFile(path)
	if e != nil {
		os.Exit(2)
	}
	var input map[string]any
	if json.Unmarshal(data, &input) != nil {
		os.Exit(3)
	}
	artifact, ok := input["artifact"].(map[string]any)
	if !ok {
		os.Exit(4)
	}
	method := "POST"
	if os.Args[len(os.Args)-1] == "query" {
		method = "GET"
		if artifact["path"] != nil {
			os.Exit(5)
		}
	} else {
		source, ok := artifact["path"].(string)
		if !ok {
			os.Exit(6)
		}
		data, e = os.ReadFile(source)
		if e != nil || len(data) != int(artifact["size"].(float64)) {
			os.Exit(7)
		}
	}
	request, e := http.NewRequest(method, os.Getenv("CUSTOM_ENDPOINT"), nil)
	if e != nil {
		os.Exit(8)
	}
	response, e := http.DefaultClient.Do(request)
	if e != nil {
		os.Exit(9)
	}
	response.Body.Close()
	result := map[string]any{"schema": 1, "intent_id": input["intent_id"], "authorization_digest": input["authorization_digest"], "app_identifier": input["app_identifier"], "artifact_id": artifact["id"], "artifact_sha256": artifact["sha256"], "version_name": input["version_name"], "version_code": input["version_code"], "status": "uploaded", "evidence_code": "remote_receipt", "remote_id": "owned-receipt", "action_confirmed": true}
	data, _ = json.Marshal(result)
	if os.WriteFile(os.Getenv("MYBUILDS_PUBLISH_RESULT"), data, 0600) != nil {
		os.Exit(10)
	}
	if os.Getenv("CUSTOM_LEAVE_UNKNOWN") == "yes" || response.Header.Get("X-Owned-Unknown") == "yes" {
		if os.WriteFile(os.Getenv("CUSTOM_OBSERVER"), []byte(os.Getenv("HOME")), 0600) != nil {
			os.Exit(12)
		}
		if os.WriteFile(filepath.Join(os.Getenv("HOME"), "unknown"), []byte("keep"), 0600) != nil {
			os.Exit(11)
		}
	}
	os.Exit(0)
}

func TestCustomAgentActualUploadAndMetadataQuery(t *testing.T) { customAgentActual(t, false) }

func customAgentActual(t *testing.T, queryCleanup bool) {
	st, admin, cfg, handler := agentControl(t)
	var writes, reads atomic.Int32
	observer := filepath.Join(t.TempDir(), "query-private")
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			writes.Add(1)
		} else if r.Method == "GET" {
			reads.Add(1)
			if queryCleanup {
				w.Header().Set("X-Owned-Unknown", "yes")
			}
		} else {
			w.WriteHeader(405)
			return
		}
		w.Write([]byte("owned"))
	}))
	defer target.Close()
	var preflight, authorizations atomic.Int32
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/preflight") {
			preflight.Add(1)
		}
		if strings.HasSuffix(r.URL.Path, "/authorize") {
			authorizations.Add(1)
		}
		handler.ServeHTTP(w, r)
	}))
	defer control.Close()
	cfg.Server = control.URL
	repo := t.TempDir()
	binary, _ := os.Executable()
	argv, _ := json.Marshal([]string{binary, "-test.run=^TestCustomAgentCommandHelper$", "--", "upload"})
	query, _ := json.Marshal([]string{binary, "-test.run=^TestCustomAgentCommandHelper$", "--", "query"})
	source := "version: 1\nparams:\n version: {default: '1.2.3'}\nsteps:\n - {kind: run, name: produce, run: 'mkdir output; printf package > output/app.bin'}\n - {kind: artifact, name: package, paths: ['output/app.bin']}\n - kind: upload\n   name: publish\n   target: custom\n   file: output/app.bin\n   app_identifier: org.example.custom\n   working_dir: .\n   result_file: result.json\n   argv: " + string(argv) + "\n   query_argv: " + string(query) + "\n   env:\n    CUSTOM_ENDPOINT: " + target.URL + "\n    CUSTOM_OBSERVER: " + observer + "\npost:\n always:\n  - {kind: run, name: cleanup, run: 'printf changed > output/app.bin'}\n"
	fixtureGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + repo, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_AUTHOR_NAME=own", "GIT_AUTHOR_EMAIL=own@example.invalid", "GIT_COMMITTER_NAME=own", "GIT_COMMITTER_EMAIL=own@example.invalid"}
		if out, e := cmd.CombinedOutput(); e != nil {
			t.Fatalf("fixture git %v bytes=%d", e, len(out))
		}
	}
	fixtureGit("init", "--initial-branch=main", "--template=")
	if e := os.WriteFile(filepath.Join(repo, "mybuilds.yml"), []byte(source), 0600); e != nil {
		t.Fatal(e)
	}
	fixtureGit("add", "mybuilds.yml")
	fixtureGit("commit", "-m", "own")
	project, e := st.CreateProject(context.Background(), admin, store.ProjectInput{Name: "custom", Repository: repo, Branches: []string{"main"}, AllowedNodes: []string{cfg.Node}, DefaultNode: cfg.Node})
	if e != nil {
		t.Fatal(e)
	}
	actor, e := st.AuthenticateNode(context.Background(), cfg.RuntimeToken)
	if e != nil {
		t.Fatal(e)
	}
	_, e = st.BindApplication(context.Background(), admin, store.BindApplicationInput{ProjectID: project.ID, NodeID: actor.ID, Store: "custom", AppIdentifier: "org.example.custom", AllowedTracks: []string{}, Custom: &protocol.CustomBindingEvidence{Source: "manual_attested", EvidenceCode: "ownership_attested", Note: "owned integration receiver", EvidenceSHA256: strings.Repeat("a", 64)}})
	if e != nil {
		t.Fatal(e)
	}
	data, _ := json.Marshal(server.TriggerRequest{Branch: "main", AllowUpload: true})
	request, _ := http.NewRequest("POST", control.URL+"/api/projects/custom/builds", bytes.NewReader(data))
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
	request.Header.Set("Idempotency-Key", "custom-once")
	request.Header.Set("Content-Type", "application/json")
	response, e := http.DefaultClient.Do(request)
	if e != nil {
		t.Fatal(e)
	}
	var batch server.BatchView
	e = json.NewDecoder(response.Body).Decode(&batch)
	response.Body.Close()
	if e != nil || response.StatusCode != 201 || len(batch.Builds) != 1 {
		t.Fatalf("actual trigger %d %v", response.StatusCode, e)
	}
	t.Setenv("JAVA_HOME", filepath.Join(t.TempDir(), "absent"))
	t.Setenv("ANDROID_HOME", filepath.Join(t.TempDir(), "absent"))
	t.Setenv("ANDROID_SDK_ROOT", "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, cfg) }()
	deadline := time.Now().Add(15 * time.Second)
	var view store.BuildView
	for time.Now().Before(deadline) {
		view, e = st.GetBuild(context.Background(), batch.Builds[0].ID)
		if e == nil && view.Status != "queued" && view.Status != "running" && view.Status != "cancel_requested" {
			break
		}
		select {
		case e := <-done:
			t.Fatalf("actual agent exit early %v", e)
		case <-time.After(30 * time.Millisecond):
		}
	}
	if view.Status != "succeeded" {
		cancel()
		<-done
		t.Fatalf("actual custom build %s/%s preflight=%d authorize=%d writes=%d", view.Status, view.Reason, preflight.Load(), authorizations.Load(), writes.Load())
	}
	pubs, e := st.ListPublishes(context.Background(), project.ID, 10, "")
	if e != nil || len(pubs) != 1 || pubs[0].Status != "uploaded" || pubs[0].ApplicationProtected || writes.Load() != 1 {
		t.Fatalf("one publish %v %+v writes=%d", e, pubs, writes.Load())
	}
	queryView, e := st.RequestPublishQuery(context.Background(), admin, pubs[0].ID)
	if e != nil {
		t.Fatal(e)
	}

	if queryCleanup {
		select {
		case err := <-done:
			if err == nil || err.Error() != "agent_cleanup_error" {
				t.Fatalf("query cleanup not preserved: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("unknown query did not close agent")
		}
		entries, err := os.ReadDir(filepath.Join(cfg.DataDir, "journal"))
		if err != nil || len(entries) != 1 {
			t.Fatal("missing management protection", err, len(entries))
		}
		data, err := os.ReadFile(filepath.Join(cfg.DataDir, "journal", entries[0].Name()))
		var state journalState
		if err != nil || json.Unmarshal(data, &state) != nil || state.Ref != nil || state.ClaimKey != queryView.ID || state.SessionID == "" {
			t.Fatal("wrong management journal")
		}
		if inspectData(cfg.DataDir) != "journal_unconfirmed" {
			t.Fatal("unknown query did not protect next session")
		}
		if err = Serve(context.Background(), cfg); err == nil {
			t.Fatal("restarted across unknown query")
		}
		after, _ := os.ReadFile(filepath.Join(cfg.DataDir, "journal", entries[0].Name()))
		if !bytes.Equal(data, after) {
			t.Fatal("recovery changed unknown evidence")
		}
		q, err := st.GetPublishQuery(context.Background(), queryView.ID)
		if err != nil || q.Status != "pending" {
			t.Fatal("invented cleanup confirmation", err, q.Status)
		}
		private, err := os.ReadFile(observer)
		if err != nil {
			t.Fatal("owned helper observer missing")
		}
		info, err := os.Lstat(string(private))
		if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
			t.Fatal("owned query dir not preserved")
		}
		t.Cleanup(func() { os.RemoveAll(string(private)) })
		if reads.Load() != 1 || writes.Load() != 1 {
			t.Fatal("query/upload repeated", reads.Load(), writes.Load())
		}
		return
	}
	for time.Now().Before(deadline) {
		q, e := st.GetPublishQuery(context.Background(), queryView.ID)
		if e == nil && q.Status != "pending" {
			if q.Status != "completed" {
				t.Fatal("query", q.Status, q.Reason)
			}
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if reads.Load() != 1 || writes.Load() != 1 {
		t.Fatal("query mutation/replay", reads.Load(), writes.Load())
	}
	cancel()
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("agent stop")
	}
}
