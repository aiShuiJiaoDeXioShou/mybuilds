//go:build darwin || linux

package store

import (
	"errors"
	"gorm.io/gorm"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestLockChild(t *testing.T) {
	if os.Getenv("STORE006_CHILD") != "try" {
		return
	}
	s, err := Open(testContext, Options{Driver: os.Getenv("STORE006_DRIVER"), DSN: os.Getenv("STORE006_DSN")})
	if errors.Is(err, ErrLocked) {
		os.Exit(23)
	}
	if err != nil {
		os.Exit(24)
	}
	s.Close()
	os.Exit(0)
}

func TestIndependentProcessLock(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		child := func(want int) {
			t.Helper()
			cmd := exec.Command(os.Args[0], "-test.run=^TestLockChild$")
			dsn := opt.DSN
			if opt.Driver == "postgres" {
				dsn += " application_name=store006_child_alias"
			}
			cmd.Env = append(os.Environ(), "STORE006_CHILD=try", "STORE006_DRIVER="+opt.Driver, "STORE006_DSN="+dsn)
			err := cmd.Run()
			code := 0
			if err != nil {
				var e *exec.ExitError
				if !errors.As(err, &e) {
					t.Fatal(err)
				}
				code = e.ExitCode()
			}
			if code != want {
				t.Fatalf("child exit=%d want=%d", code, want)
			}
		}
		child(23)
		if opt.Driver == "sqlite" {
			alias := filepath.Join(t.TempDir(), "alias.db")
			if err := os.Symlink(opt.DSN, alias); err != nil {
				t.Fatal(err)
			}
			another, err := Open(testContext, Options{Driver: "sqlite", DSN: alias})
			if another != nil {
				another.Close()
			}
			if !errors.Is(err, ErrLocked) {
				t.Fatalf("alias: %v", err)
			}
			hard := filepath.Join(t.TempDir(), "hard.db")
			if err := os.Link(opt.DSN, hard); err != nil {
				t.Fatal(err)
			}
			another, err = Open(testContext, Options{Driver: "sqlite", DSN: hard})
			if another != nil {
				another.Close()
			}
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("hardlink: %v", err)
			}
			os.Remove(hard)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		if opt.Driver == "sqlite" {
			if _, err := os.Stat(opt.DSN + ".lock"); err != nil {
				t.Fatal("Close 删除了 flock 文件")
			}
		}
		child(0)
	})
}

func TestLostLockNeverWrites(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		if opt.Driver == "sqlite" {
			lock := opt.DSN + ".lock"
			if err := os.Rename(lock, lock+".old"); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(lock, nil, 0600); err != nil {
				t.Fatal(err)
			}
		} else {
			var pid int
			if err := s.conn.QueryRowContext(testContext, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
				t.Fatal(err)
			}
			pool, _ := s.db.DB()
			if _, err := pool.ExecContext(testContext, "SELECT pg_terminate_backend($1)", pid); err != nil {
				t.Fatal("terminate own session")
			}
		}
		if err := s.CheckLock(testContext); !errors.Is(err, ErrLockLost) {
			t.Fatalf("check lost: %v", err)
		}
		if _, err := s.CreateGroup(testContext, localAdmin, "forbidden-after-loss"); !errors.Is(err, ErrLockLost) {
			t.Fatalf("write after loss: %v", err)
		}
		var count int64
		s.db.Model(&groupRecord{}).Where("name = ?", "forbidden-after-loss").Count(&count)
		if count != 0 {
			t.Fatal("write committed after loss")
		}
		if err := s.CheckLock(testContext); !errors.Is(err, ErrLockLost) {
			t.Fatalf("loss recovered: %v", err)
		}
	})
}

func TestLockLostDuringTransactionRollsBack(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		started := make(chan struct{})
		proceed := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			done <- s.write(testContext, func(tx *gorm.DB) error {
				err := tx.Create(&groupRecord{ID: "uncommitted", Name: "uncommitted"}).Error
				close(started)
				<-proceed
				return err
			})
		}()
		<-started
		if opt.Driver == "sqlite" {
			if err := os.Rename(opt.DSN+".lock", opt.DSN+".lock.old"); err != nil {
				close(proceed)
				t.Fatal(err)
			}
			if err := os.WriteFile(opt.DSN+".lock", nil, 0600); err != nil {
				close(proceed)
				t.Fatal(err)
			}
		} else {
			pool, _ := s.db.DB()
			var pid int
			if err := pool.QueryRowContext(testContext, "SELECT pid FROM pg_locks WHERE locktype = 'advisory' AND classid = 1973481521 AND objid = 6 AND objsubid = 2 AND granted").Scan(&pid); err != nil {
				close(proceed)
				t.Fatal("own writer pid")
			}
			if _, err := pool.ExecContext(testContext, "SELECT pg_terminate_backend($1)", pid); err != nil {
				close(proceed)
				t.Fatal("terminate own transaction")
			}
		}
		close(proceed)
		if err := <-done; !errors.Is(err, ErrLockLost) {
			t.Fatalf("mid-transaction loss: %v", err)
		}
		var count int64
		if err := s.db.Model(&groupRecord{}).Where("name = ?", "uncommitted").Count(&count).Error; err != nil || count != 0 {
			t.Fatal("transaction survived lost ownership")
		}
	})
}
