//go:build darwin || linux

package store

import (
	"os"
	"reflect"
	"testing"

	"mybuilds/internal/protocol"
)

func TestRecoveryRetryReceiptCannotUseLostControlLock(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, e := terminalReceiptFixture(t, s)
		before := recoveryRows(t, s)
		if opt.Driver == "sqlite" {
			if err := os.Rename(opt.DSN+".lock", opt.DSN+".lock.original"); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(opt.DSN+".lock", nil, 0600); err != nil {
				t.Fatal(err)
			}
		} else {
			var pid int
			if err := s.conn.QueryRowContext(testContext, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
				t.Fatal(err)
			}
			pool, _ := s.db.DB()
			if _, err := pool.ExecContext(testContext, "SELECT pg_terminate_backend($1)", pid); err != nil {
				t.Fatal("仅终止自己的锁session", err)
			}
		}
		if err := s.Recover(testContext); err != ErrLockLost {
			t.Fatal("恢复失锁", err)
		}
		if _, err := s.Retry(testContext, localAdmin, RetryInput{BuildID: e.Ref.BuildID, Key: "lost"}); err != ErrLockLost {
			t.Fatal("重试失锁", err)
		}
		if _, err := s.TerminalReceipt(testContext, a, protocol.TerminalReceiptRequest{Ref: e.Ref, Seq: e.Seq, Digest: e.Digest}); err != ErrLockLost {
			t.Fatal("回执失锁", err)
		}
		if !reflect.DeepEqual(before, recoveryRows(t, s)) {
			t.Fatal("失锁改变持久状态")
		}
	})
}
