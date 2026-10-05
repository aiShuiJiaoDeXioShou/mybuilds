package store

import (
	"math"
	"reflect"
	"testing"
	"time"

	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
)

func retentionDeletionFixture(t *testing.T, s *Store) (NodeActor, string, protocol.NodeDeletion) {
	t.Helper()
	pid, _ := retentionJobFiles(t, s)
	if _, err := s.ScheduleRetention(testContext, localAdmin, pid, 100); err != nil {
		t.Fatal(err)
	}
	credential, err := s.RotateNodeToken(testContext, localAdmin, "linux")
	if err != nil {
		t.Fatal(err)
	}
	actor, err := s.AuthenticateNode(testContext, credential.Token)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := s.ClaimNodeDeletions(testContext, actor, 10)
	if err != nil || len(claims) != 1 {
		t.Fatal("真实Node清理事项", err, len(claims))
	}
	return actor, pid, claims[0]
}
func retentionDeletionConfirmation(t *testing.T, d protocol.NodeDeletion, a protocol.DeletionAuthority, seq int64, workspace, results, reason string) protocol.NodeDeletionConfirmation {
	t.Helper()
	in := protocol.NodeDeletionConfirmation{ID: d.ID, ResourceID: d.ResourceID, OwnershipDigest: d.OwnershipDigest, Nonce: a.Nonce, Seq: seq, WorkspaceState: workspace, ResultsState: results, Reason: reason}
	var err error
	in.Digest, err = protocol.NodeDeletionDigest(in)
	if err != nil {
		t.Fatal(err)
	}
	return in
}
func TestRetentionNodeDeletionFullReceiptRotationAndReplay(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		actor, _, d := retentionDeletionFixture(t, s)
		before := time.Now().UTC()
		authority, err := s.AuthorizeNodeDeletion(testContext, actor, d.ID)
		if err != nil {
			t.Fatal(err)
		}
		if authority.ID != d.ID || authority.NodeID != actor.ID || authority.ResourceID != d.ResourceID || authority.OwnershipDigest != d.OwnershipDigest || !validUUID(authority.Nonce) || !authority.ExpiresAt.After(before) || authority.ExpiresAt.After(time.Now().UTC().Add(5*time.Second)) {
			t.Fatal("独立5s管理权")
		}
		partial := retentionDeletionConfirmation(t, d, authority, 1, "partial", "partial", "partial")
		if err = s.ConfirmNodeDeletion(testContext, actor, partial); err != nil {
			t.Fatal(err)
		}
		var receipt nodeDeletionReceiptRecord
		s.db.First(&receipt, "delete_id = ? AND seq = 1", d.ID)
		if receipt.ResourceID != partial.ResourceID || receipt.OwnershipDigest != partial.OwnershipDigest || receipt.Nonce != partial.Nonce || receipt.Digest != partial.Digest || receipt.WorkspaceState != partial.WorkspaceState || receipt.ResultsState != partial.ResultsState || receipt.Reason != partial.Reason {
			t.Fatal("回执只存摘要不完整")
		}
		changed := partial
		changed.WorkspaceState = "deleted"
		changed.Digest, _ = protocol.NodeDeletionDigest(changed)
		if err = s.ConfirmNodeDeletion(testContext, actor, changed); err != ErrRetentionReceiptConflict {
			t.Fatal("同Seq不同内容必须冲突", err)
		}
		credential, err := s.RotateNodeToken(testContext, localAdmin, "linux")
		if err != nil {
			t.Fatal(err)
		}
		current, err := s.AuthenticateNode(testContext, credential.Token)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.ClaimNodeDeletions(testContext, actor, 10); err != ErrNodeUnauthorized {
			t.Fatal("撤销旧credential", err)
		}
		newer, err := s.AuthorizeNodeDeletion(testContext, current, d.ID)
		if err != nil || newer.Nonce == authority.Nonce {
			t.Fatal("新管理段独立nonce", err)
		}
		for range 20 {
			if err = s.ConfirmNodeDeletion(testContext, current, partial); err != nil {
				t.Fatal("旧Seq先精确receipt，不依新nonce", err)
			}
		}
		var same nodeDeletionReceiptRecord
		s.db.First(&same, "delete_id = ? AND seq = 1", d.ID)
		if !reflect.DeepEqual(receipt, same) {
			t.Fatal("重放刷新原receipt")
		}
		complete := retentionDeletionConfirmation(t, d, newer, 2, "deleted", "deleted", "")
		if err = s.ConfirmNodeDeletion(testContext, current, complete); err != nil {
			t.Fatal(err)
		}
		var deletion nodeDeletionRecord
		s.db.First(&deletion, "id = ?", d.ID)
		if deletion.Seq != 2 || deletion.CompletedAt == nil || deletion.State != "completed" {
			t.Fatal("真实两槽完成")
		}
		var job retentionJobRecord
		s.db.First(&job, "id = ?", deletion.JobID)
		if job.NodeCompletedAt == nil || job.CentralCompletedAt != nil || job.State == "completed" {
			t.Fatal("仅Node完成不能整体完成")
		}
	})
}
func TestRetentionNodeDeletionIdentityPolicyAndSequence(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		actor, _, d := retentionDeletionFixture(t, s)
		other, _ := openSession(t, s, "foreign-delete-node", 1)
		if _, err := s.AuthorizeNodeDeletion(testContext, other, d.ID); err != ErrNodeUnauthorized {
			t.Fatal("别Node不能授权", err)
		}
		authority, err := s.AuthorizeNodeDeletion(testContext, actor, d.ID)
		if err != nil {
			t.Fatal(err)
		}
		valid := retentionDeletionConfirmation(t, d, authority, 1, "deleted", "deleted", "")
		for _, bad := range []protocol.NodeDeletionConfirmation{func() protocol.NodeDeletionConfirmation {
			x := valid
			x.Seq = 0
			x.Digest, _ = protocol.NodeDeletionDigest(x)
			return x
		}(), func() protocol.NodeDeletionConfirmation {
			x := valid
			x.Seq = 2
			x.Digest, _ = protocol.NodeDeletionDigest(x)
			return x
		}(), func() protocol.NodeDeletionConfirmation {
			x := valid
			x.ResourceID = x.ID
			x.Digest, _ = protocol.NodeDeletionDigest(x)
			return x
		}(), func() protocol.NodeDeletionConfirmation {
			x := valid
			x.Nonce = x.ID
			x.Digest, _ = protocol.NodeDeletionDigest(x)
			return x
		}(), func() protocol.NodeDeletionConfirmation {
			x := valid
			x.WorkspaceState = "not_applicable"
			x.Digest, _ = protocol.NodeDeletionDigest(x)
			return x
		}(), func() protocol.NodeDeletionConfirmation {
			x := valid
			x.Reason = "private error"
			x.Digest, _ = protocol.NodeDeletionDigest(x)
			return x
		}(), func() protocol.NodeDeletionConfirmation { x := valid; x.Digest = x.OwnershipDigest; return x }()} {
			if err = s.ConfirmNodeDeletion(testContext, actor, bad); err == nil {
				t.Fatal("不完整/跳序/错误nonce/slot/摘要确认")
			}
		}
		if err = s.SyncGlobalRetention(testContext, config.Retention{Builds: 100, Days: 30}); err != nil {
			t.Fatal(err)
		}
		if _, err = s.AuthorizeNodeDeletion(testContext, actor, d.ID); err != ErrRetentionProtected {
			t.Fatal("旧资格不可授新权", err)
		}
		if err = s.ConfirmNodeDeletion(testContext, actor, valid); err != nil {
			t.Fatal("保存已有实际删除事实，不重新授权", err)
		}
		var row nodeDeletionRecord
		s.db.First(&row, "id = ?", d.ID)
		var job retentionJobRecord
		s.db.First(&job, "id = ?", row.JobID)
		if job.NodeCompletedAt != nil || job.State == "completed" {
			t.Fatal("新保护下确认不得整体完成")
		}
		// 损坏已持久游标的溢出防御，不伪造未来业务状态或完成事实。
		if err = s.writer.Model(&nodeDeletionRecord{}).Where("id = ?", d.ID).Update("seq", int64(math.MaxInt64)).Error; err != nil {
			t.Fatal(err)
		}
		valid.Seq = math.MaxInt64
		valid.Digest, _ = protocol.NodeDeletionDigest(valid)
		if err = s.ConfirmNodeDeletion(testContext, actor, valid); err == nil {
			t.Fatal("溢出游标允许新增确认")
		}
	})
}

func TestRetentionNodeDeletionExpiredAuthorityConfirmsOnlyOldFacts(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		actor, _, d := retentionDeletionFixture(t, s)
		authority, err := s.AuthorizeNodeDeletion(testContext, actor, d.ID)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Until(authority.ExpiresAt) + 10*time.Millisecond)
		if time.Now().UTC().Before(authority.ExpiresAt) {
			t.Fatal("夹具未实际过期")
		}
		old := retentionDeletionConfirmation(t, d, authority, 1, "partial", "partial", "partial")
		if err = s.ConfirmNodeDeletion(testContext, actor, old); err != nil {
			t.Fatal("原事实补ACK不能当新授权", err)
		}
		renewed, err := s.AuthorizeNodeDeletion(testContext, actor, d.ID)
		if err != nil || renewed.Nonce == authority.Nonce || !renewed.ExpiresAt.After(time.Now().UTC()) {
			t.Fatal("下段必须取得新的真实管理权", err)
		}
		next := retentionDeletionConfirmation(t, d, authority, 2, "deleted", "deleted", "")
		if err = s.ConfirmNodeDeletion(testContext, actor, next); err != ErrRetentionReceiptConflict {
			t.Fatal("旧nonce不能冒充新段事实", err)
		}
		if err = s.ConfirmNodeDeletion(testContext, actor, old); err != nil {
			t.Fatal("已保存旧Seq原nonce重放", err)
		}
	})
}
func TestRetentionNodeDeletionReceiptSQLFailureAtomic(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		actor, _, d := retentionDeletionFixture(t, s)
		authority, err := s.AuthorizeNodeDeletion(testContext, actor, d.ID)
		if err != nil {
			t.Fatal(err)
		}
		in := retentionDeletionConfirmation(t, d, authority, 1, "partial", "partial", "partial")
		var before nodeDeletionRecord
		s.db.First(&before, "id = ?", d.ID)
		sql := `CREATE TRIGGER reject_node_delete_receipt BEFORE INSERT ON node_deletion_receipts BEGIN SELECT RAISE(ABORT,'private fault'); END`
		if opt.Driver == "postgres" {
			if err = s.writer.Exec(`CREATE FUNCTION reject_node_delete_receipt() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'private fault'; END $$`).Error; err != nil {
				t.Fatal(err)
			}
			sql = `CREATE TRIGGER reject_node_delete_receipt BEFORE INSERT ON node_deletion_receipts FOR EACH ROW EXECUTE FUNCTION reject_node_delete_receipt()`
		}
		if err = s.writer.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
		if err = s.ConfirmNodeDeletion(testContext, actor, in); err == nil {
			t.Fatal("回执保存失败假ACK")
		}
		var after nodeDeletionRecord
		s.db.First(&after, "id = ?", d.ID)
		if !reflect.DeepEqual(before, after) {
			t.Fatal("SQL失败改变seq/事实")
		}
		var count int64
		s.db.Model(&nodeDeletionReceiptRecord{}).Where("delete_id = ?", d.ID).Count(&count)
		if count != 0 {
			t.Fatal("失败残留回执")
		}
		drop := `DROP TRIGGER reject_node_delete_receipt`
		if opt.Driver == "postgres" {
			drop += ` ON node_deletion_receipts`
		}
		if err = s.writer.Exec(drop).Error; err != nil {
			t.Fatal(err)
		}
		if err = s.ConfirmNodeDeletion(testContext, actor, in); err != nil {
			t.Fatal("恢复原Seq事实", err)
		}
	})
}
func TestRetentionNodeDeletionNoResultsSlot(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 1, Days: 30}); err != nil {
			t.Fatal(err)
		}
		a, g, _ := claimed(t, s)
		reg := resourceRegistration(g.Ref)
		if err := s.RegisterNodeResource(testContext, a, reg); err != nil {
			t.Fatal(err)
		}
		accept(t, s, a, event(g.Ref, 1, protocol.ExecutionProgress{Kind: "build_finished", Status: "failed", Reason: "precheck_error", PostPhase: "none", StopConfirmed: true, RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}))
		pid := retentionProjectForBuild(t, s, g.Ref.BuildID)
		var row projectRecord
		s.db.First(&row, "id = ?", pid)
		p, err := projectView(row)
		if err != nil {
			t.Fatal(err)
		}
		retentionSkipped(t, s, p, 1)
		if _, err = s.ScheduleRetention(testContext, localAdmin, pid, 100); err != nil {
			t.Fatal(err)
		}
		claims, err := s.ClaimNodeDeletions(testContext, a, 10)
		if err != nil || len(claims) != 1 || claims[0].HasResults {
			t.Fatal("真实零动作无结果槽", err, len(claims))
		}
		authority, err := s.AuthorizeNodeDeletion(testContext, a, claims[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		in := retentionDeletionConfirmation(t, claims[0], authority, 1, "deleted", "not_applicable", "")
		if err = s.ConfirmNodeDeletion(testContext, a, in); err != nil {
			t.Fatal(err)
		}
		var deletion nodeDeletionRecord
		s.db.First(&deletion, "id = ?", in.ID)
		if deletion.State != "completed" || deletion.ResultsState != "not_applicable" {
			t.Fatal("未创建结果槽不可假deleted")
		}
	})
}
