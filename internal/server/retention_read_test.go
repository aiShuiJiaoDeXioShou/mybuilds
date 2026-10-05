//go:build darwin || linux

package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"golang.org/x/sys/unix"
	"mybuilds/internal/store"
)

func retentionTestFile(t *testing.T) (string, *os.File) {
	t.Helper()
	name := filepath.Join(t.TempDir(), "owned")
	if err := os.WriteFile(name, []byte("evidence"), 0600); err != nil {
		t.Fatal(err)
	}
	return name, retentionTestOpen(t, name)
}

func retentionTestOpen(t *testing.T, name string) *os.File {
	t.Helper()
	f, err := os.OpenFile(name, os.O_RDONLY|evidenceOpenFlags(), 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func TestRetentionReadLocksActualFD(t *testing.T) {
	ctx := context.Background()
	name, first := retentionTestFile(t)
	second := retentionTestOpen(t, name)
	third := retentionTestOpen(t, name)
	id, err := retentionSharedLock(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^v1:(darwin|linux):[0-9a-f]{16}:[0-9a-f]{16}:-?[0-9]+:[0-9]{9}$`).MatchString(id) {
		t.Fatal("身份编码无效", id)
	}
	if next, err := retentionSharedLock(ctx, second); err != nil || next != id {
		t.Fatal("共享锁不能共存", next, err)
	}
	started := time.Now()
	if _, err := retentionExclusiveLock(ctx, third); !errors.Is(err, store.ErrRetentionReadersActive) {
		t.Fatal("排他锁未拒绝活读者", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("非阻塞锁发生等待")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := retentionExclusiveLock(ctx, third); !errors.Is(err, store.ErrRetentionReadersActive) {
		t.Fatal("第二读者仍持锁", err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	if next, err := retentionExclusiveLock(ctx, third); err != nil || next != id {
		t.Fatal("真实关闭后锁未释放", next, err)
	}
	fourth := retentionTestOpen(t, name)
	if _, err := retentionSharedLock(ctx, fourth); !errors.Is(err, store.ErrRetentionReadersActive) {
		t.Fatal("排他锁未拒绝新读者", err)
	}
	if err := third.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := retentionSharedLock(ctx, fourth); err != nil {
		t.Fatal("排他FD关闭后未释放", err)
	}
}

func TestRetentionReadLockDuplicatedFD(t *testing.T) {
	name, first := retentionTestFile(t)
	ctx := context.Background()
	if _, err := retentionSharedLock(ctx, first); err != nil {
		t.Fatal(err)
	}
	fd, err := unix.Dup(int(first.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	duplicate := os.NewFile(uintptr(fd), "owned-duplicate")
	defer duplicate.Close()
	other := retentionTestOpen(t, name)
	if err = first.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = retentionExclusiveLock(ctx, other); !errors.Is(err, store.ErrRetentionReadersActive) {
		t.Fatal("dup仍打开却失去锁", err)
	}
	if err = duplicate.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = retentionExclusiveLock(ctx, other); err != nil {
		t.Fatal("全部dup关闭未释放", err)
	}
}

func TestRetentionReadIdentityRenameAndReplacement(t *testing.T) {
	name, original := retentionTestFile(t)
	id, err := retentionSharedLock(context.Background(), original)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(name, name+".quarantine"); err != nil {
		t.Fatal(err)
	}
	if next, err := retentionFileIdentity(original); err != nil || next != id {
		t.Fatal("rename改变出生身份", next, err)
	}
	if err = os.WriteFile(name, []byte("evidence"), 0600); err != nil {
		t.Fatal(err)
	}
	replacement := retentionTestOpen(t, name)
	if next, err := retentionExclusiveLock(context.Background(), replacement); err != nil || next == id {
		t.Fatal("替换叶子被当作原inode", next, err)
	}
	old := retentionTestOpen(t, name+".quarantine")
	if _, err = retentionExclusiveLock(context.Background(), old); !errors.Is(err, store.ErrRetentionReadersActive) {
		t.Fatal("rename将原读锁转到替换叶子", err)
	}
}

func TestRetentionReadRejectsUnsafeFD(t *testing.T) {
	for _, kind := range []string{"public", "hardlink", "directory", "fifo", "symlink", "closed", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			name, f := retentionTestFile(t)
			ctx := context.Background()
			switch kind {
			case "public":
				if err := os.Chmod(name, 0644); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(name, name+".link"); err != nil {
					t.Fatal(err)
				}
			case "directory":
				f = retentionTestOpen(t, filepath.Dir(name))
			case "fifo":
				if err := unix.Mkfifo(name+".fifo", 0600); err != nil {
					t.Fatal(err)
				}
				f = retentionTestOpen(t, name+".fifo")
			case "symlink":
				if err := os.Symlink(name, name+".link"); err != nil {
					t.Fatal(err)
				}
				opened, err := os.OpenFile(name+".link", os.O_RDONLY|evidenceOpenFlags(), 0)
				if err == nil {
					opened.Close()
					t.Fatal("原实际open接受叶symlink")
				}
				return
			case "closed":
				if err := f.Close(); err != nil {
					t.Fatal(err)
				}
			case "cancelled":
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			}
			started := time.Now()
			if _, err := retentionSharedLock(ctx, f); err == nil {
				t.Fatal("不安全FD接受")
			}
			if time.Since(started) > time.Second {
				t.Fatal("特殊文件阻塞")
			}
			if kind == "cancelled" {
				other := retentionTestOpen(t, name)
				if _, err := retentionExclusiveLock(context.Background(), other); err != nil {
					t.Fatal("取消留下锁", err)
				}
			}
		})
	}
}
