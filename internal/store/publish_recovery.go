package store

import (
	"encoding/json"
	"gorm.io/gorm"
	"mybuilds/internal/protocol"
)

// 重启只核对原意图与持久保护；不重授、不因租约或进程停止清除unknown。
func validatePublishRecovery(tx *gorm.DB) error {
	cursor := ""
	for {
		rows := []publishIntentRecord{}
		if err := tx.Where("id > ?", cursor).Order("id").Limit(100).Find(&rows).Error; err != nil {
			return err
		}
		for _, intent := range rows {
			var grant protocol.PublishGrant
			var binding applicationRecord
			var row buildRecord
			if !validUUID(intent.ID) || !validDigest(intent.RequestDigest) || json.Unmarshal([]byte(intent.GrantJSON), &grant) != nil || grant.IntentID != intent.ID || !validRef(grant.Ref) || grant.Ref.BuildID != intent.BuildID || grant.Ref.AttemptID != intent.AttemptID {
				return errDatabase
			}
			digest, err := protocol.PublishGrantDigest(grant)
			if err != nil || digest != grant.AuthorizationDigest {
				return errDatabase
			}
			if tx.First(&binding, "id = ?", intent.BindingID).Error != nil || tx.First(&row, "id = ?", intent.BuildID).Error != nil || binding.ProjectID != row.ProjectID || binding.NodeID != grant.Ref.NodeID || binding.AppIdentifier != grant.AppIdentifier {
				return errDatabase
			}
			if intent.Status == "unknown" {
				var guard applicationGuardRecord
				if tx.First(&guard, "binding_id = ?", binding.ID).Error != nil {
					return errDatabase
				}
			}
			if intent.ReceiptDigest != "" {
				var receipt protocol.PublishReceipt
				if json.Unmarshal([]byte(intent.ReceiptJSON), &receipt) != nil || receipt.IntentID != intent.ID || receipt.AuthorizationDigest != grant.AuthorizationDigest {
					return errDatabase
				}
				d, e := protocol.PublishReceiptDigest(receipt)
				if e != nil || d != intent.ReceiptDigest {
					return errDatabase
				}
			}
			cursor = intent.ID
		}
		if len(rows) < 100 {
			break
		}
	}
	guards := []applicationGuardRecord{}
	if err := tx.Limit(retentionRelationLimit + 1).Find(&guards).Error; err != nil {
		return err
	}
	if len(guards) > retentionRelationLimit {
		return errDatabase
	}
	for _, g := range guards {
		var i publishIntentRecord
		if tx.First(&i, "id = ? AND binding_id = ?", g.IntentID, g.BindingID).Error != nil {
			return errDatabase
		}
	}
	return nil
}
func protectPublishHistory(tx *gorm.DB, id string) (bool, error) {
	var count int64
	err := tx.Model(&publishIntentRecord{}).Where("build_id = ? AND (status = ? OR binding_id IN (SELECT binding_id FROM application_guards))", id, "unknown").Count(&count).Error
	return count > 0, err
}

// 保留原grant的公开产物摘要；仅在真实退役事务中断开已确认的live文件外键。
func retirePublishArtifacts(tx *gorm.DB, id string) error {
	held, err := protectPublishHistory(tx, id)
	if err != nil {
		return err
	}
	if held {
		return ErrRetentionProtected
	}
	return tx.Model(&publishIntentRecord{}).Where("build_id = ?", id).Update("artifact_id", nil).Error
}
