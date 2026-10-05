package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"mybuilds/internal/config"
	"mybuilds/internal/mobile"
	"mybuilds/internal/protocol"
	"os"
	"path/filepath"
	"testing"
)

func TestIOSJournalRejectsUnclosedNativeTerminal(t *testing.T) {
	lock, journal := recoveryJournalFile(t)
	journal.state.IOSSigningRequired = true
	p := journal.state.PendingEvent.Progress
	p.IOSCleanupConfirmed = true
	journal.state.PendingEvent.Digest = progressHash(p)
	journal.state.PendingEvent.Progress = p
	if !mobile.IOSSigningSupported() {
		t.Skip("实际规划仅macOS")
	}
	root, _ := filepath.EvalSymlinks(t.TempDir())
	resources, err := mobile.PlanIOSResources(context.Background(), mobile.IOSSigningOptions{Workspace: root, OutputDir: "output", P12File: "absent", ProfileFile: "absent", BundleID: "com.example.app", ExportMethod: "debugging"})
	if err != nil {
		t.Fatal(err)
	}
	defer resources.Close(context.Background())
	own := resources.Ownership()
	journal.state.IOSResources = &own
	if err = journal.save(); err != nil {
		t.Fatal(err)
	}
	if _, err = readTerminalJournal(lock, journal.name); err == nil {
		t.Fatal("未关闭原生资源的终态journal被接受")
	}
	if err = closeRecoveredIOSResources(context.Background(), lock); err != nil {
		t.Fatal(err)
	}
	// 只有真实关闭后的原完整终态摘要才可被核对；故意旧摘要不可猜测修复。
	if _, err = readTerminalJournal(lock, journal.name); err == nil {
		t.Fatal("关闭变更被误当原终态精确回执")
	}
	data, info, err := lock.readJournalFile(journal.name)
	if err != nil {
		t.Fatal(err)
	}
	var state journalState
	if json.Unmarshal(data, &state) != nil {
		t.Fatal("实际关闭journal不可读")
	}
	state.PendingEvent.Progress.IOSResourceDigest = iosDigestState(state)
	state.PendingEvent.Digest = progressHash(state.PendingEvent.Progress)
	current := &executionJournal{lock: lock, name: journal.name, info: info, state: state}
	if err = current.save(); err != nil {
		t.Fatal(err)
	}
	restored, err := readTerminalJournal(lock, journal.name)
	if err != nil || !restored.state.IOSResources.Closed {
		t.Fatal("真实空计划恢复关闭后未接受精确终态", err)
	}
	if _, err = os.Stat(own.Output); !os.IsNotExist(err) {
		t.Fatal("真实自有目录未关闭")
	}
}
func TestIOSJournalPreparingCrashRemainsGuarded(t *testing.T) {
	if !mobile.IOSSigningSupported() {
		t.Skip("实际规划仅macOS")
	}
	lock, journal := recoveryJournalFile(t)
	root, _ := filepath.EvalSymlinks(t.TempDir())
	resources, err := mobile.PlanIOSResources(context.Background(), mobile.IOSSigningOptions{Workspace: root, OutputDir: "output", P12File: "absent", ProfileFile: "absent", BundleID: "com.example.app", ExportMethod: "debugging"})
	if err != nil {
		t.Fatal(err)
	}
	defer resources.Close(context.Background())
	own := resources.Ownership()
	own.Preparing = true
	journal.state.IOSSigningRequired = true
	journal.state.IOSResources = &own
	if err = journal.save(); err != nil {
		t.Fatal(err)
	}
	if err = closeRecoveredIOSResources(context.Background(), lock); err == nil {
		t.Fatal("准备中崩溃被猜测清理完成")
	}
	if _, err = os.Stat(own.Output); err != nil {
		t.Fatal("未知准备目录被删除")
	}
	if _, err = readTerminalJournal(lock, journal.name); err == nil {
		t.Fatal("未知准备证据释放journal")
	}
}
func TestIOSSecretRefsUseOnlyDeclaredNodeValues(t *testing.T) {
	data := []byte("version: 1\nios_signing: {p12: '${P12}', profile: '${PROFILE}', password: '${PASSWORD}', bundle_id: com.example.app, export_method: debugging}\nsteps: [{kind: run,run: echo hello}]\n")
	doc, err := config.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]string{"P12": "own.p12", "PROFILE": "own.profile", "PASSWORD": "sensitive-value", "UNDECLARED": "private-undeclared"}
	got, err := taskSecrets(config.AgentConfig{}, *doc.Builds["default"], values)
	if err != nil || len(got) != 3 || got["PASSWORD"] != values["PASSWORD"] || got["UNDECLARED"] != "" {
		t.Fatal("签名秘密未沿声明边界消费", err)
	}
}

// 摘要仍取唯一实际wire JSON，所有权不会进入公共事件。
func progressHash(p protocol.ExecutionProgress) string {
	data, _ := json.Marshal(p)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
