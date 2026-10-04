package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeManagementConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}
func clearServerEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{"MYBUILDS_LISTEN", "MYBUILDS_DATA_DIR", "MYBUILDS_CONCURRENCY", "MYBUILDS_DATABASE_DRIVER", "MYBUILDS_DATABASE_DSN", "MYBUILDS_SECRETS_FILE"} {
		previous, present := os.LookupEnv(key)
		os.Unsetenv(key)
		t.Cleanup(func() {
			if present {
				os.Setenv(key, previous)
			} else {
				os.Unsetenv(key)
			}
		})
	}
}
func TestServerStrictConfig(t *testing.T) {
	clearServerEnvironment(t)
	for _, body := range []string{"listen: TOKEN_SECRET\n", "database: {driver: 7}\n", "database: {driver: unknown}\n", "concurrency: \"2\"\n", "concurrency: 0\n", "listen: null\n", "listen: 127.0.0.1:8787\nlisten: TOKEN_SECRET\n", "listen: 127.0.0.1:8787\n---\nlisten: TOKEN_SECRET\n", "database: {driver: sqlite, unknown: TOKEN_SECRET}\n", "defaults: {notifications: TOKEN_SECRET}\n", "x: TOKEN_SECRET\n", "listen: &x 127.0.0.1:8787\ndata_dir: *x\n"} {
		t.Run(body, func(t *testing.T) {
			_, err := LoadServer(ServerLoadOptions{Filename: writeManagementConfig(t, body), Explicit: true})
			if err == nil {
				t.Fatal("非法配置被接受")
			}
			if strings.Contains(err.Error(), "TOKEN_SECRET") {
				t.Fatal("错误泄露输入")
			}
		})
	}
}
func TestServerPrecedenceAndPaths(t *testing.T) {
	clearServerEnvironment(t)
	p := writeManagementConfig(t, "listen: 127.0.0.1:8788\nconcurrency: 2\ndata_dir: data\ndatabase: {driver: sqlite, dsn: data/database.db}\nsecrets_file: secrets.env\n")
	t.Setenv("MYBUILDS_CONCURRENCY", "3")
	t.Setenv("MYBUILDS_LISTEN", "127.0.0.1:8789")
	n := 4
	cfg, err := LoadServer(ServerLoadOptions{Filename: p, Explicit: true, CLI: ServerOverrides{Concurrency: &n}})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Concurrency != 4 || cfg.Listen != "127.0.0.1:8789" || cfg.Database.DSN != filepath.Join(filepath.Dir(p), "data/database.db") || cfg.DataDir != filepath.Join(filepath.Dir(p), "data") || cfg.SecretsFile != filepath.Join(filepath.Dir(p), "secrets.env") {
		t.Fatalf("覆盖或路径错误: %#v", cfg)
	}
	t.Setenv("MYBUILDS_CONCURRENCY", "TOKEN_SECRET")
	if _, err := LoadServer(ServerLoadOptions{Filename: p, Explicit: true}); err == nil || strings.Contains(err.Error(), "TOKEN_SECRET") {
		t.Fatalf("环境值校验错误: %v", err)
	}
}
func TestServerDefaultMissingAndNonregular(t *testing.T) {
	clearServerEnvironment(t)
	p := filepath.Join(t.TempDir(), "missing.yml")
	cfg, err := LoadServer(ServerLoadOptions{Filename: p})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "127.0.0.1:8787" || cfg.Concurrency != 1 || cfg.Database.Driver != "sqlite" || cfg.Database.DSN != filepath.Join(cfg.DataDir, "mybuilds.db") {
		t.Fatal("缺省错误")
	}
	if _, err := LoadServer(ServerLoadOptions{Filename: p, Explicit: true}); err == nil {
		t.Fatal("显式缺失未失败")
	}
	if _, err := LoadServer(ServerLoadOptions{Filename: filepath.Dir(p), Explicit: true}); err == nil {
		t.Fatal("目录被读取")
	}
	regular := writeManagementConfig(t, "concurrency: 1\n")
	link := filepath.Join(t.TempDir(), "leaf.yml")
	if err := os.Symlink(regular, link); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadServer(ServerLoadOptions{Filename: link, Explicit: true}); err == nil {
		t.Fatal("叶子链接未拒绝")
	}
	large := writeManagementConfig(t, strings.Repeat("x", MaxConfigBytes+1))
	if _, err := LoadServer(ServerLoadOptions{Filename: large, Explicit: true}); err == nil {
		t.Fatal("超限未拒绝")
	}
}

func TestServerPostgresRequiresDSNAndListenHost(t *testing.T) {
	clearServerEnvironment(t)
	for _, body := range []string{"database: {driver: postgres}\n", "database: {driver: postgres, dsn: \"\"}\n", "listen: 'bad host:8787'\n", "listen: 'bad_host:8787'\n", "listen: '-host:8787'\n", "listen: 'a..b:8787'\n", "listen: \"host\\tname:8787\"\n"} {
		if _, err := LoadServer(ServerLoadOptions{Filename: writeManagementConfig(t, body), Explicit: true}); err == nil {
			t.Fatal("非法监听或缺少连接配置被接受")
		}
	}
	for _, address := range []string{"localhost:8787", "build.example:8787", "[::1]:8787", ":8787"} {
		if _, err := LoadServer(ServerLoadOptions{Filename: writeManagementConfig(t, "listen: '"+address+"'\n"), Explicit: true}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestServerLeasePolicy(t *testing.T) {
	clearServerEnvironment(t)
	cfg, err := LoadServer(ServerLoadOptions{Filename: writeManagementConfig(t, "heartbeat_interval: 1s\nlease_duration: 10s\n"), Explicit: true})
	if err != nil || cfg.HeartbeatInterval != time.Second || cfg.LeaseDuration != 10*time.Second {
		t.Fatal("策略加载失败", err)
	}
	cfg, err = LoadServer(ServerLoadOptions{Filename: filepath.Join(t.TempDir(), "missing")})
	if err != nil || cfg.HeartbeatInterval != 5*time.Second || cfg.LeaseDuration != 30*time.Second {
		t.Fatal("缺省策略失败", err)
	}
	for _, body := range []string{"heartbeat_interval: 5\n", "lease_duration: null\n", "heartbeat_interval: 31s\n", "lease_duration: 181s\n", "heartbeat_interval: 5s\nlease_duration: 21s\n"} {
		if _, err := LoadServer(ServerLoadOptions{Filename: writeManagementConfig(t, body), Explicit: true}); err == nil {
			t.Fatal("非法策略被接受")
		}
	}
}
