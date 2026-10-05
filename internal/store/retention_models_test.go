package store

import (
	"maps"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
)

var retentionTables = []string{"retention_policies", "retention_jobs", "retention_objects", "evidence_reads", "node_resources", "node_deletions", "node_deletion_receipts"}

func TestRetentionModelsMigrationFrom019(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		actor, session := openSession(t, s, "retention-model", 1)
		project, err := s.CreateProject(testContext, localAdmin, projectInput("retention-model"))
		if err != nil {
			t.Fatal(err)
		}
		parent := queueBuild(t, s, project, "retention-parent", "parent", false)
		child := queueBuild(t, s, project, "retention-child", "child", false)
		if err = s.writer.Table("builds").Where("id = ?", child).Update("retry_of", parent).Error; err != nil {
			t.Fatal("旧019 retry关系夹具", safeError(err))
		}
		attempt := attemptRecord{ID: uuid.NewString(), BuildID: child, NodeID: actor.ID, SessionID: session.SessionID, CredentialID: actor.CredentialID, ClaimKey: uuid.NewString(), LeaseID: uuid.NewString(), Epoch: 1}
		if err = s.writer.Create(&attempt).Error; err != nil {
			t.Fatal("旧attempt夹具", safeError(err))
		}
		receipt := executionReceiptRecord{ID: uuid.NewString(), BuildID: child, AttemptID: attempt.ID, Seq: 1, Kind: "step_finished", StopKnown: true, Digest: "original-terminal-digest", CreatedAt: time.Now().UTC()}
		if err = s.writer.Create(&receipt).Error; err != nil {
			t.Fatal("旧receipt夹具", safeError(err))
		}
		var beforeReceipt executionReceiptRecord
		if err = s.db.First(&beforeReceipt, "id = ?", receipt.ID).Error; err != nil {
			t.Fatal("读取旧receipt夹具", safeError(err))
		}
		// 移除新列/新空表，恢复真实019已有列、数据与FK；不关闭FK或重建父表。
		for i := len(retentionTables) - 1; i >= 0; i-- {
			if s.db.Migrator().HasTable(retentionTables[i]) {
				if err = s.writer.Migrator().DropTable(retentionTables[i]); err != nil {
					t.Fatal("移除新空表夹具", safeError(err))
				}
			}
		}
		for _, column := range []string{"terminal_at", "history_state", "cleaned_at"} {
			if s.db.Migrator().HasColumn("builds", column) {
				if err = s.writer.Exec("ALTER TABLE builds DROP COLUMN " + column).Error; err != nil {
					t.Fatal("移除新列夹具", safeError(err))
				}
			}
		}
		before := retentionBuildRow(t, s, child)
		for pass := 0; pass < 2; pass++ {
			if err = s.Migrate(testContext); err != nil {
				t.Fatal("019→020迁移", err)
			}
			for _, column := range []string{"terminal_at", "history_state", "cleaned_at"} {
				if !s.db.Migrator().HasColumn("builds", column) {
					t.Fatalf("缺少020列 builds.%s", column)
				}
			}
			after := retentionBuildRow(t, s, child)
			if after["terminal_at"] != nil || after["cleaned_at"] != nil || after["history_state"] != "live" {
				t.Fatal("旧无可信终态依据不能虚构时间/清理证据")
			}
			for _, column := range []string{"terminal_at", "history_state", "cleaned_at"} {
				delete(after, column)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatal("迁移改写019已有快照/报告/编号/预算/关联")
			}
			var current executionReceiptRecord
			if err = s.db.First(&current, "id = ?", receipt.ID).Error; err != nil || current.Kind != beforeReceipt.Kind || !current.StopKnown || current.Seq != beforeReceipt.Seq || current.Digest != beforeReceipt.Digest || !current.CreatedAt.Equal(beforeReceipt.CreatedAt) {
				t.Fatal("迁移改写008精确回执", safeError(err))
			}
		}
		if err = s.writer.Table("builds").Where("id = ?", parent).Delete(nil).Error; safeError(err) != ErrConflict {
			t.Fatal("RetryOf RESTRICT被破坏", safeError(err))
		}
		if err = s.writer.Table("builds").Where("id = ?", child).Delete(nil).Error; safeError(err) != ErrConflict {
			t.Fatal("attempt/receipt RESTRICT被破坏", safeError(err))
		}
	})
}

func retentionBuildRow(t *testing.T, s *Store, id string) map[string]any {
	t.Helper()
	// 沿既有迁移测试显式列投影；合法DDL前后不复用PG的SELECT *结果类型。
	columns := append(legacyBuildColumns(), "retry_of", "report_revision", "report_final", "reports_json", "report_seal_digest", "report_checked_index")
	for _, column := range []string{"terminal_at", "history_state", "cleaned_at"} {
		if s.db.Migrator().HasColumn("builds", column) {
			columns = append(columns, column)
		}
	}
	var row map[string]any
	if err := s.db.Table("builds").Select(columns).Where("id = ?", id).Take(&row).Error; err != nil {
		t.Fatal("读取迁移夹具", safeError(err))
	}
	return row
}

func TestRetentionModelsUniqueAndRestrict(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		for _, table := range retentionTables {
			if !s.db.Migrator().HasTable(table) {
				t.Fatalf("缺少020表 %s", table)
			}
		}
		actor, session := openSession(t, s, "retention-unique", 1)
		project, err := s.CreateProject(testContext, localAdmin, projectInput("retention-unique"))
		if err != nil {
			t.Fatal(err)
		}
		build := queueBuild(t, s, project, "retention-unique", "unique", false)
		attempt := attemptRecord{ID: uuid.NewString(), BuildID: build, NodeID: actor.ID, SessionID: session.SessionID, CredentialID: actor.CredentialID, ClaimKey: uuid.NewString(), LeaseID: uuid.NewString(), Epoch: 1}
		if err = s.writer.Create(&attempt).Error; err != nil {
			t.Fatal("关联夹具", safeError(err))
		}
		create := func(table string, row map[string]any) {
			t.Helper()
			// GORM会向map回写@id，不能让回写改变下一次真实约束请求。
			if e := s.writer.Table(table).Create(maps.Clone(row)).Error; e != nil {
				t.Fatalf("创建%s夹具: %v", table, safeError(e))
			}
		}
		conflict := func(table string, row map[string]any) {
			t.Helper()
			if e := s.writer.Table(table).Create(maps.Clone(row)).Error; safeError(e) != ErrConflict {
				t.Fatalf("%s没有拒绝唯一/FK冲突: %v", table, safeError(e))
			}
		}
		job := map[string]any{"id": uuid.NewString(), "project_id": project.ID, "build_id": build, "state": "pending", "reason": "", "global_version": 1, "project_version": 1}
		create("retention_jobs", job)
		jobID := job["id"].(string)
		job["id"] = uuid.NewString()
		conflict("retention_jobs", job)
		job["build_id"] = uuid.NewString()
		conflict("retention_jobs", job)
		object := map[string]any{"id": uuid.NewString(), "job_id": jobID, "kind": "artifact", "object_id": uuid.NewString(), "storage_id": uuid.NewString(), "state": "pending", "reason": "", "size": 123, "sha256": "original"}
		create("retention_objects", object)
		object["id"] = uuid.NewString()
		conflict("retention_objects", object)
		object["object_id"], object["job_id"] = uuid.NewString(), uuid.NewString()
		conflict("retention_objects", object)
		resource := map[string]any{"id": uuid.NewString(), "build_id": build, "attempt_id": attempt.ID, "node_id": actor.ID, "session_id": session.SessionID, "lease_id": attempt.LeaseID, "epoch": 1, "ownership_digest": "original", "has_workspace": true, "has_results": false}
		create("node_resources", resource)
		resourceID := resource["id"].(string)
		resource["id"] = uuid.NewString()
		conflict("node_resources", resource)
		resource["attempt_id"] = uuid.NewString()
		conflict("node_resources", resource)
		deletion := map[string]any{"id": uuid.NewString(), "job_id": jobID, "resource_id": resourceID, "node_id": actor.ID, "build_id": build, "attempt_id": attempt.ID, "ownership_digest": "original", "state": "pending", "reason": "", "seq": 0}
		create("node_deletions", deletion)
		deleteID := deletion["id"].(string)
		deletion["id"] = uuid.NewString()
		conflict("node_deletions", deletion)
		deletion["resource_id"] = uuid.NewString()
		conflict("node_deletions", deletion)
		confirmation := map[string]any{"id": uuid.NewString(), "delete_id": deleteID, "seq": 1, "nonce": "original", "digest": "original", "workspace_state": "deleted", "results_state": "not_applicable", "reason": ""}
		create("node_deletion_receipts", confirmation)
		confirmation["id"] = uuid.NewString()
		conflict("node_deletion_receipts", confirmation)
		confirmation["seq"] = 0
		conflict("node_deletion_receipts", confirmation)
		confirmation["seq"], confirmation["delete_id"] = 2, uuid.NewString()
		conflict("node_deletion_receipts", confirmation)
		create("evidence_reads", map[string]any{"id": uuid.NewString(), "build_id": build, "kind": "artifact", "object_id": uuid.NewString(), "storage_id": uuid.NewString(), "owner": uuid.NewString(), "state": "pending"})
		conflict("evidence_reads", map[string]any{"id": uuid.NewString(), "build_id": uuid.NewString(), "kind": "artifact", "object_id": uuid.NewString(), "storage_id": uuid.NewString(), "owner": uuid.NewString(), "state": "pending"})
		for _, target := range []struct{ table, id string }{{"retention_jobs", jobID}, {"node_resources", resourceID}, {"node_deletions", deleteID}, {"attempts", attempt.ID}, {"nodes", actor.ID}, {"node_sessions", session.SessionID}, {"builds", build}, {"projects", project.ID}} {
			if e := s.writer.Table(target.table).Where("id = ?", target.id).Delete(nil).Error; safeError(e) != ErrConflict {
				t.Fatalf("%s没有保留RESTRICT: %v", target.table, safeError(e))
			}
		}
	})
}
