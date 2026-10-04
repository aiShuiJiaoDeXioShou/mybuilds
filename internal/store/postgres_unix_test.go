//go:build darwin || linux

package store

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPostgresOpenChild(t *testing.T) {
	if os.Getenv("STORE006_PARSE_CHILD") != "1" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	s, err := Open(ctx, Options{Driver: "postgres", DSN: os.Getenv("STORE006_PARSE_DSN")})
	if s != nil {
		s.Close()
	}
	if errors.Is(err, ErrInvalid) {
		os.Exit(0)
	}
	os.Exit(25)
}

// 解析期阻塞不受连接 context 控制，父进程负责有界终止并等待真实子进程。
func TestPostgresFIFOOpenBounded(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "secret-fifo-marker")
	if err := unix.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, dsn   string
		environment []string
	}{
		{"passfile", "host=127.0.0.1 port=1 user=test dbname=test sslmode=disable passfile=" + fifo, nil},
		{"service", "service=secret-service-marker servicefile=" + fifo, nil},
		{"sslrootcert", "host=127.0.0.1 port=1 user=test dbname=test sslmode=verify-full sslrootcert=" + fifo, nil},
		{"sslkey", "host=127.0.0.1 port=1 user=test dbname=test sslmode=require sslcert=" + fifo + " sslkey=" + fifo, nil},
		{"url-rootcert", "postgres://test@127.0.0.1:1/test?sslmode=verify-full&sslrootcert=" + fifo, nil},
		{"env-passfile", "host=127.0.0.1 port=1 user=test dbname=test sslmode=disable", []string{"PGPASSFILE=" + fifo}},
		{"env-service", "host=127.0.0.1 port=1 user=test dbname=test sslmode=disable", []string{"PGSERVICE=secret-service-marker", "PGSERVICEFILE=" + fifo}},
		{"env-cert", "host=127.0.0.1 port=1 user=test dbname=test sslmode=verify-full", []string{"PGSSLROOTCERT=" + fifo}},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPostgresOpenChild$")
			env := []string{}
			for _, value := range os.Environ() {
				if !strings.HasPrefix(value, "PG") {
					env = append(env, value)
				}
			}
			cmd.Env = append(env, "STORE006_PARSE_CHILD=1", "STORE006_PARSE_DSN="+item.dsn)
			cmd.Env = append(cmd.Env, item.environment...)
			output, err := cmd.CombinedOutput()
			if ctx.Err() != nil {
				t.Fatal("Store.Open blocked in synchronous DSN file parsing")
			}
			if err != nil {
				t.Fatal("Store.Open did not return fixed ErrInvalid")
			}
			if strings.Contains(string(output), "secret-") {
				t.Fatal("secret leaked into child diagnostics")
			}
		})
	}
}

func TestPostgresExplicitConfig(t *testing.T) {
	for _, dsn := range []string{
		"host='/tmp/store socket' port=55436 user='test' dbname='control' password='quoted\\'secret\\\\value' sslmode=disable search_path=isolated application_name=store006",
		"postgres://test:quoted%27secret%5Cvalue@127.0.0.1:55436/control?sslmode=disable&search_path=isolated&application_name=store006",
	} {
		parsed, err := postgresConfig(testContext, dsn)
		if err != nil {
			t.Fatal(err)
		}
		if parsed.Database != "control" || parsed.User != "test" || parsed.Password != "quoted'secret\\value" || parsed.Port != 55436 || parsed.RuntimeParams["search_path"] != "isolated" || parsed.RuntimeParams["application_name"] != "store006" {
			t.Fatal("explicit DSN fields changed")
		}
	}
	parsed, err := postgresConfig(testContext, "postgres://wrong:wrong@127.0.0.1:1/wrong?host=/tmp/explicit&port=55436&user=test&dbname=control&password=explicit&sslmode=disable")
	if err != nil || parsed.Host != "/tmp/explicit" || parsed.Port != 55436 || parsed.User != "test" || parsed.Database != "control" || parsed.Password != "explicit" {
		t.Fatal("URL query precedence changed")
	}
	t.Setenv("PGHOST", "implicit-secret-host-marker")
	t.Setenv("PGPASSWORD", "implicit-secret-password-marker")
	if _, err = postgresConfig(testContext, "host=127.0.0.1 user=test dbname=control sslmode=disable"); !errors.Is(err, ErrInvalid) {
		t.Fatal("implicit PG environment accepted")
	}
	s, err := Open(testContext, Options{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "not-postgres.db")})
	if err != nil {
		t.Fatal("PG environment affected SQLite")
	}
	s.Close()
}
