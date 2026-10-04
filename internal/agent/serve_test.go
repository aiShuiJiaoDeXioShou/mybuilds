//go:build darwin || linux

package agent

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"mybuilds/internal/server"
	"mybuilds/internal/store"
)

// 使用真实Store及HTTP入口，不模拟节点身份或握手结果。
func agentControl(t *testing.T) (*store.Store, store.Actor, config.AgentConfig, http.Handler) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	st, err := store.Open(ctx, store.Options{Driver: "sqlite", DSN: filepath.Join(root, "control.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err = st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("a", 32)
	if err = st.Bootstrap(ctx, token); err != nil {
		t.Fatal(err)
	}
	actor, err := st.Authenticate(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	node, err := st.CreateNode(ctx, actor, store.NodeInput{Name: "actual-node", Capacity: 2})
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.AgentConfig{Node: "actual-node", RuntimeToken: node.Token, DataDir: filepath.Join(root, "node"), Capacity: 2, HeartbeatInterval: time.Second, LeaseDuration: 10 * time.Second}
	serverDir := filepath.Join(root, "server")
	if err := os.Mkdir(serverDir, 0700); err != nil {
		t.Fatal(err)
	}
	handler := server.New(st, config.ServerConfig{DataDir: serverDir, Concurrency: 2, HeartbeatInterval: cfg.HeartbeatInterval, LeaseDuration: cfg.LeaseDuration}).Handler()
	return st, actor, cfg, handler
}
func TestServeActualRegistrationHeartbeatAndCancellation(t *testing.T) {
	st, actor, cfg, handler := agentControl(t)
	control := httptest.NewServer(handler)
	defer control.Close()
	cfg.Server = control.URL
	// 明确不存在SDK/JDK；只上报真实失败，不伪造Android能力。
	t.Setenv("JAVA_HOME", filepath.Join(t.TempDir(), "missing"))
	t.Setenv("ANDROID_HOME", filepath.Join(t.TempDir(), "missing"))
	t.Setenv("ANDROID_SDK_ROOT", "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, cfg) }()
	var first time.Time
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		view, err := st.GetNode(context.Background(), actor, cfg.Node)
		if err != nil {
			t.Fatal(err)
		}
		if view.LastHeartbeat != nil {
			if first.IsZero() {
				first = *view.LastHeartbeat
			}
			if view.LastHeartbeat.After(first) {
				if view.LocalCapacity != 2 || !view.Healthy || !view.SessionActive {
					t.Fatal(view)
				}
				for _, tool := range view.Tools {
					if tool.Name == "android_aapt2" && tool.Status == "passed" {
						t.Fatal("invented SDK capability")
					}
				}
				cancel()
				select {
				case err := <-done:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(2 * time.Second):
					t.Fatal("cancel blocked")
				}
				return
			}
		}
		select {
		case err := <-done:
			t.Fatalf("Serve ended before real heartbeat: %v", err)
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Fatal("no real heartbeat")
}
func TestServeRejectsIdentityMismatchAndOldJournal(t *testing.T) {
	st, actor, cfg, handler := agentControl(t)
	control := httptest.NewServer(handler)
	defer control.Close()
	cfg.Server = control.URL
	cfg.Node = "different-node"
	if err := Serve(context.Background(), cfg); err == nil || err.Error() != "agent_node_identity_mismatch" {
		t.Fatalf("identity: %v", err)
	}
	cfg.Node = "actual-node"
	cfg.DataDir = filepath.Join(t.TempDir(), "old")
	if err := os.MkdirAll(filepath.Join(cfg.DataDir, "journal"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.DataDir, "journal", "old.json"), []byte(`{"pid":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := st.GetNode(context.Background(), actor, cfg.Node)
	if err != nil {
		t.Fatal(err)
	}
	if err = Serve(context.Background(), cfg); err == nil || err.Error() != "agent_journal_unconfirmed" {
		t.Fatal(err)
	}
	after, err := st.GetNode(context.Background(), actor, cfg.Node)
	if err != nil {
		t.Fatal(err)
	}
	if before.LastHeartbeat == nil || after.LastHeartbeat == nil || !before.LastHeartbeat.Equal(*after.LastHeartbeat) {
		t.Fatal("old journal contacted control")
	}
}
func TestAgentHTTPUsesExplicitCAAndRejectsRedirect(t *testing.T) {
	_, _, cfg, handler := agentControl(t)
	control := httptest.NewTLSServer(handler)
	defer control.Close()
	cfg.Server = control.URL
	client, err := newAgentHTTP(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer client.close()
	request := protocol.SessionRequest{SessionID: "cb572bf0-f905-43f0-a075-dbb6355ff0a1", Report: protocol.NodeReport{OS: "linux", Arch: "arm64", Capacity: 1, Tools: []protocol.ToolCheck{}}, HeartbeatNS: int64(time.Second), LeaseNS: int64(10 * time.Second)}
	var grant protocol.SessionGrant
	if err = client.post(context.Background(), "/api/agent/session", request, &grant); err == nil {
		t.Fatal("unknown CA accepted")
	}
	cfg.CAFile = filepath.Join(t.TempDir(), "own-ca.pem")
	if err = os.WriteFile(cfg.CAFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: control.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	trusted, err := newAgentHTTP(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer trusted.close()
	if err = trusted.post(context.Background(), "/api/agent/session", request, &grant); err != nil {
		t.Fatal(err)
	}
	// 重定向目标不含控制端token，不应被请求。
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("redirect followed") }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer redirect.Close()
	cfg.Server = redirect.URL
	cfg.CAFile = ""
	redirected, err := newAgentHTTP(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer redirected.close()
	if err = redirected.post(context.Background(), "/api/agent/session", request, &grant); err == nil || strings.Contains(err.Error(), cfg.RuntimeToken) {
		t.Fatal(err)
	}
}
