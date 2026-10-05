package store

import (
	"testing"

	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
)

// 真事务证明：原生工具或管理员标签不能替代 Flutter SDK/Dart 检查。
func TestFlutterCapabilityClaimFrozenFramework(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		actor, session, project, policy := leaseFixture(t, s)
		report := leaseReport()
		if _, err := s.Heartbeat(testContext, actor, protocol.HeartbeatRequest{SessionID: session.SessionID, Report: report}, policy); err != nil {
			t.Fatal(err)
		}
		in := enqueueInput(project, "flutter-capability")
		in.Builds[0].Snapshot.Definition.Runner = &config.Runner{Platform: "android", Framework: "flutter"}
		enqueued, err := s.Enqueue(testContext, in)
		if err != nil {
			t.Fatal(err)
		}
		claim := func() *protocol.LeaseGrant {
			t.Helper()
			grant, err := s.Claim(testContext, actor, protocol.ClaimRequest{SessionID: session.SessionID, ClaimKey: uuid.NewString()}, policy)
			if err != nil {
				t.Fatal(err)
			}
			return grant
		}
		if claim() != nil {
			t.Fatal("只原生工具错误领取 Flutter")
		}
		report.Tools = append(report.Tools, protocol.ToolCheck{Name: "flutter", Status: "passed", Version: "3.38.6"}, protocol.ToolCheck{Name: "dart", Status: "failed", Reason: "tool_version_invalid"}, protocol.ToolCheck{Name: "cocoapods", Status: "skipped", Reason: "unsupported"})
		if _, err := s.Heartbeat(testContext, actor, protocol.HeartbeatRequest{SessionID: session.SessionID, Report: report}, policy); err != nil {
			t.Fatal(err)
		}
		if claim() != nil {
			t.Fatal("坏 Dart 仍领取 Flutter")
		}
		report.Tools[len(report.Tools)-2] = protocol.ToolCheck{Name: "dart", Status: "passed", Version: "3.10.7"}
		if _, err := s.Heartbeat(testContext, actor, protocol.HeartbeatRequest{SessionID: session.SessionID, Report: report}, policy); err != nil {
			t.Fatal(err)
		}
		grant := claim()
		if grant == nil || grant.Ref.BuildID != enqueued.Builds[0].ID || grant.Task.Definition.Runner.Framework != "flutter" {
			t.Fatal("冻结框架未真实调度")
		}
		invalid := report
		invalid.Tools = append(append([]protocol.ToolCheck{}, report.Tools...), protocol.ToolCheck{Name: "invented_sdk", Status: "passed"})
		if _, err := s.Heartbeat(testContext, actor, protocol.HeartbeatRequest{SessionID: session.SessionID, Report: invalid}, policy); err != ErrInvalid {
			t.Fatal("任意工具名称被接受", err)
		}
	})
}
