package pipeline

import (
	"context"
	"errors"
	"fmt"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 尚未交付的通知必须整批预检查拒绝，不能先执行脚本或伪造远端挂起。
func TestApprovalNotifyPrecheckBeforeAnyAction(t *testing.T) {
	for _, remote := range []bool{false, true} {
		for _, mode := range []string{"unsupported", "false", "skipped"} {
			t.Run(fmt.Sprintf("remote=%t/%s", remote, mode), func(t *testing.T) {
				workspace := t.TempDir()
				notify, when := "true", ""
				if mode == "false" {
					notify = "false"
				}
				if mode == "skipped" {
					when = "\n  when: {params: {mode: 'on'}}"
				}
				source := "version: 1\nparams: {mode: 'off'}\nsteps:\n- kind: run\n  name: before\n  run: ': > marker'\n- kind: approval\n  name: review\n  notify: " + notify + when + "\n"
				doc, err := config.Parse([]byte(source))
				if err != nil {
					t.Fatal(err)
				}
				options := RunOptions{Workspace: workspace, ConfirmApproval: func(context.Context, ApprovalPrompt) (bool, error) { return true, nil }}
				events := 0
				if remote {
					parent := t.TempDir()
					os.Chmod(parent, 0700)
					options.Remote = &RemoteOptions{AuthorityContext: context.Background(), ResultParent: parent, RemainingPostBudgetNS: int64(time.Minute), Log: func(context.Context, protocol.LogRecord) error { return nil }, Progress: func(context.Context, protocol.ExecutionProgress) error { events++; return nil }}
				}
				result, err := Run(context.Background(), doc, options)
				_, markerErr := os.Stat(filepath.Join(workspace, "marker"))
				if mode == "unsupported" {
					if err == nil || err.Error() != "unsupported" || result != nil || !os.IsNotExist(markerErr) || events != 0 {
						t.Fatal("通知未在所有动作前固定拒绝", err)
					}
					return
				}
				if markerErr != nil || result == nil {
					t.Fatal("合法false/条件跳过被拒绝", err)
				}
				if remote && mode == "false" {
					if !errors.Is(err, ErrApprovalPaused) {
						t.Fatal("无通知远端审批被拒绝", err)
					}
				} else if err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
