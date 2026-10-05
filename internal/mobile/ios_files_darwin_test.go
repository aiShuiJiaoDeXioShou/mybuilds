//go:build darwin

package mobile

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestIOSMaterialRejectsFIFOAndSymlink(t *testing.T) {
	root := t.TempDir()
	fifo := filepath.Join(root, "fifo")
	if syscall.Mkfifo(fifo, 0600) != nil {
		t.Fatal("自有FIFO创建失败")
	}
	done := make(chan error, 1)
	go func() { _, err := readIOSMaterial(fifo, root, 1024); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("FIFO未拒绝")
		}
	case <-time.After(time.Second):
		t.Fatal("FIFO打开阻塞")
	}
	if os.WriteFile(root+"/file", []byte("owned"), 0600) != nil || os.Symlink(root+"/file", root+"/link") != nil {
		t.Fatal("自有symlink创建失败")
	}
	if _, err := readIOSMaterial(root+"/link", root, 1024); err == nil {
		t.Fatal("材料叶symlink未拒绝")
	}
}
