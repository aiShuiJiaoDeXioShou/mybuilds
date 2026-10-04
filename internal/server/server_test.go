package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mybuilds/internal/store"
)

func TestActualListenCancellationAndLockLoss(t *testing.T) {
	for _, lose := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "lost"}[lose], func(t *testing.T) {
			s, _, _ := serverFixture(t)
			reservation, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			s.config.Listen = reservation.Addr().String()
			reservation.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- s.ListenAndServe(ctx) }()
			client := &http.Client{Timeout: time.Second}
			deadline := time.Now().Add(3 * time.Second)
			for {
				req, _ := http.NewRequest("GET", "http://"+s.config.Listen+"/api/status", nil)
				req.Header.Set("Authorization", "Bearer "+adminToken)
				res, err := client.Do(req)
				if err == nil {
					res.Body.Close()
					if res.StatusCode != 200 {
						t.Fatal(res.StatusCode)
					}
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("未真实监听")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if lose {
				lock := filepath.Join(s.config.DataDir, "control.db.lock")
				if err := os.Rename(lock, lock+".owned-old"); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(lock, []byte("replacement"), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				cancel()
			}
			select {
			case err := <-done:
				if (lose && !errors.Is(err, store.ErrLockLost)) || (!lose && err != nil) {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("未停止服务")
			}
			connection, err := net.DialTimeout("tcp", s.config.Listen, 100*time.Millisecond)
			if err == nil {
				connection.Close()
				t.Fatal("停止后仍监听")
			}
		})
	}
}
