package store

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"mybuilds/internal/protocol"
	"slices"
)

// 不以版本列表猜原写动作。每个可核对动作都要求原授权的具体ID及真实GET关联。
func exactAppleQuery(g protocol.PublishGrant, original *protocol.AppleRemoteEvidence, m protocol.PublishMatch) bool {
	a, r := g.Apple, m.Apple
	if a == nil || r == nil || !r.ActionConfirmed || r.AppID == "" || r.RequestSHA256 != a.RequestSHA256 || !validDigest(r.ResponseSHA256) || !slices.Equal(m.VersionCodes, []int64{g.VersionCode}) {
		return false
	}
	switch a.Action {
	case "select_build":
		return a.AppStoreVersionID != "" && a.BuildID != "" && r.AppStoreVersionID == a.AppStoreVersionID && r.BuildID == a.BuildID
	case "set_release_policy":
		policy := "MANUAL"
		if a.AutomaticRelease {
			policy = "AFTER_APPROVAL"
		}
		return a.AppStoreVersionID != "" && r.AppStoreVersionID == a.AppStoreVersionID && r.ReleaseType == policy
	case "create_review":
		return original != nil && original.ReviewSubmissionID != "" && original.RequestSHA256 == a.RequestSHA256 && validDigest(original.ResponseSHA256) && r.ReviewSubmissionID == original.ReviewSubmissionID
	case "add_review_item":
		return original != nil && original.ReviewItemID != "" && original.RequestSHA256 == a.RequestSHA256 && validDigest(original.ResponseSHA256) && r.ReviewItemID == original.ReviewItemID && a.AppStoreVersionID != "" && r.AppStoreVersionID == a.AppStoreVersionID && a.ReviewSubmissionID != "" && r.ReviewSubmissionID == a.ReviewSubmissionID
	case "submit_review":
		return a.AppStoreVersionID != "" && a.BuildID != "" && a.ReviewSubmissionID != "" && a.ReviewItemID != "" && r.AppStoreVersionID == a.AppStoreVersionID && r.BuildID == a.BuildID && r.ReviewSubmissionID == a.ReviewSubmissionID && r.ReviewItemID == a.ReviewItemID && slices.Contains([]string{"WAITING_FOR_REVIEW", "IN_REVIEW", "COMPLETING", "COMPLETE"}, r.ReviewState)
	default:
		return false // upload_binary GET列表不含原transport充分关联。
	}
}

func validAppleActionEvidence(g protocol.PublishGrant, r *protocol.AppleRemoteEvidence) bool {
	a := g.Apple
	if a == nil || r == nil || !r.ActionConfirmed || r.AppID == "" || r.RequestSHA256 != a.RequestSHA256 || !validDigest(r.ResponseSHA256) {
		return false
	}
	switch a.Action {
	case "upload_binary":
		return validUUID(r.TransportID) && r.UploadedAt != nil && !r.UploadedAt.IsZero()
	case "select_build":
		return a.AppStoreVersionID != "" && a.BuildID != "" && r.AppStoreVersionID == a.AppStoreVersionID && r.BuildID == a.BuildID
	case "set_release_policy":
		policy := "MANUAL"
		if a.AutomaticRelease {
			policy = "AFTER_APPROVAL"
		}
		return a.AppStoreVersionID != "" && r.AppStoreVersionID == a.AppStoreVersionID && r.ReleaseType == policy
	case "create_review":
		return r.ReviewSubmissionID != ""
	case "add_review_item":
		return r.ReviewItemID != "" && a.ReviewSubmissionID != "" && r.ReviewSubmissionID == a.ReviewSubmissionID && a.AppStoreVersionID != "" && r.AppStoreVersionID == a.AppStoreVersionID
	case "submit_review":
		return a.ReviewSubmissionID != "" && a.AppStoreVersionID != "" && a.BuildID != "" && a.ReviewItemID != "" && r.AppStoreVersionID == a.AppStoreVersionID && r.BuildID == a.BuildID && r.ReviewItemID == a.ReviewItemID && r.ReviewSubmissionID == a.ReviewSubmissionID && slices.Contains([]string{"WAITING_FOR_REVIEW", "IN_REVIEW", "COMPLETING", "COMPLETE"}, r.ReviewState)
	}
	return false
}

// 原管理query只在完整精确证据下确认原意图，不新增grant，也不改原执行回执。
func confirmObservedPublish(tx *gorm.DB, query publishQueryRecord, intent publishIntentRecord, status string, remote protocol.PublishRemoteEvidence) error {
	data, e := encode(remote)
	if e != nil {
		return e
	}
	input, e := encode(struct{ QueryID, Status, RemoteJSON string }{query.ID, status, data})
	if e != nil {
		return e
	}
	digest := sha256.Sum256([]byte(input))
	if e = tx.Create(&publishDecisionRecord{ID: uuid.NewString(), IntentID: intent.ID, ActorID: query.ActorID, Key: query.ID, Digest: hex.EncodeToString(digest[:]), InputJSON: input}).Error; e != nil {
		return e
	}
	if e = tx.Model(&intent).Updates(map[string]any{"status": status, "remote_json": data, "evidence_code": "remote_state"}).Error; e != nil {
		return e
	}
	if e = audit(tx, Actor{ID: query.ActorID, Role: "admin"}, "publish_query_confirmed", intent.ID, "", ""); e != nil {
		return e
	}
	var row buildRecord
	var slot stepRecord
	if e = tx.First(&row, "id = ?", intent.BuildID).Error; e != nil {
		return e
	}
	if e = tx.First(&slot, "build_id = ? AND phase = ? AND \"index\" = ?", row.ID, "ordinary", intent.StepIndex).Error; e != nil {
		return e
	}
	return closePublishStep(tx, row, slot)
}
