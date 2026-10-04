package server

import (
	"bytes"
	"context"
	"encoding/json"
	"mybuilds/internal/store"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func executeManagement(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := NewCommand()
	var out, diagnostics bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&diagnostics)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}
func TestRealLocalMigrateGroupTokenAndLock(t *testing.T) {
	directory := t.TempDir()
	filename := filepath.Join(directory, "server.yml")
	if err := os.WriteFile(filename, []byte("data_dir: data\ndatabase: {driver: sqlite, dsn: data/db.sqlite}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	call := func(args ...string) string {
		t.Helper()
		out, err := executeManagement(t, append([]string{"--config", filename}, args...)...)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	call("migrate")
	call("group", "create", "team")
	out := call("group", "ls", "--json")
	if !strings.Contains(out, "team") {
		t.Fatal(out)
	}
	call("group", "rename", "team", "--name", "mobile")
	call("group", "rm", "mobile")
	out = call("token", "create", "--role", "trigger", "--json")
	var token store.TokenCreated
	if err := json.Unmarshal([]byte(out), &token); err != nil || len(token.Token) < 32 {
		t.Fatal("token创建失败", err)
	}
	out = call("token", "ls", "--json")
	if strings.Contains(out, token.Token) {
		t.Fatal("token列表泄露")
	}
	call("token", "revoke", token.ID)
	db, err := store.Open(context.Background(), store.Options{Driver: "sqlite", DSN: filepath.Join(directory, "data/db.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := executeManagement(t, "--config", filename, "group", "create", "blocked"); err == nil {
		t.Fatal("本机管理绕过独占")
	}
}
