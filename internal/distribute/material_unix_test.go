//go:build darwin || linux

package distribute

import (
	"context"
	"golang.org/x/sys/unix"
	"path/filepath"
	"testing"
	"time"
)

func TestFIFORejectedWithoutBlocking(t *testing.T) {
	p := filepath.Join(t.TempDir(), "fifo")
	if unix.Mkfifo(p, 0600) != nil {
		t.Fatal("夹具")
	}
	start := time.Now()
	if _, e := readMaterial(context.Background(), p, 4096, true); e == nil {
		t.Fatal("FIFO被接受")
	}
	if time.Since(start) > time.Second {
		t.Fatal("FIFO阻塞")
	}
}
