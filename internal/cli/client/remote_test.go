package client

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func executeRemote(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := NewCommand()
	var output, diagnostics bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&diagnostics)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return output.String(), err
}
func TestRemoteStatusAuthenticationAndFixedErrors(t *testing.T) {
	secret := strings.Repeat("REMOTE_SECRET", 4)
	t.Setenv("MYBUILDS_CLIENT_TOKEN", secret)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+secret || r.URL.Path != "/api/status" {
			t.Error("实际HTTP认证/路径错误")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"projects":0,"queued":0,"skipped":0,"running":0,"nodes":0}`))
	}))
	defer api.Close()
	out, err := executeRemote(t, "--server-url", api.URL, "status", "--json")
	if err != nil || !strings.Contains(out, "queued") || strings.Contains(out, secret) {
		t.Fatal(out, err)
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500); w.Write([]byte(secret)) }))
	defer bad.Close()
	out, err = executeRemote(t, "--server-url", bad.URL, "status")
	if err == nil || strings.Contains(out, secret) || strings.Contains(err.Error(), secret) {
		t.Fatal("HTTP错误泄露/未失败", err)
	}
}
func TestRemoteRedirectAndLocalConfigIsolation(t *testing.T) {
	t.Setenv("MYBUILDS_CLIENT_TOKEN", strings.Repeat("secret", 8))
	called := false
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer destination.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, 302) }))
	defer redirect.Close()
	if _, err := executeRemote(t, "--server-url", redirect.URL, "status"); err == nil || called {
		t.Fatal("重定向携带凭据")
	}
	filename := filepath.Join(t.TempDir(), "bad.yml")
	os.WriteFile(filename, []byte("invalid: SECRET\n"), 0600)
	for _, args := range [][]string{{"--config", filename, "--timeout", "1ns", "version"}, {"--config", filename, "--timeout", "1ns", "--help"}} {
		if _, err := executeRemote(t, args...); err != nil {
			t.Fatal("本地命令读取远端配置", err)
		}
	}
}

func TestDamagedRemoteConfigurationDoesNotAffectLocalWork(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("MYBUILDS_CLIENT_TOKEN", "")
	filename := filepath.Join(t.TempDir(), "broken-client.yml")
	if err := os.WriteFile(filename, []byte("unknown: REMOTE_SECRET\n"), 0600); err != nil {
		t.Fatal(err)
	}
	base := []string{"--config", filename, "--timeout", "1ns"}
	if _, err := executeRemote(t, append(base, "init")...); err != nil {
		t.Fatal("本地init依赖远程配置", err)
	}
	pipeline := "version: 1\ntimeout: 1s\nsteps:\n  - kind: run\n    run: sleep 0.03; printf local_ok\n"
	if err := os.WriteFile("mybuilds.yml", []byte(pipeline), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := executeRemote(t, append(base, "run", "--dry-run")...)
	if err != nil || !strings.Contains(out, "ready") {
		t.Fatal("本地preview依赖远程配置", err)
	}
	out, err = executeRemote(t, append(base, "run")...)
	if err != nil || !strings.Contains(out, "succeeded") {
		t.Fatal("remote timeout改变本地预算", err)
	}
	out, err = executeRemote(t, append(base, "doctor", "--json", "--gradle-wrapper", "missing-gradlew")...)
	if !json.Valid([]byte(out)) || err != nil && strings.Contains(err.Error(), "客户端") {
		t.Fatal("本地doctor依赖远程配置", err)
	}
}
