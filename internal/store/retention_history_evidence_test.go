package store

import (
	"reflect"
	"testing"
)

func TestRetentionHistoryPreservesActualObjectFailure(t *testing.T) {
	for _, reason := range []string{"retention_ownership_unknown", "retention_readers_active", "retention_io_error"} {
		t.Run(reason, func(t *testing.T) {
			stores(t, func(t *testing.T, s *Store, _ Options) {
				pid, _ := retentionJobFiles(t, s)
				objects, err := s.AdvanceRetention(testContext, pid, 100)
				if err != nil || len(objects) != 2 {
					t.Fatal(err, len(objects))
				}
				if err = s.RecordRetentionObjectFailure(testContext, objects[0].ID, reason); err != nil {
					t.Fatal(err)
				}
				if _, err = s.FinalizeRetention(testContext, pid, 100); err != nil {
					t.Fatal(err)
				}
				page, err := s.ListRetention(testContext, localAdmin, pid, Page{})
				if err != nil {
					t.Fatal(err)
				}
				entry := retentionEntry(t, page, objects[0].BuildID)
				state, expected := "failed", reason
				if reason == "retention_readers_active" {
					state, expected = "waiting", "readers_active"
				}
				if entry.CentralState != state || entry.Reason != expected || entry.HistoryState == "cleaned" {
					t.Fatal("收敛抹实际失败/读等待", entry.CentralState, entry.Reason)
				}
			})
		})
	}
}
func TestRetentionHistoryMissingCompleteNodeReceiptRetainsBody(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		actor, pid, d := retentionDeletionFixture(t, s)
		retentionHistoryDeleteCentral(t, s, pid)
		authority, err := s.AuthorizeNodeDeletion(testContext, actor, d.ID)
		if err != nil {
			t.Fatal(err)
		}
		for seq, state := range []string{"partial", "deleted"} {
			reason := "partial"
			if state == "deleted" {
				reason = ""
			}
			in := retentionDeletionConfirmation(t, d, authority, int64(seq+1), state, state, reason)
			if err = s.ConfirmNodeDeletion(testContext, actor, in); err != nil {
				t.Fatal(err)
			}
		}
		var before buildRecord
		s.db.First(&before, "id = ?", d.BuildID)
		if err = s.writer.Where("delete_id = ? AND seq = 1", d.ID).Delete(&nodeDeletionReceiptRecord{}).Error; err != nil {
			t.Fatal(err)
		}
		if _, err = s.FinalizeRetention(testContext, pid, 100); err != nil {
			t.Fatal(err)
		}
		var after buildRecord
		s.db.First(&after, "id = ?", d.BuildID)
		if !reflect.DeepEqual(before, after) || after.HistoryState == "cleaned" {
			t.Fatal("完整Node receipt链缺失仍删正文")
		}
	})
}

func TestRetentionHistoryPreservesActualNodePartialReason(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		actor, pid, d := retentionDeletionFixture(t, s)
		retentionHistoryDeleteCentral(t, s, pid)
		authority, err := s.AuthorizeNodeDeletion(testContext, actor, d.ID)
		if err != nil {
			t.Fatal(err)
		}
		in := retentionDeletionConfirmation(t, d, authority, 1, "partial", "partial", "partial")
		if err = s.ConfirmNodeDeletion(testContext, actor, in); err != nil {
			t.Fatal(err)
		}
		if _, err = s.FinalizeRetention(testContext, pid, 100); err != nil {
			t.Fatal(err)
		}
		page, err := s.ListRetention(testContext, localAdmin, pid, Page{})
		if err != nil {
			t.Fatal(err)
		}
		entry := retentionEntry(t, page, d.BuildID)
		if entry.Reason != "partial" || entry.NodeCompletedAt != nil || entry.HistoryState == "cleaned" {
			t.Fatal("中央已完成不能抹节点真实部分结果", entry.Reason)
		}
	})
}
func TestRetentionHistoryAuditSQLFailureRollsBackBody(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		actor, pid, d := retentionDeletionFixture(t, s)
		retentionHistoryDeleteCentral(t, s, pid)
		retentionHistoryDeleteNode(t, s, actor, d)
		var before buildRecord
		s.db.First(&before, "id = ?", d.BuildID)
		var steps []stepRecord
		var files []artifactRecord
		var receipts []executionReceiptRecord
		s.db.Where("build_id = ?", d.BuildID).Order("id").Find(&steps)
		s.db.Where("build_id = ?", d.BuildID).Order("id").Find(&files)
		s.db.Where("build_id = ?", d.BuildID).Order("id").Find(&receipts)
		sql := `CREATE TRIGGER reject_history_audit BEFORE INSERT ON audits WHEN NEW.action = 'retention_completed' BEGIN SELECT RAISE(ABORT,'private fault'); END`
		if opt.Driver == "postgres" {
			if err := s.writer.Exec(`CREATE FUNCTION reject_history_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action = 'retention_completed' THEN RAISE EXCEPTION 'private fault'; END IF; RETURN NEW; END $$`).Error; err != nil {
				t.Fatal(err)
			}
			sql = `CREATE TRIGGER reject_history_audit BEFORE INSERT ON audits FOR EACH ROW EXECUTE FUNCTION reject_history_audit()`
		}
		if err := s.writer.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := s.FinalizeRetention(testContext, pid, 100); err == nil {
			t.Fatal("真实审计SQL失败假清理完成")
		}
		var after buildRecord
		var afterSteps []stepRecord
		var afterFiles []artifactRecord
		var afterReceipts []executionReceiptRecord
		s.db.First(&after, "id = ?", d.BuildID)
		s.db.Where("build_id = ?", d.BuildID).Order("id").Find(&afterSteps)
		s.db.Where("build_id = ?", d.BuildID).Order("id").Find(&afterFiles)
		s.db.Where("build_id = ?", d.BuildID).Order("id").Find(&afterReceipts)
		if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(steps, afterSteps) || !reflect.DeepEqual(files, afterFiles) || !reflect.DeepEqual(receipts, afterReceipts) {
			t.Fatal("SQL失败部分清正文/元数据/receipt")
		}
	})
}
