package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mybuilds/internal/store"
)

func TestActualLocalNodeCommands(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "server.yml")
	os.WriteFile(cfg, []byte("data_dir: data\ndatabase: {driver: sqlite, dsn: data/db.sqlite}\n"), 0600)
	call := func(args ...string) string {
		t.Helper()
		out, err := executeManagement(t, append([]string{"--config", cfg}, args...)...)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	out := call("node", "create", "mac-node", "--labels", "android,release", "--capacity", "2", "--json")
	var created struct{ Token string }
	if err := json.Unmarshal([]byte(out), &created); err != nil || len(created.Token) < 32 {
		t.Fatal(out, err)
	}
	for _, args := range [][]string{{"node", "ls", "--json"}, {"node", "show", "mac-node", "--json"}} {
		out := call(args...)
		if !strings.Contains(out, "mac-node") || strings.Contains(out, created.Token) {
			t.Fatal("节点视图泄密", out)
		}
	}
	for _, action := range []string{"drain", "enable", "disable"} {
		call("node", action, "mac-node")
	}
	call("node", "token", "rotate", "mac-node", "--json")
	call("node", "token", "revoke", "mac-node")
	call("node", "rm", "mac-node")
	for _, args := range [][]string{{"node", "create", "bad", "--capacity", "0"}, {"node", "create", "mac-node"}, {"node", "ls", "--limit", "201"}, {"node", "create", "new", "--platform", "ios"}} {
		if _, err := executeManagement(t, append([]string{"--config", cfg}, args...)...); err == nil {
			t.Fatal("非法参数被接受", args)
		}
	}
	db, err := store.Open(context.Background(), store.Options{Driver: "sqlite", DSN: filepath.Join(dir, "data/db.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := executeManagement(t, "--config", cfg, "node", "create", "blocked"); err == nil {
		t.Fatal("本机节点管理绕独占")
	}
}

func TestLocalUnknownNodeActionsFailBeforeConfig(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "bad-server.yml")
	if err := os.WriteFile(bad, []byte("unknown: PRIVATE_SECRET\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"PRIVATE_UNKNOWN"}, {"node", "PRIVATE_UNKNOWN"}, {"node", "token", "PRIVATE_UNKNOWN"}} {
		out, err := executeManagement(t, append([]string{"--config", bad}, args...)...)
		if err == nil || strings.Contains(out+err.Error(), "PRIVATE_UNKNOWN") {
			t.Fatalf("非法操作未安全拒绝 %q %v", out, err)
		}
	}
}
