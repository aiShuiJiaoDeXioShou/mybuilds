package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"mybuilds/internal/process"
	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
)

// 真实Git夹具、Store租约/reducer和中央日志文件共同消费同一Run回执。
func TestRemoteActualStoreReducerAndConfirmedLogFiles(t *testing.T) {
	for _, mode := range []string{"success", "all_skipped", "post_receipt_timeout"} {
		skipped := mode == "all_skipped"
		postTimeout := mode == "post_receipt_timeout"
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			git, _ := exec.LookPath("git")
			if git == "" {
				t.Fatal("git missing")
			}
			runGit := func(args ...string) string {
				t.Helper()
				cmd := exec.Command(git, args...)
				cmd.Dir = root
				cmd.Env = append(environmentList(process.HostEnvironment()), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
				data, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatal("fixture git failed")
				}
				return strings.TrimSpace(string(data))
			}
			runGit("init", "--initial-branch=main", "--template=")
			runGit("-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "--allow-empty", "-m", "fixture")
			sha := runGit("rev-parse", "HEAD")
			source := `version: 1
params: {enabled: no}
steps:
 - kind: run
   name: ordinary
   run: printf actual > marker
post:
 success:
  - kind: run
    name: success
    run: printf post > post-marker
 failure:
  - kind: run
    name: failure
    run: touch forbidden
 always:
  - kind: run
    name: always
    run: printf cleanup > cleanup-marker
`
			if skipped {
				source = strings.Replace(source, "   run: printf actual > marker", "   when: {params: {enabled: yes}}\n   run: printf actual > marker", 1)
			}
			if postTimeout {
				source = strings.Replace(source, "post:", "post:\n timeout: 500ms", 1)
			}
			d := localDocument(t, source)
			st, err := store.Open(ctx, store.Options{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "control.db")})
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			if err = st.Migrate(ctx); err != nil {
				t.Fatal(err)
			}
			admin := store.Actor{ID: "local-admin", Role: "admin"}
			node, err := st.CreateNode(ctx, admin, store.NodeInput{Name: "actual", Capacity: 1})
			if err != nil {
				t.Fatal(err)
			}
			actor, err := st.AuthenticateNode(ctx, node.Token)
			if err != nil {
				t.Fatal(err)
			}
			shell, _ := exec.LookPath("sh")
			if res := process.Run(ctx, process.Command{Path: shell, Args: []string{"-c", "exit 0"}, Dir: root, Env: environmentList(process.HostEnvironment())}, io.Discard, io.Discard); res.Reason != "" || res.CleanupFailed {
				t.Fatal(res)
			}
			journal := filepath.Join(t.TempDir(), "journal")
			if err := os.Mkdir(journal, 0700); err != nil {
				t.Fatal(err)
			}
			info, err := os.Lstat(journal)
			entries, readErr := os.ReadDir(journal)
			if err != nil || readErr != nil || !info.IsDir() || info.Mode().Perm() != 0700 || len(entries) != 0 {
				t.Fatal("actual private journal invalid")
			}
			report := protocol.NodeReport{OS: runtime.GOOS, Arch: runtime.GOARCH, Capacity: 1, Tools: []protocol.ToolCheck{{Name: "shell", Status: "passed"}, {Name: "git", Status: "passed", Version: strings.Fields(runGit("--version"))[2]}, {Name: "node_journal", Status: "passed"}}}
			policy := store.LeasePolicy{Concurrency: 1, Heartbeat: time.Second, Duration: 10 * time.Second}
			session, err := st.OpenNodeSession(ctx, actor, protocol.SessionRequest{SessionID: uuid.NewString(), Report: report, HeartbeatNS: int64(policy.Heartbeat), LeaseNS: int64(policy.Duration)}, policy)
			if err != nil {
				t.Fatal(err)
			}
			project, err := st.CreateProject(ctx, admin, store.ProjectInput{Name: "actual", Repository: root, Branches: []string{"main"}, AllowedNodes: []string{"actual"}, DefaultNode: "actual"})
			if err != nil {
				t.Fatal(err)
			}
			pv, err := Preview(d, PreviewOptions{})
			if err != nil {
				t.Fatal(err)
			}
			steps := []store.StepProgress{}
			appendSteps := func(phase string, items []StepPreview) {
				for _, step := range items {
					steps = append(steps, store.StepProgress{Phase: phase, Index: step.Index, Name: step.Name, Kind: step.Kind, Condition: step.Condition, Status: "pending", Reasons: step.Reasons})
				}
			}
			appendSteps("ordinary", pv.Builds[0].Steps)
			appendSteps("success", pv.Builds[0].Post.Success)
			appendSteps("failure", pv.Builds[0].Post.Failure)
			appendSteps("always", pv.Builds[0].Post.Always)
			hash := sha256.Sum256([]byte(source))
			digest := hex.EncodeToString(hash[:])
			postBudget := int64(2 * time.Minute)
			if postTimeout {
				postBudget = int64(500 * time.Millisecond)
			}
			_, err = st.Enqueue(ctx, store.EnqueueInput{Actor: admin, ProjectID: project.ID, ProjectVersion: project.PolicyVersion, Key: "actual", RequestDigest: digest, SHA: sha, Branch: "main", Source: "repo", File: "mybuilds.yml", SourceDigest: digest, Builds: []store.PreparedBuild{{Name: "default", Status: "queued", Snapshot: store.BuildSnapshot{Definition: *d.Builds["default"], Params: map[string]string{"enabled": "no"}, Facts: map[string]string{}, Condition: "ready", Reasons: []string{"条件已满足"}, AllowedNodes: project.AllowedNodes, DefaultNode: project.DefaultNode}, PostBudgetNS: postBudget, Steps: steps}}})
			if err != nil {
				t.Fatal(err)
			}
			grant, err := st.Claim(ctx, actor, protocol.ClaimRequest{SessionID: session.SessionID, ClaimKey: uuid.NewString()}, policy)
			if err != nil || grant == nil {
				t.Fatal(grant, err)
			}
			remote := remoteOptions(t)
			remote.RemainingBudgetNS = grant.RemainingBudgetNS
			remote.RemainingPostBudgetNS = grant.RemainingPostBudgetNS
			var eventSeq, logSeq, offset int64
			storage := t.TempDir()
			remote.Log = func(ctx context.Context, record protocol.LogRecord) error {
				data, err := json.Marshal([]protocol.LogRecord{record})
				if err != nil {
					return err
				}
				hash := sha256.Sum256(data)
				id := uuid.NewString()
				file, err := os.OpenFile(filepath.Join(storage, id), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
				if err != nil {
					return err
				}
				_, err = file.Write(data)
				if err == nil {
					err = file.Sync()
				}
				closeErr := file.Close()
				if err != nil {
					return err
				}
				if closeErr != nil {
					return closeErr
				}
				committed, err := st.CommitLogChunk(ctx, actor, store.LogCommit{Ref: grant.Ref, Seq: logSeq + 1, Offset: offset, Size: int64(len(data)), Digest: hex.EncodeToString(hash[:]), StorageID: id, RecordCount: 1})
				if err != nil {
					return err
				}
				logSeq, offset = committed.Ack.Seq, committed.Ack.NextOffset
				return nil
			}
			remote.Progress = func(ctx context.Context, p protocol.ExecutionProgress) error {
				if p.Kind == "build_finished" {
					p.LastLogSeq, p.LastLogOffset = logSeq, offset
				}
				data, err := json.Marshal(p)
				if err != nil {
					return err
				}
				var wire protocol.ExecutionProgress
				if err = json.Unmarshal(data, &wire); err != nil {
					return err
				}
				hash := sha256.Sum256(data)
				ack, err := st.ApplyEvent(ctx, actor, protocol.ExecutionEvent{Ref: grant.Ref, Seq: eventSeq + 1, Digest: hex.EncodeToString(hash[:]), Progress: wire})
				if err != nil {
					t.Logf("actual event rejected: kind=%s phase=%s index=%d reason=%s %v", p.Kind, p.Phase, p.Index, p.Reason, err)
					return err
				}
				eventSeq = ack.Seq
				if postTimeout && p.Kind == "finished" && p.Phase == "always" {
					time.Sleep(750 * time.Millisecond)
				}
				return nil
			}
			result, err := runWithCleanup(t, ctx, d, RunOptions{Workspace: root, Remote: remote})
			if (err != nil) != postTimeout {
				t.Fatal(result, err)
			}
			view, err := st.GetBuild(ctx, grant.Ref.BuildID)
			if err != nil {
				t.Fatal(err)
			}
			want := "succeeded"
			if skipped {
				want = "skipped"
				requireAbsent(t, root, "marker")
			} else {
				requireFile(t, root, "marker", "actual")
				requireFile(t, root, "post-marker", "post")
				requireFile(t, root, "cleanup-marker", "cleanup")
			}
			if postTimeout {
				want = "failed"
				if view.Reason != "post_error" || view.Post[len(view.Post)-1].Status != "succeeded" {
					t.Fatal("post confirmation budget", view)
				}
			}
			if view.Status != want || view.StopUnconfirmed {
				t.Fatal(view)
			}
			chunks, err := st.ListLogChunks(ctx, admin, grant.Ref.BuildID, 0, store.Page{})
			if err != nil {
				t.Fatal(err)
			}
			for _, chunk := range chunks {
				data, err := os.ReadFile(filepath.Join(storage, chunk.StorageID))
				hash := sha256.Sum256(data)
				if err != nil || int64(len(data)) != chunk.Size || hex.EncodeToString(hash[:]) != chunk.Digest {
					t.Fatal("unconfirmed file", err)
				}
			}
			requireAbsent(t, root, "forbidden")
		})
	}
}
