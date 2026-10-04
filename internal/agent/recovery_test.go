//go:build darwin || linux

package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/unix"
	"mybuilds/internal/protocol"
)

// 每轮HTTP期限不是实际Authority取消，不能把临时慢响应判成永久取消。
func TestRecoveryHTTPRoundTimeoutIsNetworkError(t *testing.T) {
	_, _, cfg, _ := agentControl(t)
	release := make(chan struct{})
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer control.Close()
	defer close(release)
	cfg.Server = control.URL
	client, err := newAgentHTTP(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer client.close()
	client.timeout = 30 * time.Millisecond
	err = client.retryPost(context.Background(), "/api/agent/heartbeat", protocol.HeartbeatRequest{}, nil)
	if err == nil || err.Error() != "agent_network_error" {
		t.Fatalf("temporary HTTP deadline: %v", err)
	}
	cancelled, stop := context.WithCancel(context.Background())
	stop()
	if err = client.retryPost(cancelled, "/api/agent/heartbeat", protocol.HeartbeatRequest{}, nil); err == nil || err.Error() != "agent_cancelled" {
		t.Fatalf("real cancellation: %v", err)
	}
}

func TestRecoveryPermanentTLSIsNotNetworkRetry(t *testing.T) {
	_, _, cfg, handler := agentControl(t)
	control := httptest.NewTLSServer(handler)
	defer control.Close()
	cfg.Server = control.URL
	client, err := newAgentHTTP(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer client.close()
	err = client.retryPost(context.Background(), "/api/agent/heartbeat", protocol.HeartbeatRequest{}, nil)
	if err == nil || err.Error() != "agent_tls_configuration_error" {
		t.Fatalf("unknown CA must remain permanent: %v", err)
	}
}

// 断开真实连接而非替换执行器；Store、Git、Run和普通/always进程都实际运行。
func TestRecoveryShortDisconnectKeepsActualRunAndPendingClaim(t *testing.T) {
	source := `version: 1
steps:
 - kind: run
   name: actual
   run: printf once > once; sleep 5; printf done
post:
 always:
  - kind: run
    name: cleanup
    run: printf cleanup > cleanup
`
	st, _, cfg, control, id := actualFaultControl(t, source, "")
	var outage atomic.Bool
	original := control.Config.Handler
	control.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if outage.Load() && len(r.URL.Path) >= 11 && r.URL.Path[:11] == "/api/agent/" {
			if conn, _, err := w.(http.Hijacker).Hijack(); err == nil {
				conn.Close()
			}
			return
		}
		original.ServeHTTP(w, r)
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, cfg) }()
	deadline := time.Now().Add(15 * time.Second)
	started := false
	outageEnd := time.Time{}
	for time.Now().Before(deadline) {
		view, err := st.GetBuild(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if !started && view.Steps[0].Started {
			started = true
			outage.Store(true)
			outageEnd = time.Now().Add(2200 * time.Millisecond)
		}
		if started && !time.Now().Before(outageEnd) {
			outage.Store(false)
		}
		if view.Status == "succeeded" {
			if !started || view.StopUnconfirmed || !view.Steps[0].StopConfirmed || view.Steps[0].CleanupFailed || !view.Post[0].Started {
				t.Fatal("lost actual execution", view)
			}
			files, _ := filepath.Glob(filepath.Join(cfg.DataDir, "scm", "checkout-*", "workspace", "once"))
			if len(files) != 1 {
				t.Fatal("duplicate physical execution", files)
			}
			data, err := os.ReadFile(files[0])
			if err != nil || string(data) != "once" {
				t.Fatal("wrong original action", err)
			}
			waitAttemptJournalRemoved(t, cfg.DataDir, id)
			cancel()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			return
		}
		select {
		case err := <-done:
			t.Fatalf("short disconnect stopped valid execution: %v", err)
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Fatal("no actual resumed terminal")
}

func TestRecoveryGrantCannotReviveExpiredAuthority(t *testing.T) {
	_, _, cfg, _ := agentControl(t)
	lock, err := lockDataDir(cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	ref := protocol.LeaseRef{NodeID: "2807a8de-8f31-4e6b-a00a-a8f66a2d133b", SessionID: "cab3b08f-6712-48cb-9b20-cf43c63e0ac6", BuildID: "4229f3db-ce44-4812-9ac3-228280e05a04", AttemptID: "f9556b76-bc11-4f7a-8c30-f5e7b5454724", LeaseID: "b1c0ed41-a079-4c05-b2c9-8e85a8e64ba8", Epoch: 1}
	grant := protocol.LeaseGrant{Ref: ref, TTLNS: int64(2100 * time.Millisecond), Task: &protocol.TaskSnapshot{}, RemainingPostBudgetNS: 1}
	lease, err := newExecutionLease(context.Background(), lock, cfg, grant, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer lease.close()
	select {
	case <-lease.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("authority deadline did not expire")
	}
	grant.TTLNS = int64(cfg.LeaseDuration)
	if err = lease.accept(grant, time.Now()); err == nil {
		t.Fatal("late renewal revived old authority")
	}
	if unix.Kill(os.Getpid(), 0) != nil {
		t.Fatal("unrelated current process was signalled")
	}
}

// 未确认claim窗口耗尽后，恢复心跳也不得换key；另一实际执行仍可正常续租完成。
func TestRecoveryUnknownClaimExpiryKeepsOtherValidExecution(t *testing.T) {
	source := `version: 1
steps:
 - kind: run
   name: actual
   run: printf once > once; sleep 10; printf done
`
	st, _, cfg, control, id := actualFaultControl(t, source, "")
	original := control.Config.Handler
	var blockClaim atomic.Bool
	var attempts atomic.Int64
	var claimKeys sync.Map
	control.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if blockClaim.Load() && r.URL.Path == "/api/agent/claim" {
			var actual protocol.ClaimRequest
			if err := json.NewDecoder(io.LimitReader(r.Body, (1<<20)+1)).Decode(&actual); err != nil {
				t.Error("invalid real claim request", err)
			}
			claimKeys.Store(actual.ClaimKey, actual.SessionID)
			attempts.Add(1)
			if conn, _, err := w.(http.Hijacker).Hijack(); err == nil {
				conn.Close()
			}
			return
		}
		original.ServeHTTP(w, r)
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, cfg) }()
	deadline := time.Now().Add(16 * time.Second)
	var began time.Time
	var observed int64
	for time.Now().Before(deadline) {
		view, err := st.GetBuild(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if began.IsZero() && view.Steps[0].Started {
			began = time.Now()
			blockClaim.Store(true)
		}
		if !began.IsZero() && time.Since(began) > 9*time.Second && observed == 0 {
			observed = attempts.Load()
			if observed < 2 {
				t.Fatal("same request was not confirmed within original window")
			}
			files, _ := filepath.Glob(filepath.Join(cfg.DataDir, "journal", "*.json"))
			unknown := 0
			for _, file := range files {
				data, _ := os.ReadFile(file)
				var state journalState
				if json.Unmarshal(data, &state) == nil && state.Ref == nil {
					unknown++
				}
			}
			if unknown != 1 {
				t.Fatal("expired unknown claim lost or replaced", unknown)
			}
		}
		if view.Status == "succeeded" {
			keys := 0
			claimKeys.Range(func(_, value any) bool {
				keys++
				if !exactUUID(value.(string)) {
					t.Error("claim session changed")
				}
				return true
			})
			if keys != 1 {
				t.Fatal("unknown claim changed key", keys)
			}
			if observed == 0 || attempts.Load() != observed {
				t.Fatal("heartbeat cleared expired claim or reset claim origin", observed, attempts.Load())
			}
			waitAttemptJournalRemoved(t, cfg.DataDir, id)
			files, _ := filepath.Glob(filepath.Join(cfg.DataDir, "journal", "*.json"))
			if len(files) != 1 {
				t.Fatal("unknown claim evidence not preserved", len(files))
			}
			cancel()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			return
		}
		select {
		case err := <-done:
			t.Fatalf("unknown claim mis-stopped valid execution: %v", err)
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Fatal("no actual terminal while expired claim held")
}
