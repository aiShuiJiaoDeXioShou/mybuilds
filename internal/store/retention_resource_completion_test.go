package store

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
)

// 归属和物理停止均真实提交，但未上传spool/资源完成事实不能由中央ACK子集猜测。
func TestRetentionResourceInterruptedRequiresCompletion(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 1, Days: 1}); err != nil {
			t.Fatal(err)
		}
		a, g, _ := claimed(t, s)
		registration := protocol.NodeResourceRegistration{Ref: g.Ref, ID: uuid.NewString(), OwnershipDigest: strings.Repeat("a", 64), HasWorkspace: true, HasResults: true}
		if err := s.RegisterNodeResource(testContext, a, registration); err != nil {
			t.Fatal("真实登记", err)
		}
		accept(t, s, a, event(g.Ref, 1, stepProgress("intent", "", "ordinary", "compile", 1)))
		progress := stepProgress("started", "", "ordinary", "compile", 1)
		progress.Started = true
		accept(t, s, a, event(g.Ref, 2, progress))
		if err := s.SetNodeState(testContext, localAdmin, "linux", "disabled"); err != nil {
			t.Fatal(err)
		}
		if err := s.ConfirmStopped(testContext, localAdmin, protocol.StopConfirmation{Ref: g.Ref, EvidenceCode: "admin_observed_stopped", Note: "仅物理停止不能证明spool已确认"}); err != nil {
			t.Fatal(err)
		}
		page, err := s.EvaluateRetention(testContext, localAdmin, retentionProjectForBuild(t, s, g.Ref.BuildID), Page{})
		if err != nil {
			t.Fatal(err)
		}
		entry := retentionEntry(t, page, g.Ref.BuildID)
		if slices.Contains(entry.ProtectReasons, "execution_unconfirmed") || slices.Contains(entry.ProtectReasons, "stop_unconfirmed") {
			t.Fatal("精确独立Stop不应被强求伪造终态", entry)
		}
		if !slices.Contains(entry.ProtectReasons, "resource_unconfirmed") {
			t.Fatal("登记+Stop错误冒充资源及未上传spool已完成", entry)
		}
	})
}
