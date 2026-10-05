package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/google/uuid"
	"io"
	"mybuilds/internal/pipeline"
	"mybuilds/internal/protocol"
	"os"
	"path"
	"reflect"
	"time"
)

type approvalCheckpoint struct {
	State string                           `json:"state"`
	Local protocol.ApprovalLocalCheckpoint `json:"local"`
	Event *protocol.ExecutionEvent         `json:"event,omitempty"`
}
type approvalRefTransition struct {
	Event     protocol.ExecutionEvent `json:"event"`
	ResumeRef protocol.LeaseRef       `json:"resume_ref"`
}

func approvalRefKnown(state journalState, ref protocol.LeaseRef) bool {
	if state.Ref == nil {
		return false
	}
	if ref == *state.Ref {
		return true
	}
	if ref.BuildID != state.Ref.BuildID || ref.AttemptID != state.Ref.AttemptID || ref.NodeID != state.Ref.NodeID || len(state.ApprovalHistory) > 128 {
		return false
	}
	cursor := ref
	for _, proof := range state.ApprovalHistory {
		if proof.Event.Ref != cursor {
			continue
		}
		d, err := protocol.ApprovalCheckpointDigest(proof.Event.Ref, proof.Event.Seq, proof.Event.Progress)
		next := proof.ResumeRef
		if err != nil || proof.Event.Progress.Approval == nil || d != proof.Event.Progress.Approval.CheckpointDigest || next.BuildID != cursor.BuildID || next.AttemptID != cursor.AttemptID || next.NodeID != cursor.NodeID || next.Epoch != cursor.Epoch+1 || !validRef(next) {
			return false
		}
		encoded, err := json.Marshal(proof.Event.Progress)
		if err != nil || journalProgressDigest(encoded) != proof.Event.Digest {
			return false
		}
		cursor = next
		if cursor == *state.Ref {
			return true
		}
	}
	return false
}
func parseApprovalJournal(data []byte) (journalState, error) {
	var state journalState
	if len(data) > 1<<20 {
		return state, failure("journal_unconfirmed")
	}
	tokens := json.NewDecoder(bytes.NewReader(data))
	count := 0
	if journalJSONValue(tokens, 0, &count) != nil {
		return state, failure("journal_unconfirmed")
	}
	if _, err := tokens.Token(); err != io.EOF {
		return state, failure("journal_unconfirmed")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&state) != nil {
		return state, failure("journal_unconfirmed")
	}
	a := state.Approval
	if a == nil || a.Event == nil || (a.State != "prepared" && a.State != "confirmed") || state.Ref == nil || !validRef(*state.Ref) || a.Event.Ref != *state.Ref || state.PendingLog != nil || state.PendingStop != nil || !state.StopConfirmed || state.CleanupFailed || !iosClosedState(state) || a.Local.Workspace == "" || a.Local.ResultDir != state.ResultDir || a.Local.Steps == nil || len(a.Local.Steps) > 128 {
		return state, failure("journal_unconfirmed")
	}
	e := a.Event
	p := e.Progress
	if p.Approval == nil || p.LocalApproval != nil || p.Approval.NextOrdinaryIndex != len(a.Local.Steps)+2 || p.LastLogSeq != state.LastLogSeq || p.LastLogOffset != state.LastLogOffset || p.LastArtifactSeq != int64(len(state.Artifacts)) || p.Approval.ResourceID != state.ResourceID {
		return state, failure("journal_unconfirmed")
	}
	digest, err := protocol.ApprovalCheckpointDigest(e.Ref, e.Seq, p)
	encoded, _ := json.Marshal(p)
	if err != nil || digest != p.Approval.CheckpointDigest || journalProgressDigest(encoded) != e.Digest {
		return state, failure("journal_unconfirmed")
	}
	if a.State == "confirmed" {
		if state.PendingEvent != nil || state.LastEventSeq != e.Seq || state.LastEventDigest != e.Digest {
			return state, failure("journal_unconfirmed")
		}
	} else if state.PendingEvent == nil || !reflect.DeepEqual(*state.PendingEvent, *e) || state.LastEventSeq+1 != e.Seq {
		return state, failure("journal_unconfirmed")
	}

	sealedArtifacts := map[string]localArtifact{}
	for _, artifact := range state.Artifacts {
		if artifact.Declaration.Purpose != "junit" {
			sealedArtifacts[artifact.SnapshotPath] = artifact
		}
	}
	restoredCount := 0
	for _, step := range a.Local.Steps {
		for _, saved := range step.Artifacts {
			original, ok := sealedArtifacts[saved.SnapshotPath]
			if !ok || original.Declaration.SHA256 != saved.SHA256 || original.Declaration.Size != saved.Size || original.Declaration.Step != step.Name {
				return state, failure("journal_unconfirmed")
			}
			delete(sealedArtifacts, saved.SnapshotPath)
			restoredCount++
		}
	}
	if restoredCount > 128 || len(sealedArtifacts) != 0 {
		return state, failure("journal_unconfirmed")
	}
	for _, artifact := range state.Artifacts {
		if !artifact.Confirmed || !approvalRefKnown(state, artifact.Declaration.Ref) {
			return state, failure("journal_unconfirmed")
		}
	}
	return state, nil
}
func readApprovalJournal(lock *dataLock, name string) (*executionJournal, error) {
	data, info, err := lock.readJournalFile(name)
	if err != nil {
		return nil, err
	}
	state, err := parseApprovalJournal(data)
	if err != nil || state.ClaimKey+".json" != name {
		return nil, failure("journal_unconfirmed")
	}
	digest := sha256.Sum256(data)
	j := &executionJournal{lock: lock, name: name, state: state, info: info, recoveryDigest: hex.EncodeToString(digest[:])}
	if err = validateApprovalResource(j); err != nil {
		return nil, err
	}
	return j, nil
}
func (e *taskExecution) prepareApproval(ctx context.Context, p *protocol.ExecutionProgress) error {
	started := time.Now()
	if p.Approval == nil || p.LocalApproval == nil || !e.iosClosed() {
		return failure("execution_unconfirmed")
	}
	if err := e.declareApprovalReports(p); err != nil {
		return err
	}
	if err := e.uploadArtifacts(ctx); err != nil {
		return err
	}
	resourceID, ownership, err := approvalResourceEvidence(e.journal)
	if err != nil {
		return err
	}
	snapshot, err := protocol.ApprovalSnapshotDigest(*e.task)
	if err != nil {
		return failure("persistence_error")
	}
	j := e.journal
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.state.PendingLog != nil || j.state.PendingEvent != nil || j.state.CleanupFailed || !j.state.StopConfirmed {
		return failure("execution_unconfirmed")
	}
	revision := int64(len(j.state.ApprovalHistory) + 1)
	p.Approval.ID = uuid.NewString()
	p.Approval.Revision = revision
	p.Approval.SnapshotDigest = snapshot
	p.Approval.WorkspaceID = resourceID
	p.Approval.ResultID = resourceID
	p.Approval.ResourceID = resourceID
	p.Approval.OwnershipDigest = ownership
	p.Approval.Artifacts = append([]protocol.ArtifactExpectation{}, j.state.ArtifactSteps...)
	p.ArtifactSteps = append([]protocol.ArtifactExpectation{}, j.state.ArtifactSteps...)
	p.LastLogSeq = j.state.LastLogSeq
	p.LastLogOffset = j.state.LastLogOffset
	p.LastArtifactSeq = int64(len(j.state.Artifacts))
	p.Approval.PublishIntents = []protocol.PublishExpectation{}
	for _, item := range j.state.Publishes {
		if item.Grant != nil {
			status := "unknown"
			digest := ""
			if item.Receipt != nil {
				status = item.Receipt.Status
				digest = item.Receipt.Digest
			}
			p.Approval.PublishIntents = append(p.Approval.PublishIntents, protocol.PublishExpectation{IntentID: item.IntentID, Status: status, ReceiptDigest: digest})
		}
	}
	if err = fillApprovalWorkspaceIdentity(j, p.LocalApproval); err != nil {
		return err
	}
	if j.state.IOSResources != nil && p.LocalApproval.IOSteamID != j.state.IOSResources.TeamID {
		return failure("execution_unconfirmed")
	}
	if p.RemainingBudgetNS != nil {
		remaining := max(0, *p.RemainingBudgetNS-int64(time.Since(started)))
		if remaining == 0 {
			return context.DeadlineExceeded
		}
		p.RemainingBudgetNS = &remaining
	}
	j.state.Approval = &approvalCheckpoint{State: "prepared", Local: *p.LocalApproval}
	return j.saveLocked()
}
func (e *taskExecution) declareApprovalReports(p *protocol.ExecutionProgress) error {
	if p.Approval.Reports == nil {
		return nil
	}
	j := e.journal
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.state.Ref == nil || j.state.Reports == nil || !sameReportEvidence(j.state.Reports.Evidence, *p.Approval.Reports) || p.LocalApproval.Collection == nil {
		return failure("report_invalid")
	}
	for _, entry := range p.LocalApproval.Collection.Current {
		f := entry.Local.File
		found := false
		for _, a := range j.state.Artifacts {
			if a.Declaration.ID == f.ArtifactID {
				d := a.Declaration
				if d.SHA256 != f.SHA256 || d.Size != f.Size || d.ReportKey != f.Key || d.Index != f.SourceIndex || d.Step != f.SourceStep || !a.Confirmed {
					return failure("report_invalid")
				}
				found = true
			}
		}
		if found {
			continue
		}
		if !exactUUID(f.ArtifactID) || f.SourceIndex < 1 || f.SourceStep == "" || !safeDigest(f.SHA256) || len(j.state.Artifacts) >= 128 {
			return failure("report_invalid")
		}
		d := protocol.ArtifactDeclaration{Ref: *j.state.Ref, ID: f.ArtifactID, Seq: int64(len(j.state.Artifacts) + 1), Phase: "ordinary", Index: f.SourceIndex, Step: f.SourceStep, Name: path.Base(f.Path), Size: f.Size, SHA256: f.SHA256, Purpose: "junit", ReportRevision: p.Approval.Reports.Revision, ReportKey: f.Key}
		j.state.Artifacts = append(j.state.Artifacts, localArtifact{Declaration: d, SnapshotPath: entry.Local.SnapshotPath})
	}
	return j.saveLocked()
}
func confirmApprovalJournal(ctx context.Context, client *agentHTTP, j *executionJournal, nodeName string) error {
	before, err := readApprovalJournal(j.lock, j.name)
	if err != nil || !os.SameFile(before.info, j.info) || !reflect.DeepEqual(before.state, j.state) {
		return failure("journal_unconfirmed")
	}
	a := j.state.Approval
	if a == nil || a.Event == nil {
		return failure("journal_unconfirmed")
	}
	p := a.Event.Progress.Approval
	lookup := protocol.ApprovalCheckpointLookup{Ref: a.Event.Ref, ApprovalID: p.ID, Revision: p.Revision, Seq: a.Event.Seq, Digest: a.Event.Digest, CheckpointDigest: p.CheckpointDigest}
	var receipt protocol.ApprovalCheckpointReceipt
	if err := client.post(ctx, "/api/agent/approval-checkpoint", lookup, &receipt); err != nil {
		return err
	}
	if receipt.Ref != lookup.Ref || receipt.ApprovalID != lookup.ApprovalID || receipt.Revision != lookup.Revision || receipt.Seq != lookup.Seq || receipt.Digest != lookup.Digest || receipt.CheckpointDigest != lookup.CheckpointDigest || receipt.NodeName != nodeName || !receipt.StopConfirmed || !receipt.SystemResourcesClosed {
		return failure("invalid_response")
	}
	current, err := readApprovalJournal(j.lock, j.name)
	if err != nil || !os.SameFile(current.info, j.info) || !reflect.DeepEqual(current.state, j.state) || before.recoveryDigest != current.recoveryDigest {
		return failure("journal_unconfirmed")
	}
	if a.State == "prepared" {
		j.mu.Lock()
		j.state.LastEventSeq = a.Event.Seq
		j.state.LastEventDigest = a.Event.Digest
		j.state.PendingEvent = nil
		j.state.Approval.State = "confirmed"
		err = j.saveLocked()
		j.mu.Unlock()
	}

	if err != nil {
		return err
	}
	if receipt.State == "rejected" || receipt.State == "cancelled" {
		if err = closeApprovalResource(ctx, j); err != nil {
			return err
		}
		current, err := readApprovalJournal(j.lock, j.name)
		if err != nil || !os.SameFile(current.info, j.info) || !reflect.DeepEqual(current.state, j.state) {
			return failure("journal_unconfirmed")
		}
		return j.remove()
	}
	return nil
}
func adoptApprovalResume(lock *dataLock, pending *executionJournal, grant protocol.LeaseGrant) (*executionJournal, error) {
	if grant.Task == nil || grant.Task.Resume == nil {
		return pending, nil
	}
	names, err := lock.journalNames()
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		if name == pending.name {
			continue
		}
		j, err := readApprovalJournal(lock, name)
		if err != nil {
			continue
		}
		a := j.state.Approval
		proof := grant.Task.Resume
		if a.State != "confirmed" || a.Event.Progress.Approval.ID != proof.ApprovalID {
			continue
		}
		if a.Event.Ref != proof.CheckpointRef || a.Event.Progress.Approval.CheckpointDigest != proof.CheckpointDigest || a.Event.Progress.Approval.SnapshotDigest != proof.SnapshotDigest || grant.Ref.BuildID != a.Event.Ref.BuildID || grant.Ref.AttemptID != a.Event.Ref.AttemptID || grant.Ref.NodeID != a.Event.Ref.NodeID || grant.Ref.Epoch != a.Event.Ref.Epoch+1 {
			return nil, failure("invalid_response")
		}
		if err = rebindApprovalResource(j, grant); err != nil {
			return nil, err
		}
		j.mu.Lock()
		j.state.ApprovalHistory = append(j.state.ApprovalHistory, approvalRefTransition{Event: *a.Event, ResumeRef: grant.Ref})
		a.State = "resumed"
		ref := grant.Ref
		j.state.Ref = &ref
		j.state.SessionID = ref.SessionID
		j.state.RemainingBudgetNS = cloneBudget(grant.RemainingBudgetNS)
		j.state.RemainingPostBudgetNS = grant.RemainingPostBudgetNS
		err = j.saveLocked()
		j.mu.Unlock()
		if err != nil {
			return nil, err
		}
		if err = pending.remove(); err != nil {
			return nil, err
		}
		return j, nil
	}
	return nil, failure("journal_unconfirmed")
}
func approvalRunResume(j *executionJournal, task *protocol.TaskSnapshot) *pipeline.ApprovalResume {
	if task.Resume == nil || j.state.Approval == nil {
		return nil
	}
	return &pipeline.ApprovalResume{Evidence: *task.Resume, Local: j.state.Approval.Local}
}

// 原主循环的空闲管理窗口复核暂停，不授执行权或重放任何用户动作。
func reconcileApprovals(ctx context.Context, client *agentHTTP, lock *dataLock, nodeName string) error {
	names, err := lock.journalNames()
	if err != nil {
		return err
	}
	for _, name := range names {
		j, err := readApprovalJournal(lock, name)
		if err != nil {
			continue
		}
		if err = confirmApprovalJournal(ctx, client, j, nodeName); err != nil {
			return err
		}
	}
	return nil
}
