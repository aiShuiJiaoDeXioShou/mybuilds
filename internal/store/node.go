package store

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"mybuilds/internal/protocol"
)

func validUUID(id string) bool {
	u, e := uuid.Parse(id)
	return e == nil && u != uuid.Nil && u.String() == id
}
func validLabels(labels []string) bool {
	if len(labels) > 64 {
		return false
	}
	seen := map[string]bool{}
	for _, label := range labels {
		if label == "" || len(label) > 64 || strings.TrimSpace(label) != label || seen[label] {
			return false
		}
		for _, r := range label {
			if unicode.IsControl(r) {
				return false
			}
		}
		seen[label] = true
	}
	return true
}
func newNodeCredential(nodeID string) (nodeCredentialRecord, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nodeCredentialRecord{}, "", errDatabase
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	return nodeCredentialRecord{ID: uuid.NewString(), NodeID: nodeID, Digest: tokenDigest(token), CreatedAt: time.Now().UTC()}, token, nil
}
func authorizeNode(db *gorm.DB, actor NodeActor) (nodeRecord, nodeCredentialRecord, error) {
	var node nodeRecord
	var credential nodeCredentialRecord
	if !validUUID(actor.ID) || !validUUID(actor.CredentialID) {
		return node, credential, ErrNodeUnauthorized
	}
	err := db.First(&credential, "id = ? AND node_id = ? AND revoked_at IS NULL", actor.CredentialID, actor.ID).Error
	if err == gorm.ErrRecordNotFound {
		return node, credential, ErrNodeUnauthorized
	}
	if err != nil {
		return node, credential, err
	}
	err = db.First(&node, "id = ? AND state <> ?", actor.ID, "deleted").Error
	if err == gorm.ErrRecordNotFound {
		return node, credential, ErrNodeUnauthorized
	}
	return node, credential, err
}
func nodeView(db *gorm.DB, node nodeRecord) (NodeView, error) {
	view := NodeView{ID: node.ID, Name: node.Name, State: node.State, MaxCapacity: node.MaxCapacity, CreatedAt: node.CreatedAt.UTC(), Labels: []string{}, Tools: []protocol.ToolCheck{}}
	if err := json.Unmarshal([]byte(node.LabelsJSON), &view.Labels); err != nil {
		return view, errDatabase
	}
	if view.Labels == nil {
		view.Labels = []string{}
	}
	var occupied, guard int64
	if err := db.Model(&buildRecord{}).Where("node_id = ? AND (status = ? OR stop_unconfirmed = ?)", node.ID, "running", true).Count(&occupied).Error; err != nil {
		return view, err
	}
	if err := db.Model(&buildRecord{}).Where("node_id = ? AND stop_unconfirmed = ?", node.ID, true).Count(&guard).Error; err != nil {
		return view, err
	}
	view.Running = int(occupied)
	view.Quarantined = guard > 0
	if node.SessionID != nil {
		var session nodeSessionRecord
		if err := db.First(&session, "id = ? AND node_id = ?", *node.SessionID, node.ID).Error; err != nil {
			return view, err
		}
		view.OS, view.Arch, view.LocalCapacity = session.OS, session.Arch, session.Capacity
		view.EffectiveCapacity = min(node.MaxCapacity, session.Capacity)
		if err := json.Unmarshal([]byte(session.ToolsJSON), &view.Tools); err != nil {
			return view, errDatabase
		}
		if view.Tools == nil {
			view.Tools = []protocol.ToolCheck{}
		}
		heartbeat := session.LastHeartbeat.UTC()
		view.LastHeartbeat = &heartbeat
		var active int64
		if err := db.Model(&nodeCredentialRecord{}).Where("id = ? AND node_id = ? AND revoked_at IS NULL", session.CredentialID, node.ID).Count(&active).Error; err != nil {
			return view, err
		}
		now := time.Now().UTC()
		view.SessionActive = active == 1 && now.Before(heartbeat.Add(2*time.Duration(session.LeaseNS)))
		view.Healthy = active == 1 && now.Before(heartbeat.Add(3*time.Duration(session.HeartbeatNS)))
	}
	return view, nil
}

// 明确撤销执行权立即保留物理停止保护，不因随后enable而让旧fence复活。
func interruptNodeExecutions(db *gorm.DB, nodeID string, credentials []string) error {
	query := db.Model(&buildRecord{}).Where("node_id = ? AND status = ?", nodeID, "running")
	if credentials != nil {
		if len(credentials) == 0 {
			return nil
		}
		attempts := db.Model(&attemptRecord{}).Select("id").Where("node_id = ? AND credential_id IN ?", nodeID, credentials)
		query = query.Where("attempt_id IN (?)", attempts)
	}
	return query.Updates(map[string]any{"status": "interrupted", "stop_unconfirmed": true, "terminal_at": gorm.Expr("COALESCE(terminal_at, ?)", time.Now().UTC()), "reason": gorm.Expr("CASE WHEN reason = '' OR reason IS NULL THEN ? ELSE reason END", "authority_lost")}).Error
}
func activeNodeCredentials(db *gorm.DB, nodeID string) ([]string, error) {
	ids := []string{}
	err := db.Model(&nodeCredentialRecord{}).Where("node_id = ? AND revoked_at IS NULL", nodeID).Pluck("id", &ids).Error
	return ids, err
}

func (s *Store) CreateNode(ctx context.Context, actor Actor, in NodeInput) (NodeCreated, error) {
	if !validName(in.Name) || !validLabels(in.Labels) || in.Capacity < 1 || in.Capacity > 32 {
		return NodeCreated{}, ErrInvalid
	}
	if in.Labels == nil {
		in.Labels = []string{}
	}
	labels, err := encode(in.Labels)
	if err != nil {
		return NodeCreated{}, err
	}
	now := time.Now().UTC()
	node := nodeRecord{ID: uuid.NewString(), Name: in.Name, State: "enabled", LabelsJSON: labels, MaxCapacity: in.Capacity, CreatedAt: now, UpdatedAt: now}
	credential, token, err := newNodeCredential(node.ID)
	if err != nil {
		return NodeCreated{}, err
	}
	var view NodeView
	err = s.write(ctx, func(tx *gorm.DB) error {
		if e := authorize(tx, actor, "admin"); e != nil {
			return e
		}
		if e := tx.Create(&node).Error; e != nil {
			return e
		}
		if e := tx.Create(&credential).Error; e != nil {
			return e
		}
		if e := audit(tx, actor, "node_create", node.ID, "", "enabled"); e != nil {
			return e
		}
		var e error
		view, e = nodeView(tx, node)
		return e
	})
	if err != nil {
		return NodeCreated{}, err
	}
	return NodeCreated{Node: view, Token: token}, nil
}
func (s *Store) ListNodes(ctx context.Context, actor Actor, f NodeFilter) ([]NodeView, error) {
	page, err := normalizePage(f.Page)
	if err != nil {
		return nil, err
	}
	if err = s.CheckLock(ctx); err != nil {
		return nil, err
	}
	db := s.db.WithContext(ctx)
	if err = authorize(db, actor, "admin"); err != nil {
		return nil, safeError(err)
	}
	var nodes []nodeRecord
	if err = db.Where("state <> ?", "deleted").Order("created_at DESC, id DESC").Limit(page.Limit).Offset(page.Offset).Find(&nodes).Error; err != nil {
		return nil, safeError(err)
	}
	result := make([]NodeView, 0, len(nodes))
	for _, node := range nodes {
		view, e := nodeView(db, node)
		if e != nil {
			return nil, safeError(e)
		}
		result = append(result, view)
	}
	return result, nil
}
func (s *Store) GetNode(ctx context.Context, actor Actor, name string) (NodeView, error) {
	if !validName(name) {
		return NodeView{}, ErrInvalid
	}
	if err := s.CheckLock(ctx); err != nil {
		return NodeView{}, err
	}
	db := s.db.WithContext(ctx)
	if err := authorize(db, actor, "admin"); err != nil {
		return NodeView{}, safeError(err)
	}
	var node nodeRecord
	if err := db.First(&node, "name = ? AND state <> ?", name, "deleted").Error; err != nil {
		return NodeView{}, safeError(err)
	}
	view, err := nodeView(db, node)
	return view, safeError(err)
}
func (s *Store) SetNodeState(ctx context.Context, actor Actor, name, state string) error {
	if !validName(name) || !slices.Contains([]string{"enabled", "draining", "disabled"}, state) {
		return ErrInvalid
	}
	return s.write(ctx, func(tx *gorm.DB) error {
		if err := authorize(tx, actor, "admin"); err != nil {
			return err
		}
		var node nodeRecord
		if err := tx.First(&node, "name = ? AND state <> ?", name, "deleted").Error; err != nil {
			return err
		}
		if node.State == state {
			return nil
		}
		previous := node.State
		if state == "disabled" {
			if err := interruptNodeExecutions(tx, node.ID, nil); err != nil {
				return err
			}
		}
		if err := tx.Model(&node).Updates(map[string]any{"state": state, "updated_at": time.Now().UTC()}).Error; err != nil {
			return err
		}
		return audit(tx, actor, "node_state", node.ID, previous, state)
	})
}
func (s *Store) RotateNodeToken(ctx context.Context, actor Actor, name string) (NodeCreated, error) {
	if !validName(name) {
		return NodeCreated{}, ErrInvalid
	}
	var result NodeCreated
	err := s.write(ctx, func(tx *gorm.DB) error {
		if err := authorize(tx, actor, "admin"); err != nil {
			return err
		}
		var node nodeRecord
		if err := tx.First(&node, "name = ? AND state <> ?", name, "deleted").Error; err != nil {
			return err
		}
		credential, token, err := newNodeCredential(node.ID)
		if err != nil {
			return err
		}
		ids, e := activeNodeCredentials(tx, node.ID)
		if e != nil {
			return e
		}
		if e = interruptNodeExecutions(tx, node.ID, ids); e != nil {
			return e
		}
		if err = tx.Model(&nodeCredentialRecord{}).Where("node_id = ? AND revoked_at IS NULL", node.ID).Update("revoked_at", time.Now().UTC()).Error; err != nil {
			return err
		}
		if err = tx.Create(&credential).Error; err != nil {
			return err
		}
		view, err := nodeView(tx, node)
		if err != nil {
			return err
		}
		result = NodeCreated{Node: view, Token: token}
		return audit(tx, actor, "node_token_rotate", node.ID, "", "")
	})
	if err != nil {
		return NodeCreated{}, err
	}
	return result, nil
}
func (s *Store) RevokeNodeToken(ctx context.Context, actor Actor, name string) error {
	if !validName(name) {
		return ErrInvalid
	}
	return s.write(ctx, func(tx *gorm.DB) error {
		if err := authorize(tx, actor, "admin"); err != nil {
			return err
		}
		var node nodeRecord
		if err := tx.First(&node, "name = ? AND state <> ?", name, "deleted").Error; err != nil {
			return err
		}
		ids, err := activeNodeCredentials(tx, node.ID)
		if err != nil {
			return err
		}
		if err = interruptNodeExecutions(tx, node.ID, ids); err != nil {
			return err
		}
		r := tx.Model(&nodeCredentialRecord{}).Where("node_id = ? AND revoked_at IS NULL", node.ID).Update("revoked_at", time.Now().UTC())
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 0 {
			return nil
		}
		return audit(tx, actor, "node_token_revoke", node.ID, "", "")
	})
}
func (s *Store) DeleteNode(ctx context.Context, actor Actor, name string) error {
	if !validName(name) {
		return ErrInvalid
	}
	return s.write(ctx, func(tx *gorm.DB) error {
		if err := authorize(tx, actor, "admin"); err != nil {
			return err
		}
		var node nodeRecord
		if err := tx.First(&node, "name = ? AND state <> ?", name, "deleted").Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&buildRecord{}).Where("node_id = ? AND (status = ? OR stop_unconfirmed = ?)", node.ID, "running", true).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrConflict
		}
		var waiting []buildRecord
		if err := tx.Where("status = ?", "queued").Find(&waiting).Error; err != nil {
			return err
		}
		for _, b := range waiting {
			var snapshot BuildSnapshot
			if json.Unmarshal([]byte(b.SnapshotJSON), &snapshot) != nil {
				return errDatabase
			}
			if snapshot.DefaultNode == name || slices.Contains(snapshot.AllowedNodes, name) {
				return ErrConflict
			}
		}
		if err := tx.Model(&node).Updates(map[string]any{"state": "deleted", "updated_at": time.Now().UTC()}).Error; err != nil {
			return err
		}
		if err := tx.Model(&nodeCredentialRecord{}).Where("node_id = ? AND revoked_at IS NULL", node.ID).Update("revoked_at", time.Now().UTC()).Error; err != nil {
			return err
		}

		return audit(tx, actor, "node_delete", node.ID, node.State, "deleted")
	})
}
func (s *Store) AuthenticateNode(ctx context.Context, token string) (NodeActor, error) {
	if err := s.CheckLock(ctx); err != nil {
		return NodeActor{}, err
	}
	if !validToken(token) {
		return NodeActor{}, ErrNodeUnauthorized
	}
	db := s.db.WithContext(ctx)
	var credential nodeCredentialRecord
	if err := db.First(&credential, "digest = ? AND revoked_at IS NULL", tokenDigest(token)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return NodeActor{}, ErrNodeUnauthorized
		}
		return NodeActor{}, safeError(err)
	}
	actor := NodeActor{ID: credential.NodeID, CredentialID: credential.ID}
	_, _, err := authorizeNode(db, actor)
	if err != nil {
		return NodeActor{}, safeError(err)
	}
	return actor, nil
}
