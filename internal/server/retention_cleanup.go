package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"time"

	"github.com/google/uuid"
	"mybuilds/internal/store"
)

// 一轮只消费Store登记的固定对象；文件IO始终在短数据库授权之外。
func (s *Server) advanceCentralRetention(ctx context.Context, projectID string, limit int) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	objects, err := s.store.AdvanceRetention(ctx, projectID, limit)
	if err != nil {
		return err
	}
	var first error
	for _, object := range objects {
		if ctx.Err() != nil {
			return store.ErrRetentionIO
		}
		if err = s.cleanCentralRetentionObject(ctx, object); err != nil && first == nil {
			first = err
		}
	}
	return first
}

type retentionCleanupRoots struct {
	chain              []*os.Root
	source, quarantine *os.Root
}

func (r *retentionCleanupRoots) close() {
	for i := len(r.chain) - 1; i >= 0; i-- {
		_ = r.chain[i].Close()
	}
}
func (r *retentionCleanupRoots) check() error {
	for _, root := range r.chain {
		if checkEvidenceRoot(root) != nil {
			return store.ErrRetentionOwnershipUnknown
		}
	}
	return nil
}
func retentionUUID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.String() == value
}
func (s *Server) retentionCleanupRoots(o store.RetentionObject) (*retentionCleanupRoots, error) {
	if !retentionUUID(o.ID) || !retentionUUID(o.JobID) || !retentionUUID(o.ObjectID) || !retentionUUID(o.StorageID) || o.Size < 0 {
		return nil, store.ErrRetentionOwnershipUnknown
	}
	kind := "artifacts"
	if o.Kind == "log" {
		kind = "logs"
	} else if o.Kind != "artifact" && o.Kind != "junit" {
		return nil, store.ErrRetentionOwnershipUnknown
	}
	if o.QuarantineSlot != "" && o.QuarantineSlot != "retention/quarantine/"+o.JobID+"/"+o.ObjectID {
		return nil, store.ErrRetentionOwnershipUnknown
	}
	info, err := os.Lstat(s.config.DataDir)
	if err != nil || !evidenceInfo(info, true) {
		return nil, store.ErrRetentionOwnershipUnknown
	}
	data, err := os.OpenRoot(s.config.DataDir)
	if err != nil {
		return nil, store.ErrRetentionIO
	}
	r := &retentionCleanupRoots{chain: []*os.Root{data}}
	fail := func(err error) (*retentionCleanupRoots, error) { r.close(); return nil, err }
	held, err := data.Stat(".")
	if err != nil || !os.SameFile(held, info) || r.check() != nil {
		return fail(store.ErrRetentionOwnershipUnknown)
	}
	// 每一层均是固定目录名或已验证UUID，不扫描未知子项。
	open := func(parent *os.Root, name string, create bool) (*os.Root, error) {
		if r.check() != nil {
			return nil, store.ErrRetentionOwnershipUnknown
		}
		if create {
			created := parent.Mkdir(name, 0700)
			if created != nil && !os.IsExist(created) {
				return nil, store.ErrRetentionIO
			}
			if created == nil {
				if err := retentionSyncDirectory(parent); err != nil {
					return nil, err
				}
			}
		}
		before, err := parent.Lstat(name)
		if err != nil || !evidenceInfo(before, true) {
			return nil, store.ErrRetentionOwnershipUnknown
		}
		child, err := parent.OpenRoot(name)
		if err != nil {
			return nil, store.ErrRetentionOwnershipUnknown
		}
		r.chain = append(r.chain, child)
		held, err := child.Stat(".")
		current, ce := parent.Lstat(name)
		if err != nil || ce != nil || !os.SameFile(before, held) || !os.SameFile(before, current) || r.check() != nil {
			return nil, store.ErrRetentionOwnershipUnknown
		}
		return child, nil
	}
	if r.source, err = open(data, kind, false); err != nil {
		return fail(err)
	}
	parent := data
	for _, name := range []string{"retention", "quarantine", o.JobID} {
		parent, err = open(parent, name, true)
		if err != nil {
			return fail(err)
		}
	}
	r.quarantine = parent
	return r, nil
}

// ENOENT仅在所有已持目录仍是原私有目录时成立，权限/FIFO/链接均不能当缺失。
func retentionLeaf(root *os.Root, name string) (os.FileInfo, bool, error) {
	info, err := root.Lstat(name)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, store.ErrRetentionIO
	}
	if !evidenceInfo(info, false) {
		return nil, false, store.ErrRetentionOwnershipUnknown
	}
	return info, true, nil
}
func retentionSyncDirectory(root *os.Root) error {
	dir, err := root.OpenFile(".", os.O_RDONLY|evidenceOpenFlags(), 0)
	if err != nil {
		return store.ErrRetentionIO
	}
	err = dir.Sync()
	closed := dir.Close()
	if err != nil || closed != nil {
		return store.ErrRetentionIO
	}
	return nil
}

func retentionObserve(ctx context.Context, f *os.File, expected store.RetentionObject) (store.RetentionObjectObservation, error) {
	identity, err := retentionFileIdentity(f)
	if err != nil {
		return store.RetentionObjectObservation{}, err
	}
	before, err := f.Stat()
	if err != nil || before.Size() != expected.Size {
		return store.RetentionObjectObservation{}, store.ErrRetentionOwnershipUnknown
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return store.RetentionObjectObservation{}, store.ErrRetentionIO
	}
	h := sha256.New()
	buffer := make([]byte, 64*1024)
	var count int64
	for {
		if ctx.Err() != nil {
			return store.RetentionObjectObservation{}, store.ErrRetentionIO
		}
		n, readErr := f.Read(buffer)
		if n > 0 {
			count += int64(n)
			if count > expected.Size {
				return store.RetentionObjectObservation{}, store.ErrRetentionOwnershipUnknown
			}
			_, _ = h.Write(buffer[:n])
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil || n == 0 {
			return store.RetentionObjectObservation{}, store.ErrRetentionIO
		}
	}
	after, err := f.Stat()
	again, identityErr := retentionFileIdentity(f)
	digest := hex.EncodeToString(h.Sum(nil))
	if err != nil || identityErr != nil || again != identity || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) || count != expected.Size || digest != expected.SHA256 {
		return store.RetentionObjectObservation{}, store.ErrRetentionOwnershipUnknown
	}
	return store.RetentionObjectObservation{Identity: identity, Size: count, SHA256: digest}, nil
}
func retentionCheckLeaf(r *retentionCleanupRoots, root *os.Root, name string, f *os.File, identity string, verified os.FileInfo) error {
	if r.check() != nil {
		return store.ErrRetentionOwnershipUnknown
	}
	current, exists, err := retentionLeaf(root, name)
	held, he := f.Stat()
	actual, ae := retentionFileIdentity(f)
	if err != nil || !exists || he != nil || ae != nil || actual != identity || !os.SameFile(current, held) || held.Size() != verified.Size() || !held.ModTime().Equal(verified.ModTime()) {
		return store.ErrRetentionOwnershipUnknown
	}
	return nil
}
func (s *Server) cleanCentralRetentionObject(ctx context.Context, o store.RetentionObject) (err error) {
	// 初次失败只记固定原因；真实授权落盘后不能回退到无身份登记入口。
	authorizedOnce := false
	defer func() {
		if err == nil || authorizedOnce || o.Identity != "" || o.QuarantineSlot != "" {
			return
		}
		reason := ""
		switch {
		case errors.Is(err, store.ErrRetentionOwnershipUnknown):
			reason = "retention_ownership_unknown"
		case errors.Is(err, store.ErrRetentionReadersActive):
			reason = "retention_readers_active"
		case errors.Is(err, store.ErrRetentionIO):
			reason = "retention_io_error"
		}
		if reason != "" {
			_ = s.store.RecordRetentionObjectFailure(ctx, o.ID, reason)
		}
	}()
	r, err := s.retentionCleanupRoots(o)
	if err != nil {
		return err
	}
	defer r.close()
	source, sourceExists, err := retentionLeaf(r.source, o.StorageID)
	if err != nil {
		return err
	}
	target, targetExists, err := retentionLeaf(r.quarantine, o.ObjectID)
	if err != nil {
		return err
	}
	if r.check() != nil || sourceExists && targetExists {
		return store.ErrRetentionOwnershipUnknown
	}
	if !sourceExists && !targetExists {
		// 仅已确认隔离且持久出生身份的事项，允许恢复unlink后确认前的真实间隙。
		if o.State != "quarantined" || o.Identity == "" {
			return store.ErrRetentionOwnershipUnknown
		}
		if err = retentionSyncDirectory(r.quarantine); err != nil {
			return err
		}
		if r.check() != nil {
			return store.ErrRetentionOwnershipUnknown
		}
		return s.store.ConfirmRetentionObject(ctx, o.ID, store.RetentionObjectResult{State: "deleted", Identity: o.Identity})
	}
	root, name, original := r.source, o.StorageID, source
	if targetExists {
		if o.Identity == "" || o.QuarantineSlot == "" {
			return store.ErrRetentionOwnershipUnknown
		}
		root, name, original = r.quarantine, o.ObjectID, target
	} else if o.State == "quarantined" {
		return store.ErrRetentionOwnershipUnknown
	}
	f, err := root.OpenFile(name, os.O_RDONLY|evidenceOpenFlags(), 0)
	if err != nil {
		return store.ErrRetentionOwnershipUnknown
	}
	defer f.Close()
	held, err := f.Stat()
	if err != nil || !os.SameFile(original, held) {
		return store.ErrRetentionOwnershipUnknown
	}
	identity, err := retentionExclusiveLock(ctx, f)
	if err != nil {
		return err
	}
	if o.Identity != "" && identity != o.Identity {
		return store.ErrRetentionOwnershipUnknown
	}
	observed, err := retentionObserve(ctx, f, o)
	if err != nil {
		return err
	}
	if err = retentionCheckLeaf(r, root, name, f, identity, held); err != nil {
		return err
	}
	authorized, err := s.store.AuthorizeRetentionObject(ctx, o.ID, observed)
	if err != nil {
		return err
	}
	authorizedOnce = true
	// 只有授权已落盘后，失败才能以这枚持久身份登记；绝不伪造初始所有权。
	// unlink后的SQL失败保留原quarantined事实，下一轮仅复核真ENOENT后补确认。
	unlinked := false
	defer func() {
		if err == nil || unlinked {
			return
		}
		reason := "retention_io_error"
		state := "failed"
		if errors.Is(err, store.ErrRetentionOwnershipUnknown) {
			reason = "retention_ownership_unknown"
		}
		if errors.Is(err, store.ErrRetentionReadersActive) {
			reason = "retention_readers_active"
			state = "paused"
		}
		if errors.Is(err, store.ErrRetentionProtected) {
			reason = "retention_protected"
			state = "paused"
		}
		if ctx.Err() != nil {
			reason = "retention_timeout"
			if errors.Is(ctx.Err(), context.Canceled) {
				reason = "retention_cancelled"
			}
		}
		_ = s.store.ConfirmRetentionObject(ctx, o.ID, store.RetentionObjectResult{State: state, Reason: reason, Identity: identity})
	}()
	if sourceExists {
		if err = retentionCheckLeaf(r, r.source, o.StorageID, f, identity, held); err != nil {
			return err
		}
		if _, exists, e := retentionLeaf(r.quarantine, o.ObjectID); e != nil || exists {
			return store.ErrRetentionOwnershipUnknown
		}
		if err = retentionRenameExclusive(r.source, r.quarantine, o.StorageID, o.ObjectID); err != nil {
			return err
		}
	}
	if err = retentionCheckLeaf(r, r.quarantine, o.ObjectID, f, identity, held); err != nil {
		return err
	}
	if err = retentionSyncDirectory(r.source); err != nil {
		return err
	}
	if err = retentionSyncDirectory(r.quarantine); err != nil {
		return err
	}
	if r.check() != nil {
		return store.ErrRetentionOwnershipUnknown
	}
	if authorized.State != "quarantined" {
		if err = s.store.ConfirmRetentionObject(ctx, o.ID, store.RetentionObjectResult{State: "quarantined", Identity: identity}); err != nil {
			return err
		}
	}
	// 当前策略、保护、读者及原内容在每次实际unlink前重新核对。
	held, err = f.Stat()
	if err != nil {
		return store.ErrRetentionIO
	}
	observed, err = retentionObserve(ctx, f, o)
	if err != nil {
		return err
	}
	if _, err = s.store.AuthorizeRetentionObject(ctx, o.ID, observed); err != nil {
		return err
	}
	if err = retentionCheckLeaf(r, r.quarantine, o.ObjectID, f, identity, held); err != nil {
		return err
	}
	if _, exists, e := retentionLeaf(r.source, o.StorageID); e != nil || exists {
		return store.ErrRetentionOwnershipUnknown
	}
	if err = r.quarantine.Remove(o.ObjectID); err != nil {
		return store.ErrRetentionIO
	}
	unlinked = true
	if err = retentionSyncDirectory(r.quarantine); err != nil {
		return err
	}
	if r.check() != nil {
		return store.ErrRetentionOwnershipUnknown
	}
	if _, exists, e := retentionLeaf(r.quarantine, o.ObjectID); e != nil || exists {
		return store.ErrRetentionOwnershipUnknown
	}
	return s.store.ConfirmRetentionObject(ctx, o.ID, store.RetentionObjectResult{State: "deleted", Identity: identity})
}

// 两个已核根的实际目录fd交给原生排他rename，不降级成可覆盖的Rename。
func retentionRenameDirectories(fromRoot, toRoot *os.Root, from, to string) (*os.File, *os.File, error) {
	if !retentionUUID(from) || !retentionUUID(to) || checkEvidenceRoot(fromRoot) != nil || checkEvidenceRoot(toRoot) != nil {
		return nil, nil, store.ErrRetentionOwnershipUnknown
	}
	fromDir, err := fromRoot.OpenFile(".", os.O_RDONLY|evidenceOpenFlags(), 0)
	if err != nil {
		return nil, nil, store.ErrRetentionIO
	}
	toDir, err := toRoot.OpenFile(".", os.O_RDONLY|evidenceOpenFlags(), 0)
	if err != nil {
		fromDir.Close()
		return nil, nil, store.ErrRetentionIO
	}
	for _, pair := range []struct {
		root *os.Root
		file *os.File
	}{{fromRoot, fromDir}, {toRoot, toDir}} {
		a, e := pair.root.Stat(".")
		b, be := pair.file.Stat()
		if e != nil || be != nil || !evidenceInfo(b, true) || !os.SameFile(a, b) || checkEvidenceRoot(pair.root) != nil {
			fromDir.Close()
			toDir.Close()
			return nil, nil, store.ErrRetentionOwnershipUnknown
		}
	}
	return fromDir, toDir, nil
}
