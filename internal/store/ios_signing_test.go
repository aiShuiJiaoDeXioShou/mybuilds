package store

import (
	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"strings"
	"testing"
	"time"
)

func iosClaimed(t *testing.T, s *Store) (NodeActor, protocol.LeaseGrant) {
	t.Helper()
	a, session, p, policy := leaseFixture(t, s)
	report := healthyReport()
	report.OS = "darwin"
	report.Arch = "arm64"
	for i := range report.Tools {
		if report.Tools[i].Name == "ios_signing" {
			report.Tools[i].Status, report.Tools[i].Reason = "passed", ""
		}
	}
	report.Tools = append(report.Tools, protocol.ToolCheck{Name: "xcode", Status: "passed", Version: "26.0"})
	if _, err := s.Heartbeat(testContext, a, protocol.HeartbeatRequest{SessionID: session.SessionID, Report: report}, policy); err != nil {
		t.Fatal(err)
	}
	in := enqueueInput(p, "ios-owned")
	in.Builds[0].Snapshot.Definition.Runner = &config.Runner{Platform: "ios"}
	in.Builds[0].Snapshot.Definition.IOSSigning = &config.IOSSigning{P12: "${P12}", Profile: "${PROFILE}", Password: "${PASSWORD}", BundleID: "com.example.app", ExportMethod: "debugging"}
	if _, err := s.Enqueue(testContext, in); err != nil {
		t.Fatal(err)
	}
	grant, err := s.Claim(testContext, a, protocol.ClaimRequest{SessionID: session.SessionID, ClaimKey: uuid.NewString()}, policy)
	if err != nil || grant == nil {
		t.Fatal("iOS能力未被真实Store消费", err)
	}
	return a, *grant
}
func TestIOSStopRequiresNativeClosure(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, grant := iosClaimed(t, s)
		if err := s.SetNodeState(testContext, localAdmin, "linux", "disabled"); err != nil {
			t.Fatal(err)
		}
		if err := s.SetNodeState(testContext, localAdmin, "linux", "enabled"); err != nil {
			t.Fatal(err)
		}
		confirmation := protocol.StopConfirmation{Ref: grant.Ref, EvidenceCode: "process_group_reaped", Note: "自有进程组已回收"}
		if err := s.ConfirmNodeStopped(testContext, a, confirmation); err != ErrStopUnconfirmed {
			t.Fatal("进程回收代替了原生关闭", err)
		}
		view, _ := s.GetBuild(testContext, grant.Ref.BuildID)
		if !view.StopUnconfirmed {
			t.Fatal("原生未知guard已解除")
		}
		confirmation.IOSCleanupConfirmed = true
		if err := s.ConfirmNodeStopped(testContext, a, confirmation); err != nil {
			t.Fatal(err)
		}
		if err := s.ConfirmNodeStopped(testContext, a, confirmation); err != nil {
			t.Fatal("精确重放失败", err)
		}
		if err := s.ConfirmNodeStopped(testContext, a, protocol.StopConfirmation{Ref: grant.Ref, EvidenceCode: confirmation.EvidenceCode, Note: confirmation.Note}); err == nil {
			t.Fatal("重放丢失原生证明")
		}
		view, _ = s.GetBuild(testContext, grant.Ref.BuildID)
		if view.StopUnconfirmed || view.Status != "interrupted" {
			t.Fatal("关闭证明复活了旧执行")
		}
	})
}
func TestIOSTerminalRequiresNativeClosureOrGuard(t *testing.T) {
	for _, mode := range []string{"known", "unknown", "original_exit"} {
		t.Run(mode, func(t *testing.T) {
			stores(t, func(t *testing.T, s *Store, opt Options) {
				a, grant := iosClaimed(t, s)
				seq := completeRunEvents(t, s, a, grant, mode == "original_exit")
				p := protocol.ExecutionProgress{Kind: "build_finished", Status: "succeeded", Started: true, StopConfirmed: true, RemainingPostBudgetNS: int64(2*time.Minute) - 100, ArtifactSteps: []protocol.ArtifactExpectation{}}
				if _, err := s.ApplyEvent(testContext, a, event(grant.Ref, seq+1, p)); err != ErrEventConflict {
					t.Fatal("无原生关闭证据终态被接受", err)
				}
				if mode == "known" {
					p.IOSCleanupConfirmed = true
					p.IOSResourceDigest = strings.Repeat("a", 64)
				} else {
					p.Status, p.Reason, p.StopConfirmed, p.CleanupFailed = "failed", "cleanup_error", false, true
					if mode == "original_exit" {
						p.Reason = "exit"
					}
				}
				accept(t, s, a, event(grant.Ref, seq+1, p))
				view, err := s.GetBuild(testContext, grant.Ref.BuildID)
				if err != nil {
					t.Fatal(err)
				}
				if mode == "known" {
					if view.Status != "succeeded" || view.StopUnconfirmed {
						t.Fatal("真实关闭终态被保留")
					}
				} else {
					if view.Status != "interrupted" || !view.StopUnconfirmed || view.Reason != p.Reason {
						t.Fatal("未知关闭覆盖原原因或释放guard", view)
					}
				}
			})
		})
	}
}
func TestIOSPrecheckZeroResourceTerminal(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, grant := iosClaimed(t, s)
		p := protocol.ExecutionProgress{Kind: "build_finished", Status: "failed", Reason: "precheck_error", PostPhase: "none", StopConfirmed: true, IOSCleanupConfirmed: true, RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}
		accept(t, s, a, event(grant.Ref, 1, p))
		view, err := s.GetBuild(testContext, grant.Ref.BuildID)
		if err != nil || view.Steps[0].Started || view.Steps[0].Intent {
			t.Fatal("预检查伪造实际动作", err)
		}
	})
}
