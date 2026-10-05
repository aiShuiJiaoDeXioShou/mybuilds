package store

import (
	"reflect"
	"testing"
)

func TestRetentionObjectInitialFailureNeverGrantsIdentity(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		pid, _ := retentionJobFiles(t, s)
		objects, err := s.AdvanceRetention(testContext, pid, 100)
		if err != nil || len(objects) != 2 {
			t.Fatal(err, len(objects))
		}
		o := objects[0]
		for _, reason := range []string{"", "private os error", "permission_denied"} {
			if err = s.RecordRetentionObjectFailure(testContext, o.ID, reason); err != ErrRetentionObjectInvalid {
				t.Fatal("只能实际固定3安全码", err)
			}
		}
		for range 2 {
			if err = s.RecordRetentionObjectFailure(testContext, o.ID, "retention_ownership_unknown"); err != nil {
				t.Fatal(err)
			}
		}
		var failed retentionObjectRecord
		s.db.First(&failed, "id = ?", o.ID)
		if failed.State != "failed" || failed.Reason != "retention_ownership_unknown" || failed.Identity != "" || failed.QuarantineSlot != "" || failed.QuarantinedAt != nil || failed.ConfirmedAt != nil {
			t.Fatal("unknown失败赋予删除身份/确认")
		}
		var job retentionJobRecord
		s.db.First(&job, "id = ?", o.JobID)
		if job.State != "failed" || job.CentralCompletedAt != nil || job.NodeCompletedAt != nil {
			t.Fatal("未知归属假整体完成")
		}
		if err = s.RecordRetentionObjectFailure(testContext, o.ID, "retention_ownership_unknown"); err != nil {
			t.Fatal(err)
		}
		var same retentionObjectRecord
		s.db.First(&same, "id = ?", o.ID)
		if !reflect.DeepEqual(failed, same) {
			t.Fatal("同失败重放刷新原事实")
		}
		if err = s.RecordRetentionObjectFailure(testContext, o.ID, "retention_readers_active"); err != nil {
			t.Fatal(err)
		}
		s.db.First(&same, "id = ?", o.ID)
		s.db.First(&job, "id = ?", o.JobID)
		if same.State != "retired" || job.State != "waiting" || job.Reason != "readers_active" || same.ConfirmedAt != nil {
			t.Fatal("读等待不能永久failed或delete确认")
		}
		identity := "v1:linux:0000000000000001:0000000000000002:1700000000:123456789"
		authorized, err := s.AuthorizeRetentionObject(testContext, o.ID, RetentionObjectObservation{Identity: identity, Size: o.Size, SHA256: o.SHA256})
		if err != nil || authorized.Reason != "" {
			t.Fatal("原事项正确观察后可继续", err)
		}
		if err = s.RecordRetentionObjectFailure(testContext, o.ID, "retention_io_error"); err != ErrRetentionObjectInvalid {
			t.Fatal("已知身份必须用原Confirm保阶段", err)
		}
		var known retentionObjectRecord
		s.db.First(&known, "id = ?", o.ID)
		if known.Identity != authorized.Identity || known.QuarantineSlot != authorized.QuarantineSlot {
			t.Fatal("unknown入口覆盖已知身份")
		}
	})
}
