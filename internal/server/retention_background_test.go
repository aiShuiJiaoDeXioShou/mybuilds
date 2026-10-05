//go:build darwin || linux

package server

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mybuilds/internal/config"
	"mybuilds/internal/store"
)

func TestRetentionActualBackgroundMinuteAndStop(t *testing.T) {
	s, actor, project, object := retentionCleanupFixture(t, "artifact")
	s.config.Retention = config.Retention{Builds: 1, Days: 30}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s.config.Listen = listener.Addr().String()
	if err = listener.Close(); err != nil {
		t.Fatal(err)
	}
	path := retentionCleanupPath(s, object)
	control := filepath.Join(s.config.DataDir, "background-control")
	if err = os.WriteFile(control, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	done := make(chan error, 1)
	go func() { done <- s.ListenAndServe(ctx); close(done) }()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error("原Listen实际退出", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("原Listen未停止")
		}
	}()
	started := time.Now()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		if _, err = os.Lstat(path); os.IsNotExist(err) {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-done:
			t.Fatal("后台删除前服务已退出", err)
		case <-ctx.Done():
			t.Fatal("真实60s一轮没有推进", ctx.Err())
		case <-tick.C:
		}
	}
	if time.Since(started) < 59*time.Second {
		t.Fatal("并非原60s后台消费")
	}
	// unlink真实发生与随后事务确认之间允许短暂pending，不能提前取消本轮。
	for {
		page, err := s.store.ListRetention(ctx, actor, project, store.Page{})
		if err != nil || len(page.Items) != 1 {
			t.Fatal("原事项查询", err, page)
		}
		entry := page.Items[0]
		if entry.NodeState != "pending" || entry.NodeCompletedAt != nil || entry.HistoryState == "cleaned" {
			t.Fatal("中央完成不能假报离线Node", entry)
		}
		if entry.CentralState == "completed" {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("物理完成未持久确认", ctx.Err())
		case err := <-done:
			t.Fatal("确认前服务退出", err)
		case <-tick.C:
		}
	}
	data, err := os.ReadFile(control)
	if err != nil || string(data) != "keep" {
		t.Fatal("邻近文件未保持", err)
	}
}
