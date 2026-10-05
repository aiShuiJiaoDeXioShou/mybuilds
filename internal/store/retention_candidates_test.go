package store

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"mybuilds/internal/config"
)

func retentionEvaluateAt(t *testing.T, s *Store, id string, at time.Time, page Page) RetentionPage {
	t.Helper()
	var result RetentionPage
	ctx, cancel := context.WithTimeout(testContext, 5*time.Second)
	defer cancel()
	err := s.write(ctx, func(tx *gorm.DB) error { var err error; result, err = evaluateRetention(tx, id, at, page); return err })
	if err != nil {
		t.Fatal("固定UTC评估", err)
	}
	return result
}
func retentionEntry(t *testing.T, page RetentionPage, id string) RetentionEntry {
	t.Helper()
	for _, entry := range page.Items {
		if entry.BuildID == id {
			return entry
		}
	}
	t.Fatal("评估页缺少执行")
	return RetentionEntry{}
}
func retentionSkipped(t *testing.T, s *Store, p Project, n int) []string {
	t.Helper()
	in := enqueueInput(p, "skipped-rank")
	one := in.Builds[0]
	in.Builds = nil
	for i := 0; i < n; i++ {
		b := one
		b.Name = fmt.Sprintf("named-%d", i)
		b.Status = "skipped"
		b.Snapshot.Condition = "skipped"
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
func TestRetentionCandidatesProjectRankAndBoundary(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 4, Days: 1}); err != nil {
			t.Fatal(err)
		}
		p, err := s.CreateProject(testContext, localAdmin, projectInput("rank"))
		if err != nil {
			t.Fatal(err)
		}
		ids := retentionSkipped(t, s, p, 6)
		slices.Sort(ids)
		slices.Reverse(ids)
		active := queueBuild(t, s, p, "active", "active", false)
		other, err := s.CreateProject(testContext, localAdmin, projectInput("elsewhere"))
		if err != nil {
			t.Fatal(err)
		}
		retentionSkipped(t, s, other, 6)
		var first buildRecord
		if err = s.db.First(&first, "id = ?", ids[0]).Error; err != nil {
			t.Fatal(err)
		}
		at := first.TerminalAt.Add(24 * time.Hour)
		page := retentionEvaluateAt(t, s, p.ID, at, Page{Limit: 200})
		if len(page.Items) != 7 {
			t.Fatal("项目范围/活动页丢失", len(page.Items))
		}
		for i, id := range ids {
			entry := retentionEntry(t, page, id)
			if entry.Number != nil || entry.Candidate != (i >= 4) || len(entry.ProtectReasons) != 0 {
				t.Fatal("跨名称排名/等号/无编号", i, entry)
			}
		}
		if e := retentionEntry(t, page, active); e.Candidate || !slices.Contains(e.ProtectReasons, "active") {
			t.Fatal("活动挤占配额", e)
		}
		if got := page.Items[:6]; got[0].BuildID != ids[0] || got[5].BuildID != ids[5] {
			t.Fatal("同时间ID排序不稳定")
		}
		later := retentionEvaluateAt(t, s, p.ID, at.Add(time.Nanosecond), Page{Limit: 200})
		for _, id := range ids {
			if !retentionEntry(t, later, id).Candidate {
				t.Fatal("天数OR/严格边界失效")
			}
		}
		limited := retentionEvaluateAt(t, s, p.ID, at, Page{Limit: 2, Offset: 3})
		if len(limited.Items) != 2 || limited.Items[0].BuildID != ids[3] || !limited.Items[1].Candidate {
			t.Fatal("统一分页在过滤后遗漏")
		}
		public, err := s.EvaluateRetention(testContext, localAdmin, p.ID, Page{})
		if err != nil || public.Items == nil || public.Limit != 20 {
			t.Fatal("实际管理API", err)
		}
		data, _ := json.Marshal(public)
		if strings.Contains(string(data), "secret-") {
			t.Fatal("公开页回显原脚本/参数")
		}
	})
}
func TestRetentionCandidatesUnknownTimeAndCleanedDoNotRank(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 1, Days: 30}); err != nil {
			t.Fatal(err)
		}
		p, err := s.CreateProject(testContext, localAdmin, projectInput("legacy"))
		if err != nil {
			t.Fatal(err)
		}
		ids := retentionSkipped(t, s, p, 3)
		slices.Sort(ids)
		slices.Reverse(ids)
		// 已迁移历史缺证与墓碑是读取边界负例，不伪造未来审批/发布状态。
		if err = s.writer.Model(&buildRecord{}).Where("id = ?", ids[0]).Update("terminal_at", nil).Error; err != nil {
			t.Fatal(err)
		}
		if err = s.writer.Model(&buildRecord{}).Where("id = ?", ids[1]).Update("history_state", "cleaned").Error; err != nil {
			t.Fatal(err)
		}
		page, err := s.EvaluateRetention(testContext, localAdmin, p.ID, Page{Limit: 200})
		if err != nil {
			t.Fatal(err)
		}
		if e := retentionEntry(t, page, ids[0]); e.Candidate || !slices.Contains(e.ProtectReasons, "terminal_time_unknown") {
			t.Fatal("旧时间被猜测", e)
		}
		if e := retentionEntry(t, page, ids[2]); e.Candidate {
			t.Fatal("未知时间/墓碑挤占有效排名", e)
		}
		before := recoveryRows(t, s)
		created, err := s.CreateToken(testContext, localAdmin, "trigger")
		if err != nil {
			t.Fatal(err)
		}
		actor, err := s.Authenticate(testContext, created.Token)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.EvaluateRetention(testContext, actor, p.ID, Page{}); err != ErrForbidden {
			t.Fatal("非管理员评估", err)
		}
		if !reflect.DeepEqual(before, recoveryRows(t, s)) {
			t.Fatal("只读评估修改历史")
		}
		for _, page := range []Page{{Limit: 201}, {Offset: -1}} {
			if _, err = s.EvaluateRetention(testContext, localAdmin, p.ID, page); err != ErrInvalid {
				t.Fatal("无界分页", err)
			}
		}
	})
}
