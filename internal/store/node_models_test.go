package store

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
	"testing"
)

func TestNodeModelsMigration(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		if err := s.Migrate(testContext); err != nil {
			t.Fatal(err)
		}
		for _, table := range []string{"nodes", "node_credentials", "node_sessions", "attempts", "execution_receipts", "stop_confirmations", "log_chunks", "artifacts"} {
			if !s.db.Migrator().HasTable(table) {
				t.Fatalf("missing table: %s", table)
			}
		}
		project, err := s.CreateProject(testContext, localAdmin, projectInput("model"))
		if err != nil {
			t.Fatal(err)
		}
		batch := batchRecord{ID: uuid.NewString(), ProjectID: project.ID}
		if err := s.writer.Create(&batch).Error; err != nil {
			t.Fatal(err)
		}
		// 未分配身份必须为NULL；同名排队不占唯一保护。
		create := func(id, status string, guard bool) error {
			return s.writer.Table("builds").Create(map[string]any{"id": id, "batch_id": batch.ID, "project_id": project.ID, "name": "app", "status": status, "stop_unconfirmed": guard, "lease_epoch": 0}).Error
		}
		first, second := uuid.NewString(), uuid.NewString()
		if err := create(first, "queued", false); err != nil {
			t.Fatal(err)
		}
		if err := create(second, "queued", false); err != nil {
			t.Fatal(err)
		}
		var count int64
		if err := s.db.Table("builds").Where("node_id IS NULL AND session_id IS NULL AND attempt_id IS NULL AND lease_id IS NULL AND lease_expires_at IS NULL").Count(&count).Error; err != nil || count != 2 {
			t.Fatalf("nullable identity: %d %v", count, err)
		}
		if err := s.writer.Table("builds").Where("id = ?", first).Update("status", "running").Error; err != nil {
			t.Fatal(err)
		}
		if err := s.writer.Table("builds").Where("id = ?", second).Update("status", "running").Error; safeError(err) != ErrConflict {
			t.Fatalf("same-name constraint: %v", safeError(err))
		}
		if err := s.writer.Table("builds").Where("id = ?", first).Updates(map[string]any{"status": "interrupted", "stop_unconfirmed": true}).Error; err != nil {
			t.Fatal(err)
		}
		if err := s.writer.Table("builds").Where("id = ?", second).Update("status", "running").Error; safeError(err) != ErrConflict {
			t.Fatal("guard lost unique protection")
		}
		if err := s.writer.Table("builds").Where("id = ?", first).Update("stop_unconfirmed", false).Error; err != nil {
			t.Fatal(err)
		}
		if err := s.writer.Table("builds").Where("id = ?", second).Update("status", "running").Error; err != nil {
			t.Fatal(err)
		}
		for _, index := range []string{"build_status_created", "build_node_status", "build_status_expiry", "build_active_name"} {
			if !s.db.Migrator().HasIndex("builds", index) {
				t.Fatalf("missing index: %s", index)
			}
		}
		if err := s.write(testContext, func(tx *gorm.DB) error {
			if e := tx.Table("nodes").Create(map[string]any{"id": uuid.NewString(), "name": "rollback", "state": "enabled", "max_capacity": 1, "labels_json": "[]"}).Error; e != nil {
				return e
			}
			return ErrInvalid
		}); err != ErrInvalid {
			t.Fatal(err)
		}
		s.db.Table("nodes").Where("name = ?", "rollback").Count(&count)
		if count != 0 {
			t.Fatal("rollback persisted")
		}
	})
}

func TestNodeModelMetadataConstraints(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		actor, session := openSession(t, s, "linux", 1)
		p, err := s.CreateProject(testContext, localAdmin, projectInput("model-meta"))
		if err != nil {
			t.Fatal(err)
		}
		queued, err := s.Enqueue(testContext, enqueueInput(p, "meta"))
		if err != nil {
			t.Fatal(err)
		}
		buildID := queued.Builds[0].ID
		attempt := attemptRecord{ID: uuid.NewString(), BuildID: buildID, NodeID: actor.ID, SessionID: session.SessionID, CredentialID: actor.CredentialID, ClaimKey: uuid.NewString(), LeaseID: uuid.NewString(), Epoch: 1}
		if err = s.write(testContext, func(tx *gorm.DB) error { return tx.Create(&attempt).Error }); err != nil {
			t.Fatal(err)
		}
		if err = s.writer.Delete(&buildRecord{}, "id = ?", buildID).Error; safeError(err) != ErrConflict {
			t.Fatal("build attempt restrict", safeError(err))
		}
		if err = s.writer.Delete(&nodeSessionRecord{}, "id = ?", session.SessionID).Error; safeError(err) != ErrConflict {
			t.Fatal("session attempt restrict", safeError(err))
		}
		receipt := executionReceiptRecord{ID: uuid.NewString(), BuildID: buildID, AttemptID: attempt.ID, Seq: 1, Digest: "digest"}
		if err = s.writer.Create(&receipt).Error; err != nil {
			t.Fatal(err)
		}
		receipt.ID = uuid.NewString()
		if err = s.writer.Create(&receipt).Error; safeError(err) != ErrConflict {
			t.Fatal("receipt sequence", safeError(err))
		}
		receipt.ID = uuid.NewString()
		receipt.AttemptID = uuid.NewString()
		if err = s.writer.Create(&receipt).Error; safeError(err) != ErrConflict {
			t.Fatal("receipt FK", safeError(err))
		}
		log := logChunkRecord{ID: uuid.NewString(), BuildID: buildID, AttemptID: attempt.ID, Seq: 1}
		if err = s.writer.Create(&log).Error; err != nil {
			t.Fatal(err)
		}
		log.ID = uuid.NewString()
		if err = s.writer.Create(&log).Error; safeError(err) != ErrConflict {
			t.Fatal("log sequence", safeError(err))
		}
		artifact := artifactRecord{ID: uuid.NewString(), BuildID: buildID, AttemptID: attempt.ID, Seq: 1}
		if err = s.writer.Create(&artifact).Error; err != nil {
			t.Fatal(err)
		}
		artifact.ID = uuid.NewString()
		if err = s.writer.Create(&artifact).Error; safeError(err) != ErrConflict {
			t.Fatal("artifact sequence", safeError(err))
		}
		if err = s.writer.Model(&buildRecord{}).Where("id = ?", buildID).Update("lease_epoch", -1).Error; safeError(err) != ErrConflict {
			t.Fatal("negative epoch", safeError(err))
		}
		if err = s.Migrate(testContext); err != nil {
			t.Fatal("repeat migrate populated", err)
		}
	})
}
