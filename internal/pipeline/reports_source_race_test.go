//go:build darwin || linux

package pipeline

import (
	"bytes"
	"context"
	"errors"
	"mybuilds/internal/config"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// 只观察本进程已打开的自有inode和实际读偏移；不注入reader或生产执行钩子。
func reportReadInProgress(ctx context.Context, before os.FileInfo, size int64) error {
	identity, ok := before.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("fixture_identity")
	}
	for ctx.Err() == nil {
		// Darwin的/dev/fd不是普通可遍历目录；只有限核对本进程低256个描述符。
		for fd := 0; fd < 256; fd++ {
			var actual syscall.Stat_t
			if syscall.Fstat(fd, &actual) != nil || actual.Dev != identity.Dev || actual.Ino != identity.Ino {
				continue
			}
			// SEEK_CUR+0不改变位置，仅核实本次有限read已消费真实字节。
			offset, err := syscall.Seek(fd, 0, 1)
			if err == nil && offset >= 1<<20 && offset < size-(1<<20) {
				return nil
			}
		}
	}
	return errors.New("fixture_read_window")
}

func TestReportCollectionRejectsActualSourceReadRaces(t *testing.T) {
	for _, mode := range []string{"replacement", "preserved_mtime_bytes", "closed_workspace_root"} {
		t.Run(mode, func(t *testing.T) {
			work, wr, dr := reportRoots(t)
			c, err := newReportCollection(context.Background(), wr, dr, []string{"result.xml"}, true, nil, config.DefaultJUnitMaxFiles)
			if err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(work, "result.xml")
			data := bytes.Repeat([]byte("a"), 7<<20)
			if err = os.WriteFile(source, data, 0600); err != nil {
				t.Fatal(err)
			}
			before, err := os.Stat(source)
			if err != nil {
				t.Fatal(err)
			}
			// 替换材料在开始读取之前准备好，不让大文件写入耗时决定竞争窗口。
			replacement := filepath.Join(work, "replacement.bin")
			if err = os.WriteFile(replacement, bytes.Repeat([]byte("b"), len(data)), 0600); err != nil {
				t.Fatal(err)
			}
			if err = os.Chtimes(replacement, before.ModTime(), before.ModTime()); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			type mutation struct {
				at  time.Time
				err error
			}
			changed := make(chan mutation, 1)
			ready := make(chan struct{})
			go func() {
				close(ready)
				err := reportReadInProgress(ctx, before, before.Size())
				if err == nil {
					if mode == "closed_workspace_root" {
						err = wr.Close()
					} else if mode == "replacement" {
						err = os.Rename(replacement, source)
					} else {
						// 同inode/长度/mtime，必须依赖真实双读摘要来拒绝已读前缀变化。
						var file *os.File
						file, err = os.OpenFile(source, os.O_WRONLY, 0)
						if err == nil {
							_, err = file.WriteAt([]byte("b"), 0)
							closeErr := file.Close()
							if err == nil {
								err = closeErr
							}
						}
						if err == nil {
							err = os.Chtimes(source, before.ModTime(), before.ModTime())
						}
					}
				}
				changed <- mutation{at: time.Now(), err: err}
			}()
			<-ready
			_, _, readErr := c.read(ctx, "result.xml")
			finished := time.Now()
			result := <-changed
			if result.err != nil || !result.at.Before(finished) {
				t.Fatal("真实读窗口夹具未成立", result.err)
			}
			want := errReportInvalid
			if mode == "closed_workspace_root" {
				want = errReportSave
			}
			if !errors.Is(readErr, want) {
				t.Fatalf("已证明读中竞争仍被接受: %v", readErr)
			}
			after, err := os.Stat(source)
			if err != nil || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
				t.Fatal("竞态材料未保持长度/mtime")
			}
			if mode == "replacement" && os.SameFile(before, after) || mode == "preserved_mtime_bytes" && !os.SameFile(before, after) {
				t.Fatal("真实inode前置不匹配")
			}
			entries, err := os.ReadDir(dr.Name())
			if err != nil || len(entries) != 0 {
				t.Fatal("不稳定源创建可信快照")
			}
		})
	}
}
