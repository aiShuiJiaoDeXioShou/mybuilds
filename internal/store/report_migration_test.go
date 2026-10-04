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

// 引用既有007模型，确保夹具不会因当前模型关系提前包含新报告列。
type reportLegacyArtifact struct {
	ID                                   string           `gorm:"primaryKey;size:36"`
	BuildID                              string           `gorm:"not null;size:36"`
	Build                                legacy007Build   `gorm:"foreignKey:BuildID;constraint:OnDelete:RESTRICT"`
	AttemptID                            string           `gorm:"not null;uniqueIndex:artifact_sequence;size:36"`
	Attempt                              legacy007Attempt `gorm:"foreignKey:AttemptID;constraint:OnDelete:RESTRICT"`
	Seq                                  int64            `gorm:"not null;check:seq > 0;uniqueIndex:artifact_sequence"`
	Phase, Step, Name, SHA256, StorageID string
	Index                                int
	Size                                 int64
	CreatedAt                            time.Time
}

func (reportLegacyArtifact) TableName() string { return "artifacts" }

func TestReportMigrationFromAcceptedSchemas(t *testing.T) {
	drivers := []string{"sqlite"}
	if os.Getenv("MYBUILDS_TEST_POSTGRES_DSN") != "" {
		drivers = append(drivers, "postgres")
	}
	for _, driver := range drivers {
		for _, version := range []string{"007", "008"} {
			t.Run(driver+"/"+version, func(t *testing.T) {
				opt := Options{Driver: driver, DSN: filepath.Join(t.TempDir(), "control.db")}
				if driver == "postgres" {
					dsn := os.Getenv("MYBUILDS_TEST_POSTGRES_DSN")
					db, err := sql.Open("pgx", dsn)
					if err != nil {
						t.Fatal(err)
					}
					schema := fmt.Sprintf("reports019_%d", time.Now().UnixNano())
					if _, err = db.Exec("CREATE SCHEMA " + schema); err != nil {
						db.Close()
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
				if err = s.writer.AutoMigrate(&legacy007Build{}, &legacy007Attempt{}, &legacy007Receipt{}, &reportLegacyArtifact{}); err != nil {
					t.Fatal(err)
				}
				if version == "008" {
					for _, statement := range []string{
						"ALTER TABLE builds ADD COLUMN retry_of TEXT CONSTRAINT fk_builds_retry_original REFERENCES builds(id) ON DELETE RESTRICT",
						"ALTER TABLE execution_receipts ADD COLUMN kind TEXT NOT NULL DEFAULT ''",
						"ALTER TABLE execution_receipts ADD COLUMN stop_known BOOLEAN NOT NULL DEFAULT false",
					} {
						if err = s.writer.Exec(statement).Error; err != nil {
							t.Fatal(err)
						}
					}
				}
				group := groupRecord{ID: "legacy", Name: "legacy"}
				if err = s.writer.Create(&group).Error; err != nil {
					t.Fatal(err)
				}
				project := projectRecord{ID: uuid.NewString(), Name: "legacy", GroupID: group.ID, NextNumber: 8, PolicyVersion: 1}
				if err = s.writer.Create(&project).Error; err != nil {
					t.Fatal(err)
				}
				batch := batchRecord{ID: uuid.NewString(), ProjectID: project.ID}
				if err = s.writer.Create(&batch).Error; err != nil {
					t.Fatal(err)
				}
				number, budget := int64(7), int64(12345678917)
				build := legacy007Build{ID: uuid.NewString(), ProjectID: project.ID, BatchID: batch.ID, Name: "legacy", Number: &number, Status: "succeeded", Reason: "original", SnapshotJSON: `{"old":true}`, RemainingBudgetNS: &budget, CreatedAt: time.Now().UTC()}
				if err = s.writer.Create(&build).Error; err != nil {
					t.Fatal(err)
				}
				node := nodeRecord{ID: uuid.NewString(), Name: "legacy", State: "enabled", LabelsJSON: "[]", MaxCapacity: 1}
				if err = s.writer.Create(&node).Error; err != nil {
					t.Fatal(err)
				}
				credential := nodeCredentialRecord{ID: uuid.NewString(), NodeID: node.ID, Digest: "legacy"}
				if err = s.writer.Create(&credential).Error; err != nil {
					t.Fatal(err)
				}
				session := nodeSessionRecord{ID: uuid.NewString(), NodeID: node.ID, CredentialID: credential.ID, OS: "linux", Arch: "arm64", ToolsJSON: "[]", Capacity: 1}
				if err = s.writer.Create(&session).Error; err != nil {
					t.Fatal(err)
				}
				attempt := legacy007Attempt{ID: uuid.NewString(), BuildID: build.ID, NodeID: node.ID, SessionID: session.ID, CredentialID: credential.ID, ClaimKey: uuid.NewString(), LeaseID: uuid.NewString(), Epoch: 1}
				if err = s.writer.Create(&attempt).Error; err != nil {
					t.Fatal(err)
				}
				file := reportLegacyArtifact{ID: uuid.NewString(), BuildID: build.ID, AttemptID: attempt.ID, Seq: 1, Phase: "ordinary", Step: "artifact", Name: "old.xml", SHA256: "original_hash", StorageID: uuid.NewString(), Index: 2, Size: 123}
				if err = s.writer.Create(&file).Error; err != nil {
					t.Fatal(err)
				}
				receipt := legacy007Receipt{ID: uuid.NewString(), BuildID: build.ID, AttemptID: attempt.ID, Seq: 1, Digest: "original_digest"}
				if err = s.writer.Create(&receipt).Error; err != nil {
					t.Fatal(err)
				}
				expectedKind := ""
				expectedStopKnown := false
				if version == "008" {
					expectedKind, expectedStopKnown = "build_finished", true
					if err = s.writer.Table("execution_receipts").Where("id = ?", receipt.ID).Updates(map[string]any{"kind": expectedKind, "stop_known": expectedStopKnown}).Error; err != nil {
						t.Fatal(err)
					}
				}
				var before legacy007Build
				if err = s.db.Select(legacyBuildColumns()).First(&before, "id = ?", build.ID).Error; err != nil {
					t.Fatal(err)
				}
				var beforeFile reportLegacyArtifact
				oldFileColumns := []string{"id", "build_id", "attempt_id", "seq", "phase", "step", "name", "sha256", "storage_id", "index", "size", "created_at"}
				if err = s.db.Select(oldFileColumns).First(&beforeFile, "id = ?", file.ID).Error; err != nil {
					t.Fatal(err)
				}
				columns := map[string][]string{"builds": {"report_revision", "report_final", "reports_json", "report_seal_digest", "report_checked_index"}, "artifacts": {"purpose", "report_revision", "report_key", "verified_junit_json"}}
				for table, names := range columns {
					for _, name := range names {
						if s.db.Migrator().HasColumn(table, name) {
							t.Fatalf("旧夹具提前包含%s.%s", table, name)
						}
					}
				}
				for pass := 0; pass < 2; pass++ {
					if err = s.Migrate(testContext); err != nil {
						t.Fatal(err)
					}
					for table, names := range columns {
						for _, name := range names {
							if !s.db.Migrator().HasColumn(table, name) {
								t.Fatalf("缺少报告迁移%s.%s", table, name)
							}
						}
					}
					var after legacy007Build
					if err = s.db.Select(legacyBuildColumns()).First(&after, "id = ?", build.ID).Error; err != nil || !reflect.DeepEqual(before, after) {
						t.Fatalf("改写旧构建/预算: %v", err)
					}
					var afterFile reportLegacyArtifact
					if err = s.db.Select(oldFileColumns).First(&afterFile, "id = ?", file.ID).Error; err != nil || !reflect.DeepEqual(beforeFile, afterFile) {
						t.Fatalf("改写旧文件: %v", err)
					}
					var defaults struct {
						Revision     int64
						Final        bool
						JSON, Digest string
						Checked      int
					}
					if err = s.db.Table("builds").Select("report_revision AS revision,report_final AS final,reports_json AS json,report_seal_digest AS digest,report_checked_index AS checked").Where("id = ?", build.ID).Scan(&defaults).Error; err != nil || !reflect.DeepEqual(defaults, struct {
						Revision     int64
						Final        bool
						JSON, Digest string
						Checked      int
					}{}) {
						t.Fatalf("旧构建虚构报告证据: %+v %v", defaults, err)
					}
					var purposes struct {
						Purpose, ReportKey, Verified string
						Revision                     int64
					}
					if err = s.db.Table("artifacts").Select("purpose,report_key,verified_junit_json AS verified,report_revision AS revision").Where("id = ?", file.ID).Scan(&purposes).Error; err != nil || !reflect.DeepEqual(purposes, struct {
						Purpose, ReportKey, Verified string
						Revision                     int64
					}{}) {
						t.Fatalf("旧XML被猜成报告: %+v %v", purposes, err)
					}
					var old executionReceiptRecord
					if err = s.db.First(&old, "id = ?", receipt.ID).Error; err != nil || old.Digest != receipt.Digest || old.Kind != expectedKind || old.StopKnown != expectedStopKnown {
						t.Fatalf("旧回执被改写: %v", err)
					}
				}
				if err = s.writer.Delete(&legacy007Build{}, "id = ?", build.ID).Error; err == nil {
					t.Fatal("迁移破坏旧attempt/file的FK RESTRICT")
				}
				if !s.db.Migrator().HasConstraint(&buildRecord{}, "RetryOriginal") || !s.db.Migrator().HasIndex(&buildRecord{}, "RetryOf") {
					t.Fatal("丢失008关系约束")
				}
			})
		}
	}
}
