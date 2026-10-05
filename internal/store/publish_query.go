package store

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"mybuilds/internal/protocol"
)

func createPublishQuery(tx *gorm.DB, actor Actor, binding applicationRecord, intent *publishIntentRecord, kind string) error {
	var count int64
	if err := tx.Model(&publishQueryRecord{}).Where("binding_id = ? AND status = ?", binding.ID, "pending").Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrConflict
	}
	var id *string
	if intent != nil {
		id = &intent.ID
	}
	row := publishQueryRecord{ID: uuid.NewString(), BindingID: binding.ID, IntentID: id, Kind: kind, ActorID: actor.ID, Status: "pending", Nonce: uuid.NewString(), ResultJSON: "{}", ExpiresAt: time.Now().UTC().Add(30 * time.Second)}
	return tx.Create(&row).Error
}
func queryView(row publishQueryRecord) (PublishQueryView, error) {
	out := PublishQueryView{ID: row.ID, Kind: row.Kind, Status: row.Status, RequestedAt: row.CreatedAt.UTC(), CompletedAt: row.CompletedAt, Matches: []protocol.PublishMatch{}}
	if row.IntentID != nil {
		out.IntentID = *row.IntentID
	}
	if row.ResultJSON != "{}" {
		var result protocol.PublishQueryResult
		if json.Unmarshal([]byte(row.ResultJSON), &result) != nil {
			return out, errDatabase
		}
		out.Reason = result.Reason
		out.ObservedLifecycle = result.ObservedLifecycle
		out.Matches = result.Matches
	}
	return out, nil
}
func (s *Store) RequestPublishQuery(ctx context.Context, actor Actor, id string) (PublishQueryView, error) {
	if actor.Role != "admin" {
		return PublishQueryView{}, ErrForbidden
	}
	if !validUUID(id) {
		return PublishQueryView{}, ErrInvalid
	}
	var out PublishQueryView
	err := s.write(ctx, func(tx *gorm.DB) error {
		if err := authorize(tx, actor, "admin"); err != nil {
			return err
		}
		var intent publishIntentRecord
		if err := tx.First(&intent, "id = ?", id).Error; err != nil {
			return err
		}
		var binding applicationRecord
		if err := tx.First(&binding, "id = ?", intent.BindingID).Error; err != nil {
			return err
		}
		if binding.Store == "custom" {
			var grant protocol.PublishGrant
			if json.Unmarshal([]byte(intent.GrantJSON), &grant) != nil {
				return errDatabase
			}
			if _, err := customQueryContext(tx, intent, grant); err != nil {
				return err
			}
		}
		if err := createPublishQuery(tx, actor, binding, &intent, "query"); err != nil {
			return err
		}
		var query publishQueryRecord
		if err := tx.First(&query, "binding_id = ? AND status = ?", binding.ID, "pending").Error; err != nil {
			return err
		}
		var err error
		out, err = queryView(query)
		return err
	})
	return out, err
}
func (s *Store) RequestApplicationDoctor(ctx context.Context, actor Actor, id string) (PublishQueryView, error) {
	if actor.Role != "admin" {
		return PublishQueryView{}, ErrForbidden
	}
	if !validUUID(id) {
		return PublishQueryView{}, ErrInvalid
	}
	var out PublishQueryView
	err := s.write(ctx, func(tx *gorm.DB) error {
		if err := authorize(tx, actor, "admin"); err != nil {
			return err
		}
		var binding applicationRecord
		if err := tx.First(&binding, "id = ?", id).Error; err != nil {
			return err
		}
		if binding.Store == "custom" {
			return ErrInvalid
		}
		if err := createPublishQuery(tx, actor, binding, nil, "doctor"); err != nil {
			return err
		}
		var query publishQueryRecord
		if err := tx.First(&query, "binding_id = ? AND status = ?", binding.ID, "pending").Error; err != nil {
			return err
		}
		var e error
		out, e = queryView(query)
		return e
	})
	return out, err
}
func publishManagementSession(tx *gorm.DB, actor NodeActor, id string) (nodeRecord, error) {
	node, _, err := authorizeNode(tx, actor)
	if err != nil {
		return node, err
	}
	if node.State == "disabled" || node.SessionID == nil || *node.SessionID != id {
		return node, ErrSessionConflict
	}
	var session nodeSessionRecord
	if err = tx.First(&session, "id = ? AND node_id = ? AND credential_id = ?", id, node.ID, actor.CredentialID).Error; err != nil {
		return node, err
	}
	if session.HeartbeatNS <= 0 || time.Now().UTC().After(session.LastHeartbeat.Add(time.Duration(session.HeartbeatNS)*3)) {
		return node, ErrSessionExpired
	}
	return node, nil
}
func (s *Store) ClaimPublishQuery(ctx context.Context, actor NodeActor, sessionID string) (*protocol.PublishQueryTask, error) {
	if !validUUID(sessionID) {
		return nil, ErrInvalid
	}
	var task *protocol.PublishQueryTask
	err := s.write(ctx, func(tx *gorm.DB) error {
		if _, err := publishManagementSession(tx, actor, sessionID); err != nil {
			return err
		}
		// 已领取管理查询不会重派；过期只标记，不修改原intent或应用保护。
		if err := tx.Model(&publishQueryRecord{}).Where("binding_id IN (SELECT id FROM application_bindings WHERE node_id = ?) AND status = ? AND session_id <> ? AND expires_at < ?", actor.ID, "pending", "", time.Now().UTC()).Updates(map[string]any{"status": "expired"}).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&publishQueryRecord{}).Where("binding_id IN (SELECT id FROM application_bindings WHERE node_id = ?) AND status = ? AND session_id <> ?", actor.ID, "pending", "").Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return nil
		}
		var query publishQueryRecord
		err := tx.Where("binding_id IN (SELECT id FROM application_bindings WHERE node_id = ?) AND status = ? AND session_id = ?", actor.ID, "pending", "").Order("created_at,id").First(&query).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		var binding applicationRecord
		if err = tx.First(&binding, "id = ?", query.BindingID).Error; err != nil {
			return err
		}
		expiry := time.Now().UTC().Add(30 * time.Second)
		query.SessionID = sessionID
		query.ExpiresAt = expiry
		if err = tx.Model(&query).Updates(map[string]any{"session_id": sessionID, "expires_at": expiry}).Error; err != nil {
			return err
		}
		task = &protocol.PublishQueryTask{ID: query.ID, Nonce: query.Nonce, Kind: query.Kind, BindingID: binding.ID, Store: binding.Store, UploadCertificateSHA256: binding.UploadCertificateSHA256, NodeID: actor.ID, SessionID: sessionID, AppIdentifier: binding.AppIdentifier, CredentialRef: binding.CredentialRef, ExpiresAt: expiry}
		if query.IntentID != nil {
			var intent publishIntentRecord
			if err = tx.First(&intent, "id = ?", *query.IntentID).Error; err != nil {
				return err
			}
			var grant protocol.PublishGrant
			if json.Unmarshal([]byte(intent.GrantJSON), &grant) != nil {
				return errDatabase
			}
			task.IntentID = intent.ID
			task.Track = grant.Track
			task.ReleaseName = grant.ReleaseName
			task.VersionCode = grant.VersionCode
			task.VersionName = grant.VersionName
			var remote protocol.PublishRemoteEvidence
			if json.Unmarshal([]byte(intent.RemoteJSON), &remote) != nil {
				return errDatabase
			}
			task.Apple = remote.Apple
			task.AppleAuthorization = grant.Apple
			if binding.Store == "custom" {
				task.Custom, err = customQueryContext(tx, intent, grant)
				if err != nil {
					return err
				}
			}
			data, e := json.Marshal(task)
			if e != nil || len(data) > 64<<10 {
				return ErrInvalid
			}
		}
		return nil
	})
	return task, err
}

var publishQueryReasons = []string{"", "play_tools_missing", "play_tools_unverified", "play_credentials_invalid", "play_app_invalid", "play_unknown", "apple_tools_missing", "apple_tools_unverified", "apple_credentials_invalid", "apple_app_invalid", "apple_unknown", "cancelled", "timeout", "cleanup_error", "observation_insufficient"}

func validPublishQueryResult(in protocol.PublishQueryResult) bool {
	bytes, err := json.Marshal(in)
	if err != nil || len(bytes) > 64<<10 || !validUUID(in.ID) || !validUUID(in.Nonce) || !validUUID(in.BindingID) || !validUUID(in.NodeID) || !validUUID(in.SessionID) || !slices.Contains([]string{"doctor", "query"}, in.Kind) || in.Matches == nil || len(in.Matches) > 16 || in.ObservedAt.IsZero() || !slices.Contains(publishQueryReasons, in.Reason) {
		return false
	}
	if in.Kind == "doctor" && in.IntentID != "" || in.Kind == "query" && !validUUID(in.IntentID) {
		return false
	}
	for _, m := range in.Matches {
		if len(m.VersionCodes) > 64 || m.VersionCodes == nil || !boundedPublishEvidence(protocol.PublishRemoteEvidence{ReleaseName: m.ReleaseName, Track: m.Track, Lifecycle: m.Lifecycle, Apple: m.Apple, Custom: m.Custom}) {
			return false
		}
		for _, code := range m.VersionCodes {
			if code <= 0 || code > 2100000000 {
				return false
			}
		}
	}
	if !boundedPublishEvidence(protocol.PublishRemoteEvidence{Lifecycle: in.ObservedLifecycle}) {
		return false
	}
	if in.Kind == "doctor" {
		if len(in.DoctorChecks) > 16 {
			return false
		}
		seen := map[string]bool{}
		for _, check := range in.DoctorChecks {
			if seen[check.Name] || !validName(check.Name) || !slices.Contains([]string{"passed", "failed", "skipped"}, check.Status) || len(check.Version) > 64 || len(check.Reason) > 64 || strings.ContainsAny(check.Version+check.Reason, "/\\\x00\r\n") {
				return false
			}
			seen[check.Name] = true
		}
	}
	return true
}
func (s *Store) CompletePublishQuery(ctx context.Context, actor NodeActor, in protocol.PublishQueryResult) (PublishQueryView, error) {
	if !validPublishQueryResult(in) {
		return PublishQueryView{}, ErrInvalid
	}
	var out PublishQueryView
	err := s.write(ctx, func(tx *gorm.DB) error {
		if _, err := publishManagementSession(tx, actor, in.SessionID); err != nil {
			return err
		}
		if actor.ID != in.NodeID {
			return ErrForbidden
		}
		var query publishQueryRecord
		if err := tx.First(&query, "id = ? AND binding_id = ?", in.ID, in.BindingID).Error; err != nil {
			return err
		}
		expected := ""
		if query.IntentID != nil {
			expected = *query.IntentID
		}
		if query.Kind != in.Kind || query.Nonce != in.Nonce || query.SessionID != in.SessionID || expected != in.IntentID || !time.Now().UTC().Before(query.ExpiresAt) || in.ObservedAt.After(time.Now().UTC().Add(time.Second)) || in.ObservedAt.Before(query.CreatedAt) {
			return ErrConflict
		}
		var binding applicationRecord
		if err := tx.First(&binding, "id = ? AND node_id = ?", query.BindingID, actor.ID).Error; err != nil {
			return err
		}
		text, err := encode(in)
		if err != nil {
			return err
		}
		if query.Status != "pending" {
			if query.ResultJSON != text {
				return ErrConflict
			}
			out, err = queryView(query)
			return err
		}
		expiry := query.ExpiresAt
		s.transactionExpiry = &expiry
		if in.Kind == "doctor" {
			expected := []string{"fastlane", "bundletool", "google_play_credentials", "google_play_application"}
			if binding.Store == "app_store" {
				expected = []string{"fastlane", "apple_transport", "app_store_credentials", "app_store_application"}
			}
			passed := in.Reason == "" && validDigest(in.ToolLockDigest) && len(in.DoctorChecks) == len(expected)
			for _, check := range in.DoctorChecks {
				passed = passed && check.Status == "passed" && slices.Contains(expected, check.Name)
			}
			if passed {
				now := time.Now().UTC()
				if err = tx.Model(&binding).Updates(map[string]any{"status": "verified", "verified_at": now, "verification_source": "doctor_verified"}).Error; err != nil {
					return err
				}
			}
		}
		if in.Kind == "query" && binding.Store == "custom" && in.Reason == "" {
			var intent publishIntentRecord
			if err = tx.First(&intent, "id = ?", in.IntentID).Error; err != nil {
				return err
			}
			var grant protocol.PublishGrant
			if json.Unmarshal([]byte(intent.GrantJSON), &grant) != nil {
				return errDatabase
			}
			if !exactCustomQuery(in.Custom, grant) || len(in.Matches) != 1 || !slices.Equal(in.Matches[0].VersionCodes, []int64{grant.VersionCode}) || in.Matches[0].Custom == nil || !reflect.DeepEqual(*in.Matches[0].Custom, in.Custom.Remote) || in.Matches[0].Lifecycle != in.Custom.Remote.Lifecycle || in.Matches[0].Apple != nil {
				return ErrConflict
			}
			if intent.Status == "unknown" {
				if err = confirmObservedPublish(tx, query, intent, in.Custom.Remote.Lifecycle, protocol.PublishRemoteEvidence{Custom: &in.Custom.Remote}); err != nil {
					return err
				}
			}
		} else if in.Custom != nil {
			return ErrInvalid
		} else if in.Kind == "query" && binding.Store == "app_store" && in.Reason == "" && len(in.Matches) == 1 {
			var intent publishIntentRecord
			if err = tx.First(&intent, "id = ?", in.IntentID).Error; err != nil {
				return err
			}
			var grant protocol.PublishGrant
			var original protocol.PublishRemoteEvidence
			if json.Unmarshal([]byte(intent.GrantJSON), &grant) != nil || json.Unmarshal([]byte(intent.RemoteJSON), &original) != nil {
				return errDatabase
			}
			if intent.Status == "unknown" && exactAppleQuery(grant, original.Apple, in.Matches[0]) {
				if err = confirmObservedPublish(tx, query, intent, "confirmed", protocol.PublishRemoteEvidence{Apple: in.Matches[0].Apple}); err != nil {
					return err
				}
			}
		}
		// 缺充分原授权证据的GET只保存观察，不把空结果或同版本推为已确认。
		query.Status = "completed"
		if in.Reason != "" {
			query.Status = "failed"
		}
		now := time.Now().UTC()
		query.CompletedAt = &now
		query.ResultJSON = text
		if err = tx.Model(&query).Updates(map[string]any{"status": query.Status, "completed_at": now, "result_json": text}).Error; err != nil {
			return err
		}
		out, err = queryView(query)
		return err
	})
	return out, err
}
func (s *Store) GetPublishQuery(ctx context.Context, id string) (PublishQueryView, error) {
	if !validUUID(id) {
		return PublishQueryView{}, ErrInvalid
	}
	if err := s.CheckLock(ctx); err != nil {
		return PublishQueryView{}, err
	}
	var row publishQueryRecord
	if err := s.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		return PublishQueryView{}, safeError(err)
	}
	return queryView(row)
}
func (s *Store) ConfirmPublish(ctx context.Context, actor Actor, in ConfirmPublishInput) (PublishView, error) {
	if actor.Role != "admin" {
		return PublishView{}, ErrForbidden
	}
	if !validUUID(in.IntentID) || !validUUID(in.Key) || !validDigest(in.ExpectedIntentDigest) || !validDigest(in.EvidenceSHA256) || !slices.Contains([]string{"uploaded", "processing", "submitted", "published", "failed", "confirmed"}, in.Outcome) || !slices.Contains([]string{"remote_receipt", "remote_state", "confirmed_not_sent", "remote_rejected"}, in.EvidenceCode) || len(in.Note) < 1 || len(in.Note) > 2048 || !boundedPublishEvidence(in.RemoteEvidence) {
		return PublishView{}, ErrInvalid
	}
	for _, r := range in.Note {
		if unicode.IsControl(r) {
			return PublishView{}, ErrInvalid
		}
	}
	if in.Outcome == "failed" && !slices.Contains([]string{"confirmed_not_sent", "remote_rejected"}, in.EvidenceCode) || in.Outcome != "failed" && !slices.Contains([]string{"remote_receipt", "remote_state"}, in.EvidenceCode) {
		return PublishView{}, ErrInvalid
	}
	var out PublishView
	err := s.write(ctx, func(tx *gorm.DB) error {
		if err := authorize(tx, actor, "admin"); err != nil {
			return err
		}
		text, _ := encode(in)
		var decision publishDecisionRecord
		err := tx.First(&decision, "actor_id = ? AND key = ?", actor.ID, in.Key).Error
		if err == nil {
			if decision.InputJSON != text {
				return ErrConflict
			}
			var intent publishIntentRecord
			if err = tx.First(&intent, "id = ?", in.IntentID).Error; err != nil {
				return err
			}
			out, err = publishView(tx, intent)
			return err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var intent publishIntentRecord
		if err = tx.First(&intent, "id = ?", in.IntentID).Error; err != nil {
			return err
		}
		if intent.Status != "unknown" || publishIntentDigest(intent) != in.ExpectedIntentDigest {
			return ErrConflict
		}
		var grant protocol.PublishGrant
		if json.Unmarshal([]byte(intent.GrantJSON), &grant) != nil {
			return errDatabase
		}
		remote := in.RemoteEvidence
		if in.Outcome != "failed" {
			if grant.Custom != nil {
				if remote.Custom == nil || !validCustomEvidence(remote.Custom) || !remote.Custom.ActionConfirmed || remote.Custom.RemoteID == "" || remote.Custom.Lifecycle != in.Outcome || remote.Apple != nil {
					return ErrInvalid
				}
			} else if grant.Apple == nil {
				if remote.BundleSHA256 != grant.ArtifactSHA256 || remote.VersionCode != grant.VersionCode || remote.Track != grant.Track || remote.ReleaseName != grant.ReleaseName || !remote.CommitAccepted {
					return ErrInvalid
				}
			} else if !validAppleActionEvidence(grant, remote.Apple) || in.Outcome != "confirmed" {
				return ErrInvalid

			}
		}
		remoteJSON, _ := encode(remote)
		if err = tx.Create(&publishDecisionRecord{ID: uuid.NewString(), IntentID: intent.ID, ActorID: actor.ID, Key: in.Key, Digest: in.EvidenceSHA256, InputJSON: text}).Error; err != nil {
			return err
		}
		if err = tx.Model(&intent).Updates(map[string]any{"status": in.Outcome, "remote_json": remoteJSON, "evidence_code": in.EvidenceCode}).Error; err != nil {
			return err
		}
		// 同链还有其它未知动作时，单动作人工确认不能释放整个应用。
		var unknown int64
		if err = tx.Model(&publishIntentRecord{}).Where("binding_id = ? AND status = ?", intent.BindingID, "unknown").Count(&unknown).Error; err != nil {
			return err
		}
		if unknown == 0 {
			var row buildRecord
			var slot stepRecord
			if err = tx.First(&row, "id = ?", intent.BuildID).Error; err != nil {
				return err
			}
			if err = tx.First(&slot, "build_id = ? AND phase = ? AND \"index\" = ?", row.ID, "ordinary", intent.StepIndex).Error; err != nil {
				return err
			}
			if err = closePublishStep(tx, row, slot); err != nil {
				return err
			}
		}
		intent.Status = in.Outcome
		intent.RemoteJSON = remoteJSON
		intent.EvidenceCode = in.EvidenceCode
		out, err = publishView(tx, intent)
		return err
	})
	return out, err
}
