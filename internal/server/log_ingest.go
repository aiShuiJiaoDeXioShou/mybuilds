package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"unicode/utf8"

	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
)

func validateLogRecord(record protocol.LogRecord, build store.BuildView) bool {
	if record.UTC.IsZero() || record.Build != build.Name || record.Index < 1 || len(record.Text) > 8192 || !utf8.ValidString(record.Text) {
		return false
	}
	if record.Stream != "stdout" && record.Stream != "stderr" && record.Stream != "system" {
		return false
	}
	steps := build.Post
	if record.Phase == "ordinary" {
		steps = build.Steps
	} else if record.Phase != "success" && record.Phase != "failure" && record.Phase != "always" {
		return false
	}
	for _, step := range steps {
		if step.Phase == record.Phase && step.Index == record.Index && step.Name == record.Step {
			return true
		}
	}
	return false
}
func (s *Server) ingestLog(w http.ResponseWriter, r *http.Request, actor store.NodeActor) {
	var in protocol.LogChunk
	if e := readJSONLimit(r, &in, 64*1024); e != nil {
		writeError(w, e)
		return
	}
	if len(in.Records) < 1 || len(in.Records) > 16 {
		writeError(w, store.ErrInvalid)
		return
	}
	if e := s.store.CheckExecution(r.Context(), actor, in.Ref); e != nil {
		writeError(w, e)
		return
	}
	build, e := s.store.GetBuild(r.Context(), in.Ref.BuildID)
	if e != nil {
		writeError(w, e)
		return
	}
	for _, record := range in.Records {
		if !validateLogRecord(record, build) {
			writeError(w, store.ErrInvalid)
			return
		}
	}
	data, e := json.Marshal(in.Records)
	if e != nil || len(data) > 64*1024 {
		writeError(w, errTooLarge)
		return
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != in.Digest {
		writeError(w, store.ErrInvalid)
		return
	}
	root, e := s.logFiles()
	if e != nil {
		writeError(w, e)
		return
	}
	defer root.Close()
	id, e := publishEvidence(r.Context(), root, data)
	if e != nil {
		writeError(w, e)
		return
	}
	// 文件已排他发布，只有短事务末尾再验成功后才给ACK。失败孤立文件不可读。
	committed, e := s.store.CommitLogChunk(r.Context(), actor, store.LogCommit{Ref: in.Ref, Seq: in.Seq, Offset: in.Offset, Size: int64(len(data)), Digest: in.Digest, StorageID: id.id, RecordCount: len(in.Records)})
	if e != nil {
		writeError(w, e)
		return
	}
	if !committed.Created && committed.StorageID != id.id {
		if e = removeEvidenceCandidate(root, id); e != nil {
			writeError(w, e)
			return
		}
	}
	writeJSON(w, 200, committed.Ack)
}
