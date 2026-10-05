package store

import (
	"testing"

	"mybuilds/internal/config"
)

func TestRetentionJobsOldReadOwnerExactObservation(t *testing.T) {
	for _, active := range []bool{false, true} {
		name := "pending"
		if active {
			name = "active"
		}
		t.Run(name, func(t *testing.T) {
			stores(t, func(t *testing.T, s *Store, opt Options) {
				pid, files := retentionJobFiles(t, s)
				owner, err := s.RegisterEvidenceReadOwner(testContext)
				if err != nil {
					t.Fatal(err)
				}
				read, err := s.BeginEvidenceRead(testContext, localAdmin, "artifact", files[0].ID, owner)
				if err != nil {
					t.Fatal(err)
				}
				identity := "v1:linux:0000000000000001:0000000000000002:1700000000:123456789"
				if active {
					if err = s.ActivateEvidenceRead(testContext, read.ID, identity); err != nil {
						t.Fatal(err)
					}
				}
				if _, err = s.ScheduleRetention(testContext, localAdmin, pid, 100); err != nil {
					t.Fatal(err)
				}
				objects, err := s.AdvanceRetention(testContext, pid, 100)
				if err != nil || len(objects) != 2 {
					t.Fatal("未授权的待EX事项仍需私有返回", err, len(objects))
				}
				var object RetentionObject
				for _, o := range objects {
					if o.ObjectID == files[0].ID {
						object = o
					}
				}
				observed := RetentionObjectObservation{Identity: identity, Size: object.Size, SHA256: object.SHA256}
				if _, err = s.AuthorizeRetentionObject(testContext, object.ID, observed); err != ErrRetentionReadersActive {
					t.Fatal("同控制owner不能清读者", err)
				}
				if err = s.Close(); err != nil {
					t.Fatal(err)
				}
				next, err := Open(testContext, opt)
				if err != nil {
					t.Fatal(err)
				}
				defer next.Close()
				if _, err = next.AuthorizeRetentionObject(testContext, object.ID, observed); err != ErrConflict {
					t.Fatal("未绑定新owner不得修复", err)
				}
				current, err := next.RegisterEvidenceReadOwner(testContext)
				if err != nil || current == owner {
					t.Fatal("真实独占更替", err)
				}
				if active {
					replacement := observed
					replacement.Identity = "v1:linux:0000000000000001:0000000000000003:1700000000:123456789"
					if _, err = next.AuthorizeRetentionObject(testContext, object.ID, replacement); err != ErrRetentionReadersActive {
						t.Fatal("替换叶EX不能证明旧active结束", err)
					}
				}
				var before evidenceReadRecord
				next.db.First(&before, "id = ?", read.ID)
				if before.State == "closed" {
					t.Fatal("失败授权提前清读者")
				}
				if _, err = next.AuthorizeRetentionObject(testContext, object.ID, observed); err != nil {
					t.Fatal("旧owner同身份修复", err)
				}
				var closed evidenceReadRecord
				next.db.First(&closed, "id = ?", read.ID)
				if closed.State != "closed" || closed.ClosedAt == nil || closed.Owner != owner {
					t.Fatal("原读取固定关闭")
				}
				// 这是Store实际API约束；同fd实际EX与替换叶的内核门由Server消费者验收。
			})
		})
	}
}
func TestRetentionJobsReadRepairPolicyFailureAtomic(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		pid, files := retentionJobFiles(t, s)
		owner, err := s.RegisterEvidenceReadOwner(testContext)
		if err != nil {
			t.Fatal(err)
		}
		read, err := s.BeginEvidenceRead(testContext, localAdmin, "artifact", files[0].ID, owner)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.ScheduleRetention(testContext, localAdmin, pid, 100); err != nil {
			t.Fatal(err)
		}
		var object retentionObjectRecord
		s.db.First(&object, "object_id = ?", files[0].ID)
		if err = s.Close(); err != nil {
			t.Fatal(err)
		}
		next, err := Open(testContext, opt)
		if err != nil {
			t.Fatal(err)
		}
		defer next.Close()
		if _, err = next.RegisterEvidenceReadOwner(testContext); err != nil {
			t.Fatal(err)
		}
		if err = next.SyncGlobalRetention(testContext, config.Retention{Builds: 100, Days: 30}); err != nil {
			t.Fatal(err)
		}
		_, err = next.AuthorizeRetentionObject(testContext, object.ID, RetentionObjectObservation{Identity: "v1:linux:0000000000000001:0000000000000002:1700000000:123456789", Size: object.Size, SHA256: object.SHA256})
		if err != ErrRetentionProtected {
			t.Fatal("新策略未最后核", err)
		}
		var unchanged evidenceReadRecord
		next.db.First(&unchanged, "id = ?", read.ID)
		if unchanged.State != "pending" || unchanged.ClosedAt != nil {
			t.Fatal("后续保护失败必须回滚读取修复")
		}
	})
}
