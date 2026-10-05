package store

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"
	"unicode"

	"gorm.io/gorm"
	"mybuilds/internal/protocol"
)

func validLeasePolicy(p LeasePolicy) bool {
	return p.Concurrency >= 1 && p.Heartbeat >= time.Second && p.Heartbeat <= 30*time.Second && p.Duration >= 10*time.Second && p.Duration <= 180*time.Second && p.Duration >= 4*p.Heartbeat+2*time.Second
}
func validNodeReport(report protocol.NodeReport) bool {
	validArch := report.OS == "darwin" && slices.Contains([]string{"amd64", "arm64"}, report.Arch) || report.OS == "linux" && slices.Contains([]string{"386", "amd64", "arm", "arm64", "loong64", "mips", "mips64", "mips64le", "mipsle", "ppc64", "ppc64le", "riscv64", "s390x"}, report.Arch)
	if !validArch || report.Capacity < 1 || report.Capacity > 32 || len(report.Tools) > 32 {
		return false
	}
	names := []string{"shell", "git", "java", "android_aapt2", "android_apksigner", "xcode", "ios_signing", "node_journal", "flutter", "dart", "cocoapods"}
	reasons := []string{"", "tool_missing", "tool_error", "tool_version_invalid", "tool_incompatible", "tool_timeout", "cancelled", "output_limit", "cleanup_error", "unsupported", "uninitialized", "journal_unconfirmed", "data_invalid", "data_locked"}
	seen := map[string]bool{}
	for _, tool := range report.Tools {
		if !slices.Contains(names, tool.Name) || seen[tool.Name] || !slices.Contains([]string{"passed", "failed", "skipped"}, tool.Status) || !slices.Contains(reasons, tool.Reason) || len(tool.Version) > 128 {
			return false
		}
		seen[tool.Name] = true
		if tool.Version != "" {
			if strings.Trim(tool.Version, "0123456789.") != "" || strings.HasPrefix(tool.Version, ".") || strings.HasSuffix(tool.Version, ".") || strings.Contains(tool.Version, "..") {
				return false
			}
		}
		for _, c := range tool.Version {
			if unicode.IsControl(c) {
				return false
			}
		}
		// 非macOS节点不能自报原生签名能力。
		if tool.Name == "ios_signing" && tool.Status == "passed" {
			xcode := false
			for _, other := range report.Tools {
				xcode = xcode || other.Name == "xcode" && other.Status == "passed"
			}
			if report.OS != "darwin" || !xcode {
				return false
			}
		}
	}
	return true
}
func sessionGrant(node nodeRecord, session nodeSessionRecord) protocol.SessionGrant {
	return protocol.SessionGrant{NodeID: node.ID, NodeName: node.Name, SessionID: session.ID, HeartbeatNS: session.HeartbeatNS, LeaseNS: session.LeaseNS}
}
func (s *Store) OpenNodeSession(ctx context.Context, actor NodeActor, in protocol.SessionRequest, policy LeasePolicy) (protocol.SessionGrant, error) {
	if !validUUID(in.SessionID) || !validLeasePolicy(policy) || in.HeartbeatNS != int64(policy.Heartbeat) || in.LeaseNS != int64(policy.Duration) || !validNodeReport(in.Report) {
		return protocol.SessionGrant{}, ErrInvalid
	}
	tools, err := encode(in.Report.Tools)
	if err != nil {
		return protocol.SessionGrant{}, err
	}
	var grant protocol.SessionGrant
	err = s.write(ctx, func(tx *gorm.DB) error {
		node, _, err := authorizeNode(tx, actor)
		if err != nil {
			return err
		}
		if node.State == "disabled" {
			return ErrNodeUnauthorized
		}
		now := time.Now().UTC()
		if node.SessionID != nil {
			var old nodeSessionRecord
			if err = tx.First(&old, "id = ? AND node_id = ?", *node.SessionID, node.ID).Error; err != nil {
				return err
			}
			if old.ID == in.SessionID {
				if old.CredentialID != actor.CredentialID || old.HeartbeatNS != in.HeartbeatNS || old.LeaseNS != in.LeaseNS {
					return ErrSessionConflict
				}
				grant = sessionGrant(node, old)
				return nil
			}
			if now.Before(old.LastHeartbeat.Add(2 * time.Duration(old.LeaseNS))) {
				return ErrSessionConflict
			}
			var count int64
			if err = tx.Model(&buildRecord{}).Where("node_id = ? AND (status = ? OR stop_unconfirmed = ?)", node.ID, "running", true).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return ErrSessionConflict
			}
		}
		var previous int64
		if err = tx.Model(&nodeSessionRecord{}).Where("id = ?", in.SessionID).Count(&previous).Error; err != nil {
			return err
		}
		if previous != 0 {
			return ErrSessionConflict
		}
		session := nodeSessionRecord{ID: in.SessionID, NodeID: node.ID, CredentialID: actor.CredentialID, OS: in.Report.OS, Arch: in.Report.Arch, Capacity: in.Report.Capacity, ToolsJSON: tools, HeartbeatNS: in.HeartbeatNS, LeaseNS: in.LeaseNS, CreatedAt: now, LastHeartbeat: now}
		if err = tx.Create(&session).Error; err != nil {
			return err
		}
		if err = tx.Model(&node).Updates(map[string]any{"session_id": session.ID, "updated_at": now}).Error; err != nil {
			return err
		}
		grant = sessionGrant(node, session)
		_, _, err = authorizeNode(tx, actor)
		return err
	})
	if err != nil {
		return protocol.SessionGrant{}, err
	}
	return grant, nil
}
func currentSession(db *gorm.DB, node nodeRecord, actor NodeActor, id string, policy LeasePolicy) (nodeSessionRecord, error) {
	var session nodeSessionRecord
	if node.State == "disabled" {
		return session, ErrNodeUnauthorized
	}
	if node.SessionID == nil || *node.SessionID != id {
		return session, ErrSessionExpired
	}
	if err := db.First(&session, "id = ? AND node_id = ? AND credential_id = ?", id, node.ID, actor.CredentialID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return session, ErrSessionExpired
		}
		return session, err
	}
	if session.HeartbeatNS != int64(policy.Heartbeat) || session.LeaseNS != int64(policy.Duration) {
		return session, ErrSessionConflict
	}
	return session, nil
}
func (s *Store) Heartbeat(ctx context.Context, actor NodeActor, in protocol.HeartbeatRequest, policy LeasePolicy) (protocol.SessionGrant, error) {
	if !validUUID(in.SessionID) || !validLeasePolicy(policy) || !validNodeReport(in.Report) {
		return protocol.SessionGrant{}, ErrInvalid
	}
	tools, err := encode(in.Report.Tools)
	if err != nil {
		return protocol.SessionGrant{}, err
	}
	var grant protocol.SessionGrant
	err = s.write(ctx, func(tx *gorm.DB) error {
		node, _, err := authorizeNode(tx, actor)
		if err != nil {
			return err
		}
		session, err := currentSession(tx, node, actor, in.SessionID, policy)
		if err != nil {
			return err
		}
		if err = tx.Model(&session).Updates(map[string]any{"last_heartbeat": time.Now().UTC(), "os": in.Report.OS, "arch": in.Report.Arch, "capacity": in.Report.Capacity, "tools_json": tools}).Error; err != nil {
			return err
		}
		grant = sessionGrant(node, session)
		_, _, err = authorizeNode(tx, actor)
		return err
	})
	if err != nil {
		return protocol.SessionGrant{}, err
	}
	return grant, nil
}

// 快照只保存数组编码；空工具列表规范化为空数组供安全视图使用。
func decodeTools(raw string) ([]protocol.ToolCheck, error) {
	var tools []protocol.ToolCheck
	if json.Unmarshal([]byte(raw), &tools) != nil {
		return nil, errDatabase
	}
	return tools, nil
}
