package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
)

// LogPage由完整chunk组成；cursor包括被step筛选掉的已核对记录。
type LogPage struct {
	Records    []protocol.LogRecord `json:"records"`
	NextSeq    int64                `json:"next_seq"`
	NextOffset int64                `json:"next_offset"`
}

func (s *Server) readLogChunk(ctx context.Context, actor store.Actor, row store.LogStored) (records []protocol.LogRecord, err error) {
	id, e := uuid.Parse(row.StorageID)
	if e != nil || id.String() != row.StorageID || row.Size < 1 || row.Size > 64*1024 {
		return nil, errEvidence
	}
	held, e := s.openRetentionEvidence(ctx, actor, "log", row.ID)
	if e != nil {
		return nil, evidenceReadFailure(e)
	}
	defer func() {
		if closeErr := held.Close(); closeErr != nil && err == nil {
			records, err = nil, evidenceReadFailure(closeErr)
		}
	}()
	if held.Read.BuildID != row.BuildID || held.Read.StorageID != row.StorageID || held.Read.Size != row.Size || held.Read.SHA256 != row.Digest {
		return nil, store.ErrRetentionOwnershipUnknown
	}
	f := held.File
	info, e := f.Stat()
	if e != nil || !evidenceInfo(info, false) || info.Size() != row.Size {
		return nil, errEvidence
	}
	data, e := io.ReadAll(io.LimitReader(f, row.Size+1))
	if e != nil || int64(len(data)) != row.Size {
		return nil, errEvidence
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != row.Digest {
		return nil, errEvidence
	}
	if json.Unmarshal(data, &records) != nil || len(records) != row.RecordCount {
		return nil, errEvidence
	}
	return records, nil
}
func logChunkPage(row store.LogStored, records []protocol.LogRecord, step string) LogPage {
	page := LogPage{Records: []protocol.LogRecord{}, NextSeq: row.Seq, NextOffset: row.Offset + row.Size}
	for _, record := range records {
		if step == "" || record.Step == step {
			page.Records = append(page.Records, record)
		}
	}
	return page
}
func (s *Server) logRoute(w http.ResponseWriter, r *http.Request, actor store.Actor) bool {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(parts) != 4 || parts[0] != "api" || parts[1] != "builds" || parts[3] != "log" {
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
	query, e := readQuery(r, "step", "after_seq", "limit", "follow")
	if e != nil {
		writeError(w, e)
		return true
	}
	after := int64(0)
	if value, ok := query["after_seq"]; ok {
		after, e = strconv.ParseInt(value, 10, 64)
		if e != nil || after < 0 {
			writeError(w, store.ErrInvalid)
			return true
		}
	}
	page, e := readPage(query)
	if e != nil {
		writeError(w, e)
		return true
	}
	if _, ok := query["limit"]; !ok {
		page.Limit = 200
	}
	follow := query["follow"]
	if follow != "" && follow != "0" && follow != "1" {
		writeError(w, store.ErrInvalid)
		return true
	}
	step := query["step"]
	if len(step) > 255 || strings.TrimSpace(step) != step {
		writeError(w, store.ErrInvalid)
		return true
	}
	for _, r := range step {
		if unicode.IsControl(r) {
			writeError(w, store.ErrInvalid)
			return true
		}
	}
	headers := r.Header.Values("Last-Event-ID")
	if len(headers) > 1 || len(headers) == 1 && (follow != "1" || headers[0] != strconv.FormatInt(after, 10)) {
		writeError(w, store.ErrInvalid)
		return true
	}
	baseOffset := int64(0)
	if after > 0 {
		previous, e := s.store.ListLogChunks(r.Context(), actor, parts[2], after-1, store.Page{Limit: 1})
		if e != nil {
			writeError(w, e)
			return true
		}
		if len(previous) != 1 || previous[0].Seq != after {
			writeError(w, store.ErrInvalid)
			return true
		}
		baseOffset = previous[0].Offset + previous[0].Size
	}
	rows, e := s.store.ListLogChunks(r.Context(), actor, parts[2], after, page)
	if e != nil {
		writeError(w, e)
		return true
	}
	if follow == "1" {
		s.followLog(w, r, actor, parts[2], after, page, step, rows)
		return true
	}
	result := LogPage{Records: []protocol.LogRecord{}, NextSeq: after, NextOffset: baseOffset}
	for _, row := range rows {
		records, e := s.readLogChunk(r.Context(), actor, row)
		if e != nil {
			writeError(w, e)
			return true
		}
		chunk := logChunkPage(row, records, step)
		// 最坏外壳远小于1KiB，完整chunk边界留1KiB余量。
		candidate := LogPage{Records: append(append([]protocol.LogRecord{}, result.Records...), chunk.Records...), NextSeq: chunk.NextSeq, NextOffset: chunk.NextOffset}
		data, e := json.Marshal(candidate)
		if e != nil {
			writeError(w, errEvidence)
			return true
		}
		if len(data) > ((1 << 20) - 1024) {
			break
		}
		result = candidate
	}
	writeJSON(w, 200, result)
	return true
}
func (s *Server) followLog(w http.ResponseWriter, r *http.Request, actor store.Actor, build string, after int64, page store.Page, step string, rows []store.LogStored) {
	// 首批在发流头前核实，坏文件仍以固定JSON失败。
	chunks := make([]LogPage, 0, len(rows))
	for _, row := range rows {
		records, e := s.readLogChunk(r.Context(), actor, row)
		if e != nil {
			writeError(w, e)
			return
		}
		chunks = append(chunks, logChunkPage(row, records, step))
	}
	controller := http.NewResponseController(w)
	if e := controller.SetWriteDeadline(time.Time{}); e != nil {
		writeError(w, errEvidence)
		return
	}
	defer controller.SetWriteDeadline(time.Time{})
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Minute)
	defer cancel()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	write := func(text string) bool {
		if ctx.Err() != nil || controller.SetWriteDeadline(time.Now().Add(5*time.Second)) != nil {
			return false
		}
		if _, e := io.WriteString(w, text); e != nil {
			return false
		}
		return controller.Flush() == nil
	}
	if !write(": connected\n\n") {
		return
	}
	poll := time.NewTicker(250 * time.Millisecond)
	defer poll.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		for _, chunk := range chunks {
			data, e := json.Marshal(chunk)
			if e != nil || len(data) > 64*1024 {
				return
			}
			if !write(fmt.Sprintf("id: %d\nevent: log\ndata: %s\n\n", chunk.NextSeq, data)) {
				return
			}
			after = chunk.NextSeq
		}
		chunks = nil
		state, e := s.store.GetBuild(ctx, build)
		if e != nil {
			return
		}
		if len(rows) == 0 && (state.Status == "succeeded" || state.Status == "failed" || state.Status == "cancelled" || state.Status == "skipped") {
			_ = write("event: end\ndata: {}\n\n")
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			if !write(": heartbeat\n\n") {
				return
			}
		case <-poll.C:
		}
		// 每轮复核真实身份与锁；撤销后不维持已打开流。
		authorization := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		current, e := s.store.Authenticate(ctx, authorization)
		if e != nil || current != actor {
			return
		}
		rows, e = s.store.ListLogChunks(ctx, actor, build, after, page)
		if e != nil {
			return
		}
		for _, row := range rows {
			records, e := s.readLogChunk(ctx, actor, row)
			if e != nil {
				return
			}
			chunks = append(chunks, logChunkPage(row, records, step))
		}
	}
}
