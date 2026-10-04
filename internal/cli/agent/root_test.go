package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"mybuilds/internal/config"
	control "mybuilds/internal/server"
	"mybuilds/internal/store"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func executeAgent(args ...string) (string, error) {
	cmd := NewCommand()
	var out, diagnostics bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&diagnostics)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}
func TestAgentLocalCommandsDoNotLoadIdentity(t *testing.T) {
	t.Setenv("MYBUILDS_AGENT_TOKEN", "")
	t.Setenv("MYBUILDS_CLIENT_TOKEN", "")
	t.Setenv("SSL_CERT_FILE", "UNKNOWN_SECRET")
	bad := filepath.Join(t.TempDir(), "bad-agent.yml")
	os.WriteFile(bad, []byte("unknown: PRIVATE_SECRET\n"), 0600)
	for _, args := range [][]string{{"--config", bad, "--help"}, {"--config", bad, "version"}} {
		out, err := executeAgent(args...)
		if err != nil || strings.Contains(out, "SECRET") {
			t.Fatal("本地命令加载业务配置", out, err)
		}
	}
	missing := filepath.Join(t.TempDir(), "uninitialized")
	out, err := executeAgent("--config", bad, "doctor", "--data-dir", missing, "--json")
	if !json.Valid([]byte(out)) || !strings.Contains(out, "uninitialized") || strings.Contains(out, "SECRET") || err != nil && strings.Contains(err.Error(), "配置") {
		t.Fatal("本地doctor依赖token/配置", out, err)
	}
	if _, err := os.Lstat(missing); !os.IsNotExist(err) {
		t.Fatal("doctor创建数据目录")
	}
	for _, args := range [][]string{{"version", "extra"}, {"doctor", "--platform", "ios"}, {"doctor", "--token", "SECRET"}} {
		if _, err := executeAgent(args...); err == nil {
			t.Fatal("未知能力/选项被接受", args)
		}
	}
}

func TestAgentServeLoadsOnlyExplicitIdentity(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "agent.yml")
	os.WriteFile(bad, []byte("unknown: PRIVATE_SECRET\n"), 0600)
	out, err := executeAgent("--config", bad, "serve")
	if err == nil || strings.Contains(err.Error(), "unknown command") || strings.Contains(err.Error(), "PRIVATE_SECRET") || out != "" {
		t.Fatal("serve未消费真实配置", out, err)
	}
}

func TestAgentCLIRegistersActualSessionAndStops(t *testing.T) {
	t.Setenv("MYBUILDS_AGENT_TOKEN", "")
	os.Unsetenv("MYBUILDS_AGENT_TOKEN")
	dir := t.TempDir()
	st, err := store.Open(context.Background(), store.Options{Driver: "sqlite", DSN: filepath.Join(dir, "control.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	actor := store.Actor{ID: "local-admin", Role: "admin"}
	created, err := st.CreateNode(context.Background(), actor, store.NodeInput{Name: "cli-node", Capacity: 1})
	if err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(control.New(st, config.ServerConfig{DataDir: dir, Concurrency: 2, HeartbeatInterval: time.Second, LeaseDuration: 10 * time.Second}).Handler())
	defer api.Close()
	cfg := filepath.Join(dir, "agent.yml")
	os.WriteFile(cfg, []byte("server: "+api.URL+"\nnode: cli-node\ntoken: '"+created.Token+"'\ndata_dir: data\nheartbeat_interval: 1s\nlease_duration: 10s\n"), 0600)
	cmd := NewCommand()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd.SetContext(ctx)
	var out, diagnostics bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&diagnostics)
	cmd.SetArgs([]string{"--config", cfg, "serve"})
	done := make(chan error, 1)
	go func() { done <- cmd.Execute() }()
	deadline := time.Now().Add(8 * time.Second)
	for {
		view, err := st.GetNode(context.Background(), actor, "cli-node")
		if err != nil {
			t.Fatal(err)
		}
		if view.Healthy && view.SessionActive {
			break
		}
		select {
		case err := <-done:
			t.Fatal("实际注册未完成", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("节点未实际注册")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("节点CLI取消未停止")
	}
	if strings.Contains(out.String()+diagnostics.String(), created.Token) {
		t.Fatal("节点凭据进入输出")
	}
}

func TestAgentUnknownAndEmptyDoctorDirectoryRejected(t *testing.T) {
	for _, args := range [][]string{{"PRIVATE_UNKNOWN"}, {"doctor", "--data-dir", ""}} {
		out, err := executeAgent(args...)
		if err == nil || strings.Contains(out+err.Error(), "PRIVATE_UNKNOWN") {
			t.Fatalf("非法操作未安全拒绝 %q %v", out, err)
		}
	}
}
