//go:build darwin || linux

package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
	"mybuilds/internal/protocol"
)

// 归属记录只在私有data_dir；停止资料和路径不进入登记wire。
type resourceSlot struct {
	RelativePath   string `json:"relative_path"`
	Identity       string `json:"identity"`
	ParentIdentity string `json:"parent_identity"`
}
type resourceRecord struct {
	ID                  string                           `json:"id"`
	Ref                 protocol.LeaseRef                `json:"ref"`
	ClaimKey            string                           `json:"claim_key"`
	RootIdentity        string                           `json:"root_identity"`
	DirectoryIdentity   string                           `json:"directory_identity"`
	Workspace           *resourceSlot                    `json:"workspace,omitempty"`
	Results             *resourceSlot                    `json:"results,omitempty"`
	OwnershipDigest     string                           `json:"ownership_digest"`
	RegistrationPending bool                             `json:"registration_pending"`
	TerminalSeq         int64                            `json:"terminal_seq,omitempty"`
	TerminalDigest      string                           `json:"terminal_digest,omitempty"`
	StopReceipt         *protocol.StopConfirmation       `json:"stop_receipt,omitempty"`
	Completion          *protocol.NodeResourceCompletion `json:"completion,omitempty"`
	CompletionConfirmed bool                             `json:"completion_confirmed"`
	PendingLog          bool                             `json:"pending_log"`
}

func resourceRegistration(r resourceRecord) protocol.NodeResourceRegistration {
	return protocol.NodeResourceRegistration{Ref: r.Ref, ID: r.ID, OwnershipDigest: r.OwnershipDigest, HasWorkspace: r.Workspace != nil, HasResults: r.Results != nil, Completion: r.Completion}
}
func resourceDigest(r resourceRecord) string {
	body, _ := json.Marshal(struct {
		Ref          protocol.LeaseRef `json:"ref"`
		RootIdentity string            `json:"root_identity"`
		Workspace    *resourceSlot     `json:"workspace,omitempty"`
		Results      *resourceSlot     `json:"results,omitempty"`
	}{r.Ref, r.RootIdentity, r.Workspace, r.Results})
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
func resourceContext(ctx context.Context) error {
	if ctx.Err() != nil {
		return failure("authority_lost")
	}
	return nil
}

// 同fd核出生身份，每一层均不能是链接或不受限目录；不拿目录mtime/size当归属。
func resourceDirectory(ctx context.Context, lock *dataLock, name string) (string, error) {
	if err := resourceContext(ctx); err != nil {
		return "", err
	}
	if err := lock.Check(); err != nil {
		return "", err
	}
	if name != "." && (!filepath.IsLocal(name) || path.Clean(name) != name || strings.Contains(name, "\\")) {
		return "", failure("persistence_error")
	}
	parts := strings.Split(name, "/")
	var identity string
	for i := range parts {
		leaf := strings.Join(parts[:i+1], "/")
		before, e := lock.root.Lstat(leaf)
		if e != nil || !privateInfo(before, true) {
			return "", failure("persistence_error")
		}
		f, e := lock.root.OpenFile(leaf, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
		if e != nil {
			return "", failure("persistence_error")
		}
		info, e := f.Stat()
		if e != nil || !privateInfo(info, true) || !os.SameFile(before, info) {
			f.Close()
			return "", failure("persistence_error")
		}
		identity, e = resourceNativeIdentity(f, info)
		now, c := lock.root.Lstat(leaf)
		closed := f.Close()
		if e != nil || c != nil || closed != nil || !os.SameFile(info, now) {
			return "", failure("persistence_error")
		}
	}
	if err := resourceContext(ctx); err != nil {
		return "", err
	}
	if err := lock.Check(); err != nil {
		return "", err
	}
	return identity, nil
}
func (j *executionJournal) resourceRelative(absolute string) (string, error) {
	if !filepath.IsAbs(absolute) {
		return "", failure("persistence_error")
	}
	if e := j.lock.Check(); e != nil {
		return "", e
	}
	// 只规范受dataLock出生核对的根拼写；绝不Eval输入slot而跟随用户链接。
	canonical, e := filepath.EvalSymlinks(j.lock.root.Name())
	if e != nil {
		return "", failure("persistence_error")
	}
	for _, base := range []string{j.lock.root.Name(), canonical} {
		name, e := filepath.Rel(base, absolute)
		name = filepath.ToSlash(name)
		if e == nil && filepath.IsLocal(name) && name != "." && len(name) <= 512 && !strings.Contains(name, "\\") {
			if e = j.lock.Check(); e != nil {
				return "", e
			}
			return name, nil
		}
	}
	return "", failure("persistence_error")
}
func validResourcePath(name string, workspace bool) bool {
	parts := strings.Split(name, "/")
	prefix, leaf := "results", "mybuilds-"
	if workspace {
		prefix, leaf = "scm", "checkout-"
	}
	return len(parts) == 2 && parts[0] == prefix && len(parts[1]) > len(leaf) && strings.HasPrefix(parts[1], leaf) && len(parts[1]) <= 255 && path.Clean(name) == name && !strings.Contains(name, "\\")
}
func validResourceSlot(slot *resourceSlot, workspace bool) bool {
	return slot != nil && validResourcePath(slot.RelativePath, workspace) && validResourceIdentity(slot.Identity) && validResourceIdentity(slot.ParentIdentity)
}
func resourceIdentityCanonical(value string) bool {
	p := strings.Split(value, ":")
	dev, e := strconv.ParseUint(p[2], 16, 64)
	ino, i := strconv.ParseUint(p[3], 16, 64)
	sec, s := strconv.ParseInt(p[4], 10, 64)
	ns, n := strconv.ParseUint(p[5], 10, 32)
	return e == nil && i == nil && s == nil && n == nil && ns <= 999999999 && value == fmt.Sprintf("v1:%s:%016x:%016x:%d:%09d", p[1], dev, ino, sec, ns)
}
func validResourceIdentity(identity string) bool {
	// 必须是本平台实际生成的固定出生身份，不能接受路径或控制字符。
	if len(identity) > 160 {
		return false
	}
	parts := strings.Split(identity, ":")
	if len(parts) != 6 || parts[0] != "v1" || (parts[1] != "darwin" && parts[1] != "linux") || len(parts[2]) != 16 || len(parts[3]) != 16 || len(parts[5]) != 9 {
		return false
	}
	for _, field := range parts[2:4] {
		if _, e := hex.DecodeString(field); e != nil {
			return false
		}
	}
	// 重编码再比较，拒绝不规范数字/符号/纳秒。
	return resourceIdentityCanonical(identity)
}
func (j *executionJournal) readResource() (resourceRecord, os.FileInfo, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.readResourceLocked(context.Background())
}
func (j *executionJournal) readResourceLocked(ctx context.Context) (resourceRecord, os.FileInfo, error) {
	var r resourceRecord
	if !exactUUID(j.state.ResourceID) || j.state.Ref == nil || !validRef(*j.state.Ref) {
		return r, nil, failure("persistence_error")
	}
	dir, e := resourceDirectory(ctx, j.lock, "resources")
	if e != nil {
		return r, nil, e
	}
	name := "resources/" + j.state.ResourceID + ".json"
	before, e := j.lock.root.Lstat(name)
	if e != nil || !privateInfo(before, false) || before.Size() > 1<<20 {
		return r, nil, failure("persistence_error")
	}
	f, e := j.lock.root.OpenFile(name, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if e != nil {
		return r, nil, failure("persistence_error")
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !privateInfo(info, false) || !os.SameFile(info, before) {
		return r, nil, failure("persistence_error")
	}
	data, e := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	after, a := f.Stat()
	now, n := j.lock.root.Lstat(name)
	if e != nil || a != nil || n != nil || len(data) > 1<<20 || !os.SameFile(now, info) || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
		return r, nil, failure("persistence_error")
	}
	tokens := json.NewDecoder(bytes.NewReader(data))
	count := 0
	if journalJSONValue(tokens, 0, &count, 10000) != nil {
		return r, nil, failure("persistence_error")
	}
	if _, e = tokens.Token(); e != io.EOF {
		return r, nil, failure("persistence_error")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if dec.Decode(&r) != nil {
		return r, nil, failure("persistence_error")
	}
	root, e := resourceDirectory(ctx, j.lock, ".")
	if e != nil || r.ID != j.state.ResourceID || r.Ref != *j.state.Ref || r.ClaimKey != j.state.ClaimKey || r.DirectoryIdentity != dir || r.RootIdentity != root || !validResourceSlot(r.Workspace, true) || r.Results != nil && !validResourceSlot(r.Results, false) || r.OwnershipDigest != resourceDigest(r) || r.TerminalSeq < 0 || r.TerminalSeq == 0 && r.TerminalDigest != "" || r.TerminalSeq > 0 && !safeDigest(r.TerminalDigest) || r.StopReceipt != nil && r.StopReceipt.Ref != r.Ref {
		return r, nil, failure("persistence_error")
	}
	if !validResourceCompletion(r) {
		return r, nil, failure("persistence_error")
	}
	return r, info, nil
}
func (j *executionJournal) saveResourceLocked(ctx context.Context, r resourceRecord, previous os.FileInfo) error {
	dir, e := resourceDirectory(ctx, j.lock, "resources")
	if e != nil || dir != r.DirectoryIdentity {
		return failure("persistence_error")
	}
	data, e := json.Marshal(r)
	if e != nil || len(data) > 1<<20 {
		return failure("persistence_error")
	}
	if _, e = j.lock.atomicFile("resources/"+r.ID+".json", data, previous); e != nil {
		return e
	}
	actual, e := resourceDirectory(ctx, j.lock, "resources")
	if e != nil || actual != dir {
		return failure("persistence_error")
	}
	return nil
}
func (j *executionJournal) checkResourceSlots(ctx context.Context, r resourceRecord) error {
	for _, slot := range []*resourceSlot{r.Workspace, r.Results} {
		if slot != nil {
			parent, e := resourceDirectory(ctx, j.lock, path.Dir(slot.RelativePath))
			if e != nil || parent != slot.ParentIdentity {
				return failure("persistence_error")
			}
			actual, e := resourceDirectory(ctx, j.lock, slot.RelativePath)
			if e != nil || actual != slot.Identity {
				return failure("persistence_error")
			}
		}
	}
	return nil
}
func (j *executionJournal) stageWorkspace(ctx context.Context, workspace string) (protocol.NodeResourceRegistration, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	var zero protocol.NodeResourceRegistration
	if j.state.Ref == nil || !validRef(*j.state.Ref) || !j.state.StopConfirmed || j.state.CleanupFailed || j.state.Started || j.state.PendingEvent != nil {
		return zero, failure("persistence_error")
	}
	relative, e := j.resourceRelative(workspace)
	if e != nil || path.Base(relative) != "workspace" {
		return zero, failure("persistence_error")
	}
	name := path.Dir(relative)
	slot := &resourceSlot{RelativePath: name}
	if !validResourcePath(name, true) {
		return zero, failure("persistence_error")
	}
	slot.ParentIdentity, e = resourceDirectory(ctx, j.lock, path.Dir(name))
	if e != nil {
		return zero, e
	}
	slot.Identity, e = resourceDirectory(ctx, j.lock, name)
	if e != nil {
		return zero, e
	}
	// 子workspace也须是本次真实目录，不能仅用父路径当执行成功证据。
	if _, e = resourceDirectory(ctx, j.lock, relative); e != nil {
		return zero, e
	}
	if actual, e := resourceDirectory(ctx, j.lock, path.Dir(name)); e != nil || actual != slot.ParentIdentity {
		return zero, failure("persistence_error")
	}
	if j.state.ResourceID != "" {
		r, _, e := j.readResourceLocked(ctx)
		if e != nil {
			return zero, e
		}
		if *r.Workspace != *slot || r.TerminalSeq != 0 || r.StopReceipt != nil {
			return zero, failure("persistence_error")
		}
		if e = j.checkResourceSlots(ctx, r); e != nil {
			return zero, e
		}
		return resourceRegistration(r), nil
	}
	if e = j.lock.prepareDirectory("resources"); e != nil {
		return zero, e
	}
	root, e := resourceDirectory(ctx, j.lock, ".")
	if e != nil {
		return zero, e
	}
	dir, e := resourceDirectory(ctx, j.lock, "resources")
	if e != nil {
		return zero, e
	}
	r := resourceRecord{ID: uuid.NewString(), Ref: *j.state.Ref, ClaimKey: j.state.ClaimKey, RootIdentity: root, DirectoryIdentity: dir, Workspace: slot, RegistrationPending: true, PendingLog: j.state.PendingLog != nil}
	r.OwnershipDigest = resourceDigest(r)
	j.state.ResourceID = r.ID
	if e = j.saveLocked(); e != nil {
		return zero, e
	}
	if e = j.saveResourceLocked(ctx, r, nil); e != nil {
		return zero, e
	}
	return resourceRegistration(r), nil
}
func (j *executionJournal) stageResult(ctx context.Context, result string) (protocol.NodeResourceRegistration, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	var zero protocol.NodeResourceRegistration
	r, info, e := j.readResourceLocked(ctx)
	if e != nil {
		return zero, e
	}
	if r.TerminalSeq != 0 || r.StopReceipt != nil {
		return zero, failure("persistence_error")
	}
	if e = j.checkResourceSlots(ctx, r); e != nil {
		return zero, e
	}
	name, e := j.resourceRelative(result)
	if e != nil {
		return zero, e
	}
	slot := &resourceSlot{RelativePath: name}
	slot.ParentIdentity, e = resourceDirectory(ctx, j.lock, path.Dir(name))
	if e != nil {
		return zero, e
	}
	slot.Identity, e = resourceDirectory(ctx, j.lock, name)
	if e != nil || !validResourceSlot(slot, false) {
		return zero, failure("persistence_error")
	}
	if r.Results != nil {
		if *r.Results != *slot {
			return zero, failure("persistence_error")
		}
		return resourceRegistration(r), nil
	}
	// 上一登记必须精确确认，不能跨过未知workspace登记再扩槽。
	if r.RegistrationPending {
		return zero, failure("persistence_error")
	}
	if actual, e := resourceDirectory(ctx, j.lock, path.Dir(name)); e != nil || actual != slot.ParentIdentity {
		return zero, failure("persistence_error")
	}
	r.Results = slot
	r.OwnershipDigest = resourceDigest(r)
	r.RegistrationPending = true
	if e = j.saveResourceLocked(ctx, r, info); e != nil {
		return zero, e
	}
	return resourceRegistration(r), nil
}
func (j *executionJournal) ackResource(registration protocol.NodeResourceRegistration) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	ctx := context.Background()
	r, info, e := j.readResourceLocked(ctx)
	if e != nil {
		return e
	}
	if !sameResourceRegistration(resourceRegistration(r), registration) {
		return failure("invalid_response")
	}
	if e = j.checkResourceSlots(ctx, r); e != nil {
		return e
	}
	if !r.RegistrationPending {
		return nil
	}
	r.RegistrationPending = false
	return j.saveResourceLocked(ctx, r, info)
}
func (execution *taskExecution) registerWorkspace(ctx context.Context, workspace string) error {
	if e := execution.lease.check(); e != nil {
		return e
	}
	in, e := execution.journal.stageWorkspace(ctx, workspace)
	if e != nil {
		execution.lease.cancel()
		return e
	}
	return execution.registerResource(ctx, in)
}
func (execution *taskExecution) registerResult(ctx context.Context, result string) error {
	if e := execution.lease.check(); e != nil {
		return e
	}
	in, e := execution.journal.stageResult(ctx, result)
	if e != nil {
		execution.lease.cancel()
		return e
	}
	return execution.registerResource(ctx, in)
}
func (execution *taskExecution) registerResource(ctx context.Context, in protocol.NodeResourceRegistration) error {
	if e := execution.client.retryExecutionPost(ctx, "/api/agent/resources", in, nil); e != nil {
		execution.lease.cancel()
		return e
	}
	if e := execution.lease.check(); e != nil {
		return e
	}
	if e := execution.journal.ackResource(in); e != nil {
		execution.lease.cancel()
		return e
	}
	return nil
}

// 仅中央真正终态ACK或独立只读终态回执之后调用；不向旧fence提交进度。
func recordResourceTerminal(j *executionJournal, event protocol.ExecutionEvent, ack protocol.EventAck) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.state.ResourceID == "" {
		return nil
	}
	p := event.Progress
	encoded, marshalErr := json.Marshal(p)
	hash := sha256.Sum256(encoded)
	if marshalErr != nil || event.Seq < 1 || event.Digest != hex.EncodeToString(hash[:]) {
		return failure("persistence_error")
	}
	if j.state.Ref == nil || event.Ref != *j.state.Ref || p.Kind != "build_finished" || !p.StopConfirmed || p.CleanupFailed || !terminalJournalStatus(p.Status) || ack.Seq != event.Seq || ack.Digest != event.Digest || !safeDigest(ack.Digest) || j.state.PendingLog != nil || j.state.PendingStop != nil || j.state.CleanupFailed || !j.state.StopConfirmed {
		return failure("persistence_error")
	}
	if !(j.state.LastEventSeq == event.Seq && j.state.LastEventDigest == event.Digest && j.state.PendingEvent == nil) && !(j.state.PendingEvent != nil && *j.state.Ref == j.state.PendingEvent.Ref && j.state.PendingEvent.Seq == event.Seq && j.state.PendingEvent.Digest == event.Digest) {
		return failure("persistence_error")
	}
	r, info, e := j.readResourceLocked(context.Background())
	if e != nil {
		return e
	}
	if r.RegistrationPending {
		return failure("persistence_error")
	}
	if r.TerminalSeq != 0 && (r.TerminalSeq != event.Seq || r.TerminalDigest != event.Digest) {
		return failure("persistence_error")
	}
	r.TerminalSeq = event.Seq
	r.TerminalDigest = event.Digest
	r.PendingLog = false
	return j.saveResourceLocked(context.Background(), r, info)
}
func recordResourceStop(j *executionJournal, confirmation protocol.StopConfirmation) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.state.ResourceID == "" {
		return nil
	}
	if j.state.Ref == nil || confirmation.Ref != *j.state.Ref || j.state.PendingStop == nil || *j.state.PendingStop != confirmation || !j.state.StopConfirmed || j.state.CleanupFailed {
		return failure("persistence_error")
	}
	r, info, e := j.readResourceLocked(context.Background())
	if e != nil {
		return e
	}
	r.StopReceipt = &confirmation
	r.PendingLog = j.state.PendingLog != nil
	return j.saveResourceLocked(context.Background(), r, info)
}

// 当前独立节点精确终态回执已核对后，才能只读重确认原登记；绝不新建/扩槽。
func restoreResourceTerminal(ctx context.Context, client *agentHTTP, j *executionJournal, receipt protocol.TerminalReceipt, nodeName string) error {
	j.mu.Lock()
	if j.state.ResourceID == "" {
		j.mu.Unlock()
		return nil
	}
	pending := j.state.PendingEvent
	if pending == nil || j.state.Ref == nil || receipt.Ref != *j.state.Ref || receipt.Ref != pending.Ref || receipt.Seq != pending.Seq || receipt.Digest != pending.Digest || receipt.Status != pending.Progress.Status || !receipt.StopKnown || receipt.NodeName != nodeName || pending.Progress.Kind != "build_finished" || !pending.Progress.StopConfirmed || pending.Progress.CleanupFailed {
		j.mu.Unlock()
		return failure("persistence_error")
	}
	event := *pending
	r, _, e := j.readResourceLocked(ctx)
	j.mu.Unlock()
	if e != nil {
		return e
	}
	if r.RegistrationPending {
		in := resourceRegistration(r)
		if e = client.retryPost(ctx, "/api/agent/resources", in, nil); e != nil {
			return e
		}
		if e = j.ackResource(in); e != nil {
			return e
		}
	}
	return recordResourceTerminal(j, event, protocol.EventAck{Seq: receipt.Seq, Digest: receipt.Digest})
}
