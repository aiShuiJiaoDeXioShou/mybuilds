package store

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"mybuilds/internal/protocol"
)

func TestRetentionResourceCompletionMigrationKeepsOldRowsUnknown(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		a, g, _ := claimed(t, s)
		registration := protocol.NodeResourceRegistration{Ref: g.Ref, ID: uuid.NewString(), OwnershipDigest: strings.Repeat("a", 64), HasWorkspace: true, HasResults: true}
		if err := s.RegisterNodeResource(testContext, a, registration); err != nil {
			t.Fatal(err)
		}
		// 仅重建本次旧资源列版本；保留真实FK/归属，不修改任何业务终态。
		for _, query := range []string{"ALTER TABLE node_resources DROP COLUMN completed_at", "ALTER TABLE node_resources DROP COLUMN completion_json"} {
			if err := s.writer.Exec(query).Error; err != nil {
				t.Fatal("自有旧版迁移夹具", safeError(err))
			}
		}
		if err := s.Migrate(testContext); err != nil {
			t.Fatal("真实迁移", err)
		}
		for i := 0; i < 2; i++ {
			var row struct {
				CompletedAt                             *time.Time
				CompletionJSON                          string
				ID, OwnershipDigest, BuildID, AttemptID string
			}
			// PG旧SELECT *计划不可跨列变化；显式取新列与必要身份。
			if err := s.db.Table("node_resources").Select("completed_at, completion_json, id, ownership_digest, build_id, attempt_id").Where("id = ?", registration.ID).Take(&row).Error; err != nil {
				t.Fatal("读取迁移默认", safeError(err))
			}
			if row.CompletedAt != nil || row.CompletionJSON != "" || row.OwnershipDigest != registration.OwnershipDigest || row.BuildID != g.Ref.BuildID || row.AttemptID != g.Ref.AttemptID {
				t.Fatal("旧归属被冒充已完成或身份变更")
			}
			if err := s.Migrate(testContext); err != nil {
				t.Fatal("重复迁移", err)
			}
		}
	})
}
