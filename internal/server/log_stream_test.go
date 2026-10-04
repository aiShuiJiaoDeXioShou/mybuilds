package server

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"mybuilds/internal/protocol"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"mybuilds/internal/store"
)

func TestActualLogHistoryRolesCursorsAndCorruption(t *testing.T) {
	s, st, h, token, in := executionHTTPFixture(t)
	g := claimHTTP(t, h, token, in)
	chunk := logHTTPMessage(t, g.Ref)
	code, body := request(t, h, "POST", "/api/agent/logs", token, encodeMessage(t, chunk))
	if code != 200 {
		t.Fatal(code, body)
	}
	path := "/api/builds/" + g.Ref.BuildID + "/log"
	code, body = request(t, h, "GET", path+"?limit=1", adminToken, "")
	var page LogPage
	if e := json.Unmarshal([]byte(body), &page); e != nil || code != 200 || len(page.Records) != 1 || page.NextSeq != 1 || page.NextOffset != int64(len(encodeMessage(t, chunk.Records))) {
		t.Fatal(code, body, e)
	}
	code, body = request(t, h, "GET", path+"?step=other", adminToken, "")
	if e := json.Unmarshal([]byte(body), &page); e != nil || code != 200 || len(page.Records) != 0 || page.NextSeq != 1 {
		t.Fatal("筛选cursor不推进", code, body, e)
	}
	admin, _ := st.Authenticate(context.Background(), adminToken)
	for _, role := range []string{"approver", "trigger"} {
		tok, e := st.CreateToken(context.Background(), admin, role)
		if e != nil {
			t.Fatal(e)
		}
		code, _ = request(t, h, "GET", path, tok.Token, "")
		if role == "approver" && code != 200 || role == "trigger" && code != 403 {
			t.Fatal(role, code)
		}
	}
	code, _ = request(t, h, "GET", path, token, "")
	if code != 401 {
		t.Fatal("节点读取用户日志", code)
	}
	for _, query := range []string{"after_seq=-1", "after_seq=1&after_seq=2", "limit=201", "follow=2", "unknown=PRIVATE"} {
		code, body = request(t, h, "GET", path+"?"+query, adminToken, "")
		if code != 400 || strings.Contains(body, "PRIVATE") {
			t.Fatal(code, body)
		}
	}
	rows, e := st.ListLogChunks(context.Background(), admin, g.Ref.BuildID, 0, store.Page{Limit: 200})
	if e != nil {
		t.Fatal(e)
	}
	file := filepath.Join(s.config.DataDir, "logs", rows[0].StorageID)
	if e = os.WriteFile(file, []byte("broken"), 0600); e != nil {
		t.Fatal(e)
	}
	code, body = request(t, h, "GET", path, adminToken, "")
	if code != 500 || strings.Contains(body, "broken") {
		t.Fatal("坏文件可读", code, body)
	}
}
func TestActualSSEChunkAndLastEventID(t *testing.T) {
	_, _, h, token, in := executionHTTPFixture(t)
	g := claimHTTP(t, h, token, in)
	chunk := logHTTPMessage(t, g.Ref)
	code, body := request(t, h, "POST", "/api/agent/logs", token, encodeMessage(t, chunk))
	if code != 200 {
		t.Fatal(code, body)
	}
	endpoint := h.URL + "/api/builds/" + g.Ref.BuildID + "/log?follow=1"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r, e := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if e != nil {
		t.Fatal(e)
	}
	r.Header.Set("Authorization", "Bearer "+adminToken)
	res, e := h.Client().Do(r)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatal(res.StatusCode)
	}
	reader := bufio.NewReader(res.Body)
	var lines []string
	for {
		line, e := reader.ReadString('\n')
		if e != nil {
			t.Fatal(e)
		}
		lines = append(lines, line)
		if line == "\n" {
			if len(lines) > 0 && strings.HasPrefix(lines[0], ":") {
				lines = nil
				continue
			}
			break
		}
	}
	event := strings.Join(lines, "")
	if !strings.Contains(event, "id: 1\n") || !strings.Contains(event, "event: log\n") || !strings.Contains(event, `"next_seq":1`) {
		t.Fatal(event)
	}
	cancel()
	_, _ = io.Copy(io.Discard, res.Body)
	r, e = http.NewRequest("GET", endpoint+"&after_seq=1", nil)
	if e != nil {
		t.Fatal(e)
	}
	r.Header.Set("Authorization", "Bearer "+adminToken)
	r.Header.Set("Last-Event-ID", "2")
	res, e = h.Client().Do(r)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatal("冲突Last-Event-ID接受", res.StatusCode)
	}
}

func TestActualHistoryLargePagesContinueAtCompleteChunk(t *testing.T) {
	_, _, h, token, in := executionHTTPFixture(t)
	g := claimHTTP(t, h, token, in)
	chunk := logHTTPMessage(t, g.Ref)
	one := chunk.Records[0]
	one.Text = strings.Repeat("x", 3700)
	chunk.Records = make([]protocol.LogRecord, 16)
	for i := range chunk.Records {
		chunk.Records[i] = one
	}
	chunk.Digest = messageDigest(t, chunk.Records)
	size := int64(len(encodeMessage(t, chunk.Records)))
	for i := int64(1); i <= 20; i++ {
		chunk.Seq = i
		chunk.Offset = (i - 1) * size
		code, body := request(t, h, "POST", "/api/agent/logs", token, encodeMessage(t, chunk))
		if code != 200 {
			t.Fatal(code, body)
		}
	}
	path := "/api/builds/" + g.Ref.BuildID + "/log"
	code, body := request(t, h, "GET", path+"?limit=200", adminToken, "")
	var page LogPage
	if e := json.Unmarshal([]byte(body), &page); e != nil || code != 200 || len(body) > 1<<20 || page.NextSeq <= 0 || page.NextSeq >= 20 || int64(len(page.Records)) != page.NextSeq*16 || page.NextOffset != page.NextSeq*size {
		t.Fatal("分页拆chunk或不受限", code, len(body), page.NextSeq, e)
	}
	after := page.NextSeq
	code, body = request(t, h, "GET", path+"?after_seq="+strconv.FormatInt(after, 10), adminToken, "")
	if e := json.Unmarshal([]byte(body), &page); e != nil || code != 200 || page.NextSeq != 20 || page.NextOffset != 20*size || int64(len(page.Records)) != (20-after)*16 {
		t.Fatal("续读漏重复", code, page.NextSeq, e)
	}
	code, body = request(t, h, "GET", path+"?after_seq=20", adminToken, "")
	if e := json.Unmarshal([]byte(body), &page); e != nil || code != 200 || len(page.Records) != 0 || page.NextSeq != 20 || page.NextOffset != 20*size {
		t.Fatal("空页偏移回退", code, body, e)
	}
}

func TestActualSSEOutlivesOrdinaryWriteTimeout(t *testing.T) {
	s, _, _, token, in := executionHTTPFixture(t)
	slow := httptest.NewUnstartedServer(s.Handler())
	slow.Config.WriteTimeout = 100 * time.Millisecond
	slow.Start()
	defer slow.Close()
	g := claimHTTP(t, slow, token, in)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r, e := http.NewRequestWithContext(ctx, "GET", slow.URL+"/api/builds/"+g.Ref.BuildID+"/log?follow=1", nil)
	if e != nil {
		t.Fatal(e)
	}
	r.Header.Set("Authorization", "Bearer "+adminToken)
	res, e := slow.Client().Do(r)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	reader := bufio.NewReader(res.Body)
	for {
		line, e := reader.ReadString('\n')
		if e != nil {
			t.Fatal(e)
		}
		if line == "\n" {
			break
		}
	}
	time.Sleep(300 * time.Millisecond)
	chunk := logHTTPMessage(t, g.Ref)
	code, body := request(t, slow, "POST", "/api/agent/logs", token, encodeMessage(t, chunk))
	if code != 200 {
		t.Fatal(code, body)
	}
	var event strings.Builder
	for {
		line, e := reader.ReadString('\n')
		if e != nil {
			t.Fatal("SSE被普通WriteTimeout截断", e)
		}
		event.WriteString(line)
		if line == "\n" {
			break
		}
	}
	if !strings.Contains(event.String(), "id: 1\n") {
		t.Fatal(event.String())
	}
}
