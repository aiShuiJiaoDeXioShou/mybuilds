package store

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
)

func retentionJobProject(t *testing.T, s *Store) (Project, []string) {
	t.Helper()
	if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 1, Days: 30}); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(testContext, localAdmin, projectInput("job-project"))
	if err != nil {
		t.Fatal(err)
	}
	ids := retentionSkipped(t, s, p, 3)
	slices.Sort(ids)
	return p, ids
}
func TestRetentionJobsRetireStableIdentityAndScope(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		p, ids := retentionJobProject(t, s)
		out, err := s.ScheduleRetention(testContext, localAdmin, p.ID, 1)
		if err != nil {
			t.Fatal(err)
		}
		var jobs []retentionJobRecord
		s.db.Find(&jobs)
		if len(jobs) != 1 || jobs[0].RetiredAt == nil {
			t.Fatal("单轮上限/真实退役意图", len(jobs))
		}
		id := jobs[0].BuildID
		old := jobs[0]
		e := retentionEntry(t, out, id)
		if e.HistoryState != "retiring" || e.NodeState != "not_applicable" {
			t.Fatal("无节点不伪确认", e)
		}
		if _, err = s.ScheduleRetention(testContext, localAdmin, p.ID, 100); err != nil {
			t.Fatal(err)
		}
		var same retentionJobRecord
		s.db.First(&same, "id = ?", old.ID)
		if !same.RetiredAt.Equal(*old.RetiredAt) || same.ID != old.ID {
			t.Fatal("重放不得换事项/首次时间")
		}
		list, err := s.ListRetention(testContext, localAdmin, p.ID, Page{Limit: 200})
		if err != nil || len(list.Items) != 2 {
			t.Fatal("实际事项页", err, len(list.Items))
		}
		var row buildRecord
		s.db.First(&row, "id = ?", ids[2])
		if row.HistoryState != "live" {
			t.Fatal("保留配额被退役")
		}
		data, _ := json.Marshal(list)
		if strings.Contains(string(data), "secret-") {
			t.Fatal("事项公开泄密")
		}
		_, err = s.ScheduleRetention(testContext, Actor{}, p.ID, 1)
		if err == nil {
			t.Fatal("未鉴权允许退役")
		}
		ctx, cancel := context.WithCancel(testContext)
		cancel()
		if _, err = s.AdvanceRetention(ctx, p.ID, 1); err != ErrRetentionCancelled {
			t.Fatal("取消入口", err)
		}
	})
}
func retentionJobFiles(t *testing.T, s *Store) (string, []artifactRecord) {
	t.Helper()
	if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 1, Days: 30}); err != nil {
		t.Fatal(err)
	}
	a, g, ids := artifactFixture(t, s)
	reg := resourceRegistration(g.Ref)
	reg.HasResults = true
	if err := s.RegisterNodeResource(testContext, a, reg); err != nil {
		t.Fatal(err)
	}
	for i, id := range ids {
		if _, err := s.CommitArtifact(testContext, a, ArtifactCommit{Declaration: protocol.ArtifactDeclaration{Ref: g.Ref, ID: id, Seq: int64(i + 1), Phase: "ordinary", Step: "package", Index: 1, Name: "app.apk", Size: 7, SHA256: strings.Repeat("a", 64)}, StorageID: uuid.NewString()}); err != nil {
			t.Fatal(err)
		}
	}
	accept(t, s, a, event(g.Ref, 6, protocol.ExecutionProgress{Kind: "build_finished", Status: "succeeded", Started: true, StopConfirmed: true, RemainingPostBudgetNS: int64(2 * time.Minute), LastArtifactSeq: 2, ArtifactSteps: []protocol.ArtifactExpectation{{Phase: "ordinary", Index: 1, Count: 2, IDs: ids}}}))
	pid := retentionProjectForBuild(t, s, g.Ref.BuildID)
	var p projectRecord
	s.db.First(&p, "id = ?", pid)
	view, err := projectView(p)
	if err != nil {
		t.Fatal(err)
	}
	retentionSkipped(t, s, view, 1)
	var files []artifactRecord
	s.db.Where("build_id = ?", g.Ref.BuildID).Order("seq").Find(&files)
	return pid, files
}
func TestRetentionJobsObservedIdentityAndMonotoneConfirm(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		pid, files := retentionJobFiles(t, s)
		objects, err := s.AdvanceRetention(testContext, pid, 100)
		if err != nil || len(objects) != 2 {
			t.Fatal("只列真实登记文件", err, len(objects))
		}
		o := objects[0]
		var original artifactRecord
		for _, f := range files {
			if f.ID == o.ObjectID {
				original = f
			}
		}
		if o.Kind != "artifact" || o.ObjectID != original.ID || o.StorageID != original.StorageID {
			t.Fatal("对象归属", o.Kind)
		}
		observation := RetentionObjectObservation{Identity: "v1:linux:0000000000000001:0000000000000002:1700000000:123456789", Size: o.Size, SHA256: o.SHA256}
		if err = s.ConfirmRetentionObject(testContext, o.ID, RetentionObjectResult{State: "deleted", Identity: observation.Identity}); err == nil {
			t.Fatal("未经隔离直接删确认")
		}
		bad := observation
		bad.Size++
		if _, err = s.AuthorizeRetentionObject(testContext, o.ID, bad); err == nil {
			t.Fatal("观察大小不符")
		}
		authorized, err := s.AuthorizeRetentionObject(testContext, o.ID, observation)
		if err != nil {
			t.Fatal(err)
		}
		if authorized.Identity != observation.Identity || authorized.QuarantineSlot != "retention/quarantine/"+o.JobID+"/"+o.ObjectID {
			t.Fatal("隔离槽不固定")
		}
		changed := observation
		changed.Identity = "v1:linux:0000000000000001:0000000000000003:1700000000:123456789"
		if _, err = s.AuthorizeRetentionObject(testContext, o.ID, changed); err == nil {
			t.Fatal("不能授权替换inode")
		}
		for range 2 {
			if err = s.ConfirmRetentionObject(testContext, o.ID, RetentionObjectResult{State: "quarantined", Identity: observation.Identity}); err != nil {
				t.Fatal(err)
			}
		}
		if err = s.ConfirmRetentionObject(testContext, o.ID, RetentionObjectResult{State: "failed", Reason: "retention_io_error", Identity: observation.Identity}); err != nil {
			t.Fatal("隔离后IO失败", err)
		}
		if _, err = s.AuthorizeRetentionObject(testContext, o.ID, observation); err != nil {
			t.Fatal("隔离后失败重试授权", err)
		}
		for range 2 {
			if err = s.ConfirmRetentionObject(testContext, o.ID, RetentionObjectResult{State: "deleted", Identity: observation.Identity}); err != nil {
				t.Fatal(err)
			}
		}
		if err = s.ConfirmRetentionObject(testContext, o.ID, RetentionObjectResult{State: "quarantined", Identity: observation.Identity}); err == nil {
			t.Fatal("阶段倒退")
		}
		var job retentionJobRecord
		s.db.First(&job, "id = ?", o.JobID)
		if job.CentralCompletedAt != nil || job.NodeCompletedAt != nil {
			t.Fatal("只删一个文件伪整体完成")
		}
		if err = s.SyncGlobalRetention(testContext, config.Retention{Builds: 100, Days: 30}); err != nil {
			t.Fatal(err)
		}
		if _, err = s.AuthorizeRetentionObject(testContext, objects[1].ID, observation); err == nil {
			t.Fatal("旧评估授权绕新政策")
		}
	})
}

func TestRetentionJobsTransactionFailureRollsBack(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, o Options) {
		p, ids := retentionJobProject(t, s)
		sql := `CREATE TRIGGER reject_retention_job BEFORE INSERT ON retention_jobs BEGIN SELECT RAISE(ABORT,'private fault'); END`
		if o.Driver == "postgres" {
			if err := s.writer.Exec(`CREATE FUNCTION reject_retention_job() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'private fault'; END $$`).Error; err != nil {
				t.Fatal(err)
			}
			sql = `CREATE TRIGGER reject_retention_job BEFORE INSERT ON retention_jobs FOR EACH ROW EXECUTE FUNCTION reject_retention_job()`
		}
		if err := s.writer.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
		expected := errDatabase
		if o.Driver == "sqlite" {
			expected = ErrConflict
		}
		if _, err := s.ScheduleRetention(testContext, localAdmin, p.ID, 100); err != expected {
			t.Fatal("真实SQL失败固定诊断", err)
		}
		var count int64
		s.db.Model(&retentionJobRecord{}).Count(&count)
		if count != 0 {
			t.Fatal("SQL失败残留事项")
		}
		for _, id := range ids {
			var row buildRecord
			s.db.First(&row, "id = ?", id)
			if row.HistoryState != "live" {
				t.Fatal("SQL失败原构建被退役")
			}
		}
	})
}
func TestRetentionJobsNewRetryVersusRetirement(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 1, Days: 30}); err != nil {
			t.Fatal(err)
		}
		p, id := cancelledQueued(t, s)
		retentionSkipped(t, s, p, 1)
		if _, err := s.ScheduleRetention(testContext, localAdmin, p.ID, 100); err != nil {
			t.Fatal(err)
		}
		var row buildRecord
		s.db.First(&row, "id = ?", id)
		if row.HistoryState != "retiring" {
			t.Fatal("真实退役事务未成立")
		}
		if _, err := s.Retry(testContext, localAdmin, RetryInput{BuildID: id, Key: "after-retirement"}); err != ErrRetentionRetired {
			t.Fatal("新Retry必须拒已退役快照", err)
		}
	})
}
