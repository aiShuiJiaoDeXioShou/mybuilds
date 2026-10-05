//go:build darwin || linux

package config

import (
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWebhookSecretsRejectFIFOAndLeafLinkWithoutBlocking(t *testing.T) {
	dir := t.TempDir()
	if e := os.Chmod(dir, 0700); e != nil {
		t.Fatal(e)
	}
	fifo := filepath.Join(dir, "fifo")
	if e := unix.Mkfifo(fifo, 0600); e != nil {
		t.Fatal(e)
	}
	begin := time.Now()
	if _, e := LoadWebhookSecrets(fifo, []string{"HOOK"}); e == nil {
		t.Fatal("FIFO被接受")
	}
	if time.Since(begin) > time.Second {
		t.Fatal("FIFO读取阻塞")
	}
	regular := filepath.Join(dir, "actual")
	if e := os.WriteFile(regular, []byte("HOOK="+strings.Repeat("a", 40)), 0600); e != nil {
		t.Fatal(e)
	}
	link := filepath.Join(dir, "link")
	if e := os.Symlink(regular, link); e != nil {
		t.Fatal(e)
	}
	if _, e := LoadWebhookSecrets(link, []string{"HOOK"}); e == nil {
		t.Fatal("叶子链接被接受")
	}
	hard := filepath.Join(dir, "hard")
	if e := os.Link(regular, hard); e != nil {
		t.Fatal(e)
	}
	if _, e := LoadWebhookSecrets(hard, []string{"HOOK"}); e == nil {
		t.Fatal("多链接秘密被接受")
	}
}
