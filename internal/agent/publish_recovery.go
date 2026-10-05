package agent

import (
	"slices"

	"mybuilds/internal/protocol"
)

// 只核对原终态证据；启动恢复不申请grant或重发任何发布动作。
func validatePublisherJournal(state journalState) error {
	if len(state.Publishes) > 384 || state.Ref == nil || state.PendingEvent == nil {
		return failure("journal_unconfirmed")
	}
	manifest := map[string]protocol.PublishExpectation{}
	for _, item := range state.PendingEvent.Progress.PublishIntents {
		if !exactUUID(item.IntentID) || item.ReceiptDigest != "" && !safeDigest(item.ReceiptDigest) || !slices.Contains([]string{"unknown", "uploaded", "processing", "submitted", "published", "confirmed", "failed"}, item.Status) {
			return failure("journal_unconfirmed")
		}
		if _, exists := manifest[item.IntentID]; exists {
			return failure("journal_unconfirmed")
		}
		manifest[item.IntentID] = item
	}
	seen := map[string]bool{}
	for _, item := range state.Publishes {
		if !exactUUID(item.IntentID) || item.Index < 1 || seen[item.IntentID] || item.Once != (item.Grant != nil) {
			return failure("journal_unconfirmed")
		}
		seen[item.IntentID] = true
		if item.Grant != nil {
			digest, err := protocol.PublishGrantDigest(*item.Grant)
			if err != nil || !safeDigest(digest) || digest != item.Grant.AuthorizationDigest || item.Grant.IntentID != item.IntentID || item.Grant.Ref != *state.Ref {
				return failure("journal_unconfirmed")
			}
			if _, ok := manifest[item.IntentID]; !ok {
				return failure("journal_unconfirmed")
			}
		}
		if item.Receipt != nil {
			digest, err := protocol.PublishReceiptDigest(*item.Receipt)
			remote, ok := manifest[item.IntentID]
			if item.Grant == nil || err != nil || digest != item.Receipt.Digest || item.Receipt.Ref != *state.Ref || item.Receipt.IntentID != item.IntentID || item.Receipt.AuthorizationDigest != item.Grant.AuthorizationDigest || !ok || remote.ReceiptDigest != digest {
				return failure("journal_unconfirmed")
			}
		}
	}
	for id := range manifest {
		if !seen[id] {
			return failure("journal_unconfirmed")
		}
	}
	return nil
}
