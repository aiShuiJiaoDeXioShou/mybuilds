package store

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"gorm.io/gorm"
	"mybuilds/internal/config"
)

func TestRetentionRetryAncestorsAndReevaluation(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 1, Days: 1}); err != nil {
			t.Fatal(err)
		}
		p, parent := cancelledQueued(t, s)
		var row buildRecord
		s.db.First(&row, "id = ?", parent)
		at := row.TerminalAt.Add(48 * time.Hour)
		before := retentionEvaluateAt(t, s, p.ID, at, Page{Limit: 200})
		if e := retentionEntry(t, before, parent); !e.Candidate || len(e.ProtectReasons) != 0 {
			t.Fatal("真实queued取消应可评估", e)
		}
		child, err := s.Retry(testContext, localAdmin, RetryInput{BuildID: parent, Key: "child"})
		if err != nil {
			t.Fatal(err)
		}
		childID := child.Builds[0].ID
		if _, err = s.Cancel(testContext, localAdmin, childID); err != nil {
			t.Fatal(err)
		}
		grandchild, err := s.Retry(testContext, localAdmin, RetryInput{BuildID: childID, Key: "grandchild"})
		if err != nil {
			t.Fatal(err)
		}
		page := retentionEvaluateAt(t, s, p.ID, at, Page{Limit: 200})
		for _, id := range []string{parent, childID} {
			if !slices.Contains(retentionEntry(t, page, id).ProtectReasons, "retry_dependency") {
				t.Fatal("完整保留后代未保护祖先", id)
			}
		}
		if !slices.Contains(retentionEntry(t, page, grandchild.Builds[0].ID).ProtectReasons, "active") {
			t.Fatal("真实新Retry丢失活动保护")
		}
		if err = s.writer.Delete(&buildRecord{}, "id = ?", parent).Error; err == nil {
			t.Fatal("RESTRICT被破坏")
		}
		if _, err = s.SetProjectSettings(testContext, localAdmin, p.Name, retentionSettings(t, "retention: {builds: 100, days: 30}\n")); err != nil {
			t.Fatal(err)
		}
		page = retentionEvaluateAt(t, s, p.ID, at, Page{Limit: 200})
		if e := retentionEntry(t, page, parent); e.Candidate || !slices.Contains(e.ProtectReasons, "retry_dependency") {
			t.Fatal("当前策略不重新评估或误解除祖先", e)
		}
	})
}
func TestRetentionRetryCorruptCycleAndCrossProjectFailClosed(t *testing.T) {
	for _, boundary := range []string{"cycle", "cross-project"} {
		t.Run(boundary, func(t *testing.T) {
			stores(t, func(t *testing.T, s *Store, _ Options) {
				if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 1, Days: 1}); err != nil {
					t.Fatal(err)
				}
				p, parent := cancelledQueued(t, s)
				out, err := s.Retry(testContext, localAdmin, RetryInput{BuildID: parent, Key: "child"})
				if err != nil {
					t.Fatal(err)
				}
				// 在真实Retry关系上损坏已有DB关系，验证闭包安全边界，不造未来业务状态。
				if boundary == "cycle" {
					err = s.writer.Model(&buildRecord{}).Where("id = ?", parent).Update("retry_of", out.Builds[0].ID).Error
				} else {
					other, e := s.CreateProject(testContext, localAdmin, projectInput("foreign"))
					if e != nil {
						t.Fatal(e)
					}
					err = s.writer.Model(&buildRecord{}).Where("id = ?", out.Builds[0].ID).Update("project_id", other.ID).Error
				}
				if err != nil {
					t.Fatal(err)
				}
				page, err := s.EvaluateRetention(testContext, localAdmin, p.ID, Page{})
				if err != nil {
					t.Fatal(err)
				}
				if !slices.Contains(retentionEntry(t, page, parent).ProtectReasons, "retry_dependency") {
					t.Fatal("不充分闭包授权了祖先")
				}
			})
		})
	}
}
func TestRetentionRetryRelationLimitFailClosed(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 1, Days: 1}); err != nil {
			t.Fatal(err)
		}
		p, parent := cancelledQueued(t, s)
		child, err := s.Retry(testContext, localAdmin, RetryInput{BuildID: parent, Key: "seed"})
		if err != nil {
			t.Fatal(err)
		}
		var seed buildRecord
		if err = s.db.First(&seed, "id = ?", child.Builds[0].ID).Error; err != nil {
			t.Fatal(err)
		}
		// 实际DB关系限额夹具保留项目/batch/FK；不执行十万用户动作或关闭约束。
		idExpr := "'00000000-0000-4000-8000-' || printf('%012x', n)"
		series := fmt.Sprintf("WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM seq WHERE n < %d)", retentionRelationLimit)
		if opt.Driver == "postgres" {
			idExpr = "'00000000-0000-4000-8000-' || lpad(to_hex(n),12,'0')"
			series = fmt.Sprintf("WITH seq AS (SELECT generate_series(1,%d)::bigint AS n)", retentionRelationLimit)
		}
		query := series + " INSERT INTO builds (id,batch_id,project_id,retry_of,name,status,history_state) SELECT " + idExpr + ",?,?,?,'limit','queued','live' FROM seq"
		if err = s.writer.Exec(query, seed.BatchID, p.ID, parent).Error; err != nil {
			t.Fatal("有界真实关系负例", safeError(err))
		}
		var protected map[string]bool
		start := time.Now()
		ctx, cancel := context.WithTimeout(testContext, 5*time.Second)
		defer cancel()
		err = s.write(ctx, func(tx *gorm.DB) error {
			var err error
			protected, err = retentionRetryProtection(tx, p.ID)
			return err
		})
		if err != nil || !protected[""] {
			t.Fatal("100000关系超限未闭锁", err)
		}
		if time.Since(start) > 5*time.Second {
			t.Fatal("限额SQL超过预算")
		}
	})
}

func TestRetentionRetryLongLegitimateChainDoesNotProtectUnrelated(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		if err := s.SyncGlobalRetention(testContext, config.Retention{Builds: 1, Days: 1}); err != nil {
			t.Fatal(err)
		}
		p, parent := cancelledQueued(t, s)
		unrelated := retentionSkipped(t, s, p, 1)[0]
		current := parent
		for i := 0; i < 70; i++ {
			result, err := s.Retry(testContext, localAdmin, RetryInput{BuildID: current, Key: fmt.Sprintf("chain-%d", i)})
			if err != nil {
				t.Fatal("真实长Retry链", err)
			}
			current = result.Builds[0].ID
			if i < 69 {
				if _, err = s.Cancel(testContext, localAdmin, current); err != nil {
					t.Fatal(err)
				}
			}
		}
		page, err := s.EvaluateRetention(testContext, localAdmin, p.ID, Page{Limit: 200})
		if err != nil {
			t.Fatal(err)
		}
		if slices.Contains(retentionEntry(t, page, unrelated).ProtectReasons, "retry_dependency") {
			t.Fatal("文件深度64误用于真实Retry链/污染无关历史")
		}
		if !slices.Contains(retentionEntry(t, page, parent).ProtectReasons, "retry_dependency") {
			t.Fatal("长链祖先丢保护")
		}
	})
}
