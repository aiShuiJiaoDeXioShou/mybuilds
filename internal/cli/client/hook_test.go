package client

import (
	"context"
	"encoding/json"
	"mybuilds/internal/config"
	"mybuilds/internal/server"
	"mybuilds/internal/store"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebhookActualCLIManagementAndStrictSecretResponse(t *testing.T) {
	dir := t.TempDir()
	if e := os.Chmod(dir, 0700); e != nil {
		t.Fatal(e)
	}
	st, e := store.Open(context.Background(), store.Options{Driver: "sqlite", DSN: filepath.Join(dir, "control.db")})
	if e != nil {
		t.Fatal(e)
	}
	defer st.Close()
	if e = st.Migrate(context.Background()); e != nil {
		t.Fatal(e)
	}
	token := strings.Repeat("CLI_ADMIN_PRIVATE_", 3)
	if e = st.Bootstrap(context.Background(), token); e != nil {
		t.Fatal(e)
	}
	t.Setenv("MYBUILDS_CLIENT_TOKEN", token)
	api := httptest.NewServer(server.New(st, config.ServerConfig{DataDir: dir}).Handler())
	defer api.Close()
	base := []string{"--server-url", api.URL}
	invoke := func(args ...string) (string, error) {
		return executeRemote(t, append(append([]string{}, base...), args...)...)
	}
	out, e := invoke("project", "init", "hook-app", "--repo", filepath.Join(dir, "trusted-repository"), "--nodes", "node", "--default-node", "node", "--hook", "--hook-repository-key", "repo", "--settings", writeHookSettings(t, dir), "--json")
	if e != nil {
		t.Fatal(out, e)
	}
	var created server.ProjectConfigured
	if e = json.Unmarshal([]byte(out), &created); e != nil || created.Webhook == nil || len(created.Webhook.Secret) < 32 {
		t.Fatal("初始化未返回一次密钥", out, e)
	}
	secret := created.Webhook.Secret
	for _, action := range []string{"show", "events", "windows"} {
		out, e = invoke("project", "hook", action, "hook-app", "--json")
		if e != nil || strings.Contains(out, secret) || strings.Contains(out, "PRIVATE_") {
			t.Fatal("实际CLI安全视图", out, e)
		}
	}
	out, e = invoke("project", "hook", "rotate", "hook-app", "--json")
	var rotated server.WebhookConfigured
	json.Unmarshal([]byte(out), &rotated)
	if e != nil || rotated.Secret == "" || rotated.Secret == secret {
		t.Fatal("实际轮换", out, e)
	}
	out, e = invoke("project", "hook", "disable", "hook-app", "--json")
	if e != nil || strings.Contains(out, rotated.Secret) || !strings.Contains(out, `"enabled":false`) {
		t.Fatal("实际禁用", out, e)
	}
}
func writeHookSettings(t *testing.T, dir string) string {
	t.Helper()
	name := filepath.Join(dir, "hook-settings.yml")
	if e := os.WriteFile(name, []byte("triggers:\n  builds: [default]\n  quiet_period: 1s\n"), 0600); e != nil {
		t.Fatal(e)
	}
	return name
}
