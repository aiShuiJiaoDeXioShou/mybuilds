package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebhookLocalSafePagination(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(directory, "server.yml")
	if err := os.WriteFile(filename, []byte("data_dir: .\ndatabase: {driver: sqlite, dsn: db.sqlite}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(directory, "settings.yml")
	if err := os.WriteFile(settings, []byte("triggers: {builds: [default]}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	base := []string{"--config", filename}
	out, err := executeManagement(t, append(base, "project", "add", "app", "--repo", directory, "--nodes", "node", "--hook", "--hook-repository-key", "repo", "--settings", settings, "--json")...)
	if err != nil || !strings.Contains(out, `"secret"`) {
		t.Fatal("本机初始化", err)
	}
	for _, action := range []string{"events", "windows"} {
		out, err = executeManagement(t, append(base, "project", "hook", action, "app", "--limit", "1", "--offset", "0", "--json")...)
		if err != nil || !strings.Contains(out, `"items":[]`) || strings.Contains(out, directory) || strings.Contains(out, `"secret"`) {
			t.Fatal("本机安全分页", out, err)
		}
		if _, err = executeManagement(t, append(base, "project", "hook", action, "app", "--limit", "201")...); err == nil {
			t.Fatal("分页越界未拒绝")
		}
	}
	if _, err = executeManagement(t, append(base, "project", "hook", "rotate", "app")...); err == nil {
		t.Fatal("一次秘密不能落表格")
	}
}
