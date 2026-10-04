package server

import (
	"context"
	"database/sql"
	"net"
	"path/filepath"
	"testing"
	"time"
)

func TestActualRecoveryRejectsCorruptQueueBeforeListen(t *testing.T) {
	s, _, _, _, _ := executionHTTPFixture(t)
	db, err := sql.Open("sqlite", filepath.Join(s.config.DataDir, "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec("UPDATE builds SET snapshot_json = ? WHERE status = ?", `{"invalid":"PRIVATE_SNAPSHOT"}`, "queued"); err != nil {
		t.Fatal(err)
	}
	reserved, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s.config.Listen = reserved.Addr().String()
	reserved.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err = s.ListenAndServe(ctx); err == nil || err.Error() != "database_error" {
		t.Fatal("损坏持久队列仍接监听/丢安全错误", err)
	}
	c, err := net.DialTimeout("tcp", s.config.Listen, 100*time.Millisecond)
	if err == nil {
		c.Close()
		t.Fatal("恢复失败仍监听")
	}
}
