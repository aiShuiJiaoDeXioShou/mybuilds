package server

import (
	"context"
	"encoding/json"
	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func logHTTPMessage(t *testing.T, ref protocol.LeaseRef) protocol.LogChunk {
	t.Helper()
	records := []protocol.LogRecord{{UTC: time.Now().UTC(), Build: "compile", Phase: "ordinary", Index: 1, Step: "shell", Stream: "stdout", Text: "已脱敏 [REDACTED]"}}
	return protocol.LogChunk{Ref: ref, Seq: 1, Digest: messageDigest(t, records), Records: records}
}
func TestActualLogHTTPFilesAndReceipts(t *testing.T) {
	s, st, h, token, in := executionHTTPFixture(t)
	g := claimHTTP(t, h, token, in)
	chunk := logHTTPMessage(t, g.Ref)
	code, body := request(t, h, "POST", "/api/agent/logs", token, encodeMessage(t, chunk))
	if code != 200 {
		t.Fatal(code, body)
	}
	var ack protocol.LogAck
	if e := json.Unmarshal([]byte(body), &ack); e != nil || ack.NextOffset != int64(len(encodeMessage(t, chunk.Records))) {
		t.Fatal("非规范字节偏移", body, e)
	}
	code, repeated := request(t, h, "POST", "/api/agent/logs", token, encodeMessage(t, chunk))
	if code != 200 || repeated != body {
		t.Fatal("重放未返回原ACK", code, repeated)
	}
	admin, _ := st.Authenticate(context.Background(), adminToken)
	rows, e := st.ListLogChunks(context.Background(), admin, g.Ref.BuildID, 0, store.Page{Limit: 200})
	if e != nil || len(rows) != 1 {
		t.Fatal(rows, e)
	}
	raw, e := os.ReadFile(filepath.Join(s.config.DataDir, "logs", rows[0].StorageID))
	if e != nil || string(raw) != encodeMessage(t, chunk.Records) {
		t.Fatal("未确认完整文件", string(raw), e)
	}
	files, e := os.ReadDir(filepath.Join(s.config.DataDir, "logs"))
	if e != nil || len(files) != 1 {
		t.Fatal("重复候选未删除", files, e)
	}
	info, e := os.Lstat(filepath.Join(s.config.DataDir, "logs", rows[0].StorageID))
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		t.Fatal(info, e)
	}
	before := string(raw)
	chunk.Records[0].Text = "changed"
	chunk.Digest = messageDigest(t, chunk.Records)
	code, _ = request(t, h, "POST", "/api/agent/logs", token, encodeMessage(t, chunk))
	if code != 409 {
		t.Fatal("覆盖原回执", code)
	}
	raw, _ = os.ReadFile(filepath.Join(s.config.DataDir, "logs", rows[0].StorageID))
	if string(raw) != before {
		t.Fatal("原确认文件变化")
	}
}
func TestActualLogHTTPStrictLimitsAndFence(t *testing.T) {
	_, _, h, token, in := executionHTTPFixture(t)
	g := claimHTTP(t, h, token, in)
	original := logHTTPMessage(t, g.Ref)
	for _, change := range []func(*protocol.LogChunk){func(c *protocol.LogChunk) { c.Digest = strings.Repeat("a", 64) }, func(c *protocol.LogChunk) { c.Records[0].Text = strings.Repeat("x", 8193) }, func(c *protocol.LogChunk) { c.Records[0].Build = "wrong" }, func(c *protocol.LogChunk) { c.Records[0].Index = 2 }, func(c *protocol.LogChunk) { c.Records[0].Stream = "unknown" }, func(c *protocol.LogChunk) { c.Records = nil }, func(c *protocol.LogChunk) { c.Records = make([]protocol.LogRecord, 17) }} {
		c := original
		c.Records = append([]protocol.LogRecord{}, original.Records...)
		change(&c)
		if c.Digest == original.Digest {
			c.Digest = messageDigest(t, c.Records)
		}
		code, out := request(t, h, "POST", "/api/agent/logs", token, encodeMessage(t, c))
		if code != 400 && code != 413 {
			t.Fatal("非法日志接受", code, out)
		}
	}
	c := original
	c.Ref.Epoch++
	code, _ := request(t, h, "POST", "/api/agent/logs", token, encodeMessage(t, c))
	if code != 409 {
		t.Fatal("旧fence写日志", code)
	}
	code, _ = request(t, h, "POST", "/api/agent/logs", token, `{"PRIVATE":"`+strings.Repeat("x", 65536)+`"}`)
	if code != 413 {
		t.Fatal("body不限额", code)
	}
}
func TestLogPublicationCannotCommitAfterLockReplacement(t *testing.T) {
	s, st, h, token, in := executionHTTPFixture(t)
	g := claimHTTP(t, h, token, in)
	chunk := logHTTPMessage(t, g.Ref)
	// 使用真实文件发布与真实数据库调用构造发布间隙，不注入执行器。
	root, e := s.logFiles()
	if e != nil {
		t.Fatal(e)
	}
	defer root.Close()
	id, e := publishEvidence(context.Background(), root, []byte(encodeMessage(t, chunk.Records)))
	if e != nil {
		t.Fatal(e)
	}
	loseServerLock(t, s)
	actor := store.NodeActor{ID: g.Ref.NodeID}
	_, e = st.CommitLogChunk(context.Background(), actor, store.LogCommit{Ref: g.Ref, Seq: 1, Size: int64(len(encodeMessage(t, chunk.Records))), Digest: chunk.Digest, StorageID: id.id, RecordCount: 1})
	if e != store.ErrLockLost {
		t.Fatal("失锁仍写确认", e)
	}
	if _, e = os.Stat(filepath.Join(s.config.DataDir, "logs", id.id)); e != nil {
		t.Fatal("孤立文件不应假装清理", e)
	}
	code, _ := request(t, h, "POST", "/api/agent/logs", token, encodeMessage(t, chunk))
	if code != 503 {
		t.Fatal(code)
	}
}
