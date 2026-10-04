//go:build darwin || linux

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
)

// 真实旧终态已提交且丢失ACK；额外未封存检查点不能借旧receipt删除私有证据。
func TestReportsRecoveryRejectsCheckpointWithoutTerminalManifest(t *testing.T) {
	for _, mode := range []string{"checked", "final_unsealed", "sealed_without_manifest"} {
		t.Run(mode, func(t *testing.T) {
			st, _, cfg, _, id, file := actualTerminalPending(t)
			before, err := st.GetBuild(context.Background(), id)
			if err != nil {
				t.Fatal(err)
			}
			original, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var state journalState
			if json.Unmarshal(original, &state) != nil || state.Reports != nil || state.PendingEvent.Progress.ReportManifest != nil {
				t.Fatal("真实旧终态夹具不是无报告终态")
			}
			state.Reports = &reportCheckpoint{Evidence: protocol.ReportEvidence{Revision: 1, Outcome: "pending", Diagnostics: []protocol.JUnitDiagnostic{}, Files: []protocol.ReportFile{}}}
			if mode != "checked" {
				state.Reports.Final = true
				state.Reports.Evidence.Outcome = "missing"
			}
			if mode == "sealed_without_manifest" {
				state.Reports.Evidence.Sealed = true
				state.Reports.SealDigest = strings.Repeat("a", 64)
			}
			// 仅注入明确不一致的新增私有字段；保留真实中央确认的事件、完整Ref和摘要。
			data, err := json.Marshal(state)
			if err != nil || os.WriteFile(file, data, 0600) != nil {
				t.Fatal("保存自有故障证据", err)
			}
			client, lock := openRecoveryClient(t, cfg)
			if _, err = readTerminalJournal(lock, filepath.Base(file)); err == nil {
				t.Error("恢复校验接受无对应manifest的报告检查点")
			}
			if err = recoverTerminalJournals(context.Background(), client, lock, cfg.Node); err == nil {
				t.Error("真实旧receipt误清未完整核对的报告证据")
			}
			retained, err := os.ReadFile(file)
			if err != nil || !bytes.Equal(retained, data) {
				t.Error("不一致检查点未原样保留", err)
			}
			after, err := st.GetBuild(context.Background(), id)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("只读恢复改写中央既有终态", err)
			}
		})
	}
}

// 真XML经唯一Run采集、实际HTTP上传与中央seal；只丢已提交终态的响应。
func actualReportTerminalPending(t *testing.T) (*store.Store, config.AgentConfig, string, string) {
	t.Helper()
	st, _, cfg, control, id := actualFaultControl(t, `version: 1
reports:
 junit:
  paths: ['result.xml']
  required: true
steps:
 - kind: run
   name: report
   run: |
    printf once > once
    printf '<testsuite tests="1"><testcase name="real" time="0.125"/></testsuite>' > result.xml
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
			terminal = json.Unmarshal(data, &event) == nil && event.Progress.Kind == "build_finished"
		}
		original.ServeHTTP(w, r)
		if terminal {
			terminals.Add(1)
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
			t.Fatal("丢失终态ACK却返回本地成功")
		}
	case <-time.After(12 * time.Second):
		t.Fatal("真实报告终态丢ACK夹具未有界退出")
	}
	view, err := st.GetBuild(context.Background(), id)
	if err != nil || view.Status != "succeeded" || view.StopUnconfirmed || view.Reports == nil || !view.Reports.Sealed || view.Reports.Counts.Tests != 1 || len(view.Reports.Files) != 1 || terminals.Load() != 1 {
		t.Fatal("未形成真实完整中央报告终态", err)
	}
	files, _ := filepath.Glob(filepath.Join(cfg.DataDir, "journal", "*.json"))
	if len(files) != 1 {
		t.Fatal("真实报告丢ACK未保存唯一journal", len(files))
	}
	return st, cfg, id, files[0]
}

func TestReportsRecoveryActualSealRejectsCorruptCheckpoint(t *testing.T) {
	for _, mode := range []string{"counts", "file_metadata", "confirmed_metadata", "manifest_missing", "checkpoint_missing"} {
		t.Run(mode, func(t *testing.T) {
			st, cfg, id, file := actualReportTerminalPending(t)
			before, err := st.GetBuild(context.Background(), id)
			if err != nil {
				t.Fatal(err)
			}
			original, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var state journalState
			if json.Unmarshal(original, &state) != nil || state.Reports == nil || !state.Reports.Final || !state.Reports.Evidence.Sealed || len(state.Reports.Evidence.Files) != 1 || state.PendingEvent == nil || state.PendingEvent.Progress.ReportManifest == nil || len(state.Artifacts) != 1 || !state.Artifacts[0].Confirmed {
				t.Fatal("真实封存私有证据不完整")
			}
			switch mode {
			case "counts":
				state.Reports.Evidence.Counts.Tests++
			case "file_metadata":
				state.Reports.Evidence.Files[0].Size++
			case "confirmed_metadata":
				state.Artifacts[0].Declaration.ReportKey = strings.Repeat("b", 64)
			case "manifest_missing":
				state.PendingEvent.Progress.ReportManifest = nil
			case "checkpoint_missing":
				state.Reports = nil
			}
			data, err := json.Marshal(state)
			if err != nil || os.WriteFile(file, data, 0600) != nil {
				t.Fatal("保存自有报告故障证据", err)
			}
			client, lock := openRecoveryClient(t, cfg)
			if _, err = readTerminalJournal(lock, filepath.Base(file)); err == nil {
				t.Error("恢复校验接受篡改的完整报告证据")
			}
			if err = recoverTerminalJournals(context.Background(), client, lock, cfg.Node); err == nil {
				t.Error("真实receipt误清不一致的报告证据")
			}
			retained, err := os.ReadFile(file)
			if err != nil || !bytes.Equal(retained, data) {
				t.Error("报告故障证据未原样保留", err)
			}
			after, err := st.GetBuild(context.Background(), id)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("报告恢复改写既有中央终态", err)
			}
		})
	}
}

func TestReportsRecoveryActualSealClearsExactConfirmedTerminal(t *testing.T) {
	st, cfg, id, file := actualReportTerminalPending(t)
	before, err := st.GetBuild(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	client, lock := openRecoveryClient(t, cfg)
	if err = recoverTerminalJournals(context.Background(), client, lock, cfg.Node); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Lstat(file); !os.IsNotExist(err) {
		t.Fatal("真实完整报告终态未清本条journal", err)
	}
	after, err := st.GetBuild(context.Background(), id)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("成功只读恢复改写中央报告", err)
	}
	paths, _ := filepath.Glob(filepath.Join(cfg.DataDir, "scm", "checkout-*", "workspace", "once"))
	if len(paths) != 1 {
		t.Fatal("只读恢复重复执行仓库动作", len(paths))
	}
}
