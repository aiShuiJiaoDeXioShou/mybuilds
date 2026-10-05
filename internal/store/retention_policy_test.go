package store

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"mybuilds/internal/config"
)

func retentionSettings(t *testing.T, text string) config.ProjectSettings {
	t.Helper()
	settings, err := config.ParseProjectSettings([]byte(text))
	if err != nil {
		t.Fatal("严格项目设置夹具", err)
	}
	return settings
}

func retentionPolicyView(t *testing.T, s *Store, id string) EffectiveRetention {
	t.Helper()
	view, err := s.EffectiveRetention(testContext, localAdmin, id)
	if err != nil {
		t.Fatal("读取有效策略", err)
	}
	return view
}

func retentionPolicyAudits(t *testing.T, s *Store) int64 {
	t.Helper()
	var count int64
	if err := s.db.Model(&auditRecord{}).Count(&count).Error; err != nil {
		t.Fatal("读取实际审计", safeError(err))
	}
	return count
}

func TestRetentionPolicyInheritanceAndMove(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 100, Days: 30}); err != nil {
			t.Fatal(err)
		}
		first, err := s.CreateProject(testContext, localAdmin, projectInput("policy-first"))
		if err != nil {
			t.Fatal(err)
		}
		second, err := s.CreateProject(testContext, localAdmin, projectInput("policy-second"))
		if err != nil {
			t.Fatal(err)
		}
		initial := retentionPolicyView(t, s, first.ID)
		if initial != (EffectiveRetention{Builds: 100, Days: 30, BuildsSource: "global", DaysSource: "global", GlobalVersion: 1, ProjectVersion: first.PolicyVersion}) {
			t.Fatal("默认继承/来源不准确")
		}
		pipeline := retentionSettings(t, "pipeline: {source: repo, file: ci/build.yml}\nretention: {builds: 200}\n")
		first, err = s.SetProjectSettings(testContext, localAdmin, first.Name, pipeline)
		if err != nil {
			t.Fatal(err)
		}
		view := retentionPolicyView(t, s, first.ID)
		if view.Builds != 200 || view.Days != 30 || view.BuildsSource != "project" || view.DaysSource != "global" || view.ProjectVersion != first.PolicyVersion {
			t.Fatal("单字段覆盖错误")
		}
		if err = s.SyncGlobalRetention(testContext, config.Retention{Builds: 100, Days: 45}); err != nil {
			t.Fatal(err)
		}
		view = retentionPolicyView(t, s, first.ID)
		other := retentionPolicyView(t, s, second.ID)
		if view.Builds != 200 || view.Days != 45 || other.Builds != 100 || other.Days != 45 || view.GlobalVersion != 2 || other.GlobalVersion != 2 {
			t.Fatal("字段继承冻结了旧全局值或影响另一项目")
		}
		for _, sample := range []struct {
			text                     string
			builds, days             int64
			buildsSource, daysSource string
		}{
			{"retention: {builds: 300, days: 10}\n", 300, 10, "project", "project"},
			{"retention: {days: 20}\n", 100, 20, "global", "project"},
			{"retention: {}\n", 100, 45, "global", "global"},
			{"retention: {builds: 200}\n", 200, 45, "project", "global"},
			{"{}\n", 100, 45, "global", "global"},
		} {
			first, err = s.SetProjectSettings(testContext, localAdmin, first.Name, retentionSettings(t, sample.text))
			if err != nil {
				t.Fatal(err)
			}
			view = retentionPolicyView(t, s, first.ID)
			if view.Builds != sample.builds || view.Days != sample.days || view.BuildsSource != sample.buildsSource || view.DaysSource != sample.daysSource || view.ProjectVersion != first.PolicyVersion {
				t.Fatal("两字段/天数/空块/省略继承不准确")
			}
			if first.Settings.Pipeline == nil || first.Settings.Pipeline.File != pipeline.Pipeline.File {
				t.Fatal("retention-only导入重置原pipeline")
			}
		}
		if _, err = s.SetProjectSettings(testContext, localAdmin, first.Name, pipeline); err != nil {
			t.Fatal(err)
		}
		before := retentionPolicyView(t, s, first.ID)
		group, err := s.CreateGroup(testContext, localAdmin, "policy-group")
		if err != nil {
			t.Fatal(err)
		}
		moved, err := s.MoveProject(testContext, localAdmin, first.Name, group.Name)
		if err != nil || moved.ID != first.ID {
			t.Fatal("迁移改变项目身份", err)
		}
		after := retentionPolicyView(t, s, moved.ID)
		before.ProjectVersion = moved.PolicyVersion
		if before != after {
			t.Fatal("组迁移改变项目覆盖/全局来源")
		}
	})
}

func TestRetentionPolicyRestartAndNoop(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		policy := config.Retention{Builds: 100, Days: 30}
		if err := s.SyncGlobalRetention(testContext, policy); err != nil {
			t.Fatal(err)
		}
		var before retentionPolicyRecord
		if err := s.db.First(&before, 1).Error; err != nil {
			t.Fatal(safeError(err))
		}
		count := retentionPolicyAudits(t, s)
		if err := s.SyncGlobalRetention(testContext, policy); err != nil {
			t.Fatal(err)
		}
		if retentionPolicyAudits(t, s) != count {
			t.Fatal("同值同步重复审计")
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
		if err = reopened.SyncGlobalRetention(testContext, policy); err != nil {
			t.Fatal(err)
		}
		var after retentionPolicyRecord
		if err = reopened.db.First(&after, 1).Error; err != nil || !reflect.DeepEqual(before, after) || retentionPolicyAudits(t, reopened) != count {
			t.Fatal("受控重启同值刷新版本/时间/审计", safeError(err))
		}
		if err = reopened.SyncGlobalRetention(testContext, config.Retention{Builds: 200, Days: 45}); err != nil {
			t.Fatal(err)
		}
		if err = reopened.db.First(&after, 1).Error; err != nil || after.Version != before.Version+1 || after.Builds != 200 || after.Days != 45 || retentionPolicyAudits(t, reopened) != count+1 {
			t.Fatal("全局真实变更未原子增版本/审计", safeError(err))
		}
	})
}

func TestRetentionPolicyInvalidAndRolesAtomic(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 100, Days: 30}); err != nil {
			t.Fatal(err)
		}
		project, err := s.CreateProject(testContext, localAdmin, projectInput("policy-atomic"))
		if err != nil {
			t.Fatal(err)
		}
		before := retentionPolicyView(t, s, project.ID)
		count := retentionPolicyAudits(t, s)
		for _, policy := range []config.Retention{{Builds: 0, Days: 30}, {Builds: -1, Days: 30}, {Builds: 100, Days: 0}, {Builds: 100, Days: -1}, {Builds: 100, Days: 106752}, {Builds: 100, Days: math.MaxInt64}} {
			if err = s.SyncGlobalRetention(testContext, policy); err == nil || err.Error() != "retention_invalid" {
				t.Fatal("非法全局策略未安全拒绝", err)
			}
		}
		for _, text := range []string{"retention: {builds: 0}\n", "retention: {days: 0}\n", "retention: {days: 1.5}\n", "retention: {days: secret-marker}\n", "retention: null\n", "retention: {builds: null}\n", "retention: {unknown: 3}\n", "retention: {builds: 2, builds: 3}\n", "retention: {builds: 9223372036854775808}\n"} {
			if _, err = config.ParseProjectSettings([]byte(text)); err == nil {
				t.Fatal("非法整块项目设置被解析接受")
			}
		}
		settings := retentionSettings(t, "retention: {builds: 20}\n")
		for _, value := range []int64{0, -1, 106752} {
			invalid := config.ProjectSettings{Retention: &config.RetentionOverride{Days: &value}}
			if _, err = s.SetProjectSettings(testContext, localAdmin, project.Name, invalid); err != ErrInvalid {
				t.Fatal("直接Store项目策略非法值未整体拒绝", err)
			}
		}
		for _, role := range []string{"trigger", "approver"} {
			created, e := s.CreateToken(testContext, localAdmin, role)
			if e != nil {
				t.Fatal(e)
			}
			actor, e := s.Authenticate(testContext, created.Token)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = s.SetProjectSettings(testContext, actor, project.Name, settings); e != ErrForbidden {
				t.Fatal("非admin项目策略变更未拒绝", e)
			}
			if _, e = s.EffectiveRetention(testContext, actor, project.ID); e != ErrForbidden {
				t.Fatal("非admin管理读取未拒绝", e)
			}
			count++ // 只允许真实token创建审计，策略操作均不能增加审计。
		}
		created, err := s.CreateNode(testContext, localAdmin, NodeInput{Name: "policy-node", Capacity: 1})
		if err != nil {
			t.Fatal(err)
		}
		actor, err := s.AuthenticateNode(testContext, created.Token)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.EffectiveRetention(testContext, Actor{ID: actor.ID, Role: "node"}, project.ID); err != ErrUnauthorized {
			t.Fatal("节点冒充管理员读策略", err)
		}
		count++ // 真实node创建审计。
		if retentionPolicyView(t, s, project.ID) != before || retentionPolicyAudits(t, s) != count {
			t.Fatal("非法/无权限操作改变策略或审计")
		}
	})
}

func TestRetentionPolicyRollbackAndVersionOverflow(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 100, Days: 30}); err != nil {
			t.Fatal(err)
		}
		// 私有夹具移除审计表，使真实write在policy更新后失败；必须整体回滚。
		if err := s.writer.Migrator().DropTable(&auditRecord{}); err != nil {
			t.Fatal(safeError(err))
		}
		var before, after retentionPolicyRecord
		if err := s.db.First(&before, 1).Error; err != nil {
			t.Fatal(safeError(err))
		}
		if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 200, Days: 45}); err == nil {
			t.Fatal("缺失真实审计写入仍提交策略")
		}
		if err := s.db.First(&after, 1).Error; err != nil || !reflect.DeepEqual(before, after) {
			t.Fatal("SQL审计失败部分提交", safeError(err))
		}
		if err := s.writer.AutoMigrate(&auditRecord{}); err != nil {
			t.Fatal(safeError(err))
		}
		if err := s.writer.Model(&retentionPolicyRecord{}).Where("id = ?", 1).Update("version", int64(math.MaxInt64)).Error; err != nil {
			t.Fatal(safeError(err))
		}
		if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 200, Days: 45}); err != ErrConflict {
			t.Fatal("全局版本溢出未拒绝", err)
		}
		if err := s.db.First(&after, 1).Error; err != nil || after.Builds != 100 || after.Days != 30 || after.Version != math.MaxInt64 || retentionPolicyAudits(t, s) != 0 {
			t.Fatal("版本溢出仍部分写入", safeError(err))
		}
	})
}

func TestRetentionPolicyCancelledAndReadonlyNoDeletion(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 100, Days: 30}); err != nil {
			t.Fatal(err)
		}
		project, err := s.CreateProject(testContext, localAdmin, projectInput("policy-readonly"))
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "owned-evidence")
		data := []byte("实际自有对照证据")
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		before := retentionPolicyView(t, s, project.ID)
		ctx, cancel := context.WithCancel(testContext)
		cancel()
		if err = s.SyncGlobalRetention(ctx, config.Retention{Builds: 1, Days: 1}); err == nil {
			t.Fatal("取消后仍修改全局策略")
		}
		if retentionPolicyView(t, s, project.ID) != before {
			t.Fatal("取消后策略发生改变")
		}
		if _, err = s.SetProjectSettings(testContext, localAdmin, project.Name, retentionSettings(t, "retention: {builds: 1, days: 1}\n")); err != nil {
			t.Fatal(err)
		}
		view := retentionPolicyView(t, s, project.ID)
		wire, err := json.Marshal(view)
		if err != nil || len(wire) == 0 {
			t.Fatal("有效视图编码", err)
		}
		actual, err := os.ReadFile(path)
		if err != nil || !reflect.DeepEqual(data, actual) {
			t.Fatal("US1策略设置/读取删除证据", err)
		}
		for _, table := range []string{"retention_jobs", "retention_objects", "node_deletions"} {
			var n int64
			if err = s.db.Table(table).Count(&n).Error; err != nil || n != 0 {
				t.Fatal("策略操作提前创建删除事项", safeError(err))
			}
		}
	})
}

func TestRetentionPolicyMutexWaitHonorsContext(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 100, Days: 30}); err != nil {
			t.Fatal(err)
		}
		project, err := s.CreateProject(testContext, localAdmin, projectInput("policy-mutex"))
		if err != nil {
			t.Fatal(err)
		}
		for _, operation := range []string{"sync", "effective"} {
			for _, cause := range []string{"cancel", "deadline"} {
				t.Run(operation+"/"+cause, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(testContext, 50*time.Millisecond)
					defer cancel()
					s.mu.Lock()
					done := make(chan error, 1)
					go func() {
						if operation == "sync" {
							done <- s.SyncGlobalRetention(ctx, config.Retention{Builds: 200, Days: 45})
						} else {
							_, err := s.EffectiveRetention(ctx, localAdmin, project.ID)
							done <- err
						}
					}()
					if cause == "cancel" {
						cancel()
					}
					<-ctx.Done()
					blocked := false
					var actual error
					select {
					case actual = <-done:
					case <-time.After(250 * time.Millisecond):
						blocked = true
					}
					// 每条负例都先释放真实自有锁并等调用退出，不遗留goroutine。
					s.mu.Unlock()
					if blocked {
						select {
						case actual = <-done:
						case <-time.After(time.Second):
							t.Fatal("释放自有mutex后调用未退出")
						}
						t.Error("ctx结束后仍被真实Store mutex阻塞")
					}
					expected := "retention_cancelled"
					if cause == "deadline" {
						expected = "retention_timeout"
					}
					if actual == nil || actual.Error() != expected {
						t.Error("排队结束未保固定ctx原因")
					}
				})
			}
		}
	})
}
