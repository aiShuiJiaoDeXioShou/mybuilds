package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// NodeDeletionDigest仅编码当前确认事实；Digest键自身完全不参与摘要。
func NodeDeletionDigest(in NodeDeletionConfirmation) (string, error) {
	data, err := json.Marshal(struct {
		ID              string `json:"id"`
		ResourceID      string `json:"resource_id"`
		OwnershipDigest string `json:"ownership_digest"`
		Nonce           string `json:"nonce"`
		Seq             int64  `json:"seq"`
		WorkspaceState  string `json:"workspace_state"`
		ResultsState    string `json:"results_state"`
		Reason          string `json:"reason"`
	}{in.ID, in.ResourceID, in.OwnershipDigest, in.Nonce, in.Seq, in.WorkspaceState, in.ResultsState, in.Reason})
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}
