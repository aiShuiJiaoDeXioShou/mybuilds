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

func TestApprovalLocalConfirmAndReject(t *testing.T) {
	for _, allow := range []bool{false, true} {
		t.Run(map[bool]string{false: "reject", true: "approve"}[allow], func(t *testing.T) {
			workspace := t.TempDir()
			doc, err := config.Parse([]byte("version: 1\nsteps:\n- kind: run\n  name: before\n  run: echo before >> actions\n- kind: approval\n  name: review\n- kind: run\n  name: after\n  run: echo after >> actions\n"))
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			result, err := Run(context.Background(), doc, RunOptions{Workspace: workspace, ConfirmApproval: func(context.Context, ApprovalPrompt) (bool, error) { calls++; return allow, nil }})
			data, _ := os.ReadFile(filepath.Join(workspace, "actions"))
			if calls != 1 || result == nil {
				t.Fatal("审批实际消费者未调用", err)
			}
			if allow {
				if err != nil || string(data) != "before\nafter\n" {
					t.Fatal("批准未继续原Run", err)
				}
			} else {
				if err == nil || string(data) != "before\n" {
					t.Fatal("拒绝仍运行后步")
				}
			}
		})
	}
}
func TestApprovalRemotePauseResumeDoesNotRerun(t *testing.T) {
	workspace := t.TempDir()
	parent := t.TempDir()
	os.Chmod(parent, 0700)
	doc, err := config.Parse([]byte("version: 1\nsteps:\n- kind: run\n  name: before\n  run: echo before >> actions\n- kind: approval\n  name: review\n- kind: run\n  name: after\n  run: echo after >> actions\n"))
	if err != nil {
		t.Fatal(err)
	}
	remote := &RemoteOptions{AuthorityContext: context.Background(), ResultParent: parent, RemainingPostBudgetNS: int64(2 * time.Minute), Log: func(context.Context, protocol.LogRecord) error { return nil }}
	var progress []protocol.ExecutionProgress
	remote.Progress = func(_ context.Context, p protocol.ExecutionProgress) error {
		progress = append(progress, p)
		return nil
	}
	first, err := Run(context.Background(), doc, RunOptions{Workspace: workspace, Remote: remote})
	if !errors.Is(err, ErrApprovalPaused) || first == nil || first.Paused == nil {
		t.Fatal("没有实际pause", err)
	}
	if len(progress) != 4 || progress[3].Kind != "approval_checkpoint" || progress[3].LocalApproval == nil {
		t.Fatal("暂停伪终态或缺少私有持久证据")
	}
	if data, _ := os.ReadFile(filepath.Join(workspace, "actions")); string(data) != "before\n" {
		t.Fatal("暂停后步已执行")
	}
	evidence := protocol.ApprovalResumeEvidence{NextOrdinaryIndex: 3, Steps: []protocol.ApprovalStepLedger{{Phase: "ordinary", Index: 1, Name: "before", Kind: "run", Status: "succeeded", Started: true, StopConfirmed: true}, {Phase: "ordinary", Index: 2, Name: "review", Kind: "approval", Status: "succeeded", StopConfirmed: true}}}
	progress = nil
	second, err := Run(context.Background(), doc, RunOptions{Workspace: workspace, Remote: remote, Resume: &ApprovalResume{Evidence: evidence, Local: first.Paused.Local}})
	if err != nil || second == nil || second.Paused != nil {
		t.Fatal("恢复原Run失败", err)
	}
	if data, _ := os.ReadFile(filepath.Join(workspace, "actions")); string(data) != "before\nafter\n" {
		t.Fatal("原已完成脚本被重跑", string(data))
	}
	for _, p := range progress {
		if p.Name == "before" || p.Name == "review" {
			t.Fatal("重发旧步骤事件")
		}
	}
	if progress[len(progress)-1].Kind != "build_finished" || !progress[len(progress)-1].Started {
		t.Fatal("真实终态丢失原Started")
	}
}

func TestApprovalRestoredReportSnapshotRejectsChangedLeaf(t *testing.T) {
	workspace := t.TempDir()
	resultDir := t.TempDir()
	os.MkdirAll(filepath.Join(workspace, "results"), 0700)
	os.WriteFile(filepath.Join(workspace, "results", "test.xml"), []byte(`<testsuite><testcase name="ok"/></testsuite>`), 0600)
	wr, _ := os.OpenRoot(workspace)
	defer wr.Close()
	rr, _ := os.OpenRoot(resultDir)
	defer rr.Close()
	c, err := newReportCollection(context.Background(), wr, rr, []string{"results/*.xml"}, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	// baseline后真实修改产物，检查并保存原快照。
	os.WriteFile(filepath.Join(workspace, "results", "test.xml"), []byte(`<testsuite><testcase name="new"/></testsuite>`), 0600)
	_, _, err = c.check(context.Background(), 1, "test", false)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := c.checkpoint()
	if err != nil || len(saved.Current) != 1 {
		t.Fatal("实际报告未收集", err)
	}
	leaf := filepath.Join(resultDir, filepath.FromSlash(saved.Current[0].SnapshotPath))
	os.Remove(leaf)
	os.Symlink(filepath.Join(workspace, "results", "test.xml"), leaf)
	if _, err = restoreReportCollection(context.Background(), wr, rr, c.patterns, nil, true, saved); err == nil {
		t.Fatal("替换快照链接被恢复接受")
	}
}

// 中央审批ACK已返回后退出服务，不得将已暂停执行降级为独立停止确认。
func TestApprovalAcknowledgedPauseSurvivesAuthorityCancellation(t *testing.T) {
	workspace, parent := t.TempDir(), t.TempDir()
	os.Chmod(parent, 0700)
	doc, err := config.Parse([]byte("version: 1\nsteps:\n- kind: run\n  name: before\n  run: echo before >> actions\n- kind: approval\n  name: review\n- kind: run\n  name: after\n  run: echo after >> actions\n"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var kinds []string
	remote := &RemoteOptions{AuthorityContext: ctx, ResultParent: parent, RemainingPostBudgetNS: int64(time.Minute), Log: func(context.Context, protocol.LogRecord) error { return nil }, Progress: func(_ context.Context, p protocol.ExecutionProgress) error {
		kinds = append(kinds, p.Kind)
		if p.Kind == "approval_checkpoint" {
			cancel()
		}
		return nil
	}}
	result, err := Run(ctx, doc, RunOptions{Workspace: workspace, Remote: remote})
	if !errors.Is(err, ErrApprovalPaused) || result == nil || result.Paused == nil || result.Builds[0].Status != "waiting_approval" {
		t.Fatal("已ACK暂停被服务退出降级", err)
	}
	if kinds[len(kinds)-1] != "approval_checkpoint" {
		t.Fatal("暂停后伪造了终态")
	}
	data, err := os.ReadFile(filepath.Join(workspace, "actions"))
	if err != nil || string(data) != "before\n" {
		t.Fatal("暂停后执行了动作")
	}
}
