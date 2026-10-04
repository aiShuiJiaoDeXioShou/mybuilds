package store

import (
	"context"
	"slices"
	"strings"

	"gorm.io/gorm"
	"mybuilds/internal/protocol"
)

// TerminalReceipt只读核对已完成终态；当前凭据并不取得原session执行权。
func (s *Store) TerminalReceipt(ctx context.Context, actor NodeActor, in protocol.TerminalReceiptRequest) (protocol.TerminalReceipt, error) {
	if !validRef(in.Ref) || in.Seq < 1 || !validHex(in.Digest, 64) || in.Digest != strings.ToLower(in.Digest) {
		return protocol.TerminalReceipt{}, ErrInvalid
	}
	var result protocol.TerminalReceipt
	err := s.write(ctx, func(tx *gorm.DB) error {
		node, _, err := authorizeNode(tx, actor)
		if err != nil {
			return err
		}
		if node.State == "disabled" || actor.ID != in.Ref.NodeID {
			return ErrNodeUnauthorized
		}
		var row buildRecord
		if err = tx.First(&row, "id = ?", in.Ref.BuildID).Error; err != nil {
			return err
		}
		if buildRef(row) != in.Ref || !slices.Contains([]string{"succeeded", "failed", "cancelled", "skipped"}, row.Status) || row.StopUnconfirmed || row.LastEventSeq != in.Seq {
			return ErrConflict
		}
		var attempt attemptRecord
		if err = tx.First(&attempt, "id = ? AND build_id = ?", in.Ref.AttemptID, row.ID).Error; err != nil {
			return err
		}
		if attempt.NodeID != in.Ref.NodeID || attempt.SessionID != in.Ref.SessionID || attempt.LeaseID != in.Ref.LeaseID || attempt.Epoch != in.Ref.Epoch {
			return ErrConflict
		}
		var receipt executionReceiptRecord
		if err = tx.First(&receipt, "build_id = ? AND attempt_id = ? AND seq = ?", row.ID, attempt.ID, in.Seq).Error; err != nil {
			return err
		}
		if receipt.Digest != in.Digest || receipt.Kind != "build_finished" || !receipt.StopKnown {
			return ErrConflict
		}
		var cleanup int64
		if err = tx.Model(&stepRecord{}).Where("build_id = ? AND cleanup_failed = ?", row.ID, true).Count(&cleanup).Error; err != nil {
			return err
		}
		if cleanup != 0 {
			return ErrConflict
		}
		result = protocol.TerminalReceipt{Ref: in.Ref, Seq: in.Seq, Digest: in.Digest, Status: row.Status, StopKnown: true, NodeName: node.Name}
		current, _, err := authorizeNode(tx, actor)
		if err == nil && current.State == "disabled" {
			return ErrNodeUnauthorized
		}
		return err
	})
	if err != nil {
		return protocol.TerminalReceipt{}, err
	}
	return result, nil
}
