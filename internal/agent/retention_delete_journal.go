//go:build darwin || linux

package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"

	"mybuilds/internal/protocol"
)

// 删除意图、固定隔离槽和真实待补传回执均在物理动作之间落盘。
type nodeDeletionJournalState struct {
	Item              protocol.NodeDeletion              `json:"item"`
	NodeID            string                             `json:"node_id"`
	RootIdentity      string                             `json:"root_identity"`
	DirectoryIdentity string                             `json:"directory_identity"`
	Pending           *protocol.NodeDeletionConfirmation `json:"pending,omitempty"`
	Progress          *nodeDeletionProgress              `json:"progress,omitempty"`
}
type nodeDeletionProgress struct {
	QuarantineIdentity string `json:"quarantine_identity"`
	RetentionIdentity  string `json:"retention_identity"`
	WorkspaceState     string `json:"workspace_state"`
	ResultsState       string `json:"results_state"`
	Seq                int64  `json:"seq"`
	Nonce              string `json:"nonce"`
	Active             bool   `json:"active"`
}
type nodeDeletionJournal struct {
	lock   *dataLock
	state  nodeDeletionJournalState
	info   os.FileInfo
	digest string
}

func deletionExactFields(data []byte, fields ...string) bool {
	var values map[string]json.RawMessage
	if json.Unmarshal(data, &values) != nil || len(values) != len(fields) {
		return false
	}
	for _, field := range fields {
		if _, ok := values[field]; !ok {
			return false
		}
	}
	return true
}
func validDeletionConfirmation(item protocol.NodeDeletion, p protocol.NodeDeletionConfirmation) bool {
	if p.ID != item.ID || p.ResourceID != item.ResourceID || p.OwnershipDigest != item.OwnershipDigest || !exactUUID(p.Nonce) || p.Seq < 1 || !safeDigest(p.Digest) {
		return false
	}
	states := []struct {
		present bool
		state   string
	}{{item.HasWorkspace, p.WorkspaceState}, {item.HasResults, p.ResultsState}}
	complete := true
	for _, slot := range states {
		if !slot.present {
			if slot.state != "not_applicable" {
				return false
			}
			continue
		}
		switch slot.state {
		case "deleted":
		case "partial", "failed":
			complete = false
		default:
			return false
		}
	}
	if complete {
		if p.Reason != "" {
			return false
		}
	} else {
		switch p.Reason {
		case "partial", "resource_unconfirmed", "identity_mismatch", "invalid_object", "permission_denied", "persistence_error", "authority_expired", "operation_timeout":
		default:
			return false
		}
	}
	digest, e := protocol.NodeDeletionDigest(p)
	return e == nil && digest == p.Digest
}
func readNodeDeletionJournal(lock *dataLock, id string) (*nodeDeletionJournal, error) {
	data, info, parent, e := deletionPrivateFile(context.Background(), lock, "deletions", id)
	if e != nil {
		return nil, e
	}
	if e = deletionJSON(data); e != nil {
		return nil, e
	}
	var top map[string]json.RawMessage
	if json.Unmarshal(data, &top) != nil {
		return nil, failure("resource_unconfirmed")
	}
	keys := []string{"item", "node_id", "root_identity", "directory_identity"}
	if _, ok := top["pending"]; ok {
		keys = append(keys, "pending")
	}
	if _, ok := top["progress"]; ok {
		keys = append(keys, "progress")
	}
	if !deletionExactFields(data, keys...) {
		return nil, failure("resource_unconfirmed")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || !deletionExactFields(fields["item"], "id", "resource_id", "build_id", "attempt_id", "ownership_digest", "has_workspace", "has_results") || (fields["pending"] != nil && !deletionExactFields(fields["pending"], "id", "resource_id", "ownership_digest", "nonce", "seq", "digest", "workspace_state", "results_state", "reason")) || (fields["progress"] != nil && !deletionExactFields(fields["progress"], "quarantine_identity", "retention_identity", "workspace_state", "results_state", "seq", "nonce", "active")) {
		return nil, failure("resource_unconfirmed")
	}
	var state nodeDeletionJournalState
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if dec.Decode(&state) != nil || !validNodeDeletion(state.Item) || state.Item.ID != id || !exactUUID(state.NodeID) || state.DirectoryIdentity != parent || (state.Pending != nil && !validDeletionConfirmation(state.Item, *state.Pending)) || (state.Pending == nil && state.Progress == nil) {
		return nil, failure("resource_unconfirmed")
	}
	if state.Progress == nil {
		// 早期只有失败文法，没有已删除/部分隔离的物理证明，不能由它报完成。
		for _, slot := range []struct {
			present bool
			state   string
		}{{state.Item.HasWorkspace, state.Pending.WorkspaceState}, {state.Item.HasResults, state.Pending.ResultsState}} {
			if slot.present && slot.state != "failed" {
				return nil, failure("resource_unconfirmed")
			}
		}
	}
	if state.Progress != nil {
		p := state.Progress
		if !exactUUID(p.Nonce) || (state.Pending != nil && (!p.Active || state.Pending.Nonce != p.Nonce)) || p.Seq < 0 || (state.Pending != nil && (p.Seq == int64(^uint64(0)>>1) || state.Pending.Seq != p.Seq+1)) {
			return nil, failure("resource_unconfirmed")
		}
		for _, slot := range []struct {
			present bool
			stage   string
		}{{state.Item.HasWorkspace, p.WorkspaceState}, {state.Item.HasResults, p.ResultsState}} {
			if !slot.present {
				if slot.stage != "not_applicable" {
					return nil, failure("resource_unconfirmed")
				}
			} else if slot.stage != "pending" && slot.stage != "quarantined" && slot.stage != "deleted" {
				return nil, failure("resource_unconfirmed")
			}
		}
		if state.Pending != nil {
			for _, slot := range []struct{ stage, wire string }{{p.WorkspaceState, state.Pending.WorkspaceState}, {p.ResultsState, state.Pending.ResultsState}} {
				if deletionWireState(slot.stage) != slot.wire && !(deletionWireState(slot.stage) == "partial" && slot.wire == "failed") {
					return nil, failure("resource_unconfirmed")
				}
			}
		}
		if (p.RetentionIdentity != "" && !validResourceIdentity(p.RetentionIdentity)) || (p.QuarantineIdentity != "" && (!validResourceIdentity(p.QuarantineIdentity) || !validResourceIdentity(p.RetentionIdentity))) {
			return nil, failure("resource_unconfirmed")
		}
	}
	root, e := resourceDirectory(context.Background(), lock, ".")
	if e != nil || state.RootIdentity != root {
		return nil, failure("resource_unconfirmed")
	}
	if _, e = deletionResourceRecord(context.Background(), lock, state.NodeID, state.Item); e != nil {
		return nil, e
	}
	hash := sha256.Sum256(data)
	return &nodeDeletionJournal{lock: lock, state: state, info: info, digest: hex.EncodeToString(hash[:])}, nil
}
