//go:build darwin || linux

package server

import (
	"context"
	"errors"
	"os"

	"golang.org/x/sys/unix"
	"mybuilds/internal/store"
)

// 读者与清理者始终锁住实际文件描述符；成功后由原读取/删除消费者Close释放。
func retentionSharedLock(ctx context.Context, f *os.File) (string, error) {
	return retentionLockFile(ctx, f, unix.LOCK_SH)
}

func retentionExclusiveLock(ctx context.Context, f *os.File) (string, error) {
	return retentionLockFile(ctx, f, unix.LOCK_EX)
}

func retentionLockFile(ctx context.Context, f *os.File, how int) (string, error) {
	if ctx.Err() != nil {
		return "", store.ErrRetentionIO
	}
	before, err := retentionFileIdentity(f)
	if err != nil {
		return "", err
	}
	if err = unix.Flock(int(f.Fd()), how|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return "", store.ErrRetentionReadersActive
		}
		return "", store.ErrRetentionIO
	}
	after, err := retentionFileIdentity(f)
	if err == nil && before != after {
		err = store.ErrRetentionOwnershipUnknown
	}
	if err == nil && ctx.Err() != nil {
		err = store.ErrRetentionIO
	}
	if err != nil {
		if unix.Flock(int(f.Fd()), unix.LOCK_UN) != nil {
			return "", store.ErrRetentionIO
		}
		return "", err
	}
	return after, nil
}

func retentionFileIdentity(f *os.File) (string, error) {
	if f == nil {
		return "", store.ErrRetentionOwnershipUnknown
	}
	info, err := f.Stat()
	if err != nil || !evidenceInfo(info, false) {
		return "", store.ErrRetentionOwnershipUnknown
	}
	return retentionNativeIdentity(f, info)
}
