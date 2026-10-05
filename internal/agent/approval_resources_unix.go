//go:build darwin || linux

package agent

import (
	"context"
	"golang.org/x/sys/unix"
	"io"
	"mybuilds/internal/protocol"
	"os"
	"path/filepath"
)

func validateApprovalResource(j *executionJournal) error {
	r, _, err := j.readResource()
	if err != nil {
		return err
	}
	if r.Workspace == nil || r.Results == nil {
		return failure("journal_unconfirmed")
	}
	if err = j.checkResourceSlots(context.Background(), r); err != nil {
		return err
	}
	if j.state.Approval != nil {
		actual, e := resourceDirectory(context.Background(), j.lock, r.Workspace.RelativePath+"/workspace")
		if e != nil || actual != j.state.Approval.Local.WorkspaceIdentity {
			return failure("journal_unconfirmed")
		}
		if j.state.IOSResources != nil && j.state.Approval.Local.IOSteamID != j.state.IOSResources.TeamID {
			return failure("journal_unconfirmed")
		}
	}
	return nil
}
func approvalResourceEvidence(j *executionJournal) (string, string, error) {
	r, _, err := j.readResource()
	if err != nil {
		return "", "", err
	}
	if r.Workspace == nil || r.Results == nil || r.RegistrationPending {
		return "", "", failure("persistence_error")
	}
	if err = j.checkResourceSlots(context.Background(), r); err != nil {
		return "", "", err
	}
	return r.ID, r.OwnershipDigest, nil
}
func rebindApprovalResource(j *executionJournal, grant protocol.LeaseGrant) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	r, info, err := j.readResourceLocked(context.Background())
	if err != nil {
		return err
	}
	if grant.Task == nil || grant.Task.Resume == nil || grant.Task.Resume.ResourceID != r.ID || grant.Task.Resume.OwnershipDigest != r.OwnershipDigest || grant.Task.Resume.CheckpointRef != r.Ref || r.TerminalSeq != 0 || r.StopReceipt != nil || r.RegistrationPending {
		return failure("journal_unconfirmed")
	}
	if err = j.checkResourceSlots(context.Background(), r); err != nil {
		return err
	}
	r.Ref = grant.Ref
	r.RegistrationPending = true
	r.OwnershipDigest = resourceDigest(r)
	return j.saveResourceLocked(context.Background(), r, info)
}
func pausedJournalForInspection(root *os.Root, name string) bool {
	f, err := root.OpenFile(filepath.Join("journal", name), os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return false
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil || !privateInfo(before, false) || before.Size() > 1<<20 {
		return false
	}
	data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	after, e := f.Stat()
	now, n := root.Lstat(filepath.Join("journal", name))
	if err != nil || e != nil || n != nil || !os.SameFile(before, now) || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return false
	}
	state, err := parseApprovalJournal(data)
	return err == nil && state.Approval.State == "confirmed" && state.ClaimKey+".json" == name
}

func fillApprovalWorkspaceIdentity(j *executionJournal, local *protocol.ApprovalLocalCheckpoint) error {
	relative, err := j.resourceRelative(local.Workspace)
	if err != nil {
		return err
	}
	identity, err := resourceDirectory(context.Background(), j.lock, relative)
	if err != nil {
		return err
	}
	local.WorkspaceIdentity = identity
	return nil
}

// 拒绝/取消只消费已确认的安全checkpoint；不伪造build_finished或独立Stop。
func closeApprovalResource(ctx context.Context, j *executionJournal) error {
	if err := j.resourceSpoolClear(ctx); err != nil {
		return err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.state.Approval == nil || j.state.Approval.State != "confirmed" || j.state.Approval.Event == nil || j.state.PendingLog != nil || j.state.PendingEvent != nil || !j.state.StopConfirmed || j.state.CleanupFailed || !iosClosedState(j.state) {
		return failure("journal_unconfirmed")
	}
	event := j.state.Approval.Event
	r, info, err := j.readResourceLocked(ctx)
	if err != nil {
		return err
	}
	if r.RegistrationPending || r.PendingLog || r.Ref != event.Ref || r.TerminalSeq != 0 && (r.TerminalSeq != event.Seq || r.TerminalDigest != event.Digest) {
		return failure("journal_unconfirmed")
	}
	if err = j.checkResourceSlots(ctx, r); err != nil {
		return err
	}
	r.TerminalSeq = event.Seq
	r.TerminalDigest = event.Digest
	r.PendingLog = false
	return j.saveResourceLocked(ctx, r, info)
}

// 恢复仅登记经原目录/身份校验的实际资源，不重建工作区。
func (e *taskExecution) registerApprovalResource(ctx context.Context) error {
	r, _, err := e.journal.readResource()
	if err != nil {
		return err
	}
	return e.registerResource(ctx, resourceRegistration(r))
}
