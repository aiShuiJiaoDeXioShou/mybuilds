package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mybuilds/internal/protocol"
	"os"
	"strings"
)

// 只读本次私有终态证据；旧PID与旧session永远不成为执行或信号权限。
func readTerminalJournal(lock *dataLock, name string) (*executionJournal, error) {
	if !strings.HasSuffix(name, ".json") || !exactUUID(strings.TrimSuffix(name, ".json")) {
		return nil, failure("journal_unconfirmed")
	}
	data, info, err := lock.readJournalFile(name)
	if err != nil {
		return nil, err
	}
	tokens := json.NewDecoder(bytes.NewReader(data))
	count := 0
	if err = journalJSONValue(tokens, 0, &count); err != nil {
		return nil, failure("journal_unconfirmed")
	}
	if _, err = tokens.Token(); err != io.EOF {
		return nil, failure("journal_unconfirmed")
	}
	var state journalState
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&state) != nil {
		return nil, failure("journal_unconfirmed")
	}
	if state.ClaimKey+".json" != name || !exactUUID(state.ClaimKey) || state.Ref == nil || !validRef(*state.Ref) || state.Ref.SessionID != state.SessionID || state.PendingEvent == nil || state.PendingStop != nil || state.PendingLog != nil || !state.StopConfirmed || state.CleanupFailed || state.LastEventSeq < 0 || state.LastLogSeq < 0 || state.LastLogOffset < 0 {
		return nil, failure("journal_unconfirmed")
	}
	event := state.PendingEvent
	p := event.Progress
	if p.RemainingPostBudgetNS < 0 || p.ElapsedNS < 0 || p.Phase != "" || p.Index != 0 || p.Name != "" || p.StepKind != "" || len(p.ArtifactIDs) != 0 {
		return nil, failure("journal_unconfirmed")
	}
	if event.Ref != *state.Ref || event.Seq < 1 || event.Seq-1 != state.LastEventSeq || p.Kind != "build_finished" || !p.StopConfirmed || p.CleanupFailed || p.Started != state.Started || !terminalJournalStatus(p.Status) || p.At.IsZero() || p.ArtifactSteps == nil || p.LastLogSeq != state.LastLogSeq || p.LastLogOffset != state.LastLogOffset || p.LastArtifactSeq != int64(len(state.Artifacts)) || p.RemainingPostBudgetNS != state.RemainingPostBudgetNS || !sameBudget(p.RemainingBudgetNS, state.RemainingBudgetNS) {
		return nil, failure("journal_unconfirmed")
	}
	expected := state.ArtifactSteps
	if expected == nil {
		expected = []protocol.ArtifactExpectation{}
	}
	localManifest, _ := json.Marshal(expected)
	eventManifest, _ := json.Marshal(p.ArtifactSteps)
	if !bytes.Equal(localManifest, eventManifest) {
		return nil, failure("journal_unconfirmed")
	}
	for index, artifact := range state.Artifacts {
		if !artifact.Confirmed || artifact.Declaration.Ref != *state.Ref || artifact.Declaration.Seq != int64(index+1) {
			return nil, failure("journal_unconfirmed")
		}
	}
	checkpoint := &executionJournal{state: state}
	if err := (&taskExecution{journal: checkpoint}).checkSealed(&p); err != nil {
		return nil, failure("journal_unconfirmed")
	}
	encoded, err := json.Marshal(p)
	if err != nil {
		return nil, failure("journal_unconfirmed")
	}
	hash := sha256.Sum256(encoded)
	if event.Digest != hex.EncodeToString(hash[:]) {
		return nil, failure("journal_unconfirmed")
	}
	fullHash := sha256.Sum256(data)
	return &executionJournal{lock: lock, name: name, state: state, info: info, recoveryDigest: hex.EncodeToString(fullHash[:])}, nil
}
func terminalJournalStatus(value string) bool {
	switch value {
	case "succeeded", "failed", "cancelled", "skipped":
		return true
	}
	return false
}
func sameBudget(a, b *int64) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b && *a >= 0
}

// journal是单个有界对象，拒绝重复/null/过深数据，不以Decoder的后值覆盖解释证据。
func journalJSONValue(dec *json.Decoder, depth int, count *int) error {
	*count++
	if depth > 64 || *count > 10000 {
		return failure("journal_unconfirmed")
	}
	value, err := dec.Token()
	if err != nil || value == nil {
		return failure("journal_unconfirmed")
	}
	delim, ok := value.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for dec.More() {
			raw, err := dec.Token()
			key, ok := raw.(string)
			if err != nil || !ok || seen[strings.ToLower(key)] {
				return failure("journal_unconfirmed")
			}
			seen[strings.ToLower(key)] = true
			if err = journalJSONValue(dec, depth+1, count); err != nil {
				return err
			}
		}
		end, err := dec.Token()
		if err != nil || end != json.Delim('}') {
			return failure("journal_unconfirmed")
		}
	case '[':
		for dec.More() {
			if err := journalJSONValue(dec, depth+1, count); err != nil {
				return err
			}
		}
		end, err := dec.Token()
		if err != nil || end != json.Delim(']') {
			return failure("journal_unconfirmed")
		}
	default:
		return failure("journal_unconfirmed")
	}
	return nil
}

// 当前节点只读核对；成功只清精确本条，其它未知证据仍阻止新session。
func recoverTerminalJournals(ctx context.Context, client *agentHTTP, lock *dataLock, nodeName string) error {
	names, err := lock.journalNames()
	if err != nil {
		return err
	}
	blocked := false
	for _, name := range names {
		journal, err := readTerminalJournal(lock, name)
		if err != nil {
			blocked = true
			continue
		}
		pending := journal.state.PendingEvent
		request := protocol.TerminalReceiptRequest{Ref: pending.Ref, Seq: pending.Seq, Digest: pending.Digest}
		var receipt protocol.TerminalReceipt
		if err = client.post(ctx, "/api/agent/terminal-receipt", request, &receipt); err != nil {
			blocked = true
			continue
		}
		if receipt.Ref != pending.Ref || receipt.Seq != pending.Seq || receipt.Digest != pending.Digest || receipt.Status != pending.Progress.Status || !receipt.StopKnown || receipt.NodeName != nodeName {
			return failure("invalid_response")
		}
		// 网络等待期间即便同inode被改写，也不能删除已经变化的自有证据。
		current, err := readTerminalJournal(lock, name)
		if err != nil || !os.SameFile(journal.info, current.info) || journal.recoveryDigest != current.recoveryDigest {
			return failure("data_invalid")
		}
		if err = journal.remove(); err != nil {
			return err
		}
	}
	if blocked {
		return failure("journal_unconfirmed")
	}
	return nil
}
