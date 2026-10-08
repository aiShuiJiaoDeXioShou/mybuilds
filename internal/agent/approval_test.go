//go:build darwin || linux

package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	clientcli "mybuilds/internal/cli/client"
	"mybuilds/internal/config"
	"mybuilds/internal/pipeline"
	"mybuilds/internal/store"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// 实际HTTP、固定Git、唯一Run与原工作区的完整暂停/批准/恢复，不用模拟执行器。
func TestServeApprovalActualPauseHTTPResumeReports(t *testing.T) {
	serveApprovalReports(t, 1)
}

func TestServeApprovalMaximumReports(t *testing.T) {
	serveApprovalReports(t, config.MaximumJUnitMaxFiles)
}

func serveApprovalReports(t *testing.T, count int) {
	t.Helper()
	st, admin, cfg, handler := agentControl(t)
	cfg.Capacity = 1
	idleClaims := make(chan time.Time, 1)
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/agent/claim" {
			handler.ServeHTTP(w, r)
			return
		}
		started := time.Now()
		reply := httptest.NewRecorder()
		handler.ServeHTTP(reply, r)
		for key, values := range reply.Header() {
			w.Header()[key] = values
		}
		w.WriteHeader(reply.Code)
		w.Write(reply.Body.Bytes())
		if reply.Code == http.StatusNoContent {
			select {
			case idleClaims <- started:
			default:
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
	source := `version: 1
reports: {junit: {paths: ['result*.xml']}}
steps:
 - kind: run
   name: before
   run: printf '<testsuite><testcase name="before"/></testsuite>' > result.xml; i=1; while [ $i -lt ` + fmt.Sprint(count) + ` ]; do cp result.xml result-$i.xml; i=$((i+1)); done; printf before >> count
 - kind: approval
   name: review
 - kind: run
   name: after
   run: printf after >> count
post:
 always:
  - kind: run
    name: cleanup
    run: printf post > post
`
	if count > 1 {
		source = strings.Replace(source, "paths: ['result*.xml']", fmt.Sprintf("paths: ['result*.xml'], max_files: %d", count), 1)
	}
	if err = os.WriteFile(filepath.Join(repo, "mybuilds.yml"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	runGit("add", "mybuilds.yml")
	runGit("commit", "-m", "own")
	sha := runGit("rev-parse", "HEAD")
	doc, err := config.Parse([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	preview, err := pipeline.Preview(doc, pipeline.PreviewOptions{})
	if err != nil {
		t.Fatal(err)
	}
	project, err := st.CreateProject(context.Background(), admin, store.ProjectInput{Name: "approval-actual", Repository: repo, Branches: []string{"main"}, AllowedNodes: []string{cfg.Node}, DefaultNode: cfg.Node})
	if err != nil {
		t.Fatal(err)
	}
	steps := []store.StepProgress{}
	b := preview.Builds[0]
	for _, group := range []struct {
		phase string
		list  []pipeline.StepPreview
	}{{"ordinary", b.Steps}, {"always", b.Post.Always}} {
		for _, s := range group.list {
			steps = append(steps, store.StepProgress{Phase: group.phase, Index: s.Index, Name: s.Name, Kind: s.Kind, Condition: s.Condition, Status: "pending", Reasons: s.Reasons})
		}
	}
	h := sha256.Sum256([]byte(source))
	digest := hex.EncodeToString(h[:])
	batch, err := st.Enqueue(context.Background(), store.EnqueueInput{Actor: admin, ProjectID: project.ID, ProjectVersion: project.PolicyVersion, Key: "actual-approval", RequestDigest: digest, SHA: sha, Branch: "main", Source: "repo", File: "mybuilds.yml", SourceDigest: digest, Builds: []store.PreparedBuild{{Name: "default", Status: "queued", PostBudgetNS: int64(2 * time.Minute), Snapshot: store.BuildSnapshot{Definition: *doc.Builds["default"], Params: map[string]string{}, Facts: map[string]string{}, Condition: "ready", Reasons: []string{"条件已满足"}, AllowedNodes: project.AllowedNodes, DefaultNode: project.DefaultNode}, Steps: steps}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("JAVA_HOME", filepath.Join(t.TempDir(), "missing"))
	t.Setenv("ANDROID_HOME", filepath.Join(t.TempDir(), "missing"))
	t.Setenv("ANDROID_SDK_ROOT", "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, cfg) }()
	decided := false
	oldAttempt := ""
	oldEpoch := int64(0)
	oldWork := ""
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		view, err := st.GetBuild(context.Background(), batch.Builds[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		if view.Status == "waiting_approval" && !decided {
			oldAttempt = view.AttemptID
			oldEpoch = view.LeaseEpoch
			works, _ := filepath.Glob(filepath.Join(cfg.DataDir, "scm", "checkout-*"))
			if len(works) != 1 {
				t.Fatal("暂停原工作区缺失")
			}
			oldWork = filepath.Join(works[0], "workspace")
			data, _ := os.ReadFile(filepath.Join(oldWork, "count"))
			if string(data) != "before" {
				t.Fatal("暂停实际前步证据")
			}
			if _, err = os.Stat(filepath.Join(oldWork, "post")); !os.IsNotExist(err) {
				t.Fatal("挂起执行了post")
			}
			items, err := st.ListApprovals(context.Background(), admin, store.ApprovalFilter{})
			if err != nil || len(items) != 1 {
				t.Fatal("真实审批无法读取", err)
			}
			a := items[0]
			if a.Reports == nil || a.Reports.Sealed || a.Reports.Outcome != "pending" || a.Reports.Counts.Tests != int64(count) {
				t.Fatal("midrun报告伪装final")
			}
			files, err := st.ListArtifacts(context.Background(), admin, view.ID, store.Page{Limit: 1})
			if err != nil || len(files) != 1 {
				t.Fatal("原XML在安全审批缺失", err, len(files))
			}
			// 只有实际ACK后的已确认暂停可用于已知安全重启，不能把竞争中的unknown Claim/ACK当已确认。
			pausedReady := false
			for until := time.Now().Add(time.Second); time.Now().Before(until); {
				names, _ := filepath.Glob(filepath.Join(cfg.DataDir, "journal", "*.json"))
				if len(names) == 1 {
					raw, _ := os.ReadFile(names[0])
					state, e := parseApprovalJournal(raw)
					if e == nil && state.Approval.State == "confirmed" {
						pausedReady = true
						break
					}
				}
				time.Sleep(time.Millisecond)
			}
			if !pausedReady {
				t.Fatal("真实暂停ACK没有完成")
			}
			// 等一次已返回204且本地移除的空claim，再在下一200ms轮询前退出。
			// 直接取消正在发送的空claim会正确保留unknown journal，不能作为安全重启夹具。
			for len(idleClaims) > 0 {
				<-idleClaims
			}
			safeIdle := false
			for until := time.Now().Add(3 * time.Second); time.Now().Before(until) && !safeIdle; {
				select {
				case started := <-idleClaims:
					for time.Since(started) < 100*time.Millisecond {
						names, _ := filepath.Glob(filepath.Join(cfg.DataDir, "journal", "*.json"))
						if len(names) == 1 {
							safeIdle = true
							break
						}
						time.Sleep(time.Millisecond)
					}
				case <-time.After(20 * time.Millisecond):
				}
			}
			if !safeIdle {
				t.Fatal("未确认空claim安全退出窗口")
			}
			// 真正退出并重启原Agent，再沿原暂停journal恢复；不复用活内存。
			cancel()
			select {
			case e := <-done:
				if e != nil {
					t.Fatal(e)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("原Agent没有退出")
			}
			// 退出不能给已确认checkpoint追加独立Stop，真实暂停必须仍可严格读取。
			names, _ := filepath.Glob(filepath.Join(cfg.DataDir, "journal", "*.json"))
			if len(names) != 1 {
				t.Fatal("已确认暂停退出后出现未知journal")
			}
			raw, err := os.ReadFile(names[0])
			if _, strict := parseApprovalJournal(raw); err != nil || strict != nil {
				t.Fatal("退出覆盖了已确认暂停", strict)
			}
			time.Sleep(2*cfg.LeaseDuration + 100*time.Millisecond) // 保留007原session更换安全窗口，不靠改租约字段制造重启。
			deadline = time.Now().Add(120 * time.Second)
			ctx, cancel = context.WithCancel(context.Background())
			defer cancel()
			go func() { done <- Serve(ctx, cfg) }()
			clientFile := filepath.Join(t.TempDir(), "client.yml")
			if err = os.WriteFile(clientFile, []byte("server: "+control.URL+"\ntoken: ${MYBUILDS_CLIENT_TOKEN}\n"), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("MYBUILDS_CLIENT_TOKEN", strings.Repeat("a", 32))
			cmd := clientcli.NewCommand()
			var output bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetErr(&output)
			cmd.SetArgs([]string{"--config", clientFile, "approve", view.ID, "--approval-id", a.ID, "--revision", strconv.FormatInt(a.Revision, 10), "--checkpoint-digest", a.CheckpointDigest, "--json"})
			if err = cmd.Execute(); err != nil {
				t.Fatal("实际批准", err)
			}
			decided = true
		}
		if view.Status == "succeeded" {
			if !decided || view.AttemptID != oldAttempt || view.LeaseEpoch != oldEpoch+1 || view.Reports == nil || !view.Reports.Sealed || view.Reports.Counts.Tests != int64(count) {
				t.Fatal("恢复真实归属/报告不符", view.Status)
			}
			data, _ := os.ReadFile(filepath.Join(oldWork, "count"))
			if string(data) != "beforeafter" {
				t.Fatal("重跑了原脚本或未继续")
			}
			works, _ := filepath.Glob(filepath.Join(cfg.DataDir, "scm", "checkout-*"))
			if len(works) != 1 {
				t.Fatal("恢复重新checkout")
			}
			waitAttemptJournalRemoved(t, cfg.DataDir, view.ID)
			cancel()
			if err = <-done; err != nil {
				t.Fatal(err)
			}
			return
		}
		select {
		case err := <-done:
			t.Fatal("真实Agent提前退出", err)
		case <-time.After(20 * time.Millisecond):
		}
	}
	cancel()
	<-done
	t.Fatal("真实审批闭环超时")
}
