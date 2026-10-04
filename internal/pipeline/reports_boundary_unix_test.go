//go:build darwin || linux

package pipeline

import (
	"context"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestReportCollectionRejectsFIFOAndFIFOPrefix(t *testing.T) {
	for _, pattern := range []string{"pipe", "pipe/*.xml"} {
		t.Run(pattern, func(t *testing.T) {
			work, wr, dr := reportRoots(t)
			if err := syscall.Mkfifo(filepath.Join(work, "pipe"), 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			before := time.Now()
			_, err := newReportCollection(ctx, wr, dr, []string{pattern}, false, nil)
			if err == nil || time.Since(before) > time.Second {
				t.Fatalf("FIFO不应被读取或阻塞: %v", err)
			}
		})
	}
}
