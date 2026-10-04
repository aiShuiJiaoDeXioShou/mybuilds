//go:build darwin || linux

package pipeline

import (
	"context"
	"net"
	"path/filepath"
	"syscall"
	"testing"
)

func TestArtifactSpecialFiles(t *testing.T) {
	root, dir := artifactWorkspace(t)
	if err := syscall.Mkfifo(filepath.Join(dir, "pipe"), 0600); err != nil {
		t.Fatal(err)
	}
	socket, err := net.Listen("unix", filepath.Join(dir, "socket"))
	if err != nil {
		t.Fatal(err)
	}
	defer socket.Close()
	for _, pattern := range []string{"pipe", "socket", "*", "pipe/*.apk", "socket/*.apk"} {
		parent := t.TempDir()
		records, err := collectArtifacts(context.Background(), root, filepath.Join(parent, "snapshot"), []string{pattern})
		if err == nil || len(records) != 0 {
			t.Fatal("特殊文件未拒绝", pattern, records, err)
		}
		artifactAssertEmpty(t, parent)
	}
}
