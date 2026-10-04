//go:build darwin || linux

package scm

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestSSHCredentialsFIFORejectedWithoutBlocking(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "private-fifo")
	if err := syscall.Mkfifo(filename, 0600); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	_, err := readPrivate(filename, 1<<20)
	requireCode(t, err, "scm_credentials_invalid")
	if time.Since(started) > time.Second {
		t.Fatal("读取FIFO阻塞")
	}
}
