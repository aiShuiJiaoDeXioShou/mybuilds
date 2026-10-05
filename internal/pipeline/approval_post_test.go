package pipeline

import (
	"context"
	"errors"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 最后ordinary审批恢复后，仅执行真实历史动作所选post，不重跑已完成步骤。
func TestApprovalFinalResumePreparesPostFromStartedHistory(t *testing.T) {
	for _, mode := range []string{"started", "skipped", "only_approval"} {
		t.Run(mode, func(t *testing.T) {
			workspace, parent := t.TempDir(), t.TempDir()
			os.Chmod(parent, 0700)
			before := "- kind: run\n  name: before\n  run: echo before >> actions\n"
			if mode == "skipped" {
				before += "  when: {params: {mode: 'on'}}\n"
			}
			if mode == "only_approval" {
				before = ""
			}
			source := "version: 1\nparams: {mode: 'off'}\nsteps:\n" + before + "- kind: approval\n  name: first\n- kind: approval\n  name: second\npost:\n  always:\n  - kind: run\n    name: cleanup\n    run: echo post >> post\n"
			doc, err := config.Parse([]byte(source))
			if err != nil {
				t.Fatal(err)
			}
			remote := &RemoteOptions{AuthorityContext: context.Background(), ResultParent: parent, RemainingPostBudgetNS: int64(time.Minute), Log: func(context.Context, protocol.LogRecord) error { return nil }}
			events := []protocol.ExecutionProgress{}
			remote.Progress = func(_ context.Context, p protocol.ExecutionProgress) error { events = append(events, p); return nil }
			options := RunOptions{Workspace: workspace, Remote: remote}
			for pauseIndex := 0; pauseIndex < 2; pauseIndex++ {
				result, err := Run(context.Background(), doc, options)
				if !errors.Is(err, ErrApprovalPaused) || result == nil || result.Paused == nil {
					t.Fatal("真实暂停未发生", err)
				}
				if _, err := os.Stat(filepath.Join(workspace, "post")); !os.IsNotExist(err) {
					t.Fatal("审批前执行了post")
				}
				local := result.Paused.Local
				ledger := []protocol.ApprovalStepLedger{}
				for i, s := range local.Steps {
					ledger = append(ledger, protocol.ApprovalStepLedger{Phase: "ordinary", Index: i + 1, Name: s.Name, Kind: s.Kind, Status: s.Status, Started: s.Started, StopConfirmed: !s.CleanupFailed})
				}
				ledger = append(ledger, protocol.ApprovalStepLedger{Phase: "ordinary", Index: result.Paused.Index, Name: result.Paused.Step, Kind: "approval", Status: "succeeded", StopConfirmed: true})
				options.Resume = &ApprovalResume{Evidence: protocol.ApprovalResumeEvidence{NextOrdinaryIndex: result.Paused.Index + 1, Steps: ledger}, Local: local}
			}
			events = nil
			result, err := Run(context.Background(), doc, options)
			if err != nil || result == nil || result.Paused != nil {
				t.Fatal("最后仅post恢复失败", err)
			}
			actions, _ := os.ReadFile(filepath.Join(workspace, "actions"))
			post, postErr := os.ReadFile(filepath.Join(workspace, "post"))
			if mode == "started" {
				if string(actions) != "before\n" || string(post) != "post\n" || result.Builds[0].Post[0].Status != "succeeded" {
					t.Fatal("历史动作未触发真正post或脚本被重复运行")
				}
			} else if !os.IsNotExist(postErr) || len(actions) != 0 {
				t.Fatal("无实际普通动作却运行post")
			}
			for _, e := range events {
				if e.Phase == "ordinary" {
					t.Fatal("最后恢复重发了旧ordinary")
				}
			}
		})
	}
}
