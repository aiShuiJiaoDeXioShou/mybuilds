package store

import (
	"fmt"
	"github.com/google/uuid"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"mybuilds/internal/protocol"
)

func TestRecoverCrossesPageBoundaryAndPreservesSnapshots(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		p, err := s.CreateProject(testContext, localAdmin, projectInput("page-app"))
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 101; i++ {
			queueBuild(t, s, p, fmt.Sprintf("page-%d", i), fmt.Sprintf("build-%d", i), false)
		}
		before := recoveryRows(t, s)
		if err = s.Recover(testContext); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, recoveryRows(t, s)) {
			t.Fatal("跨页修改冻结快照")
		}
		last := before[len(before)-1]
		if err = s.writer.Model(&buildRecord{}).Where("id = ?", last.ID).Update("snapshot_json", "broken").Error; err != nil {
			t.Fatal(err)
		}
		if err = s.Recover(testContext); err != errDatabase {
			t.Fatal("漏验证第二页", err)
		}
	})
}
func TestRecoverRejectsActiveIdentityAndProgressCorruption(t *testing.T) {
	for _, name := range []string{"node-session", "node-disabled", "credential-revoked", "intent-stopped", "started-cleanup"} {
		t.Run(name, func(t *testing.T) {
			stores(t, func(t *testing.T, s *Store, opt Options) {
				a, g, _ := claimed(t, s)
				p := stepProgress("intent", "", "ordinary", "compile", 1)
				accept(t, s, a, event(g.Ref, 1, p))
				switch name {
				case "node-session":
					s.writer.Model(&nodeRecord{}).Where("id = ?", a.ID).Update("session_id", nil)
				case "node-disabled":
					s.writer.Model(&nodeRecord{}).Where("id = ?", a.ID).Update("state", "disabled")
				case "credential-revoked":
					s.writer.Model(&nodeCredentialRecord{}).Where("id = ?", a.CredentialID).Update("revoked_at", time.Now().UTC())
				case "intent-stopped":
					s.writer.Model(&stepRecord{}).Where("build_id = ? AND phase = ?", g.Ref.BuildID, "ordinary").Update("stop_confirmed", true)
				case "started-cleanup":
					p.Kind = "started"
					p.Started = true
					accept(t, s, a, event(g.Ref, 2, p))
					s.writer.Model(&stepRecord{}).Where("build_id = ? AND phase = ?", g.Ref.BuildID, "ordinary").Update("cleanup_failed", true)
				}
				if err := s.Recover(testContext); err != errDatabase {
					t.Fatal("活动结构异常未拒绝", err)
				}
			})
		})
	}
}
func TestRecoverExpirationStopCompetitionPreservesOriginalEvidence(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, g, _ := claimed(t, s)
		completeRunEvents(t, s, a, g, true)
		expiry := time.Now().UTC()
		s.writer.Model(&buildRecord{}).Where("id = ?", g.Ref.BuildID).Update("lease_expires_at", expiry)
		s.writer.Model(&attemptRecord{}).Where("id = ?", g.Ref.AttemptID).Update("lease_expires_at", expiry)
		if err := s.ExpireLeases(testContext); err != nil {
			t.Fatal(err)
		}
		before := recoveryRows(t, s)
		confirmation := protocol.StopConfirmation{Ref: g.Ref, EvidenceCode: "admin_observed_stopped", Note: "自有fixture已独立确认停止"}
		var wg sync.WaitGroup
		errs := make(chan error, 3)
		for _, fn := range []func() error{func() error { return s.Recover(testContext) }, func() error { return s.ExpireLeases(testContext) }, func() error { return s.ConfirmStopped(testContext, localAdmin, confirmation) }} {
			wg.Add(1)
			go func(fn func() error) { defer wg.Done(); errs <- fn() }(fn)
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		after := recoveryRows(t, s)
		before[0].StopUnconfirmed = false
		if !reflect.DeepEqual(before, after) {
			t.Fatal("停止确认/恢复改写原原因或证据")
		}
	})
}

func TestRecoverPreservesCentralLogArtifactAndNanosecondEvidence(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, g, ids := artifactFixture(t, s)
		for i, id := range ids {
			declaration := protocol.ArtifactDeclaration{Ref: g.Ref, ID: id, Seq: int64(i + 1), Phase: "ordinary", Index: 1, Step: "package", Name: fmt.Sprintf("app-%d.apk", i), Size: 7, SHA256: strings.Repeat("a", 64)}
			if _, err := s.CommitArtifact(testContext, a, ArtifactCommit{Declaration: declaration, StorageID: uuid.NewString()}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := s.CommitLogChunk(testContext, a, LogCommit{Ref: g.Ref, Seq: 1, Offset: 0, Size: 17, Digest: strings.Repeat("b", 64), StorageID: uuid.NewString(), RecordCount: 1}); err != nil {
			t.Fatal(err)
		}
		before := recoveryRows(t, s)
		var oldLogs []logChunkRecord
		var oldFiles []artifactRecord
		var oldSteps []stepRecord
		s.db.Find(&oldLogs)
		s.db.Order("id").Find(&oldFiles)
		s.db.Order("id").Find(&oldSteps)
		if err := s.Recover(testContext); err != nil {
			t.Fatal(err)
		}
		var logs []logChunkRecord
		var files []artifactRecord
		var steps []stepRecord
		s.db.Find(&logs)
		s.db.Order("id").Find(&files)
		s.db.Order("id").Find(&steps)
		if !reflect.DeepEqual(before, recoveryRows(t, s)) || !reflect.DeepEqual(oldLogs, logs) || !reflect.DeepEqual(oldFiles, files) || !reflect.DeepEqual(oldSteps, steps) {
			t.Fatal("恢复修改文件/日志/步骤/NS证据")
		}
	})
}
