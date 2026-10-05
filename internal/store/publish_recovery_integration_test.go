package store

import (
	"testing"

	"github.com/google/uuid"
)

func TestPublishUnknownSurvivesRecoveryAndConstraints(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		a, _, in := publishFixture(t, s)
		grant, err := s.AuthorizePublish(testContext, a, in)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.Recover(testContext); err != nil {
			t.Fatal("真实重启核对原未知", err)
		}
		view, err := s.GetPublish(testContext, in.IntentID)
		if err != nil || view.Status != "unknown" || !view.ApplicationProtected {
			t.Fatal("恢复以停止或过期抹unknown", err)
		}
		var row publishIntentRecord
		if err = s.db.First(&row, "id = ?", in.IntentID).Error; err != nil {
			t.Fatal(err)
		}
		copyRow := row
		copyRow.ID = uuid.NewString()
		if err = s.writer.Create(&copyRow).Error; err == nil {
			t.Fatal("原attempt/action第二持久意图")
		}
		var binding applicationRecord
		if err = s.db.First(&binding, "id = ?", row.BindingID).Error; err != nil {
			t.Fatal(err)
		}
		copyBinding := binding
		copyBinding.ID = uuid.NewString()
		if err = s.writer.Create(&copyBinding).Error; err == nil {
			t.Fatal("应用跨项目重复归属")
		}
		if err = s.writer.Delete(&binding).Error; err == nil {
			t.Fatal("原guard引用binding可被删")
		}
		if err = s.writer.Model(&publishIntentRecord{}).Where("id = ?", row.ID).Update("grant_json", `{}`).Error; err != nil {
			t.Fatal(err)
		}
		if err = s.Recover(testContext); err == nil {
			t.Fatal("损坏授权凭空恢复")
		}
		view, err = s.GetPublish(testContext, grant.IntentID)
		if err != nil || view.Status != "unknown" || !view.ApplicationProtected {
			t.Fatal("拒绝损坏恢复后丢失原未知保护", err)
		}
	})
}
