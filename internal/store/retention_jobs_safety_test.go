package store

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"mybuilds/internal/config"
)

func TestRetentionJobsConcurrentSingleIntent(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		p, _ := retentionJobProject(t, s)
		var wg sync.WaitGroup
		errs := make(chan error, 20)
		for range 20 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := s.ScheduleRetention(testContext, localAdmin, p.ID, 100)
				errs <- err
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		var jobs []retentionJobRecord
		if err := s.db.Find(&jobs).Error; err != nil || len(jobs) != 2 {
			t.Fatal("20并发创建重复或漏事项", err, len(jobs))
		}
		for _, job := range jobs {
			var n int64
			s.db.Model(&retentionJobRecord{}).Where("build_id = ?", job.BuildID).Count(&n)
			if n != 1 || job.RetiredAt == nil {
				t.Fatal("build唯一job")
			}
			if job.NodeCompletedAt != nil {
				t.Fatal("无节点伪确认")
			}
		}
	})
}
func TestRetentionJobsRetryAndActiveProtection(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 1, Days: 30}); err != nil {
			t.Fatal(err)
		}
		p, id := cancelledQueued(t, s)
		child, err := s.Retry(testContext, localAdmin, RetryInput{BuildID: id, Key: "before-retirement"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.Cancel(testContext, localAdmin, child.Builds[0].ID); err != nil {
			t.Fatal(err)
		}
		retentionSkipped(t, s, p, 1)
		page, err := s.ScheduleRetention(testContext, localAdmin, p.ID, 100)
		if err != nil {
			t.Fatal(err)
		}
		e := retentionEntry(t, page, id)
		if !e.Candidate || !slices.Contains(e.ProtectReasons, "retry_dependency") || e.JobID != "" || e.HistoryState != "live" {
			t.Fatal("祖先未保持原保护", e)
		}
		var jobs []retentionJobRecord
		s.db.Find(&jobs)
		if len(jobs) != 1 || jobs[0].BuildID != child.Builds[0].ID {
			t.Fatal("真正子构建先退役", len(jobs))
		}
	})
}
func TestRetentionJobsObjectInsertFailureAtomic(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, o Options) {
		pid, _ := retentionJobFiles(t, s)
		sql := `CREATE TRIGGER reject_retention_object BEFORE INSERT ON retention_objects BEGIN SELECT RAISE(ABORT,'private fault'); END`
		if o.Driver == "postgres" {
			if err := s.writer.Exec(`CREATE FUNCTION reject_retention_object() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'private fault'; END $$`).Error; err != nil {
				t.Fatal(err)
			}
			sql = `CREATE TRIGGER reject_retention_object BEFORE INSERT ON retention_objects FOR EACH ROW EXECUTE FUNCTION reject_retention_object()`
		}
		if err := s.writer.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := s.ScheduleRetention(testContext, localAdmin, pid, 100); err == nil {
			t.Fatal("真实对象SQL失败未传播")
		}
		for _, model := range []any{&retentionJobRecord{}, &retentionObjectRecord{}, &nodeDeletionRecord{}} {
			var n int64
			if err := s.db.Model(model).Count(&n).Error; err != nil || n != 0 {
				t.Fatal("对象失败残留跨表意图", err, n)
			}
		}
		var rows []buildRecord
		s.db.Where("project_id = ?", pid).Find(&rows)
		for _, row := range rows {
			if row.HistoryState != "live" {
				t.Fatal("失败提前退役")
			}
		}
	})
}
func TestRetentionJobsActualPendingReadBeforeRetirement(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		pid, files := retentionJobFiles(t, s)
		owner, err := s.RegisterEvidenceReadOwner(testContext)
		if err != nil {
			t.Fatal(err)
		}
		read, err := s.BeginEvidenceRead(testContext, localAdmin, "artifact", files[0].ID, owner)
		if err != nil {
			t.Fatal(err)
		}
		page, err := s.ScheduleRetention(testContext, localAdmin, pid, 100)
		if err != nil {
			t.Fatal(err)
		}
		entry := retentionEntry(t, page, files[0].BuildID)
		if entry.HistoryState != "retiring" || !slices.Contains(entry.ProtectReasons, "readers_active") {
			t.Fatal("真实pending读仅等待读不阻退役", entry)
		}
		if _, err = s.BeginEvidenceRead(testContext, localAdmin, "artifact", files[0].ID, owner); err != ErrRetentionRetired {
			t.Fatal("退役后新读未拒绝", err)
		}
		objects, err := s.AdvanceRetention(testContext, pid, 100)
		if err != nil || len(objects) != 2 {
			t.Fatal("pending读须返回未授权事项", err, len(objects))
		}
		var target RetentionObject
		for _, object := range objects {
			if object.ObjectID == files[0].ID {
				target = object
			}
		}
		if _, err = s.AuthorizeRetentionObject(testContext, target.ID, RetentionObjectObservation{Identity: "v1:linux:0000000000000001:0000000000000002:1700000000:123456789", Size: target.Size, SHA256: target.SHA256}); err != ErrRetentionReadersActive {
			t.Fatal("当前pending不能被EX观察清空", err)
		}
		// 仅待激活授权的SQL竞争；这里没有SH/EX fd，不冒充文件流验收。
		if err = s.EndEvidenceRead(testContext, read.ID); err != nil {
			t.Fatal(err)
		}
		objects, err = s.AdvanceRetention(testContext, pid, 100)
		if err != nil || len(objects) != 2 {
			t.Fatal("实际pending关闭后原事项继续", err, len(objects))
		}
	})
}
func TestRetentionJobsCancelledWaitingMutex(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		p, _ := retentionJobProject(t, s)
		s.mu.Lock()
		ctx, cancel := context.WithCancel(testContext)
		done := make(chan error, 1)
		go func() { _, err := s.ScheduleRetention(ctx, localAdmin, p.ID, 100); done <- err }()
		cancel()
		select {
		case err := <-done:
			if err != ErrRetentionCancelled {
				s.mu.Unlock()
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			s.mu.Unlock()
			<-done
			t.Fatal("退役等待mutex未响应取消")
		}
		s.mu.Unlock()
		var n int64
		s.db.Model(&retentionJobRecord{}).Count(&n)
		if n != 0 {
			t.Fatal("取消后仍创建意图")
		}
	})
}
