package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func clearClientEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{"MYBUILDS_SERVER_URL", "MYBUILDS_CLIENT_TOKEN", "MYBUILDS_CLIENT_TIMEOUT"} {
		old, present := os.LookupEnv(key)
		os.Unsetenv(key)
		t.Cleanup(func() {
			if present {
				os.Setenv(key, old)
			} else {
				os.Unsetenv(key)
			}
		})
	}
}
func TestClientStrictAndPrivate(t *testing.T) {
	clearClientEnvironment(t)
	token := strings.Repeat("SECRET", 8)
	for _, body := range []string{"server: 7\n", "token: null\n", "timeout: 30\n", "timeout: 0s\n", "unknown: SECRET\n", "token: weak\n", "token: '${MISSING_CONFIG_SECRET}'\n", "server: http://example.com\ntoken: '" + token + "'\n", "server: https://user:SECRET@example.com\ntoken: '" + token + "'\n", "server: https://example.com/?SECRET\ntoken: '" + token + "'\n", "server: https://example.com/#SECRET\ntoken: '" + token + "'\n", "token: '" + token + "'\ntoken: '" + token + "'\n"} {
		_, err := LoadClient(ClientLoadOptions{Filename: writeManagementConfig(t, body), Explicit: true})
		if err == nil || strings.Contains(err.Error(), "SECRET") {
			t.Fatalf("严格校验/脱敏失败: %v", err)
		}
	}
	p := writeManagementConfig(t, "token: '"+token+"'\n")
	if err := os.Chmod(p, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadClient(ClientLoadOptions{Filename: p, Explicit: true}); err == nil {
		t.Fatal("公开凭据文件被接受")
	}
}
func TestClientPrecedenceReferencesAndJSON(t *testing.T) {
	clearClientEnvironment(t)
	token := strings.Repeat("SECRET", 8)
	t.Setenv("CONFIG_CLIENT_SECRET", token)
	p := writeManagementConfig(t, "server: https://example.com\ntimeout: 10s\ntoken: '${CONFIG_CLIENT_SECRET}'\n")
	t.Setenv("MYBUILDS_SERVER_URL", "http://localhost:8788")
	t.Setenv("MYBUILDS_CLIENT_TIMEOUT", "20s")
	timeout := time.Second
	server := "http://127.0.0.1:8789"
	cfg, err := LoadClient(ClientLoadOptions{Filename: p, Explicit: true, ServerURL: &server, Timeout: &timeout})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server != server || cfg.Timeout != timeout || cfg.RuntimeToken != token {
		t.Fatal("覆盖优先级错误")
	}
	b, err := json.Marshal(cfg)
	if err != nil || strings.Contains(string(b), "SECRET") {
		t.Fatal("公开JSON泄露token")
	}
	t.Setenv("MYBUILDS_CLIENT_TOKEN", "weak")
	if _, err := LoadClient(ClientLoadOptions{Filename: p, Explicit: true}); err == nil {
		t.Fatal("弱环境token被接受")
	}
}
func TestClientMissingAndProjectLoad(t *testing.T) {
	clearClientEnvironment(t)
	t.Setenv("MYBUILDS_CLIENT_TOKEN", strings.Repeat("secret", 8))
	p := t.TempDir() + "/missing.yml"
	cfg, err := LoadClient(ClientLoadOptions{Filename: p})
	if err != nil || cfg.Timeout != 30*time.Second {
		t.Fatal("默认配置错误", err)
	}
	if _, err := LoadClient(ClientLoadOptions{Filename: p, Explicit: true}); err == nil {
		t.Fatal("显式缺失被忽略")
	}
	settings, err := LoadProjectSettings(writeManagementConfig(t, "pipeline: {params: {channel: dev}}\n"))
	if err != nil || settings.Pipeline.Params["channel"] != "dev" {
		t.Fatal("settings加载错误", err)
	}
}

func TestClientReferenceIsNotRecursive(t *testing.T) {
	clearClientEnvironment(t)
	literal := strings.Repeat("token", 8) + "${DO_NOT_READ}"
	t.Setenv("CONFIG_CLIENT_SECRET", literal)
	t.Setenv("DO_NOT_READ", "unexpected")
	cfg, err := LoadClient(ClientLoadOptions{Filename: writeManagementConfig(t, "token: '${CONFIG_CLIENT_SECRET}'\n"), Explicit: true})
	if err != nil || cfg.RuntimeToken != literal {
		t.Fatal("环境引用被递归解释", err)
	}
}

func TestClientCAConfigurationPrecedence(t *testing.T) {
	clearClientEnvironment(t)
	t.Setenv("MYBUILDS_CA_FILE", "")
	p := writeManagementConfig(t, "token: '"+strings.Repeat("SECRET", 8)+"'\nca_file: file-ca.pem\n")
	os.Unsetenv("MYBUILDS_CA_FILE")
	cfg, err := LoadClient(ClientLoadOptions{Filename: p, Explicit: true})
	if err != nil || cfg.CAFile != filepath.Join(filepath.Dir(p), "file-ca.pem") {
		t.Fatal("CA相对路径错误", err)
	}
	t.Setenv("MYBUILDS_CA_FILE", "env-ca.pem")
	cli := "cli-ca.pem"
	cfg, err = LoadClient(ClientLoadOptions{Filename: p, Explicit: true, CAFile: &cli})
	if err != nil || cfg.CAFile != filepath.Join(filepath.Dir(p), cli) {
		t.Fatal("CA覆盖错误", err)
	}
	if _, err := LoadClient(ClientLoadOptions{Filename: writeManagementConfig(t, "ca_file: 7\n"), Explicit: true}); err == nil {
		t.Fatal("非字符串CA被接受")
	}
}
