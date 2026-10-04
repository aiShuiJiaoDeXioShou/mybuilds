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
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
)

// 中央终态来自实际Claim→Checkout→Run→事件/日志，仅真正提交后的响应被丢弃。
func actualTerminalPending(t *testing.T) (*store.Store, store.Actor, config.AgentConfig, *httptest.Server, string, string) {
	t.Helper()
	st, admin, cfg, control, id := actualFaultControl(t, `version: 1
steps:
 - kind: run
   name: actual
   run: printf once > once; printf real
`, "")
	original := control.Config.Handler
	var terminals atomic.Int64
	control.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		terminal := false
		if r.URL.Path == "/api/agent/events" {
			data, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
			if err != nil {
				t.Error(err)
			}
			r.Body = io.NopCloser(bytes.NewReader(data))
			var event protocol.ExecutionEvent
			if json.Unmarshal(data, &event) == nil && event.Progress.Kind == "build_finished" {
				terminal = true
				terminals.Add(1)
			}
		}
		original.ServeHTTP(w, r)
		if terminal {
			if conn, _, err := w.(http.Hijacker).Hijack(); err == nil {
				conn.Close()
			}
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, cfg) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("lost terminal reported local success")
		}
	case <-time.After(8 * time.Second):
		t.Fatal("terminal response failure did not finish")
	}
	before, err := st.GetBuild(context.Background(), id)
	if err != nil || before.Status != "succeeded" || before.StopUnconfirmed || terminals.Load() != 1 {
		t.Fatal("not exact actual central terminal", before.Status, terminals.Load(), err)
	}
	files, _ := filepath.Glob(filepath.Join(cfg.DataDir, "journal", "*.json"))
	if len(files) != 1 {
		t.Fatal("lost ACK did not preserve exact pending", len(files))
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var state journalState
	if json.Unmarshal(data, &state) != nil || state.PendingEvent == nil || state.PendingEvent.Progress.Kind != "build_finished" || !state.StopConfirmed || state.CleanupFailed {
		t.Fatal("no real local stop/pending proof")
	}
	return st, admin, cfg, control, id, files[0]
}
func openRecoveryClient(t *testing.T, cfg config.AgentConfig) (*agentHTTP, *dataLock) {
	t.Helper()
	client, err := newAgentHTTP(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.close)
	lock, err := lockDataDir(cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { lock.Close() })
	return client, lock
}
func TestRecoveryActualCurrentRotatedIdentityClearsOnlyConfirmedJournal(t *testing.T) {
	st, admin, cfg, _, id, file := actualTerminalPending(t)
	before, _ := st.GetBuild(context.Background(), id)
	rotated, err := st.RotateNodeToken(context.Background(), admin, cfg.Node)
	if err != nil {
		t.Fatal(err)
	}
	cfg.RuntimeToken = rotated.Token
	client, lock := openRecoveryClient(t, cfg)
	if err = recoverTerminalJournals(context.Background(), client, lock, cfg.Node); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Lstat(file); !os.IsNotExist(err) {
		t.Fatal("exact journal not removed", err)
	}
	after, err := st.GetBuild(context.Background(), id)
	a, _ := json.Marshal(before)
	b, _ := json.Marshal(after)
	if err != nil || !bytes.Equal(a, b) {
		t.Fatal("readonly changed central evidence", err)
	}
	counts, _ := filepath.Glob(filepath.Join(cfg.DataDir, "scm", "checkout-*", "workspace", "once"))
	if len(counts) != 1 {
		t.Fatal("readonly reran action", len(counts))
	}
}
func TestRecoveryConfirmedJournalAndOtherUnknownStillRejectServe(t *testing.T) {
	_, _, cfg, _, _, file := actualTerminalPending(t)
	unknown := filepath.Join(cfg.DataDir, "journal", uuid.NewString()+".json")
	if err := os.WriteFile(unknown, []byte(`{"claim_key":"unknown"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Serve(context.Background(), cfg); err == nil || err.Error() != "agent_journal_unconfirmed" {
		t.Fatal("unknown permitted new session", err)
	}
	if _, err := os.Lstat(file); !os.IsNotExist(err) {
		t.Fatal("confirmed journal not cleared", err)
	}
	if data, err := os.ReadFile(unknown); err != nil || string(data) != `{"claim_key":"unknown"}` {
		t.Fatal("unknown evidence altered", err)
	}
}
func TestRecoveryActualReadonlyNegativeResponsesPreserveLocalEvidence(t *testing.T) {
	for _, mode := range []string{"name", "ref", "seq", "digest", "status", "stop", "unknownfield", "duplicate", "oversized", "crossnode", "revoked", "disabled", "notfound"} {
		t.Run(mode, func(t *testing.T) {
			st, admin, cfg, control, _, file := actualTerminalPending(t)
			before, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			original := control.Config.Handler
			if mode == "crossnode" {
				node, err := st.CreateNode(context.Background(), admin, store.NodeInput{Name: "other", Capacity: 1})
				if err != nil {
					t.Fatal(err)
				}
				cfg.RuntimeToken = node.Token
			}
			if mode == "revoked" {
				if err := st.RevokeNodeToken(context.Background(), admin, cfg.Node); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "disabled" {
				if err := st.SetNodeState(context.Background(), admin, cfg.Node, "disabled"); err != nil {
					t.Fatal(err)
				}
			}
			control.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/agent/terminal-receipt" || mode == "crossnode" || mode == "revoked" || mode == "disabled" {
					original.ServeHTTP(w, r)
					return
				}
				// 负例仅损坏真正Store生成的HTTP回执，绝不模拟中央确认成功。
				recorder := httptest.NewRecorder()
				original.ServeHTTP(recorder, r)
				if recorder.Code != 200 {
					t.Error("real readonly unavailable", recorder.Code)
					w.WriteHeader(recorder.Code)
					w.Write(recorder.Body.Bytes())
					return
				}
				var receipt protocol.TerminalReceipt
				if json.Unmarshal(recorder.Body.Bytes(), &receipt) != nil {
					t.Error("bad actual receipt")
				}
				switch mode {
				case "name":
					receipt.NodeName = "different"
				case "ref":
					receipt.Ref.AttemptID = uuid.NewString()
				case "seq":
					receipt.Seq++
				case "digest":
					receipt.Digest = strings.Repeat("a", 64)
				case "status":
					receipt.Status = "failed"
				case "stop":
					receipt.StopKnown = false
				case "notfound":
					w.WriteHeader(404)
					io.WriteString(w, `{"error":{"code":"not_found","message":"missing"}}`)
					return
				case "oversized":
					w.WriteHeader(200)
					io.WriteString(w, strings.Repeat("x", (1<<20)+1))
					return
				}
				data, _ := json.Marshal(receipt)
				if mode == "duplicate" {
					data = append(data[:len(data)-1], []byte(`,"stop_known":true}`)...)
				}
				if mode == "unknownfield" {
					data = append(data[:len(data)-1], []byte(`,"private_field":"marker"}`)...)
				}
				w.WriteHeader(200)
				w.Write(data)
			})
			client, lock := openRecoveryClient(t, cfg)
			if err = recoverTerminalJournals(context.Background(), client, lock, cfg.Node); err == nil {
				t.Fatal("unsafe readonly result accepted")
			}
			after, err := os.ReadFile(file)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("failed readonly modified local proof", err)
			}
		})
	}
}
func TestRecoveryActualReadonlyWaitDetectsSameInodeRewrite(t *testing.T) {
	_, _, cfg, control, _, file := actualTerminalPending(t)
	original := control.Config.Handler
	changed := []byte("unknown-local-proof")
	control.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/agent/terminal-receipt" {
			original.ServeHTTP(w, r)
			return
		}
		recorder := httptest.NewRecorder()
		original.ServeHTTP(recorder, r)
		if recorder.Code != 200 {
			t.Error("actual readonly rejected", recorder.Code)
		}
		if err := os.WriteFile(file, changed, 0600); err != nil {
			t.Error(err)
		}
		w.WriteHeader(recorder.Code)
		w.Write(recorder.Body.Bytes())
	})
	client, lock := openRecoveryClient(t, cfg)
	if err := recoverTerminalJournals(context.Background(), client, lock, cfg.Node); err == nil {
		t.Fatal("changed same inode deleted")
	}
	if data, err := os.ReadFile(file); err != nil || !bytes.Equal(data, changed) {
		t.Fatal("changed proof lost", err)
	}
}

func TestRecoveryActualReadonlyWaitPreservesReplacement(t *testing.T) {
	for _, mode := range []string{"file", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			_, _, cfg, control, _, file := actualTerminalPending(t)
			original := control.Config.Handler
			before, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			saved := file + ".saved"
			target := filepath.Join(t.TempDir(), "own-outside")
			replacement := []byte("own-unknown-replacement")
			if err = os.WriteFile(target, replacement, 0600); err != nil {
				t.Fatal(err)
			}
			control.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/agent/terminal-receipt" {
					original.ServeHTTP(w, r)
					return
				}
				recorder := httptest.NewRecorder()
				original.ServeHTTP(recorder, r)
				if recorder.Code != 200 {
					t.Error("actual readonly rejected", recorder.Code)
				}
				if err := os.Rename(file, saved); err != nil {
					t.Error(err)
				}
				if mode == "symlink" {
					err = os.Symlink(target, file)
				} else {
					err = os.WriteFile(file, replacement, 0600)
				}
				if err != nil {
					t.Error(err)
				}
				w.WriteHeader(recorder.Code)
				w.Write(recorder.Body.Bytes())
			})
			client, lock := openRecoveryClient(t, cfg)
			if err = recoverTerminalJournals(context.Background(), client, lock, cfg.Node); err == nil {
				t.Fatal("replacement deleted")
			}
			for path, want := range map[string][]byte{saved: before, file: replacement, target: replacement} {
				got, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(got, want) {
					t.Fatal("replacement or saved proof altered", err)
				}
			}
			if mode == "symlink" {
				info, err := os.Lstat(file)
				if err != nil || info.Mode()&os.ModeSymlink == 0 {
					t.Fatal("replacement symlink removed", err)
				}
			}
		})
	}
}
func TestRecoveryUnknownJournalNeverSignalsOldPID(t *testing.T) {
	_, _, cfg, _, _, _ := actualTerminalPending(t)
	unrelated := exec.Command("sleep", "30")
	if err := unrelated.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { unrelated.Process.Kill(); unrelated.Wait() })
	unknown := filepath.Join(cfg.DataDir, "journal", uuid.NewString()+".json")
	data, _ := json.Marshal(journalState{ClaimKey: uuid.NewString(), SessionID: uuid.NewString(), PID: unrelated.Process.Pid, PGID: unrelated.Process.Pid})
	if err := os.WriteFile(unknown, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := Serve(context.Background(), cfg); err == nil || err.Error() != "agent_journal_unconfirmed" {
		t.Fatal("unknown journal accepted", err)
	}
	if err := unix.Kill(unrelated.Process.Pid, 0); err != nil {
		t.Fatal("old PID signalled", err)
	}
	got, err := os.ReadFile(unknown)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("unknown journal changed", err)
	}
}
