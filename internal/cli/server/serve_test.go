package server

import (
	"bytes"
	"context"
	"mybuilds/internal/store"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestActualServeBootstrapCancelAndRelease(t *testing.T) {
	directory := t.TempDir()
	filename := filepath.Join(directory, "server.yml")
	os.WriteFile(filename, []byte("data_dir: private-data\ndatabase: {driver: sqlite, dsn: db.sqlite}\n"), 0600)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	secret := strings.Repeat("BOOTSTRAP_SECRET", 3)
	t.Setenv("MYBUILDS_BOOTSTRAP_ADMIN_TOKEN", secret)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := NewCommand()
	var out, diagnostics bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&diagnostics)
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"--config", filename, "serve", "--listen", address, "--concurrency", "2"})
	finished := make(chan error, 1)
	go func() { finished <- cmd.Execute() }()
	ready := false
	client := &http.Client{Timeout: time.Second}
	for ctx.Err() == nil {
		req, _ := http.NewRequest("GET", "http://"+address+"/api/status", nil)
		req.Header.Set("Authorization", "Bearer "+secret)
		response, err := client.Do(req)
		if err == nil {
			response.Body.Close()
			if response.StatusCode == 200 {
				ready = true
				break
			}
		}
		select {
		case err := <-finished:
			t.Fatalf("服务提前结束: %v", err)
		case <-time.After(10 * time.Millisecond):
		}
	}
	if !ready {
		t.Fatal("真实服务未启动")
	}
	cancel()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("取消未关闭真实服务")
	}
	if strings.Contains(out.String()+diagnostics.String(), secret) {
		t.Fatal("bootstrap秘密泄露")
	}
	info, statErr := os.Stat(filepath.Join(directory, "private-data"))
	if statErr != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		t.Fatal("未创建私有DataDir", statErr)
	}
	db, err := store.Open(context.Background(), store.Options{Driver: "sqlite", DSN: filepath.Join(directory, "db.sqlite")})
	if err != nil {
		t.Fatal("服务取消未释放独占", err)
	}
	db.Close()
}
