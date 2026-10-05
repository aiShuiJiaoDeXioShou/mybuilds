package store

import (
	"github.com/google/uuid"
	"mybuilds/internal/protocol"
	"reflect"
	"testing"
)

// 迁移先验证真实旧执行收据，不把新业务状态手填成通过。
func TestApprovalMigrationKeepsActualOldEvidence(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, g, _ := claimed(t, s)
		seq := completeRunEvents(t, s, a, g, false)
		terminal := protocol.ExecutionProgress{Kind: "build_finished", Status: "succeeded", Started: true, StopConfirmed: true, RemainingPostBudgetNS: 119999999900, ArtifactSteps: []protocol.ArtifactExpectation{}}
		accept(t, s, a, event(g.Ref, seq+1, terminal))
		before := recoveryRows(t, s)
		if err := s.Migrate(testContext); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, recoveryRows(t, s)) {
			t.Fatal("迁移重写旧步骤或收据")
		}
		if !s.writer.Migrator().HasTable("approvals") {
			t.Fatal("缺少实际审批关系表")
		}
		if !s.writer.Migrator().HasColumn(&buildRecord{}, "CurrentApprovalID") {
			t.Fatal("缺少当前审批外键位置")
		}
	})
}

// 模型约束用真实已领取记录作父关系，只检查迁移约束，不伪造业务挂起。
func TestApprovalMigrationUniqueAndRestrict(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		_, g, _ := claimed(t, s)
		record := approvalRecord{ID: uuid.NewString(), BuildID: g.Ref.BuildID, AttemptID: g.Ref.AttemptID, NodeID: g.Ref.NodeID, OrdinaryIndex: 1, Revision: 1, State: "pending", CheckpointRefJSON: "{}"}
		if err := s.writer.Create(&record).Error; err != nil {
			t.Fatal("真实父关系的约束夹具无法写入")
		}
		duplicate := record
		duplicate.ID = uuid.NewString()
		duplicate.Revision = 2
		if s.writer.Create(&duplicate).Error == nil {
			t.Fatal("同build/index没有唯一约束")
		}
		duplicate = record
		duplicate.ID = uuid.NewString()
		duplicate.OrdinaryIndex = 2
		if s.writer.Create(&duplicate).Error == nil {
			t.Fatal("同build/revision没有唯一约束")
		}
		duplicate = record
		duplicate.ID = uuid.NewString()
		duplicate.OrdinaryIndex = 2
		duplicate.Revision = 0
		if s.writer.Create(&duplicate).Error == nil {
			t.Fatal("revision零值被接受")
		}
		duplicate = record
		duplicate.ID = uuid.NewString()
		duplicate.BuildID = uuid.NewString()
		if s.writer.Create(&duplicate).Error == nil {
			t.Fatal("没有真实build FK")
		}
		if s.writer.Delete(&attemptRecord{}, "id = ?", g.Ref.AttemptID).Error == nil {
			t.Fatal("审批父attempt可被删除")
		}
		if s.writer.Delete(&nodeRecord{}, "id = ?", g.Ref.NodeID).Error == nil {
			t.Fatal("审批父node可被删除")
		}
		if err := s.Migrate(testContext); err != nil {
			t.Fatal("重复迁移失败", err)
		}
		var count int64
		if err := s.db.Model(&approvalRecord{}).Where("id = ?", record.ID).Count(&count).Error; err != nil || count != 1 {
			t.Fatal("重复迁移丢失约束夹具")
		}
	})
}
