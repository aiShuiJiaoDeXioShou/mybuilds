package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
)

func safeArtifactName(name string) bool {
	if name == "" || len(name) > 255 || name == "." || name == ".." || !utf8.ValidString(name) || strings.TrimSpace(name) != name || strings.ContainsAny(name, "/\\") {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func canonicalUUID(id string) bool { v, e := uuid.Parse(id); return e == nil && v.String() == id }
func artifactDeclaration(r *http.Request, id string) (protocol.ArtifactDeclaration, error) {
	var in protocol.ArtifactDeclaration
	headers := r.Header.Values("X-Mybuilds-Artifact")
	if len(headers) != 1 || len(r.Header.Values("Content-Encoding")) != 0 {
		return in, store.ErrInvalid
	}
	if len(headers[0]) > 8192 {
		return in, errTooLarge
	}
	data, e := base64.RawURLEncoding.Strict().DecodeString(headers[0])
	if e != nil {
		return in, store.ErrInvalid
	}
	message := r.Clone(r.Context())
	message.Body = io.NopCloser(bytes.NewReader(data))
	message.Header = make(http.Header)
	message.Header.Set("Content-Type", "application/json")
	if e = readJSONLimit(message, &in, 8192); e != nil {
		return in, e
	}
	canonical, e := json.Marshal(in)
	if e != nil || !bytes.Equal(data, canonical) {
		return in, store.ErrInvalid
	}
	_, e = hex.DecodeString(in.SHA256)
	if !canonicalUUID(id) || in.ID != id || in.Seq < 1 || in.Index < 1 || !validObjectName(in.Step) || !safeArtifactName(in.Name) || len(in.SHA256) != 64 || e != nil || strings.ToLower(in.SHA256) != in.SHA256 || (in.Phase != "ordinary" && in.Phase != "success" && in.Phase != "failure" && in.Phase != "always") {
		return in, store.ErrInvalid
	}
	if in.Size < 0 || in.Size > 1<<30 {
		return in, errTooLarge
	}
	if r.ContentLength != in.Size {
		return in, store.ErrInvalid
	}
	return in, nil
}
func (s *Server) nodeArtifactRoute(w http.ResponseWriter, r *http.Request, actor store.NodeActor) bool {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(parts) != 4 || parts[0] != "api" || parts[1] != "agent" || parts[2] != "artifacts" {
		return false
	}
	id := parts[3]
	if r.Method == "GET" {
		query, e := readQuery(r, "node_id", "session_id", "build_id", "attempt_id", "lease_id", "epoch")
		if e != nil || len(query) != 6 {
			writeError(w, store.ErrInvalid)
			return true
		}
		epoch, e := strconv.ParseInt(query["epoch"], 10, 64)
		if e != nil {
			writeError(w, store.ErrInvalid)
			return true
		}
		ref := protocol.LeaseRef{NodeID: query["node_id"], SessionID: query["session_id"], BuildID: query["build_id"], AttemptID: query["attempt_id"], LeaseID: query["lease_id"], Epoch: epoch}
		result, e := s.store.FindNodeArtifact(r.Context(), actor, ref, id)
		if e != nil {
			writeError(w, e)
			return true
		}
		writeJSON(w, 200, result)
		return true
	}
	if r.Method != "PUT" {
		writeError(w, store.ErrNotFound)
		return true
	}
	// 上传结束后关闭此连接；错误路径不让net/http再无界排空未完整body。
	w.Header().Set("Connection", "close")
	controller := http.NewResponseController(w)
	defer func() { _ = controller.SetReadDeadline(time.Now()) }()
	if _, e := readQuery(r); e != nil {
		writeError(w, e)
		return true
	}
	in, e := artifactDeclaration(r, id)
	if e != nil {
		writeError(w, e)
		return true
	}
	if e = s.store.CheckExecution(r.Context(), actor, in.Ref); e != nil {
		writeError(w, e)
		return true
	}
	deadline := time.Now().Add(2 * time.Minute)
	if e = controller.SetReadDeadline(deadline); e != nil {
		writeError(w, errEvidence)
		return true
	}
	if e = controller.SetWriteDeadline(deadline); e != nil {
		writeError(w, errEvidence)
		return true
	}
	ctx, cancel := context.WithDeadline(r.Context(), deadline)
	defer cancel()
	done := make(chan struct{})
	stopped := make(chan struct{})
	authorityLost := make(chan error, 1)
	defer func() { close(done); cancel(); <-stopped }()
	// 真实权限核对失败或请求取消时唤醒慢body，不留下弃置的读取goroutine。
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				_ = controller.SetReadDeadline(time.Now())
				return
			case <-ticker.C:
				check, stop := context.WithTimeout(ctx, 500*time.Millisecond)
				err := s.store.CheckExecution(check, actor, in.Ref)
				stop()
				if err != nil {
					authorityLost <- err
					cancel()
					_ = controller.SetReadDeadline(time.Now())
					return
				}
			}
		}
	}()
	root, e := s.artifactFiles()
	if e != nil {
		writeError(w, e)
		return true
	}
	defer root.Close()
	candidate, e := publishEvidenceStream(ctx, root, r.Body, in.Size, in.SHA256)
	if e != nil {
		select {
		case authority := <-authorityLost:
			e = authority
		default:
			if authority := s.store.CheckExecution(r.Context(), actor, in.Ref); authority != nil {
				e = authority
			}
		}
		writeError(w, e)
		return true
	}
	committed, e := s.store.CommitArtifact(ctx, actor, store.ArtifactCommit{Declaration: in, StorageID: candidate.id})
	if e != nil {
		writeError(w, e)
		return true
	}
	if !committed.Created && committed.StorageID != candidate.id {
		if e = removeEvidenceCandidate(root, candidate); e != nil {
			writeError(w, e)
			return true
		}
	}
	writeJSON(w, 200, committed.View)
	return true
}

func (s *Server) artifactReadRoute(w http.ResponseWriter, r *http.Request, actor store.Actor) bool {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	list := len(parts) == 4 && parts[0] == "api" && parts[1] == "builds" && parts[3] == "artifacts"
	single := len(parts) >= 3 && parts[0] == "api" && parts[1] == "artifacts"
	if !list && !single {
		return false
	}
	if actor.Role != "admin" && actor.Role != "approver" {
		writeError(w, store.ErrForbidden)
		return true
	}
	if r.Method != "GET" {
		writeError(w, store.ErrNotFound)
		return true
	}
	if list {
		query, e := readQuery(r, "limit", "offset")
		if e != nil {
			writeError(w, e)
			return true
		}
		page, e := readPage(query)
		if e != nil {
			writeError(w, e)
			return true
		}
		items, e := s.store.ListArtifacts(r.Context(), actor, parts[2], page)
		if e != nil {
			writeError(w, e)
			return true
		}
		writeJSON(w, 200, map[string]any{"items": items, "limit": page.Limit, "offset": page.Offset})
		return true
	}
	if len(parts) != 3 && len(parts) != 4 {
		writeError(w, store.ErrNotFound)
		return true
	}
	if _, e := readQuery(r); e != nil {
		writeError(w, e)
		return true
	}
	stored, e := s.store.GetArtifact(r.Context(), actor, parts[2])
	if e != nil {
		writeError(w, e)
		return true
	}
	if len(parts) == 3 {
		writeJSON(w, 200, stored.View)
		return true
	}
	if parts[3] != stored.View.Name || !safeArtifactName(parts[3]) {
		writeError(w, store.ErrNotFound)
		return true
	}
	s.downloadArtifact(w, r, stored)
	return true
}
func (s *Server) downloadArtifact(w http.ResponseWriter, r *http.Request, stored store.ArtifactStored) {
	if !canonicalUUID(stored.StorageID) || stored.View.Size < 0 || stored.View.Size > 1<<30 {
		writeError(w, errEvidence)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	root, e := s.artifactFiles()
	if e != nil {
		writeError(w, e)
		return
	}
	defer root.Close()
	f, e := root.OpenFile(stored.StorageID, os.O_RDONLY|evidenceOpenFlags(), 0)
	if e != nil {
		writeError(w, errEvidence)
		return
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !evidenceInfo(info, false) || info.Size() != stored.View.Size {
		writeError(w, errEvidence)
		return
	}
	hash := sha256.New()
	if e = copyArtifactBytes(ctx, hash, f, stored.View.Size); e != nil || hex.EncodeToString(hash.Sum(nil)) != stored.View.SHA256 {
		writeError(w, errEvidence)
		return
	}
	if _, e = f.Seek(0, io.SeekStart); e != nil {
		writeError(w, errEvidence)
		return
	}
	controller := http.NewResponseController(w)
	deadline, _ := ctx.Deadline()
	if e = controller.SetWriteDeadline(deadline); e != nil {
		writeError(w, errEvidence)
		return
	}
	defer controller.SetWriteDeadline(time.Time{})
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(stored.View.Size, 10))
	w.Header().Set("X-Content-SHA256", stored.View.SHA256)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": stored.View.Name}))
	w.WriteHeader(200)
	if e = copyArtifactBytes(ctx, w, f, stored.View.Size); e != nil {
		_ = controller.SetWriteDeadline(time.Now())
	}
}
func copyArtifactBytes(ctx context.Context, target io.Writer, source io.Reader, size int64) error {
	buffer := make([]byte, 64*1024)
	var copied int64
	for copied < size {
		if ctx.Err() != nil {
			return errEvidence
		}
		n, e := source.Read(buffer[:min(int64(len(buffer)), size-copied)])
		if n > 0 {
			written, we := target.Write(buffer[:n])
			if we != nil || written != n {
				return errEvidence
			}
			copied += int64(n)
		}
		if e != nil {
			if e == io.EOF && copied == size {
				return nil
			}
			return errEvidence
		}
		if n == 0 {
			return errEvidence
		}
	}
	return nil
}
