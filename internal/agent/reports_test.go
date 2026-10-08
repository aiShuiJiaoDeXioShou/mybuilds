//go:build darwin || linux

package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mybuilds/internal/config"
	"mybuilds/internal/pipeline"
	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
)

// 实际Store入队、HTTP、固定Git checkout、唯一Run与中央原XML共同验收；不造上传回执。
func TestServeActualJUnitFinalRevisionAndOriginalDownload(t *testing.T) {
	serveActualJUnit(t, 1)
}

func TestServeActualJUnitMaximumFiles(t *testing.T) {
	serveActualJUnit(t, config.MaximumJUnitMaxFiles)
}

func serveActualJUnit(t *testing.T, count int) {
	t.Helper()
	st, admin, cfg, handler := agentControl(t)
	control := httptest.NewServer(handler)
	defer control.Close()
	cfg.Server = control.URL
	repo := t.TempDir()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal("git missing")
	}
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(git, args...)
		cmd.Dir = repo
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + repo, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_AUTHOR_NAME=fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid", "GIT_COMMITTER_NAME=fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid"}
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatal("自有Git夹具失败")
		}
		return strings.TrimSpace(string(output))
	}
	runGit("init", "--initial-branch=main", "--template=")
	original := `<testsuite tests="1"><testcase name="actual" time="0.125"/></testsuite>`
	source := `version: 1
reports: {junit: {paths: ['result*.xml']}}
steps:
 - kind: run
   name: tests
   run: printf '` + original + `' > result.xml; i=1; while [ $i -lt ` + fmt.Sprint(count) + ` ]; do cp result.xml result-$i.xml; i=$((i+1)); done; printf once >> count
 - kind: artifact
   name: ordinary-artifact
   paths: [count]
post:
 always:
  - kind: run
    name: cleanup
    run: printf '<testsuite tests="0"/>' > result.xml; printf cleanup > cleanup
  - kind: artifact
    name: post-artifact
    paths: [cleanup]
`
	if count > 1 {
		source = strings.Replace(source, "paths: ['result*.xml']", fmt.Sprintf("paths: ['result*.xml'], max_files: %d", count), 1)
	}
	if err := os.WriteFile(filepath.Join(repo, "mybuilds.yml"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	runGit("add", "mybuilds.yml")
	runGit("commit", "-m", "own")
	sha := runGit("rev-parse", "HEAD")
	document, err := config.Parse([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	preview, err := pipeline.Preview(document, pipeline.PreviewOptions{})
	if err != nil {
		t.Fatal(err)
	}
	project, err := st.CreateProject(context.Background(), admin, store.ProjectInput{Name: "reports-actual", Repository: repo, Branches: []string{"main"}, AllowedNodes: []string{cfg.Node}, DefaultNode: cfg.Node})
	if err != nil {
		t.Fatal(err)
	}
	steps := []store.StepProgress{}
	addSteps := func(phase string, list []pipeline.StepPreview) {
		for _, s := range list {
			steps = append(steps, store.StepProgress{Phase: phase, Index: s.Index, Name: s.Name, Kind: s.Kind, Condition: s.Condition, Status: "pending", Reasons: s.Reasons})
		}
	}
	b := preview.Builds[0]
	addSteps("ordinary", b.Steps)
	addSteps("success", b.Post.Success)
	addSteps("failure", b.Post.Failure)
	addSteps("always", b.Post.Always)
	hash := sha256.Sum256([]byte(source))
	digest := hex.EncodeToString(hash[:])
	batch, err := st.Enqueue(context.Background(), store.EnqueueInput{Actor: admin, ProjectID: project.ID, ProjectVersion: project.PolicyVersion, Key: "actual-reports", RequestDigest: digest, SHA: sha, Branch: "main", Source: "repo", File: "mybuilds.yml", SourceDigest: digest, Builds: []store.PreparedBuild{{Name: "default", Status: "queued", PostBudgetNS: int64(2 * time.Minute), Snapshot: store.BuildSnapshot{Definition: *document.Builds["default"], Params: map[string]string{}, Facts: map[string]string{}, Condition: "ready", Reasons: []string{"条件已满足"}, AllowedNodes: project.AllowedNodes, DefaultNode: project.DefaultNode}, Steps: steps}}})
	if err != nil {
		t.Fatal("真实报告入队", err)
	}
	t.Setenv("JAVA_HOME", filepath.Join(t.TempDir(), "missing"))
	t.Setenv("ANDROID_HOME", filepath.Join(t.TempDir(), "missing"))
	t.Setenv("ANDROID_SDK_ROOT", "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, cfg) }()
	logState := func() {
		view, _ := st.GetBuild(context.Background(), batch.Builds[0].ID)
		for _, step := range view.Steps {
			t.Logf("阶段%s/%d %s: %s/%s", step.Phase, step.Index, step.Kind, step.Status, step.Reason)
		}
		names, _ := filepath.Glob(filepath.Join(cfg.DataDir, "journal", "*.json"))
		for _, name := range names {
			raw, _ := os.ReadFile(name)
			var state journalState
			if json.Unmarshal(raw, &state) == nil {
				kind := ""
				if state.PendingEvent != nil {
					kind = state.PendingEvent.Progress.Kind
				}
				confirmed := 0
				for _, a := range state.Artifacts {
					if a.Confirmed {
						confirmed++
					}
				}
				t.Logf("journal seq=%d pending=%s files=%d confirmed=%d stopped=%t", state.LastEventSeq, kind, len(state.Artifacts), confirmed, state.StopConfirmed)
			}
		}
	}
	waitUntil := time.Now().Add(180 * time.Second)
	lastDebug := time.Now()
	for time.Now().Before(waitUntil) {
		if count > 1 && time.Since(lastDebug) > 20*time.Second {
			logState()
			lastDebug = time.Now()
		}
		view, err := st.GetBuild(context.Background(), batch.Builds[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-done:
			logState()
			t.Fatalf("真实Agent未完成报告链便退出: %v，中央%s/%s", err, view.Status, view.Reason)
		default:
		}
		if view.Status == "succeeded" {
			encoded, _ := json.Marshal(view)
			var public struct {
				Reports *struct {
					Sealed bool `json:"sealed"`
					Counts struct {
						Tests int64 `json:"tests"`
					} `json:"counts"`
				} `json:"reports"`
				ReportSealDigest string `json:"report_seal_digest"`
			}
			if json.Unmarshal(encoded, &public) != nil || public.Reports == nil || !public.Reports.Sealed || public.Reports.Counts.Tests != int64(count) || public.ReportSealDigest == "" {
				t.Fatal("中央真实报告未完整封存")
			}
			files := []protocol.ArtifactView{}
			for offset := 0; ; offset += 200 {
				page, err := st.ListArtifacts(context.Background(), admin, view.ID, store.Page{Limit: 200, Offset: offset})
				if err != nil {
					t.Fatal(err)
				}
				files = append(files, page...)
				if len(page) < 200 {
					break
				}
			}
			if len(files) != count+2 {
				t.Fatal("真实两用途文件", len(files))
			}
			found := 0
			for _, f := range files {
				if f.Purpose != "junit" {
					continue
				}
				found++
				want := sha256.Sum256([]byte(original))
				if f.SHA256 != hex.EncodeToString(want[:]) || f.Size != int64(len(original)) || f.ReportRevision < 1 || f.ReportKey == "" || f.Index != 1 || f.Step != "tests" {
					t.Fatal("原XML元数据不精确")
				}
				req, _ := http.NewRequest(http.MethodGet, control.URL+"/api/artifacts/"+f.ID+"/"+f.Name, nil)
				req.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
				response, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				data, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
				response.Body.Close()
				if response.StatusCode != 200 || readErr != nil || !bytes.Equal(data, []byte(original)) {
					t.Fatal("中央原字节下载失败")
				}
			}
			if found != count {
				t.Fatal("没有真实junit用途")
			}
			waitAttemptJournalRemoved(t, cfg.DataDir, view.ID)
			cancel()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			paths, _ := filepath.Glob(filepath.Join(cfg.DataDir, "scm", "checkout-*", "workspace", "count"))
			if len(paths) != 1 {
				t.Fatal("不止一次checkout")
			}
			data, err := os.ReadFile(paths[0])
			if err != nil || string(data) != "once" {
				t.Fatal("实际Run不止一次")
			}
			sources, _ := filepath.Glob(filepath.Join(cfg.DataDir, "scm", "checkout-*", "workspace", "result.xml"))
			data, err = os.ReadFile(sources[0])
			if err != nil || string(data) != `<testsuite tests="0"/>` {
				t.Fatal("post未实际改写源")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	view, _ := st.GetBuild(context.Background(), batch.Builds[0].ID)
	logState()
	cancel()
	<-done
	t.Fatalf("真实报告未达完整终态: %s/%s，ordinary=%d，report=%v", view.Status, view.Reason, len(view.Steps), view.Reports != nil)
}
