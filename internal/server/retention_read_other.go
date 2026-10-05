//go:build !darwin && !linux

package server

import (
	"context"
	"os"

	"mybuilds/internal/store"
)

// 未验收平台明确拒绝，不将编译能力当作文件锁和出生身份支持。
func retentionSharedLock(context.Context, *os.File) (string, error) {
	return "", store.ErrRetentionOwnershipUnknown
}
func retentionExclusiveLock(context.Context, *os.File) (string, error) {
	return "", store.ErrRetentionOwnershipUnknown
}
func retentionFileIdentity(*os.File) (string, error) { return "", store.ErrRetentionOwnershipUnknown }
