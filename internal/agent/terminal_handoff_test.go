//go:build darwin || linux

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mybuilds/internal/protocol"
	"mybuilds/internal/server"
)

// 真正提交终态再延迟合法回执；续租不能把已完成执行的回执读取撤销。
func TestRecoveryActualTerminalACKHandoffKeepsNextQueuedExecution(t *testing.T) {
	st, _, cfg, control, first := actualFaultControl(t, `version: 1
steps:
 - kind: run
   name: actual
   run: printf once > once; sleep 0.70
`, "")
	cfg.Capacity = 1
	original := control.Config.Handler
	var committed, afterTerminalRenew atomic.Int64
	var terminalRefs sync.Map
	control.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/agent/renew" {
			data, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
			if err != nil {
				t.Error(err)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(data))
			var ref protocol.LeaseRef
			if json.Unmarshal(data, &ref) == nil {
				if _, exists := terminalRefs.Load(ref.BuildID); exists {
					afterTerminalRenew.Add(1)
				}
			}
		}
		if r.URL.Path != "/api/agent/events" {
			original.ServeHTTP(w, r)
			return
		}
		data, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
		if err != nil {
			t.Error(err)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(data))
		var event protocol.ExecutionEvent
		if json.Unmarshal(data, &event) != nil || event.Progress.Kind != "build_finished" {
			original.ServeHTTP(w, r)
			return
		}
		recorder := httptest.NewRecorder()
		original.ServeHTTP(recorder, r)
		if recorder.Code != 200 {
			t.Error("真实终态提交失败", recorder.Code)
		}
		terminalRefs.Store(event.Ref.BuildID, true)
		committed.Add(1)
		time.Sleep(800 * time.Millisecond)
		for key, values := range recorder.Header() {
			w.Header()[key] = values
		}
		w.WriteHeader(recorder.Code)
		w.Write(recorder.Body.Bytes())
	})
	request, _ := http.NewRequest(http.MethodPost, control.URL+"/api/projects/fault/builds", strings.NewReader(`{"branch":"main"}`))
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "next-queued-after-terminal")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var batch server.BatchView
	err = json.NewDecoder(response.Body).Decode(&batch)
	response.Body.Close()
	if err != nil || response.StatusCode != 201 || len(batch.Builds) != 1 {
		t.Fatal("真实queued失败", err, response.StatusCode)
	}
	second := batch.Builds[0].ID
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, cfg) }()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		a, err := st.GetBuild(context.Background(), first)
		if err != nil {
			t.Fatal(err)
		}
		b, err := st.GetBuild(context.Background(), second)
		if err != nil {
			t.Fatal(err)
		}
		if a.Status == "succeeded" && b.Status == "succeeded" {
			waitAttemptJournalRemoved(t, cfg.DataDir, second)
			counts, _ := filepath.Glob(filepath.Join(cfg.DataDir, "scm", "checkout-*", "workspace", "once"))
			if len(counts) != 2 || committed.Load() != 2 || afterTerminalRenew.Load() != 0 {
				t.Fatal("交接重复或继续原续租", len(counts), committed.Load(), afterTerminalRenew.Load())
			}
			for _, name := range counts {
				data, err := os.ReadFile(name)
				if err != nil || string(data) != "once" {
					t.Fatal("动作不是一次", err)
				}
			}
			cancel()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			return
		}
		select {
		case err := <-done:
			t.Fatal("终态ACK与续租竞态中止后续queued", err, a.Status, b.Status)
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Fatal("未完成第二个queued")
}
