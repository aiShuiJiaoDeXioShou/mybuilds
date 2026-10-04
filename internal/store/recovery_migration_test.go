package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
)

// 这些具体模型逐字段复制自已验收的007；关系也指向旧模型，避免建夹具时提前添加008列。
type legacy007Build struct {
	ID                                                       string        `gorm:"primaryKey;size:36;index:build_status_created,priority:3"`
	BatchID                                                  string        `gorm:"not null;size:36"`
	Batch                                                    batchRecord   `gorm:"foreignKey:BatchID;constraint:OnDelete:RESTRICT"`
	ProjectID                                                string        `gorm:"not null;size:36;uniqueIndex:project_number"`
	Project                                                  projectRecord `gorm:"foreignKey:ProjectID;constraint:OnDelete:RESTRICT"`
	Number                                                   *int64        `gorm:"uniqueIndex:project_number"`
	Position                                                 int
	Name                                                     string `gorm:"not null"`
	Status                                                   string `gorm:"index:build_status_created,priority:1;index:build_node_status,priority:2;index:build_status_expiry,priority:1"`
	Reason, SnapshotJSON                                     string
	NodeID                                                   *string    `gorm:"index:build_node_status,priority:1;size:36"`
	Node                                                     nodeRecord `gorm:"foreignKey:NodeID;constraint:OnDelete:RESTRICT"`
	SessionID, AttemptID, LeaseID                            *string
	LeaseEpoch                                               int64      `gorm:"not null;default:0;check:lease_epoch >= 0"`
	LeaseExpiresAt                                           *time.Time `gorm:"index:build_status_expiry,priority:2"`
	CancelRequested                                          bool       `gorm:"not null;default:false"`
	StopUnconfirmed                                          bool       `gorm:"not null;default:false"`
	RemainingPostBudgetNS                                    int64
	PostPhase                                                string
	LastEventSeq, LastLogSeq, LastLogOffset, LastArtifactSeq int64
	ParameterKeysJSON, Condition, ReasonsJSON                string
	InitialBudgetNS, RemainingBudgetNS                       *int64
	PostBudgetNS                                             int64
	CreatedAt                                                time.Time `gorm:"index:build_status_created,priority:2"`
}

func (legacy007Build) TableName() string { return "builds" }

type legacy007Attempt struct {
	ID             string               `gorm:"primaryKey;size:36"`
	BuildID        string               `gorm:"not null;uniqueIndex;size:36"`
	Build          legacy007Build       `gorm:"foreignKey:BuildID;constraint:OnDelete:RESTRICT"`
	NodeID         string               `gorm:"not null;index;size:36"`
	Node           nodeRecord           `gorm:"foreignKey:NodeID;constraint:OnDelete:RESTRICT"`
	SessionID      string               `gorm:"not null;uniqueIndex:session_claim;size:36"`
	Session        nodeSessionRecord    `gorm:"foreignKey:SessionID;constraint:OnDelete:RESTRICT"`
	CredentialID   string               `gorm:"not null;size:36"`
	Credential     nodeCredentialRecord `gorm:"foreignKey:CredentialID;constraint:OnDelete:RESTRICT"`
	ClaimKey       string               `gorm:"not null;uniqueIndex:session_claim;size:36"`
	LeaseID        string               `gorm:"not null;uniqueIndex;size:36"`
	Epoch          int64                `gorm:"not null;check:epoch > 0"`
	LeaseExpiresAt time.Time
	CreatedAt      time.Time
}

func (legacy007Attempt) TableName() string { return "attempts" }

type legacy007Receipt struct {
	ID        string           `gorm:"primaryKey;size:36"`
	BuildID   string           `gorm:"not null;uniqueIndex:event_sequence;size:36"`
	Build     legacy007Build   `gorm:"foreignKey:BuildID;constraint:OnDelete:RESTRICT"`
	AttemptID string           `gorm:"not null;uniqueIndex:event_sequence;size:36"`
	Attempt   legacy007Attempt `gorm:"foreignKey:AttemptID;constraint:OnDelete:RESTRICT"`
	Seq       int64            `gorm:"not null;uniqueIndex:event_sequence;check:seq > 0"`
	Digest    string
	CreatedAt time.Time
}

func (legacy007Receipt) TableName() string { return "execution_receipts" }

// 每个数据库使用自有临时目录/schema，先创建真实007表，再调用正式Migrate。
func TestRecoveryMigrationFrom007(t *testing.T) {
	drivers := []string{"sqlite"}
	if os.Getenv("MYBUILDS_TEST_POSTGRES_DSN") != "" {
		drivers = append(drivers, "postgres")
	}
	for _, driver := range drivers {
		t.Run(driver, func(t *testing.T) {
			opt := Options{Driver: driver, DSN: filepath.Join(t.TempDir(), "control.db")}
			if driver == "postgres" {
				dsn := os.Getenv("MYBUILDS_TEST_POSTGRES_DSN")
				db, err := sql.Open("pgx", dsn)
				if err != nil {
					t.Fatal(err)
				}
				schema := fmt.Sprintf("recovery008_%d", time.Now().UnixNano())
				if _, err = db.Exec("CREATE SCHEMA " + schema); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { db.Exec("DROP SCHEMA " + schema + " CASCADE"); db.Close() })
				opt.DSN = dsn + " search_path=" + schema
			}
			s, err := Open(testContext, opt)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if err = s.writer.AutoMigrate(&legacy007Build{}, &legacy007Attempt{}, &legacy007Receipt{}); err != nil {
				t.Fatal(err)
			}
			for _, c := range []struct{ table, column string }{{"builds", "retry_of"}, {"execution_receipts", "kind"}, {"execution_receipts", "stop_known"}} {
				if s.db.Migrator().HasColumn(c.table, c.column) {
					t.Fatalf("007夹具提前包含 %s.%s", c.table, c.column)
				}
			}
			group := groupRecord{ID: "legacy-default", Name: "legacy-default"}
			if err = s.writer.Create(&group).Error; err != nil {
				t.Fatal(err)
			}
			project := projectRecord{ID: uuid.NewString(), Name: "legacy", GroupID: group.ID, NextNumber: 8, PolicyVersion: 1}
			if err = s.writer.Create(&project).Error; err != nil {
				t.Fatal(err)
			}
			batch := batchRecord{ID: uuid.NewString(), ProjectID: project.ID, SHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Branch: "main"}
			if err = s.writer.Create(&batch).Error; err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC().Truncate(time.Second)
			var original []legacy007Build
			for i, status := range []string{"queued", "skipped", "succeeded", "failed", "cancelled", "interrupted", "running"} {
				number := int64(i + 1)
				budget := int64(91000000017)
				remaining := int64(80000000013)
				row := legacy007Build{ID: uuid.NewString(), BatchID: batch.ID, ProjectID: project.ID, Number: &number, Name: fmt.Sprintf("legacy-%d", i), Status: status, Reason: "original_reason", SnapshotJSON: "{\"legacy\":true}", InitialBudgetNS: &budget, RemainingBudgetNS: &remaining, PostBudgetNS: 120000000000, RemainingPostBudgetNS: 110000000007, CreatedAt: now}
				if err = s.writer.Create(&row).Error; err != nil {
					t.Fatal(err)
				}
				original = append(original, row)
			}
			node := nodeRecord{ID: uuid.NewString(), Name: "legacy-node", State: "enabled", LabelsJSON: "[]", MaxCapacity: 1}
			if err = s.writer.Create(&node).Error; err != nil {
				t.Fatal(err)
			}
			credential := nodeCredentialRecord{ID: uuid.NewString(), NodeID: node.ID, Digest: "legacy-digest"}
			if err = s.writer.Create(&credential).Error; err != nil {
				t.Fatal(err)
			}
			session := nodeSessionRecord{ID: uuid.NewString(), NodeID: node.ID, CredentialID: credential.ID, OS: "linux", Arch: "amd64", ToolsJSON: "[]", Capacity: 1}
			if err = s.writer.Create(&session).Error; err != nil {
				t.Fatal(err)
			}
			attempt := legacy007Attempt{ID: uuid.NewString(), BuildID: original[2].ID, NodeID: node.ID, SessionID: session.ID, CredentialID: credential.ID, ClaimKey: uuid.NewString(), LeaseID: uuid.NewString(), Epoch: 1, LeaseExpiresAt: now.Add(time.Minute)}
			if err = s.writer.Create(&attempt).Error; err != nil {
				t.Fatal(err)
			}
			receipt := legacy007Receipt{ID: uuid.NewString(), BuildID: attempt.BuildID, AttemptID: attempt.ID, Seq: 1, Digest: "original_digest", CreatedAt: now}
			if err = s.writer.Create(&receipt).Error; err != nil {
				t.Fatal(err)
			}
			// 从数据库重新读取原值，以其真实驱动时间精度为比较基准。
			var before []legacy007Build
			if err = s.db.Select(legacyBuildColumns()).Order("id").Find(&before).Error; err != nil {
				t.Fatal(err)
			}
			var beforeReceipt legacy007Receipt
			if err = s.db.Select("id,build_id,attempt_id,seq,digest,created_at").First(&beforeReceipt, "id = ?", receipt.ID).Error; err != nil {
				t.Fatal(err)
			}
			for pass := 0; pass < 2; pass++ {
				if err = s.Migrate(testContext); err != nil {
					t.Fatalf("迁移%d: %v", pass, err)
				}
				for _, c := range []struct{ table, column string }{{"builds", "retry_of"}, {"execution_receipts", "kind"}, {"execution_receipts", "stop_known"}} {
					if !s.db.Migrator().HasColumn(c.table, c.column) {
						t.Fatalf("缺迁移列 %s.%s", c.table, c.column)
					}
				}
				if !s.db.Migrator().HasConstraint(&buildRecord{}, "RetryOriginal") || !s.db.Migrator().HasIndex(&buildRecord{}, "RetryOf") {
					t.Fatal("模型自关联FK或索引未被真实迁移识别")
				}
				var after []legacy007Build
				if err = s.db.Select(legacyBuildColumns()).Order("id").Find(&after).Error; err != nil || !reflect.DeepEqual(before, after) {
					t.Fatalf("旧构建被改写: %v", err)
				}
				var afterReceipt legacy007Receipt
				if err = s.db.Select("id,build_id,attempt_id,seq,digest,created_at").First(&afterReceipt, "id = ?", receipt.ID).Error; err != nil || !reflect.DeepEqual(beforeReceipt, afterReceipt) {
					t.Fatalf("旧回执被改写: %v", err)
				}
				var fields struct {
					Kind      string
					StopKnown bool
				}
				if err = s.db.Table("execution_receipts").Select("kind,stop_known").Where("id = ?", receipt.ID).Scan(&fields).Error; err != nil || fields.Kind != "" || fields.StopKnown {
					t.Fatalf("旧终态不能推断证据: %+v %v", fields, err)
				}
				var count int64
				if err = s.db.Table("builds").Where("retry_of IS NOT NULL").Count(&count).Error; err != nil || count != 0 {
					t.Fatalf("旧构建虚构重试关系: %v", err)
				}
			}
			for _, column := range []string{"kind", "stop_known"} {
				if err = s.writer.Table("execution_receipts").Where("id = ?", receipt.ID).Update(column, nil).Error; err == nil {
					t.Fatalf("回执证据列允许NULL: %s", column)
				}
			}
			newReceipt := legacy007Receipt{ID: uuid.NewString(), BuildID: receipt.BuildID, AttemptID: receipt.AttemptID, Seq: 2, Digest: "new_digest", CreatedAt: now}
			if err = s.writer.Create(&newReceipt).Error; err != nil {
				t.Fatal(err)
			}
			var defaults executionReceiptRecord
			if err = s.db.First(&defaults, "id = ?", newReceipt.ID).Error; err != nil || defaults.Kind != "" || defaults.StopKnown {
				t.Fatalf("新增回执默认值不安全: %v", err)
			}
			// 原queued没有其它从属记录：删除拒绝只能来自新建的自关联RESTRICT。
			parentID, childID := original[0].ID, original[1].ID
			if err = s.writer.Table("builds").Where("id = ?", childID).Update("retry_of", uuid.NewString()).Error; err == nil {
				t.Fatal("不存在原构建仍接受FK")
			}
			if err = s.writer.Table("builds").Where("id = ?", childID).Update("retry_of", parentID).Error; err != nil {
				t.Fatal(err)
			}
			if err = s.writer.Delete(&legacy007Build{}, "id = ?", parentID).Error; err == nil {
				t.Fatal("删除被重试引用的原构建未拒绝")
			}
			if err = s.writer.Delete(&legacy007Build{}, "id = ?", childID).Error; err != nil {
				t.Fatal(err)
			}
			if err = s.writer.Delete(&legacy007Build{}, "id = ?", parentID).Error; err != nil {
				t.Fatalf("无关联仍拒绝删除: %v", err)
			}
		})
	}
}

func legacyBuildColumns() []string {
	// 显式旧列投影，避免PG已有SELECT * prepared plan因合法DDL扩列改变结果形状。
	return []string{"id", "batch_id", "project_id", "number", "position", "name", "status", "reason", "snapshot_json", "node_id", "session_id", "attempt_id", "lease_id", "lease_epoch", "lease_expires_at", "cancel_requested", "stop_unconfirmed", "remaining_post_budget_ns", "post_phase", "last_event_seq", "last_log_seq", "last_log_offset", "last_artifact_seq", "parameter_keys_json", "condition", "reasons_json", "initial_budget_ns", "remaining_budget_ns", "post_budget_ns", "created_at"}
}
