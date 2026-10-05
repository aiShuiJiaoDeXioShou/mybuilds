package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"

	"github.com/google/uuid"
	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
)

var errEvidence = errors.New("evidence_storage_error")

// 原下载和日志接口保持既有损坏文件错误；退役410沿新明确契约返回。
func evidenceReadFailure(err error) error {
	if errors.Is(err, store.ErrRetentionOwnershipUnknown) || errors.Is(err, store.ErrRetentionReadersActive) || errors.Is(err, store.ErrRetentionIO) {
		return errEvidence
	}
	return err
}

type publishedEvidence struct {
	id    string
	info  os.FileInfo
	junit *protocol.JUnitResult
}

// logFiles限定到控制端自有目录；路径不来自节点请求。
func (s *Server) logFiles() (*os.Root, error)      { return s.evidenceFiles("logs") }
func (s *Server) artifactFiles() (*os.Root, error) { return s.evidenceFiles("artifacts") }
func (s *Server) evidenceFiles(kind string) (*os.Root, error) {
	if kind != "logs" && kind != "artifacts" {
		return nil, errEvidence
	}
	info, e := os.Lstat(s.config.DataDir)
	if e != nil || !evidenceInfo(info, true) {
		return nil, errEvidence
	}
	root, e := os.OpenRoot(s.config.DataDir)
	if e != nil {
		return nil, errEvidence
	}
	defer root.Close()
	if e = root.Mkdir(kind, 0700); e != nil && !os.IsExist(e) {
		return nil, errEvidence
	}
	leaf, e := root.Lstat(kind)
	if e != nil || !evidenceInfo(leaf, true) {
		return nil, errEvidence
	}
	child, e := root.OpenRoot(kind)
	if e != nil {
		return nil, errEvidence
	}
	// 打开后仍核对同一目录，避免叶目录替换绕过权限检查。
	opened, e := child.Stat(".")
	current, ce := root.Lstat(kind)
	parent, pe := os.Lstat(s.config.DataDir)
	if e != nil || ce != nil || pe != nil || !os.SameFile(opened, leaf) || !os.SameFile(current, leaf) || !os.SameFile(parent, info) {
		child.Close()
		return nil, errEvidence
	}
	return child, nil
}
func checkEvidenceRoot(root *os.Root) error {
	held, e := root.Stat(".")
	current, ce := os.Lstat(root.Name())
	if e != nil || ce != nil || !evidenceInfo(current, true) || !os.SameFile(held, current) {
		return errEvidence
	}
	return nil
}

// publishEvidence先完整同步stage，再排他发布新UUID；不覆盖任何现有文件。
func publishEvidence(ctx context.Context, root *os.Root, data []byte) (publishedEvidence, error) {
	sum := sha256.Sum256(data)
	return publishEvidenceStream(ctx, root, bytes.NewReader(data), int64(len(data)), hex.EncodeToString(sum[:]))
}
func publishEvidenceStream(ctx context.Context, root *os.Root, source io.Reader, size int64, digest string) (publishedEvidence, error) {
	return publishArtifactEvidenceStream(ctx, root, source, size, digest, "")
}
func publishArtifactEvidenceStream(ctx context.Context, root *os.Root, source io.Reader, size int64, digest, purpose string) (publishedEvidence, error) {
	if e := ctx.Err(); e != nil {
		return publishedEvidence{}, errEvidence
	}
	if e := checkEvidenceRoot(root); e != nil {
		return publishedEvidence{}, e
	}
	stage := ".stage-" + uuid.NewString()
	flags := os.O_WRONLY
	if purpose == "junit" {
		flags = os.O_RDWR
	}
	f, e := root.OpenFile(stage, flags|os.O_CREATE|os.O_EXCL|evidenceOpenFlags(), 0600)
	if e != nil {
		return publishedEvidence{}, errEvidence
	}
	info, e := f.Stat()
	if e != nil || !evidenceInfo(info, false) {
		f.Close()
		return publishedEvidence{}, errEvidence
	}
	defer func() {
		current, e := root.Lstat(stage)
		if e == nil && os.SameFile(current, info) && evidenceInfo(current, false) {
			_ = root.Remove(stage)
		}
	}()
	hash := sha256.New()
	buffer := make([]byte, 64*1024)
	var total int64
	for total <= size {
		if ctx.Err() != nil {
			e = errEvidence
			break
		}
		want := min(int64(len(buffer)), size+1-total)
		if want == 0 {
			break
		}
		var n int
		var readErr error
		n, readErr = source.Read(buffer[:want])
		if n > 0 {
			total += int64(n)
			if total > size {
				e = store.ErrInvalid
				break
			}
			var written int
			written, e = f.Write(buffer[:n])
			if e != nil || written != n {
				e = errEvidence
				break
			}
			_, _ = hash.Write(buffer[:n])
		}
		if readErr == io.EOF {
			e = nil
			break
		}
		if readErr != nil {
			e = errEvidence
			break
		}
		if n == 0 {
			e = errEvidence
			break
		}
	}
	if e == nil && (total != size || hex.EncodeToString(hash.Sum(nil)) != digest) {
		e = store.ErrInvalid
	}
	if e == nil {
		e = f.Sync()
	}
	var junit *protocol.JUnitResult
	if e == nil && purpose == "junit" {
		junit, e = parseJUnitStage(ctx, f, size, digest)
	}
	closed := f.Close()
	if e != nil {
		return publishedEvidence{}, e
	}
	if closed != nil {
		return publishedEvidence{}, errEvidence
	}
	if e = ctx.Err(); e != nil {
		return publishedEvidence{}, errEvidence
	}
	if e = checkEvidenceRoot(root); e != nil {
		return publishedEvidence{}, e
	}
	current, e := root.Lstat(stage)
	if e != nil || !os.SameFile(info, current) || !evidenceInfo(current, false) {
		return publishedEvidence{}, errEvidence
	}
	id := uuid.NewString()
	if e = root.Link(stage, id); e != nil {
		return publishedEvidence{}, errEvidence
	}
	if e = root.Remove(stage); e != nil {
		return publishedEvidence{}, errEvidence
	}
	// 目录项也落盘后才允许数据库确认。
	dir, e := root.OpenFile(".", os.O_RDONLY|evidenceOpenFlags(), 0)
	if e != nil {
		return publishedEvidence{}, errEvidence
	}
	e = dir.Sync()
	closed = dir.Close()
	if e != nil || closed != nil {
		return publishedEvidence{}, errEvidence
	}
	if e = checkEvidenceRoot(root); e != nil {
		return publishedEvidence{}, e
	}
	return publishedEvidence{id: id, info: info, junit: junit}, nil
}
func removeEvidenceCandidate(root *os.Root, candidate publishedEvidence) error {
	id := candidate.id
	if e := checkEvidenceRoot(root); e != nil {
		return e
	}
	info, e := root.Lstat(id)
	if e != nil || !evidenceInfo(info, false) || !os.SameFile(info, candidate.info) {
		return errEvidence
	}
	if e = root.Remove(id); e != nil {
		return errEvidence
	}
	return nil
}
