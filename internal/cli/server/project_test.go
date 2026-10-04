package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalProjectSettingsAndRegistration(t *testing.T) {
	directory := t.TempDir()
	filename := filepath.Join(directory, "server.yml")
	os.WriteFile(filename, []byte("database: {driver: sqlite, dsn: db.sqlite}\n"), 0600)
	settings := filepath.Join(directory, "settings.yml")
	os.WriteFile(settings, []byte("pipeline: {builds: {android: {params: {channel: PARAM_SECRET}}}}\n"), 0600)
	base := []string{"--config", filename}
	call := func(args ...string) string {
		t.Helper()
		out, err := executeManagement(t, append(base, args...)...)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out, "PARAM_SECRET") {
			t.Fatal("公开项目视图泄露settings")
		}
		return out
	}
	call("group", "create", "mobile")
	call("project", "add", "app", "--repo", directory, "--nodes", "mac-1", "--settings", settings, "--json")
	out := call("project", "ls", "--json")
	if !strings.Contains(out, "app") || strings.Contains(out, directory) {
		t.Fatal("公开项目视图错误", out)
	}
	call("project", "move", "app", "--group", "mobile")
	call("project", "set", "app", "--settings", settings)
	call("project", "rm", "app")
	if _, err := executeManagement(t, append(base, "project", "add", "bad", "--repo", directory, "--nodes", "mac-1", "--settings", settings, "--file", "mybuilds.yml")...); err == nil {
		t.Fatal("settings/file冲突未拒绝")
	}
	if _, err := executeManagement(t, append(base, "project", "add", "bad", "--repo", directory, "--nodes", "mac-1", "--platform", "ios")...); err == nil {
		t.Fatal("未实现平台选项被接受")
	}
}
