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

func realRemoteAPI(t *testing.T) (*store.Store, string) {
	t.Helper()
	db, err := store.Open(context.Background(), store.Options{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "control.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	created, err := db.CreateToken(context.Background(), store.Actor{ID: "local-admin", Role: "admin"}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("MYBUILDS_CLIENT_TOKEN", created.Token)
	api := httptest.NewServer(server.New(db, config.ServerConfig{DataDir: t.TempDir(), Concurrency: 1}).Handler())
	t.Cleanup(api.Close)
	return db, api.URL
}
func TestActualRemoteGroupProjectStatus(t *testing.T) {
	_, address := realRemoteAPI(t)
	directory := t.TempDir()
	t.Chdir(directory)
	if err := os.WriteFile("settings.yml", []byte("pipeline: {builds: {android: {params: {channel: SETTINGS_SECRET}}}}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	call := func(args ...string) string {
		t.Helper()
		out, err := executeRemote(t, append([]string{"--server-url", address}, args...)...)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out, "SETTINGS_SECRET") {
			t.Fatal("设置泄露")
		}
		return out
	}
	call("group", "create", "mobile")
	call("group", "rename", "mobile", "--name", "apps")
	out := call("group", "ls", "--json")
	if !strings.Contains(out, "apps") {
		t.Fatal(out)
	}
	call("project", "init", "app", "--repo", directory, "--nodes", "mac-1", "--settings", "settings.yml", "--json")
	call("project", "move", "app", "--group", "apps")
	call("project", "set", "app", "--settings", "settings.yml")
	out = call("project", "ls", "--group", "apps", "--json")
	var result struct {
		Items []server.ProjectView `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil || len(result.Items) != 1 || result.Items[0].Name != "app" || result.Items[0].Group != "apps" {
		t.Fatal(out, err)
	}
	if strings.Contains(out, directory) {
		t.Fatal("仓库URL泄露")
	}
	call("status", "--json")
	call("project", "rm", "app")
	call("group", "rm", "apps")
	for _, args := range [][]string{{"project", "init", "bad", "--repo", directory, "--nodes", "mac-1", "--settings", "settings.yml", "--file", "mybuilds.yml"}, {"project", "init", "bad", "--repo", directory, "--nodes", "mac-1", "--hook"}, {"project", "ls", "--limit", "201"}, {"group", "rename", "default", "--name", "bad"}} {
		if _, err := executeRemote(t, append([]string{"--server-url", address}, args...)...); err == nil {
			t.Fatal("非法管理命令被接受")
		}
	}
}
