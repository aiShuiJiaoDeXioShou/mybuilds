package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func clearAgentEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("MYBUILDS_AGENT_TOKEN", "")
	os.Unsetenv("MYBUILDS_AGENT_TOKEN")
}
func agentBody() string {
	return "server: https://build.example.com\nnode: mac-build-01\ntoken: '" + strings.Repeat("SECRET", 8) + "'\n"
}
func TestAgentConfiguration(t *testing.T) {
	clearAgentEnvironment(t)
	p := writeManagementConfig(t, agentBody()+"data_dir: node-data\nca_file: private-ca.pem\n")
	cfg, err := LoadAgent(AgentLoadOptions{Filename: p, Explicit: true})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Node != "mac-build-01" || cfg.Capacity != 1 || cfg.HeartbeatInterval != 5*time.Second || cfg.LeaseDuration != 30*time.Second || cfg.DataDir != filepath.Join(filepath.Dir(p), "node-data") || cfg.CAFile != filepath.Join(filepath.Dir(p), "private-ca.pem") {
		t.Fatalf("配置/缺省错误: %+v", cfg)
	}
	b, _ := json.Marshal(cfg)
	if strings.Contains(string(b), "SECRET") {
		t.Fatal("token泄漏")
	}
	t.Setenv("MYBUILDS_SERVER_URL", "http://evil.example")
	t.Setenv("MYBUILDS_CLIENT_TOKEN", strings.Repeat("OTHER", 8))
	cfg, err = LoadAgent(AgentLoadOptions{Filename: p, Explicit: true})
	if err != nil || cfg.Server != "https://build.example.com" {
		t.Fatal("未声明环境污染Agent", err)
	}
}
func TestAgentStrictConfiguration(t *testing.T) {
	clearAgentEnvironment(t)
	cases := []string{"unknown: SECRET\n", "capacity: 0\n", "capacity: 33\n", "capacity: '2'\n", "heartbeat_interval: 5\n", "heartbeat_interval: null\n", "heartbeat_interval: 0s\n", "lease_duration: 9s\n", "heartbeat_interval: 5s\nlease_duration: 20s\n", "heartbeat_interval: 5s\nheartbeat_interval: 6s\n", "ca_file: 7\n"}
	for _, extra := range cases {
		p := writeManagementConfig(t, agentBody()+extra)
		_, err := LoadAgent(AgentLoadOptions{Filename: p, Explicit: true})
		if err == nil || strings.Contains(err.Error(), "SECRET") {
			t.Fatalf("严格配置失败 %q: %v", extra, err)
		}
	}
	for _, body := range []string{"node: n\n", strings.ReplaceAll(agentBody(), "mac-build-01", "../node"), strings.ReplaceAll(agentBody(), "https://build.example.com", "http://build.example.com"), strings.ReplaceAll(agentBody(), "https://build.example.com", "https://user:SECRET@build.example.com")} {
		_, err := LoadAgent(AgentLoadOptions{Filename: writeManagementConfig(t, body), Explicit: true})
		if err == nil || strings.Contains(err.Error(), "SECRET") {
			t.Fatal("缺失/非法身份或URL被接受", err)
		}
	}
	if _, err := LoadAgent(AgentLoadOptions{Filename: filepath.Join(t.TempDir(), "missing")}); err == nil {
		t.Fatal("Agent缺失配置被忽略")
	}
}
func TestAgentPrivateFilesAndReferences(t *testing.T) {
	clearAgentEnvironment(t)
	literal := strings.Repeat("secret", 8) + "${DO_NOT_READ}"
	t.Setenv("AGENT_TOKEN_REFERENCE", literal)
	t.Setenv("DO_NOT_READ", "wrong")
	p := writeManagementConfig(t, "server: http://localhost:8787\nnode: node-a\ntoken: '${AGENT_TOKEN_REFERENCE}'\n")
	cfg, err := LoadAgent(AgentLoadOptions{Filename: p, Explicit: true})
	if err != nil || cfg.RuntimeToken != literal || cfg.TokenEnv != "AGENT_TOKEN_REFERENCE" {
		t.Fatal("引用递归或来源丢失", err)
	}
	t.Setenv("MYBUILDS_AGENT_TOKEN", strings.Repeat("override", 8))
	cfg, err = LoadAgent(AgentLoadOptions{Filename: p, Explicit: true})
	if err != nil || cfg.RuntimeToken != strings.Repeat("override", 8) || cfg.TokenEnv != "MYBUILDS_AGENT_TOKEN" {
		t.Fatal("覆盖错误", err)
	}
	os.Chmod(p, 0644)
	if _, err := LoadAgent(AgentLoadOptions{Filename: p, Explicit: true}); err == nil {
		t.Fatal("公开token声明文件被接受")
	}
	os.Chmod(p, 0600)
	secret := filepath.Join(filepath.Dir(p), "secrets.env")
	os.WriteFile(secret, []byte("DECLARED=value\n"), 0644)
	p = writeManagementConfig(t, agentBody()+"secrets_file: '"+secret+"'\n")
	if _, err := LoadAgent(AgentLoadOptions{Filename: p, Explicit: true}); err == nil {
		t.Fatal("弱权限secret被接受")
	}
	os.Chmod(secret, 0600)
	if _, err := LoadAgent(AgentLoadOptions{Filename: p, Explicit: true}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "data")
	os.Mkdir(dir, 0755)
	p = writeManagementConfig(t, agentBody()+"data_dir: '"+dir+"'\n")
	if _, err := LoadAgent(AgentLoadOptions{Filename: p, Explicit: true}); err == nil {
		t.Fatal("弱权限data_dir被接受")
	}
	os.Chmod(dir, 0700)
	if _, err := LoadAgent(AgentLoadOptions{Filename: p, Explicit: true}); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	os.Symlink(p, link)
	if _, err := LoadAgent(AgentLoadOptions{Filename: link, Explicit: true}); err == nil {
		t.Fatal("链接配置被读取")
	}
}
