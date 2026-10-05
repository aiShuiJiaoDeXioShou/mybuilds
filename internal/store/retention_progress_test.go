package store

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
)

// 本组只沿真实业务API建立事实，不手改历史、时间或清理游标。
func retentionProgressAdmin(t *testing.T, s *Store) Actor {
	t.Helper()
	token := uuid.NewString() + uuid.NewString()
	if err := s.Bootstrap(testContext, token); err != nil {
		t.Fatal(err)
	}
	a, err := s.Authenticate(testContext, token)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SyncGlobalRetention(testContext, config.Retention{Builds: 1, Days: 30}); err != nil {
		t.Fatal(err)
	}
	return a
}

func retentionProgressProject(t *testing.T, s *Store, a Actor, name string) Project {
	t.Helper()
	p, err := s.CreateProject(testContext, a, projectInput(name))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func retentionProgressSkipped(t *testing.T, s *Store, a Actor, p Project, key string, n int) []string {
	t.Helper()
	in := enqueueInput(p, p.Name+"-"+key)
	in.Actor = a
	one := in.Builds[0]
	in.Builds = nil
	for i := range n {
		b := one
		b.Name = fmt.Sprintf("skipped-%d", i)
		b.Status, b.Snapshot.Condition = "skipped", "skipped"
		in.Builds = append(in.Builds, b)
	}
	out, err := s.Enqueue(testContext, in)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, b := range out.Builds {
		ids = append(ids, b.ID)
	}
	return ids
}

func TestRetentionProgressGlobalBeyondHundredProjects(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		a := retentionProgressAdmin(t, s)
		projects := []Project{}
		for i := range 101 {
			p := retentionProgressProject(t, s, a, fmt.Sprintf("progress-%03d", i))
			retentionProgressSkipped(t, s, a, p, "initial", 2)
			projects = append(projects, p)
		}
		for range 4 {
			if _, err := s.AdvanceRetention(testContext, "", 100); err != nil {
				t.Fatal(err)
			}
		}
		missing := 0
		for _, p := range projects {
			page, err := s.ListRetention(testContext, a, p.ID, Page{Limit: 20})
			if err != nil {
				t.Fatal(err)
			}
			if len(page.Items) == 0 {
				missing++
			}
		}
		if missing != 0 {
			t.Fatalf("连续四轮仍有%d个合法项目从未登记候选", missing)
		}
	})
}

func TestRetentionProgressSchedulePastStablePrefix(t *testing.T) {
	for _, prefix := range []string{"retry_protected", "existing_job"} {
		t.Run(prefix, func(t *testing.T) {
			stores(t, func(t *testing.T, s *Store, _ Options) {
				a := retentionProgressAdmin(t, s)
				p := retentionProgressProject(t, s, a, "schedule-prefix")
				if prefix == "retry_protected" {
					in := enqueueInput(p, "parent")
					in.Actor = a
					out, err := s.Enqueue(testContext, in)
					if err != nil {
						t.Fatal(err)
					}
					parent := out.Builds[0].ID
					if _, err = s.Cancel(testContext, a, parent); err != nil {
						t.Fatal(err)
					}
					if _, err = s.Retry(testContext, a, RetryInput{BuildID: parent, Key: "live-child"}); err != nil {
						t.Fatal(err)
					}
				}
				retentionProgressSkipped(t, s, a, p, "later", 3)
				for range 4 {
					if _, err := s.ScheduleRetention(testContext, a, p.ID, 1); err != nil {
						t.Fatal(err)
					}
				}
				page, err := s.ListRetention(testContext, a, p.ID, Page{Limit: 20})
				if err != nil {
					t.Fatal(err)
				}
				if len(page.Items) != 2 {
					t.Fatalf("limit=1连续四轮被%s前缀占满，合法后续候选事项=%d，期望2", prefix, len(page.Items))
				}
			})
		})
	}
}

// 真Claim、资源登记与零动作终态产生可清节点事项；并不宣称实际OS删除。
func retentionProgressNodeJob(t *testing.T, s *Store, admin Actor, a NodeActor, session protocol.SessionGrant, name string) (Project, string) {
	t.Helper()
	p := retentionProgressProject(t, s, admin, name)
	in := enqueueInput(p, p.Name+"-node-build")
	in.Actor = admin
	out, err := s.Enqueue(testContext, in)
	if err != nil {
		t.Fatal(err)
	}
	policy := LeasePolicy{Concurrency: 2, Heartbeat: time.Second, Duration: 10 * time.Second}
	if _, err = s.Heartbeat(testContext, a, protocol.HeartbeatRequest{SessionID: session.SessionID, Report: healthyReport()}, policy); err != nil {
		t.Fatal(err)
	}
	g, err := s.Claim(testContext, a, protocol.ClaimRequest{SessionID: session.SessionID, ClaimKey: uuid.NewString()}, policy)
	if err != nil || g == nil || g.Ref.BuildID != out.Builds[0].ID {
		t.Fatal("真实Claim", err)
	}
	if err = s.RegisterNodeResource(testContext, a, resourceRegistration(g.Ref)); err != nil {
		t.Fatal(err)
	}
	accept(t, s, a, event(g.Ref, 1, protocol.ExecutionProgress{Kind: "build_finished", Status: "failed", Reason: "precheck_error", PostPhase: "none", StopConfirmed: true, RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}))
	retentionProgressSkipped(t, s, admin, p, "retained", 1)
	if _, err = s.ScheduleRetention(testContext, admin, p.ID, 100); err != nil {
		t.Fatal(err)
	}
	return p, g.Ref.BuildID
}

func TestRetentionProgressNodeClaimPastUnavailablePrefix(t *testing.T) {
	for _, prefix := range []string{"protected", "failed"} {
		t.Run(prefix, func(t *testing.T) {
			stores(t, func(t *testing.T, s *Store, _ Options) {
				admin := retentionProgressAdmin(t, s)
				a, session := openSession(t, s, "linux", 2)
				first, _ := retentionProgressNodeJob(t, s, admin, a, session, "node-first")
				initial, err := s.ClaimNodeDeletions(testContext, a, 1)
				if err != nil || len(initial) != 1 {
					t.Fatal("首事项", err)
				}
				if prefix == "protected" {
					if _, err = s.SetProjectSettings(testContext, admin, first.Name, retentionSettings(t, "retention: {builds: 100}\n")); err != nil {
						t.Fatal(err)
					}
				} else {
					auth, err := s.AuthorizeNodeDeletion(testContext, a, initial[0].ID)
					if err != nil {
						t.Fatal(err)
					}
					in := retentionDeletionConfirmation(t, initial[0], auth, 1, "failed", "not_applicable", "permission_denied")
					if err = s.ConfirmNodeDeletion(testContext, a, in); err != nil {
						t.Fatal(err)
					}
				}
				_, later := retentionProgressNodeJob(t, s, admin, a, session, "node-later")
				seen := false
				for range 4 {
					claims, err := s.ClaimNodeDeletions(testContext, a, 1)
					if err != nil {
						t.Fatal(err)
					}
					for _, d := range claims {
						if d.BuildID == later {
							seen = true
						}
					}
				}
				if !seen {
					t.Fatalf("Node limit=1四轮被%s前缀阻塞，后续合法事项从未领取", prefix)
				}
			})
		})
	}
}

func TestRetentionProgressFinalizePastPendingNode(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		admin := retentionProgressAdmin(t, s)
		a, session := openSession(t, s, "linux", 2)
		_, pending := retentionProgressNodeJob(t, s, admin, a, session, "pending-node")
		p := retentionProgressProject(t, s, admin, "later-no-node")
		ids := retentionProgressSkipped(t, s, admin, p, "no-node", 2)
		if _, err := s.ScheduleRetention(testContext, admin, p.ID, 100); err != nil {
			t.Fatal(err)
		}
		for range 4 {
			if _, err := s.FinalizeRetention(testContext, "", 1); err != nil {
				t.Fatal(err)
			}
		}
		var cleaned int64
		if err := s.db.Model(&buildRecord{}).Where("id IN ? AND history_state = 'cleaned'", ids).Count(&cleaned).Error; err != nil {
			t.Fatal(err)
		}
		if cleaned != 1 {
			t.Fatalf("limit=1被旧Nodepending前缀阻塞，无Node合法后续完成数=%d，期望1", cleaned)
		}
		view, err := s.GetBuild(testContext, pending)
		if err != nil || view.HistoryState == "cleaned" {
			t.Fatal("不能假确认旧Node", err)
		}
	})
}

func TestRetentionProgressPositionSurvivesControlReopen(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		admin := retentionProgressAdmin(t, s)
		p := retentionProgressProject(t, s, admin, "reopen-position")
		retentionProgressSkipped(t, s, admin, p, "initial", 3)
		if _, err := s.ScheduleRetention(testContext, admin, p.ID, 1); err != nil {
			t.Fatal(err)
		}
		var before projectRecord
		if err := s.db.First(&before, "id = ?", p.ID).Error; err != nil || before.RetentionCandidateCursor == "" {
			t.Fatal("持久扫描位置", err)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		reopened, err := Open(testContext, opt)
		if err != nil {
			t.Fatal(err)
		}
		defer reopened.Close()
		if err = reopened.Migrate(testContext); err != nil {
			t.Fatal(err)
		}
		var after projectRecord
		if err = reopened.db.First(&after, "id = ?", p.ID).Error; err != nil || after.RetentionCandidateCursor != before.RetentionCandidateCursor {
			t.Fatal("重开重置位置", err)
		}
		if _, err = reopened.ScheduleRetention(testContext, admin, p.ID, 1); err != nil {
			t.Fatal(err)
		}
		page, err := reopened.ListRetention(testContext, admin, p.ID, Page{Limit: 20})
		if err != nil || len(page.Items) != 2 {
			t.Fatal("重开没有继续后续合法事项", err, len(page.Items))
		}
		if err = reopened.db.First(&after, "id = ?", p.ID).Error; err != nil {
			t.Fatal(err)
		}
		if after.PolicyVersion != before.PolicyVersion || after.NextNumber != before.NextNumber || !after.UpdatedAt.Equal(before.UpdatedAt) {
			t.Fatal("扫描改变业务版本/编号/更新时间")
		}
	})
}

func TestRetentionProgressNodeRotationKeepsPosition(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		admin := retentionProgressAdmin(t, s)
		a, session := openSession(t, s, "linux", 2)
		_, first := retentionProgressNodeJob(t, s, admin, a, session, "rotate-first")
		_, second := retentionProgressNodeJob(t, s, admin, a, session, "rotate-second")
		claims, err := s.ClaimNodeDeletions(testContext, a, 1)
		if err != nil || len(claims) != 1 || claims[0].BuildID != first {
			t.Fatal("首次扫描", err)
		}
		var before nodeRecord
		if err = s.db.First(&before, "id = ?", a.ID).Error; err != nil {
			t.Fatal(err)
		}
		credential, err := s.RotateNodeToken(testContext, admin, "linux")
		if err != nil {
			t.Fatal(err)
		}
		current, err := s.AuthenticateNode(testContext, credential.Token)
		if err != nil {
			t.Fatal(err)
		}
		var rotated nodeRecord
		if err = s.db.First(&rotated, "id = ?", a.ID).Error; err != nil || rotated.RetentionDeletionCursor != before.RetentionDeletionCursor {
			t.Fatal("轮换重置位置", err)
		}
		claims, err = s.ClaimNodeDeletions(testContext, current, 1)
		if err != nil || len(claims) != 1 || claims[0].BuildID != second {
			t.Fatal("当前身份未续扫下一事项", err)
		}
		claims, err = s.ClaimNodeDeletions(testContext, current, 1)
		if err != nil || len(claims) != 1 || claims[0].BuildID != first {
			t.Fatal("有限回绕漏旧事项", err)
		}
	})
}

func TestRetentionProgressCursorSQLFailureRollsBack(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		admin := retentionProgressAdmin(t, s)
		p := retentionProgressProject(t, s, admin, "cursor-fault")
		retentionProgressSkipped(t, s, admin, p, "initial", 3)
		var before projectRecord
		if err := s.db.First(&before, "id = ?", p.ID).Error; err != nil {
			t.Fatal(err)
		}
		sql := `CREATE TRIGGER reject_retention_position BEFORE UPDATE OF retention_candidate_cursor ON projects BEGIN SELECT RAISE(ABORT,'private fault'); END`
		if opt.Driver == "postgres" {
			if err := s.writer.Exec(`CREATE FUNCTION reject_retention_position() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'private fault'; END $$`).Error; err != nil {
				t.Fatal(err)
			}
			sql = `CREATE TRIGGER reject_retention_position BEFORE UPDATE OF retention_candidate_cursor ON projects FOR EACH ROW EXECUTE FUNCTION reject_retention_position()`
		}
		if err := s.writer.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := s.ScheduleRetention(testContext, admin, p.ID, 1); err == nil {
			t.Fatal("游标SQL故障仍提交")
		}
		var after projectRecord
		if err := s.db.First(&after, "id = ?", p.ID).Error; err != nil {
			t.Fatal(err)
		}
		if before != after {
			t.Fatal("游标失败部分改写项目")
		}
		page, err := s.ListRetention(testContext, admin, p.ID, Page{Limit: 20})
		if err != nil || len(page.Items) != 0 {
			t.Fatal("游标失败残留退役事项", err)
		}
		if opt.Driver == "postgres" {
			sql = `DROP TRIGGER reject_retention_position ON projects`
		} else {
			sql = `DROP TRIGGER reject_retention_position`
		}
		if err = s.writer.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
		if _, err = s.ScheduleRetention(testContext, admin, p.ID, 1); err != nil {
			t.Fatal(err)
		}
	})
}

func TestRetentionProgressGlobalCursorOnlyVisitedProject(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		admin := retentionProgressAdmin(t, s)
		for i := range 3 {
			p := retentionProgressProject(t, s, admin, fmt.Sprintf("visited-%d", i))
			retentionProgressSkipped(t, s, admin, p, "initial", 2)
		}
		if _, err := s.AdvanceRetention(testContext, "", 1); err != nil {
			t.Fatal(err)
		}
		var policy retentionPolicyRecord
		if err := s.db.First(&policy, 1).Error; err != nil {
			t.Fatal(err)
		}
		var job retentionJobRecord
		if err := s.db.First(&job).Error; err != nil || policy.ProjectCursor != job.ProjectID {
			t.Fatal("提前结束跳过尚未访问项目", err)
		}
	})
}

func TestRetentionProgressObjectPastFailurePrefix(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		retentionProgressAdmin(t, s)
		pid, _ := retentionJobFiles(t, s)
		first, err := s.AdvanceRetention(testContext, pid, 1)
		if err != nil || len(first) != 1 {
			t.Fatal("首次真实对象", err)
		}
		if err = s.RecordRetentionObjectFailure(testContext, first[0].ID, "retention_ownership_unknown"); err != nil {
			t.Fatal(err)
		}
		second, err := s.AdvanceRetention(testContext, pid, 1)
		if err != nil || len(second) != 1 || second[0].ID == first[0].ID {
			t.Fatal("失败对象永久占首位", err)
		}
		again, err := s.AdvanceRetention(testContext, pid, 1)
		if err != nil || len(again) != 1 || again[0].ID != first[0].ID {
			t.Fatal("失败对象必须有限回绕重核", err)
		}
	})
}

func TestRetentionProgressDeletedProjectCursorWrapsBoundedly(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		admin := retentionProgressAdmin(t, s)
		for i := range 2 {
			retentionProgressProject(t, s, admin, fmt.Sprintf("empty-%d", i))
		}
		if _, err := s.AdvanceRetention(testContext, "", 100); err != nil {
			t.Fatal(err)
		}
		var policy retentionPolicyRecord
		if err := s.db.First(&policy, 1).Error; err != nil {
			t.Fatal(err)
		}
		var previous projectRecord
		if err := s.db.First(&previous, "id = ?", policy.ProjectCursor).Error; err != nil {
			t.Fatal(err)
		}
		if err := s.DeleteProject(testContext, admin, previous.Name); err != nil {
			t.Fatal(err)
		}
		p := retentionProgressProject(t, s, admin, "after-project-delete")
		retentionProgressSkipped(t, s, admin, p, "initial", 2)
		if _, err := s.AdvanceRetention(testContext, "", 100); err != nil {
			t.Fatal("合法已删除UUID位置不能闭锁", err)
		}
		page, err := s.ListRetention(testContext, admin, p.ID, Page{Limit: 20})
		if err != nil || len(page.Items) != 1 {
			t.Fatal("删除旧空项目后没有实际续扫", err)
		}
	})
}

func TestRetentionProgressMalformedCursorCannotGrant(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		admin := retentionProgressAdmin(t, s)
		p := retentionProgressProject(t, s, admin, "invalid-position")
		retentionProgressSkipped(t, s, admin, p, "initial", 2)
		// 只损坏新私有SQL位置，绝不伪造业务事实或释放保护。
		if err := s.writer.Model(&projectRecord{}).Where("id = ?", p.ID).UpdateColumn("retention_candidate_cursor", "invalid-position").Error; err != nil {
			t.Fatal(err)
		}
		if _, err := s.ScheduleRetention(testContext, admin, p.ID, 1); err != ErrRetentionObjectInvalid {
			t.Fatal("非法位置没有安全拒绝", err)
		}
		page, err := s.ListRetention(testContext, admin, p.ID, Page{Limit: 20})
		if err != nil || len(page.Items) != 0 {
			t.Fatal("非法位置产生退役事项", err)
		}
		if err := s.writer.Model(&projectRecord{}).Where("id = ?", p.ID).UpdateColumn("retention_candidate_cursor", uuid.NewString()).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := s.ScheduleRetention(testContext, admin, p.ID, 1); err != nil {
			t.Fatal("合法缺失排序源没有有界回绕", err)
		}
		page, err = s.ListRetention(testContext, admin, p.ID, Page{Limit: 20})
		if err != nil || len(page.Items) != 1 {
			t.Fatal("缺失排序源回绕未重新核实际候选", err)
		}
	})
}
