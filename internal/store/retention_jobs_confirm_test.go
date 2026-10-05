package store

import (
	"reflect"
	"testing"
)

func TestRetentionJobsConfirmSQLFailurePreservesQuarantine(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		pid, _ := retentionJobFiles(t, s)
		objects, err := s.AdvanceRetention(testContext, pid, 100)
		if err != nil || len(objects) != 2 {
			t.Fatal(err, len(objects))
		}
		object := objects[0]
		identity := "v1:linux:0000000000000001:0000000000000002:1700000000:123456789"
		observed := RetentionObjectObservation{Identity: identity, Size: object.Size, SHA256: object.SHA256}
		if _, err = s.AuthorizeRetentionObject(testContext, object.ID, observed); err != nil {
			t.Fatal(err)
		}
		if err = s.ConfirmRetentionObject(testContext, object.ID, RetentionObjectResult{State: "quarantined", Identity: identity}); err != nil {
			t.Fatal(err)
		}
		var before retentionObjectRecord
		s.db.First(&before, "id = ?", object.ID)
		if before.QuarantinedAt == nil {
			t.Fatal("隔离确认未持久")
		}
		var jobBefore retentionJobRecord
		s.db.First(&jobBefore, "id = ?", object.JobID)
		sql := `CREATE TRIGGER reject_retention_confirm BEFORE UPDATE OF state ON retention_objects BEGIN SELECT RAISE(ABORT,'private fault'); END`
		if opt.Driver == "postgres" {
			if err = s.writer.Exec(`CREATE FUNCTION reject_retention_confirm() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'private fault'; END $$`).Error; err != nil {
				t.Fatal(err)
			}
			sql = `CREATE TRIGGER reject_retention_confirm BEFORE UPDATE OF state ON retention_objects FOR EACH ROW EXECUTE FUNCTION reject_retention_confirm()`
		}
		if err = s.writer.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
		if err = s.ConfirmRetentionObject(testContext, object.ID, RetentionObjectResult{State: "deleted", Identity: identity}); err == nil {
			t.Fatal("真实保存失败假删确认")
		}
		var after retentionObjectRecord
		s.db.First(&after, "id = ?", object.ID)
		var jobAfter retentionJobRecord
		s.db.First(&jobAfter, "id = ?", object.JobID)
		if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(jobBefore, jobAfter) {
			t.Fatal("保存失败丢隔离事实或变job")
		}
		drop := `DROP TRIGGER reject_retention_confirm`
		if opt.Driver == "postgres" {
			drop += ` ON retention_objects`
		}
		if err = s.writer.Exec(drop).Error; err != nil {
			t.Fatal(err)
		}
		if err = s.ConfirmRetentionObject(testContext, object.ID, RetentionObjectResult{State: "deleted", Identity: identity}); err != nil {
			t.Fatal("原事项恢复", err)
		}
		var deleted retentionObjectRecord
		s.db.First(&deleted, "id = ?", object.ID)
		if deleted.ID != before.ID || deleted.Identity != before.Identity || deleted.QuarantineSlot != before.QuarantineSlot || !deleted.QuarantinedAt.Equal(*before.QuarantinedAt) {
			t.Fatal("恢复换身份/隔离首次时间")
		}
	})
}
func TestRetentionJobsAllCentralObjectsStillWaitRealNode(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		pid, _ := retentionJobFiles(t, s)
		objects, err := s.AdvanceRetention(testContext, pid, 100)
		if err != nil || len(objects) != 2 {
			t.Fatal(err, len(objects))
		}
		for _, object := range objects {
			identity := "v1:linux:0000000000000001:0000000000000002:1700000000:123456789"
			if _, err = s.AuthorizeRetentionObject(testContext, object.ID, RetentionObjectObservation{Identity: identity, Size: object.Size, SHA256: object.SHA256}); err != nil {
				t.Fatal(err)
			}
			for _, state := range []string{"quarantined", "deleted"} {
				if err = s.ConfirmRetentionObject(testContext, object.ID, RetentionObjectResult{State: state, Identity: identity}); err != nil {
					t.Fatal(err)
				}
			}
		}
		var job retentionJobRecord
		s.db.First(&job, "id = ?", objects[0].JobID)
		if job.CentralCompletedAt == nil || job.NodeCompletedAt != nil || job.State == "completed" {
			t.Fatal("中央清完假称离线node已完成")
		}
		var pending []nodeDeletionRecord
		s.db.Where("job_id = ?", job.ID).Find(&pending)
		if len(pending) != 1 || pending[0].State != "pending" || pending[0].CompletedAt != nil {
			t.Fatal("丢真实node删除意图")
		}
		page, err := s.ListRetention(testContext, localAdmin, pid, Page{})
		if err != nil {
			t.Fatal(err)
		}
		entry := retentionEntry(t, page, job.BuildID)
		if entry.CentralState != "completed" || entry.NodeState != "pending" || entry.HistoryState != "partial" {
			t.Fatal("中央/节点两阶段视图", entry)
		}
	})
}
