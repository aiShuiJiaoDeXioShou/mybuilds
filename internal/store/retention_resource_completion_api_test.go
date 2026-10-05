package store

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
)

func TestRetentionResourceCompletionActualConsumerReevaluatesGuard(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 1, Days: 1}); err != nil {
			t.Fatal(err)
		}
		a, g, _ := claimed(t, s)
		registration := protocol.NodeResourceRegistration{Ref: g.Ref, ID: uuid.NewString(), OwnershipDigest: strings.Repeat("a", 64), HasWorkspace: true, HasResults: true}
		if err := s.RegisterNodeResource(testContext, a, registration); err != nil {
			t.Fatal(err)
		}
		accept(t, s, a, event(g.Ref, 1, stepProgress("intent", "", "ordinary", "compile", 1)))
		progress := stepProgress("started", "", "ordinary", "compile", 1)
		progress.Started = true
		accept(t, s, a, event(g.Ref, 2, progress))
		if err := s.SetNodeState(testContext, localAdmin, "linux", "disabled"); err != nil {
			t.Fatal(err)
		}
		if err := s.SetNodeState(testContext, localAdmin, "linux", "enabled"); err != nil {
			t.Fatal(err)
		}
		if err := s.ConfirmNodeStopped(testContext, a, protocol.StopConfirmation{Ref: g.Ref, EvidenceCode: "process_group_reaped", Note: "真实独立节点物理stop ACK"}); err != nil {
			t.Fatal(err)
		}
		var row buildRecord
		if err := s.db.First(&row, "id = ?", g.Ref.BuildID).Error; err != nil {
			t.Fatal(err)
		}
		at := row.TerminalAt.Add(48 * time.Hour)
		before := retentionEvaluateAt(t, s, row.ProjectID, at, Page{})
		if !slices.Contains(retentionEntry(t, before, row.ID).ProtectReasons, "resource_unconfirmed") {
			t.Fatal("未完成资源被放开")
		}
		registration.Completion = &protocol.NodeResourceCompletion{LastEventSeq: row.LastEventSeq, LastLogSeq: row.LastLogSeq, LastLogOffset: row.LastLogOffset, LastArtifactSeq: row.LastArtifactSeq, StopCode: "process_group_reaped"}
		if err := s.RegisterNodeResource(testContext, a, registration); err != nil {
			t.Fatal("真实完成consumer", err)
		}
		after := retentionEvaluateAt(t, s, row.ProjectID, at, Page{})
		entry := retentionEntry(t, after, row.ID)
		if !entry.Candidate || len(entry.ProtectReasons) != 0 || !entry.TerminalAt.Equal(*row.TerminalAt) {
			t.Fatal("实际完成ACK未解除资源保护或刷新终态", entry)
		}
		var resource nodeResourceRecord
		if err := s.db.First(&resource, "id = ?", registration.ID).Error; err != nil {
			t.Fatal(err)
		}
		if resource.CompletedAt == nil || resource.CompletionJSON == "" || resource.TerminalSeq != 0 || resource.TerminalDigest != "" {
			t.Fatal("伪造终态或未保存真实完成")
		}
		if err := s.RegisterNodeResource(testContext, a, registration); err != nil {
			t.Fatal("精确重放", err)
		}
		var again nodeResourceRecord
		if err := s.db.First(&again, "id = ?", registration.ID).Error; err != nil {
			t.Fatal(err)
		}
		if !again.CompletedAt.Equal(*resource.CompletedAt) || again.CompletionJSON != resource.CompletionJSON {
			t.Fatal("重放刷新完成事实")
		}
	})
}
