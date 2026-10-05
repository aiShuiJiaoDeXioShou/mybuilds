package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func retentionReadLog(t *testing.T, s *Store) logChunkRecord {
	t.Helper()
	a, grant, _ := claimed(t, s)
	data := []byte("真实只读日志夹具\n")
	digest := sha256.Sum256(data)
	storageID := uuid.NewString()
	if err := os.WriteFile(filepath.Join(t.TempDir(), storageID), data, 0600); err != nil {
		t.Fatal(err)
	}
	_, err := s.CommitLogChunk(testContext, a, LogCommit{Ref: grant.Ref, Seq: 1, Size: int64(len(data)), Digest: hex.EncodeToString(digest[:]), StorageID: storageID, RecordCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	var row logChunkRecord
	if err = s.db.First(&row, "build_id = ?", grant.Ref.BuildID).Error; err != nil {
		t.Fatal(err)
	}
	return row
}

func TestRetentionEvidenceReadHundredRetirementRaces(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		projectID, files := retentionJobFiles(t, s)
		owner, err := s.RegisterEvidenceReadOwner(testContext)
		if err != nil {
			t.Fatal(err)
		}
		identity := "v1:linux:0000000000000001:0000000000000002:1:000000000"
		active, err := s.BeginEvidenceRead(testContext, localAdmin, "artifact", files[0].ID, owner)
		if err != nil || s.ActivateEvidenceRead(testContext, active.ID, identity) != nil {
			t.Fatal("原active登记", err)
		}
		start := make(chan struct{})
		failures := make(chan error, 300)
		var wg sync.WaitGroup
		for range 100 {
			wg.Add(2)
			go func() {
				defer wg.Done()
				<-start
				read, err := s.BeginEvidenceRead(testContext, localAdmin, "artifact", files[0].ID, owner)
				if err == ErrRetentionRetired {
					return
				}
				if err != nil {
					failures <- err
					return
				}
				if err = s.ActivateEvidenceRead(testContext, read.ID, identity); err != nil && err != ErrRetentionRetired {
					failures <- err
				}
				if err = s.EndEvidenceRead(testContext, read.ID); err != nil {
					failures <- err
				}
			}()
			go func() {
				defer wg.Done()
				<-start
				_, err := s.ScheduleRetention(testContext, localAdmin, projectID, 100)
				if err != nil {
					failures <- err
				}
			}()
		}
		close(start)
		wg.Wait()
		close(failures)
		for err := range failures {
			t.Fatal("实际Begin/退役/Activate竞争异常", err)
		}
		var old evidenceReadRecord
		if err = s.db.First(&old, "id = ?", active.ID).Error; err != nil || old.State != "active" {
			t.Fatal("原active不得因退役假关闭", err)
		}
		if _, err = s.BeginEvidenceRead(testContext, localAdmin, "artifact", files[0].ID, owner); err != ErrRetentionRetired {
			t.Fatal("新Begin拒绝退役", err)
		}
		if err = s.EndEvidenceRead(testContext, active.ID); err != nil {
			t.Fatal("原active可以完成关闭", err)
		}
		var count int64
		if err = s.db.Model(&evidenceReadRecord{}).Where("state <> 'closed'").Count(&count).Error; err != nil || count != 0 {
			t.Fatal("所有真实本轮登记有限结束", count, err)
		}
		objects, err := s.AdvanceRetention(testContext, projectID, 100)
		if err != nil || len(objects) != 2 {
			t.Fatal("关闭后仍保原两登记对象，不假删除", len(objects), err)
		}
	})
}

func TestRetentionEvidenceReadEndSQLFailureKeepsActive(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		row := retentionReadLog(t, s)
		owner, err := s.RegisterEvidenceReadOwner(testContext)
		if err != nil {
			t.Fatal(err)
		}
		read, err := s.BeginEvidenceRead(testContext, localAdmin, "log", row.ID, owner)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.ActivateEvidenceRead(testContext, read.ID, "v1:linux:0000000000000001:0000000000000002:1:000000000"); err != nil {
			t.Fatal(err)
		}
		var before evidenceReadRecord
		if err = s.db.First(&before, "id = ?", read.ID).Error; err != nil {
			t.Fatal(err)
		}
		query := `CREATE TRIGGER reject_retention_read_end BEFORE UPDATE OF state ON evidence_reads BEGIN SELECT RAISE(ABORT,'private fixture fault'); END`
		if opt.Driver == "postgres" {
			if err = s.writer.Exec(`CREATE FUNCTION reject_retention_read_end() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'private fixture fault'; END $$`).Error; err != nil {
				t.Fatal(err)
			}
			query = `CREATE TRIGGER reject_retention_read_end BEFORE UPDATE ON evidence_reads FOR EACH ROW EXECUTE FUNCTION reject_retention_read_end()`
		}
		if err = s.writer.Exec(query).Error; err != nil {
			t.Fatal(err)
		}
		expected := ErrConflict
		if opt.Driver == "postgres" {
			expected = errDatabase
		}
		if err = s.EndEvidenceRead(testContext, read.ID); err != expected {
			t.Fatal("真实End SQL失败", err)
		}
		var after evidenceReadRecord
		if err = s.db.First(&after, "id = ?", read.ID).Error; err != nil || !reflect.DeepEqual(before, after) {
			t.Fatal("End失败不能假关闭", err)
		}
		query = `DROP TRIGGER reject_retention_read_end`
		if opt.Driver == "postgres" {
			query += ` ON evidence_reads`
		}
		if err = s.writer.Exec(query).Error; err != nil {
			t.Fatal(err)
		}
		if opt.Driver == "postgres" {
			if err = s.writer.Exec(`DROP FUNCTION reject_retention_read_end()`).Error; err != nil {
				t.Fatal(err)
			}
		}
		if err = s.EndEvidenceRead(testContext, read.ID); err != nil {
			t.Fatal("同原read恢复确认", err)
		}
	})
}

func TestRetentionEvidenceReadLifecycleAndRoles(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		row := retentionReadLog(t, s)
		chunks, err := s.ListLogChunks(testContext, localAdmin, row.BuildID, 0, Page{})
		if err != nil || len(chunks) != 1 || chunks[0].ID != row.ID || chunks[0].ID == row.StorageID {
			t.Fatal("实际log业务身份不同于StorageID", chunks, err)
		}
		owner, err := s.RegisterEvidenceReadOwner(testContext)
		if err != nil || !validUUID(owner) {
			t.Fatal("启动实例真实owner", owner, err)
		}
		if again, err := s.RegisterEvidenceReadOwner(testContext); err != nil || again != owner {
			t.Fatal("同实例owner不能刷新", again, err)
		}
		read, err := s.BeginEvidenceRead(testContext, localAdmin, "log", row.ID, owner)
		if err != nil || read.BuildID != row.BuildID || read.ObjectID != row.ID || read.StorageID != row.StorageID || read.Size != row.Size || read.SHA256 != row.Digest || !validUUID(read.ID) {
			t.Fatal("只读登记保真实业务ID与文件元数据", read, err)
		}
		var pending evidenceReadRecord
		if err = s.db.First(&pending, "id = ?", read.ID).Error; err != nil || pending.State != "pending" || pending.Identity != "" {
			t.Fatal("实际fd锁前不得active", err)
		}
		for _, identity := range []string{"", "fake", "v1:windows:0000000000000001:0000000000000002:1:000000000", "v1:linux:0000000000000001:0000000000000002:01:000000000"} {
			if s.ActivateEvidenceRead(testContext, read.ID, identity) != ErrInvalid {
				t.Fatal("非法固定身份拒绝", identity)
			}
		}
		identity := "v1:linux:0000000000000001:0000000000000002:1:000000000"
		if err = s.ActivateEvidenceRead(testContext, read.ID, identity); err != nil {
			t.Fatal(err)
		}
		var active evidenceReadRecord
		if err = s.db.First(&active, "id = ?", read.ID).Error; err != nil || active.State != "active" || active.Identity != identity {
			t.Fatal("激活持久固定观察", err)
		}
		if s.ActivateEvidenceRead(testContext, read.ID, identity) != nil || s.ActivateEvidenceRead(testContext, read.ID, "v1:linux:0000000000000001:0000000000000003:1:000000000") != ErrConflict {
			t.Fatal("激活重复幂等但不得替换fd身份")
		}
		if err = s.EndEvidenceRead(testContext, read.ID); err != nil {
			t.Fatal(err)
		}
		var closed, again evidenceReadRecord
		if err = s.db.First(&closed, "id = ?", read.ID).Error; err != nil || closed.State != "closed" || closed.ClosedAt == nil {
			t.Fatal("关闭后固定确认", err)
		}
		if err = s.EndEvidenceRead(testContext, read.ID); err != nil {
			t.Fatal(err)
		}
		if err = s.db.First(&again, "id = ?", read.ID).Error; err != nil || !reflect.DeepEqual(closed, again) {
			t.Fatal("重放不能刷新关闭", err)
		}
		if s.ActivateEvidenceRead(testContext, read.ID, identity) != ErrConflict {
			t.Fatal("closed不得重新激活")
		}
		for _, role := range []string{"trigger", "approver"} {
			token, err := s.CreateToken(testContext, localAdmin, role)
			if err != nil {
				t.Fatal(err)
			}
			actor, err := s.Authenticate(testContext, token.Token)
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.BeginEvidenceRead(testContext, actor, "log", row.ID, owner)
			if role == "trigger" && err != ErrForbidden || role == "approver" && err != nil {
				t.Fatal("原证据读取权限", role, err)
			}
		}
		if _, err = s.BeginEvidenceRead(testContext, localAdmin, "log", row.ID, uuid.NewString()); err != ErrConflict {
			t.Fatal("陌生owner拒绝", err)
		}
		if _, err = s.BeginEvidenceRead(testContext, localAdmin, "log", row.StorageID, owner); err != ErrNotFound {
			t.Fatal("storage不能代替业务ID", err)
		}
	})
}

func TestRetentionEvidenceReadNewStoreReplacesOwnerPreservesActive(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		row := retentionReadLog(t, s)
		owner, err := s.RegisterEvidenceReadOwner(testContext)
		if err != nil {
			t.Fatal(err)
		}
		read, err := s.BeginEvidenceRead(testContext, localAdmin, "log", row.ID, owner)
		if err != nil {
			t.Fatal(err)
		}
		identity := "v1:darwin:0000000000000001:0000000000000002:1:000000000"
		if err = s.ActivateEvidenceRead(testContext, read.ID, identity); err != nil {
			t.Fatal(err)
		}
		if err = s.Close(); err != nil {
			t.Fatal(err)
		}
		next, err := Open(testContext, opt)
		if err != nil {
			t.Fatal(err)
		}
		defer next.Close()
		current, err := next.RegisterEvidenceReadOwner(testContext)
		if err != nil || current == owner {
			t.Fatal("真正新控制端owner更新", err)
		}
		if s.ActivateEvidenceRead(testContext, read.ID, identity) != ErrLockLost {
			t.Fatal("旧Store不能激活")
		}
		if next.ActivateEvidenceRead(testContext, read.ID, identity) != ErrConflict || next.EndEvidenceRead(testContext, read.ID) != ErrConflict {
			t.Fatal("新实例不能假装旧fd已关")
		}
		var old evidenceReadRecord
		if err = next.db.First(&old, "id = ?", read.ID).Error; err != nil || old.State != "active" || old.Owner != owner || old.Identity != identity {
			t.Fatal("旧active保护保留", err)
		}
	})
}

func TestRetentionEvidenceReadMutexCancel(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		ctx, cancel := context.WithCancel(testContext)
		s.mu.Lock()
		done := make(chan error, 1)
		go func() { _, err := s.RegisterEvidenceReadOwner(ctx); done <- err }()
		cancel()
		select {
		case err := <-done:
			s.mu.Unlock()
			if err != ErrRetentionCancelled {
				t.Fatal(err)
			}
		case <-time.After(300 * time.Millisecond):
			s.mu.Unlock()
			<-done
			t.Fatal("读取登记等待不得越界")
		}
	})
}
