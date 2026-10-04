//go:build darwin || linux

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentcli "mybuilds/internal/cli/agent"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"mybuilds/internal/server"
	"mybuilds/internal/store"
)

// 全链路使用实际Git、Agent入口、Run和中央文件，不用预制进度替代执行。
func TestActualCLIControllerAgentRunEvidenceLifecycle(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(ctx, store.Options{Driver: "sqlite", DSN: filepath.Join(root, "control.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err = st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	admin := store.Actor{ID: "local-admin", Role: "admin"}
	credential, err := st.CreateToken(ctx, admin, "admin")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("MYBUILDS_CLIENT_TOKEN", credential.Token)
	t.Setenv("MYBUILDS_AGENT_TOKEN", "")
	if err := os.Unsetenv("MYBUILDS_AGENT_TOKEN"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JAVA_HOME", filepath.Join(root, "missing-java"))
	t.Setenv("ANDROID_HOME", filepath.Join(root, "missing-android"))
	t.Setenv("ANDROID_SDK_ROOT", "")
	api := httptest.NewServer(server.New(st, config.ServerConfig{DataDir: root, Concurrency: 1, HeartbeatInterval: time.Second, LeaseDuration: 10 * time.Second}).Handler())
	defer api.Close()
	call := func(args ...string) string {
		t.Helper()
		out, err := executeRemote(t, append([]string{"--server-url", api.URL}, args...)...)
		if err != nil {
			t.Fatalf("真实CLI %v: %v", args, err)
		}
		if strings.Contains(out, "PRIVATE") || strings.Contains(out, credential.Token) {
			t.Fatal("公开输出泄露", out)
		}
		return out
	}
	var node store.NodeCreated
	if err = json.Unmarshal([]byte(call("node", "create", "cli-worker", "--json")), &node); err != nil {
		t.Fatal(err)
	}
	call("group", "create", "cli-group")
	repository := filepath.Join(root, "source")
	if err = os.Mkdir(repository, 0700); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		t.Helper()
		command := exec.Command("git", append([]string{"-c", "core.hooksPath=" + filepath.Join(root, "no-hooks")}, args...)...)
		command.Dir = repository
		command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + root, "GIT_CONFIG_SYSTEM=/dev/null", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid"}
		if _, err := command.CombinedOutput(); err != nil {
			t.Fatal("自有Git失败", err)
		}
	}
	git("init", "--initial-branch=main", "--template=")
	pipeline := `version: 1
builds:
  app:
    params:
      delay: {default: '0.15'}
    env:
      DELAY: '{{delay}}'
      VALUE: '${BUILD_SECRET}'
    timeout: 20s
    steps:
      - kind: run
        name: compile
        run: 'printf log_marker; printf "$VALUE"; sleep "$DELAY"; printf "payload\000\377" > output.bin'
      - kind: artifact
        name: collect
        paths: [output.bin]
    post:
      always:
        - kind: run
          name: cleanup
          run: printf cleanup_marker
`
	if err = os.WriteFile(filepath.Join(repository, "mybuilds.yml"), []byte(pipeline), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "mybuilds.yml")
	git("commit", "-m", "own fixture")
	call("project", "init", "cli-app", "--repo", repository, "--nodes", "cli-worker", "--default-node", "cli-worker", "--group", "cli-group")
	secrets := filepath.Join(root, "secrets.env")
	if err = os.WriteFile(secrets, []byte("BUILD_SECRET=PRIVATE_BUILD_SECRET\n"), 0600); err != nil {
		t.Fatal(err)
	}
	agentFile := filepath.Join(root, "agent.yml")
	if err = os.WriteFile(agentFile, []byte("server: "+api.URL+"\nnode: cli-worker\ntoken: '"+node.Token+"'\ndata_dir: node-data\nsecrets_file: secrets.env\ncapacity: 1\nheartbeat_interval: 1s\nlease_duration: 10s\n"), 0600); err != nil {
		t.Fatal(err)
	}
	live, stop := context.WithCancel(ctx)
	agent := agentcli.NewCommand()
	agent.SetArgs([]string{"--config", agentFile, "serve"})
	agent.SetOut(io.Discard)
	agent.SetErr(io.Discard)
	done := make(chan error, 1)
	go func() { done <- agent.ExecuteContext(live) }()
	joined := false
	defer func() {
		stop()
		if !joined {
			select {
			case err := <-done:
				if err != nil {
					t.Error("Agent退出", err)
				}
			case <-time.After(4 * time.Second):
				t.Error("Agent退出超时")
			}
		}
	}()
	trigger := func(key, delay string) store.BuildView {
		t.Helper()
		out := call("trigger", "cli-app", "--build", "app", "--param", "app:delay="+delay, "--idempotency-key", key, "--json")
		var batch server.BatchView
		if err := json.Unmarshal([]byte(out), &batch); err != nil || len(batch.Builds) != 1 {
			t.Fatal(out, err)
		}
		view, err := st.GetBuild(ctx, batch.Builds[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		return view
	}
	wait := func(id, status string, started bool) store.BuildView {
		t.Helper()
		until := time.Now().Add(15 * time.Second)
		for time.Now().Before(until) {
			view, err := st.GetBuild(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if started && len(view.Steps) > 0 && view.Steps[0].Started || !started && view.Status == status {
				return view
			}
			select {
			case err := <-done:
				joined = true
				t.Fatalf("Agent提前退出 %v，实际中央状态 %#v", err, view)
			default:
			}
			if view.Status == "failed" || view.Status == "interrupted" {
				t.Fatal("实际执行失败", view.Status, view.Reason)
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("实际执行等待超时", id, status)
		return store.BuildView{}
	}
	first := trigger("cli-evidence-success", "0.15")
	first = wait(first.ID, "succeeded", false)
	if first.NodeName != "cli-worker" || first.AttemptID == "" || len(first.Steps) != 2 || !first.Steps[0].Started || !first.Steps[0].StopConfirmed || len(first.Post) != 1 || !first.Post[0].Started {
		t.Fatal("缺实际Run证据", first)
	}
	call("node", "ls", "--json")
	doctor, _ := executeRemote(t, "--server-url", api.URL, "doctor", "--node", "cli-worker", "--json")
	if !json.Valid([]byte(doctor)) || strings.Contains(doctor, "PRIVATE") || strings.Contains(doctor, node.Token) {
		t.Fatal("远程doctor证据无效", doctor)
	}
	call("build", "ls", "--project", "cli-app", "--json")
	call("build", "show", first.ID, "--json")
	for _, args := range [][]string{{"logs", first.ID, "--json"}, {"logs", first.ID, "--follow", "--stream-timeout", "2s"}} {
		out := call(args...)
		if !strings.Contains(out, "log_marker") || !strings.Contains(out, "cleanup_marker") {
			t.Fatal("缺真实日志", out)
		}
	}
	var listing struct {
		Items []protocol.ArtifactView `json:"items"`
	}
	if err = json.Unmarshal([]byte(call("artifact", "ls", first.ID, "--json")), &listing); err != nil || len(listing.Items) != 1 {
		t.Fatal("缺中央产物", listing, err)
	}
	output := filepath.Join(root, "download.bin")
	call("artifact", "download", listing.Items[0].ID, "--output", output)
	content, err := os.ReadFile(output)
	if err != nil || !bytes.Equal(content, []byte("payload\x00\xff")) {
		t.Fatal("真实中央下载字节", content, err)
	}
	second := trigger("cli-evidence-cancel", "8")
	wait(second.ID, "", true)
	call("build", "cancel", second.ID)
	second = wait(second.ID, "cancelled", false)
	if second.StopUnconfirmed || len(second.Post) != 1 || !second.Post[0].Started {
		t.Fatal("取消未实际回收或always未执行", second)
	}
	approver, err := st.CreateToken(ctx, admin, "approver")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("MYBUILDS_CLIENT_TOKEN", approver.Token)
	call("logs", first.ID, "--json")
	call("artifact", "ls", first.ID, "--json")
	for _, args := range [][]string{{"node", "disable", "cli-worker"}, {"trigger", "cli-app", "--build", "app"}, {"build", "cancel", first.ID}} {
		if _, err := executeRemote(t, append([]string{"--server-url", api.URL}, args...)...); err == nil {
			t.Fatal("approver越权", args)
		}
	}
	triggerToken, err := st.CreateToken(ctx, admin, "trigger")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("MYBUILDS_CLIENT_TOKEN", triggerToken.Token)
	for _, args := range [][]string{{"logs", first.ID}, {"artifact", "ls", first.ID}, {"node", "ls"}} {
		if _, err := executeRemote(t, append([]string{"--server-url", api.URL}, args...)...); err == nil {
			t.Fatal("trigger越权读取", args)
		}
	}
	t.Setenv("MYBUILDS_CLIENT_TOKEN", node.Token)
	if _, err := executeRemote(t, "--server-url", api.URL, "status"); err == nil {
		t.Fatal("节点凭据授予用户权限")
	}
	t.Setenv("MYBUILDS_CLIENT_TOKEN", credential.Token)
	stop()
	select {
	case err := <-done:
		joined = true
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("Agent退出超时")
	}
	call("artifact", "download", listing.Items[0].ID, "--output", filepath.Join(root, "offline.bin"))
	if content, err = os.ReadFile(filepath.Join(root, "offline.bin")); err != nil || !bytes.Equal(content, []byte("payload\x00\xff")) {
		t.Fatal("离线中央下载", err)
	}
}
