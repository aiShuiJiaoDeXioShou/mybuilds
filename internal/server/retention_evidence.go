package server

import (
	"context"
	"os"
	"time"

	"mybuilds/internal/store"
)

// 只有真实fd消费者持有此对象；SH锁生命周期覆盖实际复制，不覆盖SSE空闲等待。
type retentionEvidence struct {
	File   *os.File
	Read   store.EvidenceRead
	root   *os.Root
	server *Server
	ctx    context.Context
}

func (s *Server) openRetentionEvidence(ctx context.Context, actor store.Actor, kind, objectID string) (result *retentionEvidence, err error) {
	if s.evidenceReadError != nil {
		return nil, s.evidenceReadError
	}
	read, err := s.store.BeginEvidenceRead(ctx, actor, kind, objectID, s.evidenceReadOwner)
	if err != nil {
		return nil, err
	}
	held := &retentionEvidence{Read: read, server: s, ctx: ctx}
	defer func() {
		if err != nil {
			if closeErr := held.Close(); closeErr != nil {
				err = closeErr
			}
			result = nil
		}
	}()
	if kind == "log" {
		held.root, err = s.logFiles()
	} else {
		held.root, err = s.artifactFiles()
	}
	if err != nil {
		return nil, store.ErrRetentionOwnershipUnknown
	}
	held.File, err = held.root.OpenFile(read.StorageID, os.O_RDONLY|evidenceOpenFlags(), 0)
	if err != nil {
		return nil, store.ErrRetentionOwnershipUnknown
	}
	identity, err := retentionSharedLock(ctx, held.File)
	if err != nil {
		return nil, err
	}
	info, err := held.File.Stat()
	current, currentErr := held.root.Lstat(read.StorageID)
	if err != nil || currentErr != nil || !evidenceInfo(info, false) || !evidenceInfo(current, false) || info.Size() != read.Size || !os.SameFile(info, current) || checkEvidenceRoot(held.root) != nil {
		return nil, store.ErrRetentionOwnershipUnknown
	}
	if err = s.store.ActivateEvidenceRead(ctx, read.ID, identity); err != nil {
		return nil, err
	}
	return held, nil
}

// 先关真实fd和目录根，再解除数据库保护；取消请求不能取消此有限收尾。
func (held *retentionEvidence) Close() error {
	if held == nil {
		return nil
	}
	failed := false
	if held.File != nil {
		if err := held.File.Close(); err != nil {
			failed = true
		} else {
			held.File = nil
		}
	}
	if held.root != nil {
		if err := held.root.Close(); err != nil {
			failed = true
		} else {
			held.root = nil
		}
	}
	// 关闭不确定时保留登记；不把错误或超时当作锁已释放。
	if failed {
		return store.ErrRetentionIO
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(held.ctx), 5*time.Second)
	defer cancel()
	return held.server.store.EndEvidenceRead(ctx, held.Read.ID)
}
