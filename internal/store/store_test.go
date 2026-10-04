package store

import (
	"context"
	"database/sql"
	"fmt"
	_ "github.com/jackc/pgx/v5/stdlib"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var testContext = context.Background()
var localAdmin = Actor{ID: "local-admin", Role: "admin"}

// 每个用例建立自有 schema；不与其他分区共享业务表。
func stores(t *testing.T, fn func(*testing.T, *Store, Options)) {
	t.Helper()
	drivers := []string{"sqlite"}
	if os.Getenv("MYBUILDS_TEST_POSTGRES_DSN") == "" {
		t.Log("未提供 MYBUILDS_TEST_POSTGRES_DSN；本次仅验证 SQLite，PostgreSQL 待真实验证")
	}
	if os.Getenv("MYBUILDS_TEST_POSTGRES_DSN") != "" {
		drivers = append(drivers, "postgres")
	}
	for _, driver := range drivers {
		t.Run(driver, func(t *testing.T) {
			opt := Options{Driver: driver, DSN: filepath.Join(t.TempDir(), "control.db")}
			if driver == "postgres" {
				dsn := os.Getenv("MYBUILDS_TEST_POSTGRES_DSN")
				db, err := sql.Open("pgx", dsn)
				if err != nil {
					t.Fatal("fixture open")
				}
				schema := fmt.Sprintf("store006_%d", time.Now().UnixNano())
				if _, err = db.Exec("CREATE SCHEMA " + schema); err != nil {
					t.Fatal("fixture schema")
				}
				t.Cleanup(func() { db.Exec("DROP SCHEMA " + schema + " CASCADE"); db.Close() })
				opt.DSN = dsn + " search_path=" + schema
			}
			s, err := Open(testContext, opt)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			t.Cleanup(func() { s.Close() })
			if err = s.Migrate(testContext); err != nil {
				t.Fatalf("migrate: %v", err)
			}
			fn(t, s, opt)
		})
	}
}

func TestMigrateAndStatus(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		if err := s.Migrate(testContext); err != nil {
			t.Fatal(err)
		}
		groups, err := s.ListGroups(testContext, Page{})
		if err != nil || len(groups) != 1 || groups[0].Name != "default" {
			t.Fatalf("default: %v %v", groups, err)
		}
		status, err := s.Status(testContext)
		if err != nil || status != (QueueStatus{}) {
			t.Fatalf("status: %v %v", status, err)
		}
		if opt.Driver == "sqlite" {
			var version string
			s.db.Raw("SELECT sqlite_version()").Scan(&version)
			if version != "3.53.3" {
				t.Fatalf("SQLite version %s", version)
			}
			pool, _ := s.db.DB()
			var connections []*sql.Conn
			for i := 0; i < 4; i++ {
				c, e := pool.Conn(testContext)
				if e != nil {
					t.Fatal(e)
				}
				connections = append(connections, c)
			}
			defer func() {
				for _, c := range connections {
					c.Close()
				}
			}()
			for _, c := range connections {
				var wal string
				var fk, busy int
				c.QueryRowContext(testContext, "PRAGMA journal_mode").Scan(&wal)
				c.QueryRowContext(testContext, "PRAGMA foreign_keys").Scan(&fk)
				c.QueryRowContext(testContext, "PRAGMA busy_timeout").Scan(&busy)
				if wal != "wal" || fk != 1 || busy != 5000 {
					t.Fatalf("pragmas: %s/%d/%d", wal, fk, busy)
				}
			}
			t.Log("SQLite 3.53.3; four connections WAL/FK/busy verified")
		} else {
			var version string
			s.db.Raw("SHOW server_version").Scan(&version)
			t.Log("PostgreSQL", version)
		}
		if err = s.Close(); err != nil {
			t.Fatal(err)
		}
		reopened, err := Open(testContext, opt)
		if err != nil {
			t.Fatal(err)
		}
		defer reopened.Close()
		if err = reopened.Migrate(testContext); err != nil {
			t.Fatal(err)
		}
		groups, err = reopened.ListGroups(testContext, Page{})
		if err != nil || len(groups) != 1 {
			t.Fatal("restart default changed")
		}
	})
}

func TestSafeErrors(t *testing.T) {
	for _, opt := range []Options{{Driver: "password-marker", DSN: "secret-marker"}, {Driver: "sqlite", DSN: "file:secret-marker?mode=memory"}, {Driver: "postgres", DSN: "password=secret-marker invalid syntax"}} {
		s, err := Open(testContext, opt)
		if s != nil {
			s.Close()
		}
		if err == nil || strings.Contains(err.Error(), "marker") {
			t.Fatalf("unsafe error: %v", err)
		}
	}
}
