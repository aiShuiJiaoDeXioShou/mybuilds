package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"mybuilds/internal/protocol"
	"mybuilds/internal/reports"
	"mybuilds/internal/store"
)

// parseJUnitStage只读取尚未发布的私有普通fd；结果由HTTP内部交给Store，节点不能自填。
func parseJUnitStage(parent context.Context, stage *os.File, size int64, digest string) (*protocol.JUnitResult, error) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	if ctx.Err() != nil || stage == nil {
		return nil, errEvidence
	}
	if size < 1 {
		return nil, store.ErrInvalid
	}
	if size > 8<<20 {
		return nil, errTooLarge
	}
	if _, err := hex.DecodeString(digest); err != nil || len(digest) != 64 || strings.ToLower(digest) != digest {
		return nil, store.ErrInvalid
	}
	before, err := stage.Stat()
	if err != nil || !evidenceInfo(before, false) {
		return nil, errEvidence
	}
	if before.Size() != size {
		return nil, store.ErrInvalid
	}
	if _, err = stage.Seek(0, io.SeekStart); err != nil {
		return nil, errEvidence
	}
	limited := &io.LimitedReader{R: stage, N: size + 1}
	hash := sha256.New()
	result, parseErr := reports.ParseJUnit(ctx, io.TeeReader(limited, hash), nil)
	if ctx.Err() != nil {
		return nil, errEvidence
	}
	after, err := stage.Stat()
	if err != nil || !evidenceInfo(after, false) || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return nil, errEvidence
	}
	if parseErr != nil {
		switch {
		case errors.Is(parseErr, reports.ErrLimit):
			return nil, errTooLarge
		case errors.Is(parseErr, reports.ErrInvalid), errors.Is(parseErr, reports.ErrSecret):
			return nil, store.ErrInvalid
		default:
			return nil, errEvidence
		}
	}
	if size+1-limited.N != size || hex.EncodeToString(hash.Sum(nil)) != digest {
		return nil, store.ErrInvalid
	}
	return &result, nil
}
