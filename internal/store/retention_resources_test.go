package store

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
)

func resourceRegistration(ref protocol.LeaseRef) protocol.NodeResourceRegistration {
	return protocol.NodeResourceRegistration{Ref: ref, ID: uuid.NewString(), OwnershipDigest: strings.Repeat("a", 64), HasWorkspace: true}
}

func readRegisteredResource(t *testing.T, s *Store, id string) nodeResourceRecord {
	t.Helper()
	var row nodeResourceRecord
	if err := s.db.First(&row, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	return row
}

func TestRetentionResourceRegistrationStableSlots(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		a, grant, _ := claimed(t, s)
		in := resourceRegistration(grant.Ref)
		before := time.Now().UTC()
		if err := s.RegisterNodeResource(testContext, a, in); err != nil {
			t.Fatal(err)
		}
		first := readRegisteredResource(t, s, in.ID)
		if first.RegisteredAt.Before(before) || first.RegisteredAt.After(time.Now().UTC()) || first.AttemptID != grant.Ref.AttemptID || first.NodeID != a.ID || !first.HasWorkspace || first.HasResults {
			t.Fatal("真实fence登记与服务端时间", first)
		}
		for range 20 {
			if err := s.RegisterNodeResource(testContext, a, in); err != nil {
				t.Fatal(err)
			}
		}
		if got := readRegisteredResource(t, s, in.ID); !reflect.DeepEqual(got, first) {
			t.Fatal("同请求不能更新归属或时间")
		}
		expanded := in
		expanded.HasResults, expanded.OwnershipDigest = true, strings.Repeat("b", 64)
		if err := s.RegisterNodeResource(testContext, a, expanded); err != nil {
			t.Fatal("真实结果槽仅增加一次", err)
		}
		full := readRegisteredResource(t, s, in.ID)
		if !full.HasResults || !full.HasWorkspace || full.OwnershipDigest != expanded.OwnershipDigest || full.RegisteredAt != first.RegisteredAt {
			t.Fatal("扩槽保持原身份与首次登记时间")
		}
		for _, changed := range []protocol.NodeResourceRegistration{in, func() protocol.NodeResourceRegistration { x := expanded; x.ID = uuid.NewString(); return x }(), func() protocol.NodeResourceRegistration {
			x := expanded
			x.OwnershipDigest = strings.Repeat("c", 64)
			return x
		}(), func() protocol.NodeResourceRegistration { x := expanded; x.HasWorkspace = false; return x }()} {
			if err := s.RegisterNodeResource(testContext, a, changed); err != ErrConflict {
				t.Fatal("换身份/减槽/同槽改摘要必须拒绝", err)
			}
		}
		if got := readRegisteredResource(t, s, in.ID); !reflect.DeepEqual(got, full) {
			t.Fatal("拒绝不得部分更新")
		}
	})
}

func TestRetentionResourceRegistrationFenceAndInvalid(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		a, grant, _ := claimed(t, s)
		in := resourceRegistration(grant.Ref)
		for _, changed := range []protocol.NodeResourceRegistration{func() protocol.NodeResourceRegistration { x := in; x.ID = "../unknown"; return x }(), func() protocol.NodeResourceRegistration {
			x := in
			x.OwnershipDigest = strings.Repeat("A", 64)
			return x
		}(), func() protocol.NodeResourceRegistration { x := in; x.HasWorkspace = false; return x }(), func() protocol.NodeResourceRegistration { x := in; x.Ref.Epoch = 0; return x }()} {
			if err := s.RegisterNodeResource(testContext, a, changed); err != ErrInvalid {
				t.Fatal("非法值", err)
			}
		}
		other, _ := openSession(t, s, "resource-other", 1)
		if err := s.RegisterNodeResource(testContext, other, in); err != ErrNodeUnauthorized {
			t.Fatal("另一Node不得登记原资源", err)
		}
		for _, changed := range []protocol.NodeResourceRegistration{func() protocol.NodeResourceRegistration { x := in; x.Ref.SessionID = uuid.NewString(); return x }(), func() protocol.NodeResourceRegistration { x := in; x.Ref.LeaseID = uuid.NewString(); return x }(), func() protocol.NodeResourceRegistration { x := in; x.Ref.Epoch++; return x }()} {
			if err := s.RegisterNodeResource(testContext, a, changed); err != ErrLeaseInvalid {
				t.Fatal("原完整fence逐项核对", err)
			}
		}
		var count int64
		if err := s.db.Model(&nodeResourceRecord{}).Count(&count).Error; err != nil || count != 0 {
			t.Fatal("拒绝不得产生资源", err, count)
		}
	})
}

func TestRetentionResourceRegistrationTerminalReadonlyAndRotation(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		a, grant, _ := claimed(t, s)
		in := resourceRegistration(grant.Ref)
		if err := s.RegisterNodeResource(testContext, a, in); err != nil {
			t.Fatal(err)
		}
		p := protocol.ExecutionProgress{Kind: "build_finished", Status: "failed", Reason: "precheck_error", PostPhase: "none", StopConfirmed: true, RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}
		accept(t, s, a, event(grant.Ref, 1, p))
		before := readRegisteredResource(t, s, in.ID)
		buildBefore, err := s.GetBuild(testContext, grant.Ref.BuildID)
		if err != nil {
			t.Fatal(err)
		}
		rotated, err := s.RotateNodeToken(testContext, localAdmin, "linux")
		if err != nil {
			t.Fatal(err)
		}
		current, err := s.AuthenticateNode(testContext, rotated.Token)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.RegisterNodeResource(testContext, a, in); err != ErrNodeUnauthorized {
			t.Fatal("撤销原credential不能只读确认", err)
		}
		for range 20 {
			if err := s.RegisterNodeResource(testContext, current, in); err != nil {
				t.Fatal("当前同Node可只读确认精确原登记", err)
			}
		}
		for _, changed := range []protocol.NodeResourceRegistration{func() protocol.NodeResourceRegistration { x := in; x.ID = uuid.NewString(); return x }(), func() protocol.NodeResourceRegistration {
			x := in
			x.HasResults = true
			x.OwnershipDigest = strings.Repeat("b", 64)
			return x
		}(), func() protocol.NodeResourceRegistration { x := in; x.Ref.SessionID = uuid.NewString(); return x }(), func() protocol.NodeResourceRegistration {
			x := in
			x.OwnershipDigest = strings.Repeat("b", 64)
			return x
		}()} {
			if err := s.RegisterNodeResource(testContext, current, changed); err != ErrConflict {
				t.Fatal("终态禁止创建/扩槽/换归属", err)
			}
		}
		if got := readRegisteredResource(t, s, in.ID); !reflect.DeepEqual(got, before) {
			t.Fatal("终态确认不得改登记")
		}
		if got, err := s.GetBuild(testContext, grant.Ref.BuildID); err != nil || !reflect.DeepEqual(got, buildBefore) {
			t.Fatal("终态确认不能改执行或停止事实", err)
		}
	})
}

func TestRetentionResourceRegistrationInterruptedGuardIsReadonly(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		a, grant, _ := claimed(t, s)
		in := resourceRegistration(grant.Ref)
		if err := s.RegisterNodeResource(testContext, a, in); err != nil {
			t.Fatal(err)
		}
		if err := s.SetNodeState(testContext, localAdmin, "linux", "disabled"); err != nil {
			t.Fatal(err)
		}
		if err := s.SetNodeState(testContext, localAdmin, "linux", "enabled"); err != nil {
			t.Fatal(err)
		}
		before, err := s.GetBuild(testContext, grant.Ref.BuildID)
		if err != nil || !before.StopUnconfirmed || before.Status != "interrupted" {
			t.Fatal("真实disable/enable保持停止未知", err)
		}
		if err := s.RegisterNodeResource(testContext, a, in); err != nil {
			t.Fatal("精确登记只读恢复不授执行权", err)
		}
		after, err := s.GetBuild(testContext, grant.Ref.BuildID)
		if err != nil || !reflect.DeepEqual(before, after) {
			t.Fatal("登记ACK不能解除guard", err)
		}
		if err := s.CheckExecution(testContext, a, grant.Ref); err == nil {
			t.Fatal("不能恢复旧执行权")
		}
	})
}

func TestRetentionResourceRegistrationMissingTerminalAndDisabled(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		a, grant, _ := claimed(t, s)
		in := resourceRegistration(grant.Ref)
		p := protocol.ExecutionProgress{Kind: "build_finished", Status: "failed", Reason: "checkout_error", PostPhase: "none", StopConfirmed: true, RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}
		accept(t, s, a, event(grant.Ref, 1, p))
		if err := s.RegisterNodeResource(testContext, a, in); err != ErrConflict {
			t.Fatal("终态不能补猜未登记目录", err)
		}
		if err := s.SetNodeState(testContext, localAdmin, "linux", "disabled"); err != nil {
			t.Fatal(err)
		}
		if err := s.RegisterNodeResource(testContext, a, in); err != ErrNodeUnauthorized {
			t.Fatal("disabled当前身份不能确认", err)
		}
	})
}

func TestRetentionResourceRegistrationCancelledQueueWait(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		a, grant, _ := claimed(t, s)
		in := resourceRegistration(grant.Ref)
		s.mu.Lock()
		ctx, cancel := context.WithCancel(testContext)
		cancel()
		done := make(chan error, 1)
		go func() { done <- s.RegisterNodeResource(ctx, a, in) }()
		select {
		case err := <-done:
			s.mu.Unlock()
			if err != ErrRetentionCancelled {
				t.Fatal("取消排队固定错误", err)
			}
		case <-time.After(time.Second):
			s.mu.Unlock()
			<-done
			t.Fatal("已取消ctx不得无限等待锁")
		}
	})
}

func TestRetentionResourceTerminalProofUsesActualEventTransaction(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		a, grant, _ := claimed(t, s)
		in := resourceRegistration(grant.Ref)
		if err := s.RegisterNodeResource(testContext, a, in); err != nil {
			t.Fatal(err)
		}
		first := readRegisteredResource(t, s, in.ID)
		p := protocol.ExecutionProgress{Kind: "build_finished", Status: "failed", Reason: "precheck_error", PostPhase: "none", StopConfirmed: true, RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}
		terminal := event(grant.Ref, 1, p)
		ack, err := s.ApplyEvent(testContext, a, terminal)
		if err != nil || ack.Seq != terminal.Seq || ack.Digest != terminal.Digest {
			t.Fatal("原实际终态ACK", err)
		}
		got := readRegisteredResource(t, s, in.ID)
		if got.TerminalSeq != terminal.Seq || got.TerminalDigest != terminal.Digest || got.RegisteredAt != first.RegisteredAt || got.OwnershipDigest != first.OwnershipDigest {
			t.Fatal("同终态事务保存精确资源回执，归属和登记时间不变")
		}
		if err := s.RegisterNodeResource(testContext, a, in); err != nil {
			t.Fatal(err)
		}
		if again := readRegisteredResource(t, s, in.ID); !reflect.DeepEqual(again, got) {
			t.Fatal("只读重确认不能改精确资源回执")
		}
	})
}

func TestRetentionResourceCompletionActualStopAndStableReplay(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		a, grant, _ := claimed(t, s)
		in := resourceRegistration(grant.Ref)
		if err := s.RegisterNodeResource(testContext, a, in); err != nil {
			t.Fatal(err)
		}
		completion := protocol.NodeResourceCompletion{StopCode: "process_group_reaped"}
		complete := in
		complete.Completion = &completion
		if err := s.RegisterNodeResource(testContext, a, complete); err != ErrConflict {
			t.Fatal("running不能提前确认完成", err)
		}
		if err := s.SetNodeState(testContext, localAdmin, "linux", "disabled"); err != nil {
			t.Fatal(err)
		}
		if err := s.SetNodeState(testContext, localAdmin, "linux", "enabled"); err != nil {
			t.Fatal(err)
		}
		if err := s.RegisterNodeResource(testContext, a, complete); err != ErrConflict {
			t.Fatal("停止未知不能提前确认完成", err)
		}
		confirmation := protocol.StopConfirmation{Ref: grant.Ref, EvidenceCode: "process_group_reaped", Note: "测试租约没有用户动作，独立停止证据"}
		if err := s.ConfirmNodeStopped(testContext, a, confirmation); err != nil {
			t.Fatal(err)
		}
		before, err := s.GetBuild(testContext, grant.Ref.BuildID)
		if err != nil || before.Status != "interrupted" || before.StopUnconfirmed {
			t.Fatal(err)
		}
		if err := s.RegisterNodeResource(testContext, a, complete); err != nil {
			t.Fatal("原精确归属、Stop及中央游标确认完成", err)
		}
		registered := readRegisteredResource(t, s, in.ID)
		if registered.CompletedAt == nil || registered.CompletionJSON == "" || registered.TerminalSeq != 0 || registered.TerminalDigest != "" {
			t.Fatal("必须保存真实完成，不伪造终态回执")
		}
		for range 20 {
			if err := s.RegisterNodeResource(testContext, a, complete); err != nil {
				t.Fatal(err)
			}
		}
		if got := readRegisteredResource(t, s, in.ID); !reflect.DeepEqual(got, registered) {
			t.Fatal("完成精确重放不刷新时间或摘要")
		}
		if got, err := s.GetBuild(testContext, grant.Ref.BuildID); err != nil || !reflect.DeepEqual(got, before) {
			t.Fatal("完成不能修改原状态/TerminalAt/Stop", err)
		}
		if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 100, Days: 30}); err != nil {
			t.Fatal(err)
		}
		project, err := s.GetProject(testContext, "app")
		if err != nil {
			t.Fatal(err)
		}
		page, err := s.EvaluateRetention(testContext, localAdmin, project.ID, Page{})
		if err != nil || len(page.Items) != 1 || len(page.Items[0].ProtectReasons) != 0 {
			t.Fatal("实际完成确认可解除资源保护，其它事实仍单独核对", err, page)
		}
		rotated, err := s.RotateNodeToken(testContext, localAdmin, "linux")
		if err != nil {
			t.Fatal(err)
		}
		current, err := s.AuthenticateNode(testContext, rotated.Token)
		if err != nil || s.RegisterNodeResource(testContext, current, complete) != nil {
			t.Fatal("当前同Node可恢复精确已确认完成", err)
		}
		if s.RegisterNodeResource(testContext, a, complete) != ErrNodeUnauthorized {
			t.Fatal("旧credential拒绝")
		}
	})
}

func TestRetentionResourceCompletionRejectsUnknownAndCursorChanges(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		a, grant, _ := claimed(t, s)
		in := resourceRegistration(grant.Ref)
		if err := s.RegisterNodeResource(testContext, a, in); err != nil {
			t.Fatal(err)
		}
		if err := s.SetNodeState(testContext, localAdmin, "linux", "disabled"); err != nil {
			t.Fatal(err)
		}
		if err := s.SetNodeState(testContext, localAdmin, "linux", "enabled"); err != nil {
			t.Fatal(err)
		}
		if err := s.ConfirmNodeStopped(testContext, a, protocol.StopConfirmation{Ref: grant.Ref, EvidenceCode: "process_group_reaped", Note: "测试租约无动作"}); err != nil {
			t.Fatal(err)
		}
		before := readRegisteredResource(t, s, in.ID)
		for _, completion := range []protocol.NodeResourceCompletion{{StopCode: "process_group_reaped", LastEventSeq: 1}, {StopCode: "process_group_reaped", LastLogSeq: 1, LastLogOffset: 10}, {StopCode: "process_group_reaped", LastArtifactSeq: 1}} {
			request := in
			request.Completion = &completion
			if err := s.RegisterNodeResource(testContext, a, request); err != ErrConflict {
				t.Fatal("不同已确认游标不得假完成", err)
			}
		}
		for _, completion := range []protocol.NodeResourceCompletion{{StopCode: "admin_observed_stopped"}, {StopCode: "process_group_reaped", LastEventSeq: -1}, {StopCode: "process_group_reaped", LastLogSeq: -1}, {StopCode: "process_group_reaped", LastLogOffset: -1}, {StopCode: "process_group_reaped", LastArtifactSeq: -1}, {StopCode: "process_group_reaped", LastLogOffset: 1}} {
			request := in
			request.Completion = &completion
			if err := s.RegisterNodeResource(testContext, a, request); err != ErrInvalid {
				t.Fatal("无效完成字段拒绝", err)
			}
		}
		if got := readRegisteredResource(t, s, in.ID); !reflect.DeepEqual(got, before) {
			t.Fatal("任意失败不能部分确认")
		}
	})
}
