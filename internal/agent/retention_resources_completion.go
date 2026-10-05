//go:build darwin || linux

package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"strings"

	"golang.org/x/sys/unix"
	"mybuilds/internal/protocol"
)

func sameResourceRegistration(a, b protocol.NodeResourceRegistration) bool {
	if a.Ref != b.Ref || a.ID != b.ID || a.OwnershipDigest != b.OwnershipDigest || a.HasWorkspace != b.HasWorkspace || a.HasResults != b.HasResults {
		return false
	}
	return a.Completion == nil && b.Completion == nil || a.Completion != nil && b.Completion != nil && *a.Completion == *b.Completion
}
func validResourceCompletion(r resourceRecord) bool {
	c := r.Completion
	if c == nil {
		return !r.CompletionConfirmed
	}
	return r.StopReceipt != nil && c.StopCode == "process_group_reaped" && r.StopReceipt.EvidenceCode == c.StopCode && c.LastEventSeq >= 0 && c.LastLogSeq >= 0 && c.LastLogOffset >= 0 && c.LastArtifactSeq >= 0 && (!r.CompletionConfirmed || !r.RegistrationPending && !r.PendingLog)
}

// nil PendingLog还不证明spool已fsync清理；只读检查本claim的所有残留，不删陌生文件。
func (j *executionJournal) resourceSpoolClear(ctx context.Context) error {
	if e := resourceContext(ctx); e != nil {
		return e
	}
	if e := j.lock.Check(); e != nil {
		return e
	}
	info, e := j.lock.root.Lstat("spool")
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil || !privateInfo(info, true) {
		return failure("execution_unconfirmed")
	}
	f, e := j.lock.root.OpenFile("spool", os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if e != nil {
		return failure("execution_unconfirmed")
	}
	defer f.Close()
	opened, e := f.Stat()
	if e != nil || !os.SameFile(info, opened) {
		return failure("execution_unconfirmed")
	}
	entries, e := f.ReadDir(8193)
	if e != nil && e != io.EOF || len(entries) > 8192 {
		return failure("execution_unconfirmed")
	}
	for _, entry := range entries {
		if e := resourceContext(ctx); e != nil {
			return e
		}
		if strings.HasPrefix(entry.Name(), j.state.ClaimKey+"-") {
			return failure("execution_unconfirmed")
		}
	}
	now, e := j.lock.root.Lstat("spool")
	if e != nil || !os.SameFile(opened, now) {
		return failure("execution_unconfirmed")
	}
	return j.lock.Check()
}
func (j *executionJournal) stoppedResourceKnown(ctx context.Context, confirmation protocol.StopConfirmation) error {
	if j.state.Ref == nil || !validRef(*j.state.Ref) || confirmation.Ref != *j.state.Ref || j.state.PendingStop == nil || *j.state.PendingStop != confirmation || confirmation.EvidenceCode != "process_group_reaped" || confirmation.Note != "本次执行进程组已完成真实回收" || !j.state.StopConfirmed || j.state.CleanupFailed || j.state.PendingEvent != nil || j.state.PendingLog != nil || j.state.LastEventSeq < 0 || j.state.LastLogSeq < 0 || j.state.LastLogOffset < 0 || j.state.LastLogSeq == 0 && j.state.LastLogOffset != 0 {
		return failure("execution_unconfirmed")
	}
	for index, artifact := range j.state.Artifacts {
		d := artifact.Declaration
		if !artifact.Confirmed || d.Ref != *j.state.Ref || d.Seq != int64(index+1) || !exactUUID(d.ID) || d.Size < 0 || !safeDigest(d.SHA256) {
			return failure("execution_unconfirmed")
		}
	}
	return j.resourceSpoolClear(ctx)
}
func (j *executionJournal) prepareResourceCompletion(ctx context.Context, confirmation protocol.StopConfirmation) (protocol.NodeResourceRegistration, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	var zero protocol.NodeResourceRegistration
	if e := j.stoppedResourceKnown(ctx, confirmation); e != nil {
		return zero, e
	}
	r, info, e := j.readResourceLocked(ctx)
	if e != nil {
		return zero, e
	}
	if r.RegistrationPending || r.TerminalSeq != 0 || r.StopReceipt == nil || *r.StopReceipt != confirmation {
		return zero, failure("execution_unconfirmed")
	}
	if e = j.checkResourceSlots(ctx, r); e != nil {
		return zero, e
	}
	completion := protocol.NodeResourceCompletion{LastEventSeq: j.state.LastEventSeq, LastLogSeq: j.state.LastLogSeq, LastLogOffset: j.state.LastLogOffset, LastArtifactSeq: int64(len(j.state.Artifacts)), StopCode: confirmation.EvidenceCode}
	if r.Completion != nil {
		if *r.Completion != completion {
			return zero, failure("persistence_error")
		}
		return resourceRegistration(r), nil
	}
	r.Completion = &completion
	r.CompletionConfirmed = false
	r.PendingLog = false
	if e = j.saveResourceLocked(ctx, r, info); e != nil {
		return zero, e
	}
	return resourceRegistration(r), nil
}
func (j *executionJournal) ackResourceCompletion(in protocol.NodeResourceRegistration) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	ctx := context.Background()
	r, info, e := j.readResourceLocked(ctx)
	if e != nil {
		return e
	}
	if in.Completion == nil || r.Completion == nil || !sameResourceRegistration(resourceRegistration(r), in) || r.StopReceipt == nil {
		return failure("invalid_response")
	}
	if e = j.stoppedResourceKnown(ctx, *r.StopReceipt); e != nil {
		return e
	}
	if e = j.checkResourceSlots(ctx, r); e != nil {
		return e
	}
	expected := protocol.NodeResourceCompletion{LastEventSeq: j.state.LastEventSeq, LastLogSeq: j.state.LastLogSeq, LastLogOffset: j.state.LastLogOffset, LastArtifactSeq: int64(len(j.state.Artifacts)), StopCode: r.StopReceipt.EvidenceCode}
	if expected != *in.Completion {
		return failure("invalid_response")
	}
	if r.CompletionConfirmed {
		return nil
	}
	r.CompletionConfirmed = true
	return j.saveResourceLocked(ctx, r, info)
}

// 已收到独立Stop真实ACK；complete只确认原游标，不复活执行租约或删journal。
func completeStoppedResource(ctx context.Context, client *agentHTTP, j *executionJournal, confirmation protocol.StopConfirmation) error {
	if j.state.ResourceID == "" {
		return nil
	}
	if e := recordResourceStop(j, confirmation); e != nil {
		return e
	}
	in, e := j.prepareResourceCompletion(ctx, confirmation)
	if e != nil {
		return e
	}
	if e = client.retryPost(ctx, "/api/agent/resources", in, nil); e != nil {
		return e
	}
	return j.ackResourceCompletion(in)
}

// 仅已知物理停止+零pending的登记资源可恢复；旧PID和普通未知journal不解释。
func readStoppedResourceJournal(lock *dataLock, name string) (*executionJournal, error) {
	if !strings.HasSuffix(name, ".json") || !exactUUID(strings.TrimSuffix(name, ".json")) {
		return nil, failure("journal_unconfirmed")
	}
	data, info, e := lock.readJournalFile(name)
	if e != nil {
		return nil, e
	}
	tokens := json.NewDecoder(bytes.NewReader(data))
	count := 0
	if journalJSONValue(tokens, 0, &count) != nil {
		return nil, failure("journal_unconfirmed")
	}
	if _, e = tokens.Token(); e != io.EOF {
		return nil, failure("journal_unconfirmed")
	}
	var state journalState
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if dec.Decode(&state) != nil || state.ClaimKey+".json" != name || !exactUUID(state.ClaimKey) || !exactUUID(state.SessionID) || !exactUUID(state.ResourceID) || state.Ref == nil || state.Ref.SessionID != state.SessionID || state.PendingStop == nil || state.RemainingPostBudgetNS < 0 || state.RemainingBudgetNS != nil && *state.RemainingBudgetNS < 0 {
		return nil, failure("journal_unconfirmed")
	}
	digest := sha256.Sum256(data)
	j := &executionJournal{lock: lock, name: name, state: state, info: info, recoveryDigest: hex.EncodeToString(digest[:])}
	if e = j.stoppedResourceKnown(context.Background(), *state.PendingStop); e != nil {
		return nil, failure("journal_unconfirmed")
	}
	r, _, e := j.readResource()
	if e != nil || r.RegistrationPending || r.TerminalSeq != 0 {
		return nil, failure("journal_unconfirmed")
	}
	if e = j.checkResourceSlots(context.Background(), r); e != nil {
		return nil, failure("journal_unconfirmed")
	}
	return j, nil
}
func restoreStoppedResource(ctx context.Context, client *agentHTTP, j *executionJournal) error {
	if j.recoveryDigest == "" || j.state.PendingStop == nil {
		return failure("journal_unconfirmed")
	}
	current, e := readStoppedResourceJournal(j.lock, j.name)
	if e != nil || !os.SameFile(j.info, current.info) || j.recoveryDigest != current.recoveryDigest {
		return failure("data_invalid")
	}
	confirmation := *j.state.PendingStop
	if e = client.retryPost(ctx, "/api/agent/stop-confirmation", confirmation, nil); e != nil {
		return e
	}
	// 网络期间同inode被改写同样不能用旧对象释放本地证据。
	current, e = readStoppedResourceJournal(j.lock, j.name)
	if e != nil || !os.SameFile(j.info, current.info) || j.recoveryDigest != current.recoveryDigest {
		return failure("data_invalid")
	}
	return completeStoppedResource(ctx, client, j, confirmation)
}
