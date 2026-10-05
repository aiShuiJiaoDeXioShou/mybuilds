//go:build darwin || linux

package agent

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFlutterNodeDoctorActualOptionalCapabilities(t *testing.T) {
	sdk := os.Getenv("MYBUILDS_FLUTTER_REAL_SDK")
	if sdk == "" {
		t.Skip("未声明已初始化真实Flutter SDK")
	}
	canonical, err := filepath.EvalSymlinks(sdk)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Join(canonical, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MYBUILDS_AGENT_TOKEN", "NODE_SECRET_MUST_NOT_APPEAR")
	report, err := Doctor(context.Background(), filepath.Join(t.TempDir(), "uninitialized"))
	if err != nil {
		t.Fatal(err)
	}
	tools := map[string]string{}
	for _, tool := range report.Tools {
		if _, ok := tools[tool.Name]; ok {
			t.Fatal("重复工具名称")
		}
		tools[tool.Name] = tool.Status
	}
	if tools["flutter"] != "passed" || tools["dart"] != "passed" || tools["shell"] != "passed" || tools["git"] != "passed" {
		t.Fatalf("真实能力: %+v", report)
	}
	encoded, _ := json.Marshal(report)
	if strings.Contains(string(encoded), "NODE_SECRET_MUST_NOT_APPEAR") || strings.Contains(string(encoded), canonical) {
		t.Fatal("节点公开能力泄露私有输入")
	}
}

func TestFlutterPrecheckUnknownStopKeepsDurableJournal(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "data")
	lock, err := lockDataDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := lock.prepareJournal(); err != nil {
		t.Fatal(err)
	}
	journal, err := newJournal(lock, uuid.NewString(), uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	journal.state.StopConfirmed = true
	if err := journal.save(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	execution := taskExecution{lease: &executionLease{ctx: ctx, cancel: cancel}, journal: journal}
	if err := execution.unconfirmedFlutterPrecheck(); err == nil {
		t.Fatal("未知预检被误报成功")
	}
	if ctx.Err() == nil {
		t.Fatal("未闭锁执行权")
	}
	encoded, err := os.ReadFile(filepath.Join(directory, "journal", journal.name))
	if err != nil {
		t.Fatal(err)
	}
	var saved journalState
	if json.Unmarshal(encoded, &saved) != nil || saved.StopConfirmed || !saved.CleanupFailed || saved.PendingEvent != nil || saved.Started {
		t.Fatal("伪造用户动作或停止回执")
	}
	if inspectData(directory) != "journal_unconfirmed" {
		t.Fatal("未知预检journal被清除")
	}
}
