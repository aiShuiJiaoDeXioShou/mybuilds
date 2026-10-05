//go:build darwin || linux

package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"os"
	"path"
	"strings"
	"time"

	"golang.org/x/sys/unix"
	"mybuilds/internal/protocol"
)

func (j *nodeDeletionJournal) current(ctx context.Context) error {
	data, info, _, e := deletionPrivateFile(ctx, j.lock, "deletions", j.state.Item.ID)
	if e != nil {
		return e
	}
	sum := sha256.Sum256(data)
	if !os.SameFile(info, j.info) || hex.EncodeToString(sum[:]) != j.digest {
		return failure("resource_unconfirmed")
	}
	return nil
}
func (j *nodeDeletionJournal) save(ctx context.Context) error {
	if e := resourceContext(ctx); e != nil {
		return e
	}
	if j.info != nil {
		if e := j.current(ctx); e != nil {
			return e
		}
	}
	data, e := json.Marshal(j.state)
	if e != nil {
		return failure("persistence_error")
	}
	info, e := j.lock.atomicFile("deletions/"+j.state.Item.ID+".json", data, j.info)
	if e != nil {
		return e
	}
	sum := sha256.Sum256(data)
	j.info = info
	j.digest = hex.EncodeToString(sum[:])
	return nil
}
func (j *nodeDeletionJournal) confirm(ctx context.Context, client *agentHTTP) error {
	if j.state.Pending == nil {
		return nil
	}
	if e := j.current(ctx); e != nil {
		return e
	}
	original := *j.state.Pending
	var receipt protocol.NodeDeletionReceipt
	if e := client.post(ctx, "/api/agent/deletions/"+j.state.Item.ID+"/confirm", original, &receipt); e != nil {
		return e
	}
	if receipt.ID != original.ID || receipt.Seq != original.Seq || receipt.Digest != original.Digest {
		return failure("invalid_response")
	}
	if e := j.current(ctx); e != nil {
		return e
	}
	complete := (!j.state.Item.HasWorkspace || original.WorkspaceState == "deleted") && (!j.state.Item.HasResults || original.ResultsState == "deleted")
	if complete || j.state.Progress == nil {
		return j.lock.removeFile("deletions/"+j.state.Item.ID+".json", j.info)
	}
	j.state.Progress.Seq = original.Seq
	j.state.Progress.Active = false
	j.state.Pending = nil
	return j.save(ctx)
}

// 恢复仅补原已落盘确认；不取新权限、不解释PID、不做任何文件删除。
func recoverNodeDeletionConfirmations(ctx context.Context, client *agentHTTP, lock *dataLock, nodeID string) error {
	if !exactUUID(nodeID) || client == nil {
		return failure("resource_unconfirmed")
	}
	if e := lock.Check(); e != nil {
		return e
	}
	info, e := lock.root.Lstat("deletions")
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil || !privateInfo(info, true) {
		return failure("resource_unconfirmed")
	}
	f, e := lock.root.OpenFile("deletions", os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if e != nil {
		return failure("resource_unconfirmed")
	}
	defer f.Close()
	opened, e := f.Stat()
	if e != nil || !os.SameFile(info, opened) {
		return failure("resource_unconfirmed")
	}
	entries, e := f.ReadDir(129)
	if e != nil && e != io.EOF || len(entries) > 128 {
		return failure("resource_unconfirmed")
	}
	for _, entry := range entries {
		if e = resourceContext(ctx); e != nil {
			return e
		}
		if !strings.HasSuffix(entry.Name(), ".json") || !exactUUID(strings.TrimSuffix(entry.Name(), ".json")) {
			return failure("resource_unconfirmed")
		}
		j, e := readNodeDeletionJournal(lock, strings.TrimSuffix(entry.Name(), ".json"))
		if e != nil {
			return e
		}
		if j.state.NodeID != nodeID {
			return failure("resource_unconfirmed")
		}
		if j.state.Pending == nil {
			record, e := deletionResourceRecord(ctx, lock, nodeID, j.state.Item)
			if e != nil {
				return e
			}
			if e = deletionKnownProgress(ctx, j, record); e != nil {
				return e
			}
		}
		if e = j.confirm(ctx, client); e != nil {
			return e
		}
	}
	current, e := lock.root.Lstat("deletions")
	if e != nil || !os.SameFile(info, current) {
		return failure("resource_unconfirmed")
	}
	return lock.Check()
}

// 原始归属来自登记文件；网络事项绝不提供路径或新的槽。
func advanceNodeDeletion(ctx context.Context, client *agentHTTP, lock *dataLock, nodeID string, item protocol.NodeDeletion) error {
	if !exactUUID(nodeID) || !validNodeDeletion(item) {
		return failure("resource_unconfirmed")
	}
	if client == nil || client.paused.Load() {
		return failure("authority_expired")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var j *nodeDeletionJournal
	if _, e := lock.root.Lstat("deletions/" + item.ID + ".json"); e == nil {
		var err error
		j, err = readNodeDeletionJournal(lock, item.ID)
		if err != nil {
			return err
		}
		if j.state.Item != item || j.state.NodeID != nodeID {
			return failure("resource_unconfirmed")
		}
		if j.state.Pending != nil {
			p := *j.state.Pending
			if e = j.confirm(ctx, client); e != nil {
				return e
			}
			if (!item.HasWorkspace || p.WorkspaceState == "deleted") && (!item.HasResults || p.ResultsState == "deleted") {
				return nil
			}
		}
		if j.state.Progress == nil {
			return failure("resource_unconfirmed")
		}
	} else if !os.IsNotExist(e) {
		return failure("resource_unconfirmed")
	}
	visited := 0
	for {
		if client.paused.Load() {
			return failure("authority_expired")
		}
		for _, d := range []struct {
			name   string
			absent bool
		}{{"journal", false}, {"spool", true}} {
			if e := deletionDirectoryEmpty(ctx, lock, d.name, d.absent); e != nil {
				return e
			}
		}
		r, e := deletionResourceRecord(ctx, lock, nodeID, item)
		if e != nil {
			return e
		}
		if j == nil {
			if _, e = deletionResourceGuard(ctx, lock, nodeID, item); e != nil {
				return e
			}
		}
		requested := time.Now()
		var authority protocol.DeletionAuthority
		if e = client.post(ctx, "/api/agent/deletions/"+item.ID+"/authorize", struct{}{}, &authority); e != nil {
			return e
		}
		deadline, e := deletionDeadline(requested, authority, nodeID, item)
		if e != nil {
			return e
		}
		if j == nil {
			if e = lock.prepareDirectory("deletions"); e != nil {
				return e
			}
			root, e := resourceDirectory(ctx, lock, ".")
			if e != nil {
				return e
			}
			dir, e := resourceDirectory(ctx, lock, "deletions")
			if e != nil {
				return e
			}
			progress := &nodeDeletionProgress{WorkspaceState: "not_applicable", ResultsState: "not_applicable", Nonce: authority.Nonce, Active: true}
			if item.HasWorkspace {
				progress.WorkspaceState = "pending"
			}
			if item.HasResults {
				progress.ResultsState = "pending"
			}
			j = &nodeDeletionJournal{lock: lock, state: nodeDeletionJournalState{Item: item, NodeID: nodeID, RootIdentity: root, DirectoryIdentity: dir, Progress: progress}}
			if e = j.save(ctx); e != nil {
				return e
			}
		}
		j.state.Progress.Nonce = authority.Nonce
		j.state.Progress.Active = true
		if e = j.save(ctx); e != nil {
			return e
		}
		segment := &deletionSegment{ctx: ctx, client: client, j: j, deadline: deadline, started: time.Now(), visited: &visited}
		e = segment.run(r)
		if e != nil && e.Error() == "agent_persistence_error" {
			return e
		}
		p := j.state.Progress
		reason := ""
		workspace, results := deletionWireState(p.WorkspaceState), deletionWireState(p.ResultsState)
		if workspace == "partial" || results == "partial" {
			reason = "partial"
		}
		if e != nil {
			reason = deletionSafeReason(e)
			if workspace == "partial" {
				workspace = "failed"
			}
			if results == "partial" {
				results = "failed"
			}
		}
		if p.Seq == math.MaxInt64 {
			return failure("resource_unconfirmed")
		}
		confirmation := protocol.NodeDeletionConfirmation{ID: item.ID, ResourceID: item.ResourceID, OwnershipDigest: item.OwnershipDigest, Nonce: authority.Nonce, Seq: p.Seq + 1, WorkspaceState: workspace, ResultsState: results, Reason: reason}
		digest, e2 := protocol.NodeDeletionDigest(confirmation)
		confirmation.Digest = digest
		if e2 != nil || !validDeletionConfirmation(item, confirmation) {
			return failure("resource_unconfirmed")
		}
		j.state.Pending = &confirmation
		if e2 = j.save(ctx); e2 != nil {
			return e2
		}
		if e2 = j.confirm(ctx, client); e2 != nil {
			return e2
		}
		if e != nil {
			return e
		}
		if reason == "" {
			return nil
		}
	}
}
func deletionWireState(stage string) string {
	if stage == "pending" || stage == "quarantined" {
		return "partial"
	}
	return stage
}
func deletionSafeReason(e error) string {
	switch e.Error() {
	case "agent_identity_mismatch":
		return "identity_mismatch"
	case "agent_invalid_object":
		return "invalid_object"
	case "agent_permission_denied":
		return "permission_denied"
	case "agent_authority_expired", "agent_authority_lost":
		return "authority_expired"
	case "agent_operation_timeout":
		return "operation_timeout"
	case "agent_resource_unconfirmed":
		return "resource_unconfirmed"
	default:
		return "persistence_error"
	}
}

type deletionSegment struct {
	ctx               context.Context
	client            *agentHTTP
	j                 *nodeDeletionJournal
	deadline, started time.Time
	count             int
	visited           *int
	ancestors         []deletionAncestor
}

func (s *deletionSegment) check() error {
	if s.client.paused.Load() || !time.Now().Before(s.deadline) {
		return failure("authority_expired")
	}
	if e := resourceContext(s.ctx); e != nil {
		return e
	}
	if s.j.state.Progress.RetentionIdentity != "" {
		actual, e := resourceDirectory(s.ctx, s.j.lock, "retention")
		if e != nil || actual != s.j.state.Progress.RetentionIdentity {
			return failure("identity_mismatch")
		}
	}
	return s.j.lock.Check()
}
func (s *deletionSegment) exhausted() bool {
	return s.count >= 100 || time.Since(s.started) >= 500*time.Millisecond
}
func (s *deletionSegment) run(r resourceRecord) error {
	if e := s.check(); e != nil {
		return e
	}
	lock := s.j.lock
	p := s.j.state.Progress
	q := "retention/" + s.j.state.Item.ID
	if e := lock.prepareDirectory("retention"); e != nil {
		return e
	}
	retention, e := resourceDirectory(s.ctx, lock, "retention")
	if e != nil {
		return e
	}
	if p.RetentionIdentity == "" {
		p.RetentionIdentity = retention
		if e = s.j.save(s.ctx); e != nil {
			return e
		}
	} else if p.RetentionIdentity != retention {
		return failure("identity_mismatch")
	}
	if p.QuarantineIdentity == "" {
		if e := s.check(); e != nil {
			return e
		}
		if e := lock.root.Mkdir(q, 0700); e != nil {
			return failure("identity_mismatch")
		}
		s.count++
		identity, e := resourceDirectory(s.ctx, lock, q)
		if e != nil {
			return e
		}
		p.QuarantineIdentity = identity
		if e = lock.syncDirectory("retention"); e != nil {
			return e
		}
		if e = s.j.save(s.ctx); e != nil {
			return e
		}
	}
	identity, e := resourceDirectory(s.ctx, lock, q)
	if e != nil || identity != p.QuarantineIdentity {
		return failure("identity_mismatch")
	}
	for _, slot := range []struct {
		name     string
		original *resourceSlot
		stage    *string
	}{{"workspace", r.Workspace, &p.WorkspaceState}, {"results", r.Results, &p.ResultsState}} {
		if slot.original == nil || *slot.stage == "deleted" {
			continue
		}
		if s.exhausted() {
			return nil
		}
		if e = s.check(); e != nil {
			return e
		}
		target := q + "/" + slot.name
		if *slot.stage == "pending" {
			// 同一源/目标出生身份能识别rename成功而随后journal保存失败的窗口。
			at, e := resourceDirectory(s.ctx, lock, target)
			if e == nil {
				if at != slot.original.Identity {
					return failure("identity_mismatch")
				}
				if _, e = lock.root.Lstat(slot.original.RelativePath); !os.IsNotExist(e) {
					return failure("identity_mismatch")
				}
			} else {
				if _, e = lock.root.Lstat(target); !os.IsNotExist(e) {
					return failure("identity_mismatch")
				}
				source, e := resourceDirectory(s.ctx, lock, slot.original.RelativePath)
				if e != nil || source != slot.original.Identity {
					return failure("identity_mismatch")
				}
				parent, e := resourceDirectory(s.ctx, lock, path.Dir(slot.original.RelativePath))
				if e != nil || parent != slot.original.ParentIdentity {
					return failure("identity_mismatch")
				}
				from, e := lock.root.OpenFile(path.Dir(slot.original.RelativePath), os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
				if e != nil {
					return failure("permission_denied")
				}
				to, e := lock.root.OpenFile(q, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
				if e != nil {
					from.Close()
					return failure("permission_denied")
				}
				e = s.check()
				if e == nil {
					e = deletionRenameNoReplace(from, path.Base(slot.original.RelativePath), to, slot.name)
				}
				if e == nil {
					e = from.Sync()
				}
				if e == nil {
					e = to.Sync()
				}
				from.Close()
				to.Close()
				if e != nil {
					return failure("persistence_error")
				}
				s.count++
				actual, e := resourceDirectory(s.ctx, lock, target)
				if e != nil || actual != slot.original.Identity {
					return failure("identity_mismatch")
				}
			}
			*slot.stage = "quarantined"
			if e = s.j.save(s.ctx); e != nil {
				return e
			}
		}
		if _, e = lock.root.Lstat(target); os.IsNotExist(e) {
			// unlink完成后未保存的窗口只有固定隔离槽不存在；原源也必须保持不存在。
			if _, e = lock.root.Lstat(slot.original.RelativePath); !os.IsNotExist(e) {
				return failure("identity_mismatch")
			}
			*slot.stage = "deleted"
			if e = s.j.save(s.ctx); e != nil {
				return e
			}
			continue
		}
		actual, e := resourceDirectory(s.ctx, lock, target)
		if e != nil || actual != slot.original.Identity {
			return failure("identity_mismatch")
		}
		parent, e := lock.root.OpenFile(q, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
		if e != nil {
			return failure("permission_denied")
		}
		complete, e := s.removeEntry(parent, q, slot.name, 0, slot.original.Identity)
		parent.Close()
		if e != nil {
			return e
		}
		if complete {
			*slot.stage = "deleted"
			if e = s.j.save(s.ctx); e != nil {
				return e
			}
		}
	}
	return nil
}

// 使用已打开目录FD进行单项删除，拒链接、特殊文件、多硬链接和出生身份替换。
func (s *deletionSegment) removeEntry(parent *os.File, parentPath, name string, depth int, expected string) (bool, error) {
	if s.visited == nil || *s.visited >= 100000 {
		return false, failure("invalid_object")
	}
	*s.visited++
	if depth > 64 {
		return false, failure("invalid_object")
	}
	if s.exhausted() {
		return false, nil
	}
	if e := s.check(); e != nil {
		return false, e
	}
	qidentity, e := resourceDirectory(s.ctx, s.j.lock, "retention/"+s.j.state.Item.ID)
	if e != nil || qidentity != s.j.state.Progress.QuarantineIdentity {
		return false, failure("identity_mismatch")
	}
	if e = s.ancestorsMatch(); e != nil {
		return false, e
	}
	if depth == 0 {
		info, e := parent.Stat()
		if e != nil {
			return false, failure("identity_mismatch")
		}
		identity, e := deletionObjectIdentity(parent, info)
		if e != nil || identity != s.j.state.Progress.QuarantineIdentity {
			return false, failure("identity_mismatch")
		}
	}
	var before unix.Stat_t
	if e = unix.Fstatat(int(parent.Fd()), name, &before, unix.AT_SYMLINK_NOFOLLOW); e != nil {
		return false, failure("permission_denied")
	}
	if before.Mode&unix.S_IFMT != unix.S_IFDIR && (before.Mode&unix.S_IFMT != unix.S_IFREG || before.Nlink != 1) {
		return false, failure("invalid_object")
	}
	fd, e := unix.Openat(int(parent.Fd()), name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if e != nil {
		return false, failure("permission_denied")
	}
	f := os.NewFile(uintptr(fd), name)
	defer f.Close()
	info, e := f.Stat()
	if e != nil {
		return false, failure("invalid_object")
	}
	identity, e := deletionObjectIdentity(f, info)
	if e != nil {
		return false, e
	}
	if expected != "" && expected != identity {
		return false, failure("identity_mismatch")
	}
	var current unix.Stat_t
	if e = unix.Fstat(fd, &current); e != nil || before.Dev != current.Dev || before.Ino != current.Ino || before.Mode&unix.S_IFMT != current.Mode&unix.S_IFMT || current.Uid != uint32(os.Geteuid()) || (current.Mode&unix.S_IFMT == unix.S_IFREG && current.Nlink != 1) {
		return false, failure("identity_mismatch")
	}
	if info.IsDir() {
		s.ancestors = append(s.ancestors, deletionAncestor{parent: parent, child: f, name: name, identity: identity})
		defer func() { s.ancestors = s.ancestors[:len(s.ancestors)-1] }()
		for {
			if s.exhausted() {
				return false, nil
			}
			entries, e := f.ReadDir(1)
			if e != nil && e != io.EOF {
				return false, failure("permission_denied")
			}
			if len(entries) == 0 {
				break
			}
			child := entries[0].Name()
			if child == "." || child == ".." || strings.ContainsAny(child, "/\\\x00") {
				return false, failure("invalid_object")
			}
			done, e := s.removeEntry(f, parentPath+"/"+name, child, depth+1, "")
			if e != nil || !done {
				return false, e
			}
		}
	}
	if s.exhausted() {
		return false, nil
	}
	if e = s.check(); e != nil {
		return false, e
	}
	if e = s.ancestorsMatch(); e != nil {
		return false, e
	}
	same, e := deletionObjectIdentity(f, info)
	if e != nil || same != identity {
		return false, failure("identity_mismatch")
	}
	if e = unix.Fstatat(int(parent.Fd()), name, &current, unix.AT_SYMLINK_NOFOLLOW); e != nil || before.Dev != current.Dev || before.Ino != current.Ino || before.Mode&unix.S_IFMT != current.Mode&unix.S_IFMT || current.Uid != uint32(os.Geteuid()) || (current.Mode&unix.S_IFMT == unix.S_IFREG && current.Nlink != 1) {
		return false, failure("identity_mismatch")
	}
	flags := 0
	if info.IsDir() {
		flags = unix.AT_REMOVEDIR
	}
	if e = unix.Unlinkat(int(parent.Fd()), name, flags); e != nil {
		return false, failure("permission_denied")
	}
	s.count++
	if e = parent.Sync(); e != nil {
		return false, failure("persistence_error")
	}
	return true, nil
}

// 递归只保留当前实际打开的祖先FD，不反复解析深路径或缓存未来对象。
type deletionAncestor struct {
	parent, child  *os.File
	name, identity string
}

func (s *deletionSegment) ancestorsMatch() error {
	for _, a := range s.ancestors {
		info, e := a.child.Stat()
		if e != nil {
			return failure("identity_mismatch")
		}
		identity, e := deletionObjectIdentity(a.child, info)
		if e != nil || identity != a.identity {
			return failure("identity_mismatch")
		}
		var current, opened unix.Stat_t
		if unix.Fstat(int(a.child.Fd()), &opened) != nil || unix.Fstatat(int(a.parent.Fd()), a.name, &current, unix.AT_SYMLINK_NOFOLLOW) != nil || current.Dev != opened.Dev || current.Ino != opened.Ino || current.Mode&unix.S_IFMT != unix.S_IFDIR || current.Uid != uint32(os.Geteuid()) {
			return failure("identity_mismatch")
		}
	}
	return nil
}

// 重启只核已知隔离状态；可验证的半途进度留原文件，由后续新管理段推进。
func deletionKnownProgress(ctx context.Context, j *nodeDeletionJournal, r resourceRecord) error {
	p := j.state.Progress
	if p == nil {
		return failure("resource_unconfirmed")
	}
	q := "retention/" + j.state.Item.ID
	if p.RetentionIdentity != "" {
		actual, e := resourceDirectory(ctx, j.lock, "retention")
		if e != nil || actual != p.RetentionIdentity {
			return failure("resource_unconfirmed")
		}
	}
	qinfo, e := j.lock.root.Lstat(q)
	if p.QuarantineIdentity == "" {
		if !os.IsNotExist(e) {
			return failure("resource_unconfirmed")
		}
	} else {
		if e != nil || !privateInfo(qinfo, true) {
			return failure("resource_unconfirmed")
		}
		actual, e := resourceDirectory(ctx, j.lock, q)
		if e != nil || actual != p.QuarantineIdentity {
			return failure("resource_unconfirmed")
		}
		f, e := j.lock.root.OpenFile(q, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
		if e != nil {
			return failure("resource_unconfirmed")
		}
		entries, e := f.ReadDir(3)
		f.Close()
		if e != nil && e != io.EOF || len(entries) > 2 {
			return failure("resource_unconfirmed")
		}
		for _, entry := range entries {
			if entry.Name() != "workspace" && entry.Name() != "results" {
				return failure("resource_unconfirmed")
			}
		}
	}
	for _, slot := range []struct {
		name     string
		original *resourceSlot
		stage    string
	}{{"workspace", r.Workspace, p.WorkspaceState}, {"results", r.Results, p.ResultsState}} {
		if slot.original == nil {
			if _, e = j.lock.root.Lstat(q + "/" + slot.name); !os.IsNotExist(e) {
				return failure("resource_unconfirmed")
			}
			continue
		}
		_, sourceErr := j.lock.root.Lstat(slot.original.RelativePath)
		_, targetErr := j.lock.root.Lstat(q + "/" + slot.name)
		if slot.stage == "pending" && sourceErr == nil {
			original, e := resourceDirectory(ctx, j.lock, slot.original.RelativePath)
			if e != nil || original != slot.original.Identity || !os.IsNotExist(targetErr) {
				return failure("resource_unconfirmed")
			}
			parent, e := resourceDirectory(ctx, j.lock, path.Dir(slot.original.RelativePath))
			if e != nil || parent != slot.original.ParentIdentity {
				return failure("resource_unconfirmed")
			}
			continue
		}
		if !os.IsNotExist(sourceErr) || p.QuarantineIdentity == "" {
			return failure("resource_unconfirmed")
		}
		if slot.stage == "deleted" {
			if !os.IsNotExist(targetErr) {
				return failure("resource_unconfirmed")
			}
			continue
		}
		if os.IsNotExist(targetErr) && slot.stage == "quarantined" {
			continue
		}
		target, e := resourceDirectory(ctx, j.lock, q+"/"+slot.name)
		if e != nil || target != slot.original.Identity {
			return failure("resource_unconfirmed")
		}
	}
	return resourceContext(ctx)
}
