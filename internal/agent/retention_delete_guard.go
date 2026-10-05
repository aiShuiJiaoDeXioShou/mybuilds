//go:build darwin || linux

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"time"

	"golang.org/x/sys/unix"
	"mybuilds/internal/protocol"
)

// 只有已登记的固定ID可读；不扫描源码、进程表或任意资源路径。
func deletionPrivateFile(ctx context.Context, lock *dataLock, directory, id string) ([]byte, os.FileInfo, string, error) {
	if directory != "resources" && directory != "deletions" || !exactUUID(id) {
		return nil, nil, "", failure("resource_unconfirmed")
	}
	if e := resourceContext(ctx); e != nil {
		return nil, nil, "", e
	}
	if e := lock.Check(); e != nil {
		return nil, nil, "", e
	}
	parent, e := resourceDirectory(ctx, lock, directory)
	if e != nil {
		return nil, nil, "", e
	}
	name := directory + "/" + id + ".json"
	before, e := lock.root.Lstat(name)
	if e != nil || !privateInfo(before, false) || before.Size() > 1<<20 {
		return nil, nil, "", failure("resource_unconfirmed")
	}
	f, e := lock.root.OpenFile(name, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if e != nil {
		return nil, nil, "", failure("resource_unconfirmed")
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !privateInfo(info, false) || !os.SameFile(before, info) {
		return nil, nil, "", failure("resource_unconfirmed")
	}
	data, e := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	after, a := f.Stat()
	now, n := lock.root.Lstat(name)
	if e != nil || a != nil || n != nil || !privateInfo(after, false) || !privateInfo(now, false) || len(data) == 0 || len(data) > 1<<20 || !os.SameFile(after, info) || !os.SameFile(now, info) || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
		return nil, nil, "", failure("resource_unconfirmed")
	}
	actual, e := resourceDirectory(ctx, lock, directory)
	if e != nil || actual != parent {
		return nil, nil, "", failure("resource_unconfirmed")
	}
	if e = resourceContext(ctx); e != nil {
		return nil, nil, "", e
	}
	if e = lock.Check(); e != nil {
		return nil, nil, "", e
	}
	return data, info, parent, nil
}
func deletionJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	count := 0
	if e := journalJSONValue(dec, 0, &count); e != nil {
		return failure("resource_unconfirmed")
	}
	if _, e := dec.Token(); e != io.EOF {
		return failure("resource_unconfirmed")
	}
	return nil
}

// 管理删除没有执行租约；本地活动/未知日志在每段前独立挡住操作。
func deletionDirectoryEmpty(ctx context.Context, lock *dataLock, name string, allowAbsent bool) error {
	if e := resourceContext(ctx); e != nil {
		return e
	}
	info, e := lock.root.Lstat(name)
	if os.IsNotExist(e) && allowAbsent {
		return nil
	}
	if e != nil || !privateInfo(info, true) {
		return failure("resource_unconfirmed")
	}
	f, e := lock.root.OpenFile(name, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if e != nil {
		return failure("resource_unconfirmed")
	}
	defer f.Close()
	opened, e := f.Stat()
	if e != nil || !privateInfo(opened, true) || !os.SameFile(opened, info) {
		return failure("resource_unconfirmed")
	}
	entries, e := f.ReadDir(1)
	if e != nil && e != io.EOF || len(entries) != 0 {
		return failure("resource_unconfirmed")
	}
	now, e := lock.root.Lstat(name)
	if e != nil || !privateInfo(now, true) || !os.SameFile(opened, now) {
		return failure("resource_unconfirmed")
	}
	return resourceContext(ctx)
}
func deletionResourceGuard(ctx context.Context, lock *dataLock, nodeID string, item protocol.NodeDeletion) (resourceRecord, error) {
	var zero resourceRecord
	if !exactUUID(nodeID) || !validNodeDeletion(item) {
		return zero, failure("resource_unconfirmed")
	}
	if e := lock.Check(); e != nil {
		return zero, e
	}
	for _, d := range []struct {
		name   string
		absent bool
	}{{"journal", false}, {"spool", true}} {
		if e := deletionDirectoryEmpty(ctx, lock, d.name, d.absent); e != nil {
			return zero, e
		}
	}
	r, e := deletionResourceRecord(ctx, lock, nodeID, item)
	if e != nil {
		return zero, e
	}
	j := &executionJournal{lock: lock, state: journalState{ResourceID: item.ResourceID, Ref: &r.Ref, ClaimKey: r.ClaimKey}}
	if e = j.checkResourceSlots(ctx, r); e != nil {
		return zero, e
	}
	if e = lock.Check(); e != nil {
		return zero, e
	}
	if e = resourceContext(ctx); e != nil {
		return zero, e
	}
	return r, nil
}

// 补传确认只核不可变归属，不以原目录已删除为由误拒真实旧回执。
func deletionResourceRecord(ctx context.Context, lock *dataLock, nodeID string, item protocol.NodeDeletion) (resourceRecord, error) {
	var zero resourceRecord
	if !exactUUID(nodeID) || !validNodeDeletion(item) {
		return zero, failure("resource_unconfirmed")
	}
	data, info, _, e := deletionPrivateFile(ctx, lock, "resources", item.ResourceID)
	if e != nil {
		return zero, e
	}
	if e = deletionJSON(data); e != nil {
		return zero, e
	}
	var initial resourceRecord
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if dec.Decode(&initial) != nil {
		return zero, failure("resource_unconfirmed")
	}
	j := &executionJournal{lock: lock, state: journalState{ResourceID: item.ResourceID, Ref: &initial.Ref, ClaimKey: initial.ClaimKey}}
	r, again, e := j.readResource()
	beforeJSON, beforeErr := json.Marshal(initial)
	afterJSON, afterErr := json.Marshal(r)
	if e != nil || beforeErr != nil || afterErr != nil || !bytes.Equal(beforeJSON, afterJSON) || !os.SameFile(info, again) || r.Ref.NodeID != nodeID || r.Ref.BuildID != item.BuildID || r.Ref.AttemptID != item.AttemptID || r.OwnershipDigest != item.OwnershipDigest || (r.Workspace != nil) != item.HasWorkspace || (r.Results != nil) != item.HasResults || r.RegistrationPending || r.PendingLog {
		return zero, failure("resource_unconfirmed")
	}
	if r.TerminalSeq == 0 && !r.CompletionConfirmed {
		return zero, failure("resource_unconfirmed")
	}

	if e = resourceContext(ctx); e != nil {
		return zero, e
	}
	return r, nil
}

// 单调时钟只来自本次真实请求起点，远端ExpiresAt仅审计，不能延长权限。
func deletionDeadline(requested time.Time, authority protocol.DeletionAuthority, nodeID string, item protocol.NodeDeletion) (time.Time, error) {
	if requested.IsZero() || requested.After(time.Now()) || !validNodeDeletion(item) || !exactUUID(nodeID) || authority.NodeID != nodeID || authority.ID != item.ID || authority.ResourceID != item.ResourceID || authority.OwnershipDigest != item.OwnershipDigest || !exactUUID(authority.Nonce) || authority.ExpiresAt.IsZero() {
		return time.Time{}, failure("invalid_response")
	}
	deadline := requested.Add(5*time.Second - 100*time.Millisecond)
	if !time.Now().Before(deadline) {
		return time.Time{}, failure("authority_expired")
	}
	return deadline, nil
}
