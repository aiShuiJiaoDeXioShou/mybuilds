//go:build darwin || linux

package config

import (
	"golang.org/x/sys/unix"
	"path/filepath"
	"testing"
	"time"
)

func TestManagementReadersRejectFIFO(t *testing.T) {
	clearClientEnvironment(t)
	clearServerEnvironment(t)
	p := filepath.Join(t.TempDir(), "config.fifo")
	if err := unix.Mkfifo(p, 0600); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := LoadServer(ServerLoadOptions{Filename: p, Explicit: true}); err == nil {
		t.Fatal("服务端FIFO被接受")
	}
	if _, err := LoadClient(ClientLoadOptions{Filename: p, Explicit: true}); err == nil {
		t.Fatal("客户端FIFO被接受")
	}
	if _, err := LoadProjectSettings(p); err == nil {
		t.Fatal("settings FIFO被接受")
	}
	if time.Since(start) > time.Second {
		t.Fatal("非普通文件读取阻塞")
	}
}
