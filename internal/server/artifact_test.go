package server

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
)

func artifactHTTPFixture(t *testing.T) (*Server, *store.Store, *httptest.Server, string, protocol.ArtifactDeclaration, []byte) {
	t.Helper()
	s, st, h, token, in := executionHTTPBuildFixture(t, config.Build{Steps: []config.Step{{Name: "collect", Kind: "artifact", Paths: []string{"out/*"}}}})
	g := claimHTTP(t, h, token, in)
	id := uuid.NewString()
	p := protocol.ExecutionProgress{Kind: "intent", Phase: "ordinary", Name: "collect", StepKind: "artifact", Index: 1, At: time.Now().UTC(), RemainingPostBudgetNS: g.RemainingPostBudgetNS, ArtifactSteps: []protocol.ArtifactExpectation{}}
	for seq, kind := range []string{"intent", "started", "finished"} {
		p.Kind = kind
		p.At = time.Now().UTC()
		if kind != "intent" {
			p.Started = true
		}
		if kind == "finished" {
			p.Status = "succeeded"
			p.StopConfirmed = true
			p.ArtifactIDs = []string{id}
		}
		ev := protocol.ExecutionEvent{Ref: g.Ref, Seq: int64(seq + 1), Progress: p, Digest: messageDigest(t, p)}
		code, out := request(t, h, "POST", "/api/agent/events", token, encodeMessage(t, ev))
		if code != 200 {
			t.Fatal(code, out)
		}
	}
	data := []byte("actual artifact bytes\x00\xff")
	sum := sha256.Sum256(data)
	return s, st, h, token, protocol.ArtifactDeclaration{Ref: g.Ref, ID: id, Seq: 1, Phase: "ordinary", Step: "collect", Index: 1, Name: "app.apk", Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}, data
}
func uploadArtifactHTTP(t *testing.T, h *httptest.Server, token string, d protocol.ArtifactDeclaration, data []byte) (int, []byte) {
	t.Helper()
	r, e := http.NewRequest("PUT", h.URL+"/api/agent/artifacts/"+d.ID, bytes.NewReader(data))
	if e != nil {
		t.Fatal(e)
	}
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("X-Mybuilds-Artifact", base64.RawURLEncoding.EncodeToString([]byte(encodeMessage(t, d))))
	res, e := h.Client().Do(r)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	b, e := io.ReadAll(res.Body)
	if e != nil {
		t.Fatal(e)
	}
	return res.StatusCode, b
}
func TestActualArtifactUploadAndRepeatedCanonical(t *testing.T) {
	_, _, h, token, d, data := artifactHTTPFixture(t)
	code, body := uploadArtifactHTTP(t, h, token, d, data)
	var v protocol.ArtifactView
	if e := json.Unmarshal(body, &v); e != nil || code != 200 || v.ID != d.ID || v.Size != int64(len(data)) {
		t.Fatal(code, string(body), e)
	}
	code, repeated := uploadArtifactHTTP(t, h, token, d, data)
	if code != 200 || !bytes.Equal(repeated, body) {
		t.Fatal("重发非原meta", code, string(repeated))
	}
	d.SHA256 = messageDigest(t, []string{"bad"})
	code, _ = uploadArtifactHTTP(t, h, token, d, data)
	if code != 400 {
		t.Fatal("坏SHA接受", code)
	}
}

func TestActualArtifactSlowBodyOutlivesOrdinaryTimeout(t *testing.T) {
	s, _, _, token, d, data := artifactHTTPFixture(t)
	h := httptest.NewUnstartedServer(s.Handler())
	h.Config.ReadTimeout = 100 * time.Millisecond
	h.Config.WriteTimeout = 100 * time.Millisecond
	h.Start()
	defer h.Close()
	reader, writer := io.Pipe()
	go func() {
		_, e := writer.Write(data[:5])
		if e == nil {
			time.Sleep(300 * time.Millisecond)
			_, e = writer.Write(data[5:])
		}
		_ = writer.CloseWithError(e)
	}()
	r, e := http.NewRequest("PUT", h.URL+"/api/agent/artifacts/"+d.ID, reader)
	if e != nil {
		t.Fatal(e)
	}
	r.ContentLength = int64(len(data))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("X-Mybuilds-Artifact", base64.RawURLEncoding.EncodeToString([]byte(encodeMessage(t, d))))
	res, e := h.Client().Do(r)
	if e != nil {
		t.Fatal("普通超时截断上传", e)
	}
	defer res.Body.Close()
	body, e := io.ReadAll(res.Body)
	if e != nil || res.StatusCode != 200 {
		t.Fatal(res.StatusCode, string(body), e)
	}
}

func TestActualCentralArtifactReadableAfterNodeDisabled(t *testing.T) {
	s, st, h, token, d, data := artifactHTTPFixture(t)
	code, body := uploadArtifactHTTP(t, h, token, d, data)
	if code != 200 {
		t.Fatal(code, string(body))
	}
	code, out := request(t, h, "POST", "/api/nodes/worker/disable", adminToken, "")
	if code != 204 {
		t.Fatal(code, out)
	}
	metaPath := "/api/artifacts/" + d.ID
	code, out = request(t, h, "GET", metaPath, adminToken, "")
	var view protocol.ArtifactView
	if e := json.Unmarshal([]byte(out), &view); e != nil || code != 200 || view.Size != int64(len(data)) || strings.Contains(out, "storage_id") || strings.Contains(out, s.config.DataDir) {
		t.Fatal(code, out, e)
	}
	r, e := http.NewRequest("GET", h.URL+metaPath+"/"+d.Name, nil)
	if e != nil {
		t.Fatal(e)
	}
	r.Header.Set("Authorization", "Bearer "+adminToken)
	res, e := h.Client().Do(r)
	if e != nil {
		t.Fatal(e)
	}
	got, e := io.ReadAll(res.Body)
	res.Body.Close()
	if e != nil || res.StatusCode != 200 || !bytes.Equal(got, data) || res.Header.Get("X-Content-SHA256") != d.SHA256 || res.ContentLength != int64(len(data)) {
		t.Fatal("中央完整bytes错误", res.StatusCode, got, e)
	}
	admin, _ := st.Authenticate(context.Background(), adminToken)
	stored, e := st.GetArtifact(context.Background(), admin, d.ID)
	if e != nil {
		t.Fatal(e)
	}
	file := filepath.Join(s.config.DataDir, "artifacts", stored.StorageID)
	if e = os.WriteFile(file, bytes.Repeat([]byte("x"), len(data)), 0600); e != nil {
		t.Fatal(e)
	}
	code, out = request(t, h, "GET", metaPath+"/"+d.Name, adminToken, "")
	if code != 500 {
		t.Fatal("坏SHA文件仍可下载", code, out)
	}
}

func TestActualArtifactHeadersAndInputLimits(t *testing.T) {
	_, st, h, token, d, data := artifactHTTPFixture(t)
	cases := []struct {
		name   string
		code   int
		change func(*http.Request)
	}{
		{"duplicate_header", 400, func(r *http.Request) { r.Header.Add("X-Mybuilds-Artifact", r.Header.Get("X-Mybuilds-Artifact")) }},
		{"header_limit", 413, func(r *http.Request) { r.Header.Set("X-Mybuilds-Artifact", strings.Repeat("x", 8193)) }},
		{"encoding", 400, func(r *http.Request) { r.Header.Set("Content-Encoding", "identity") }},
		{"length", 400, func(r *http.Request) {
			v := d
			v.Size++
			r.Header.Set("X-Mybuilds-Artifact", base64.RawURLEncoding.EncodeToString([]byte(encodeMessage(t, v))))
		}},
		{"name", 400, func(r *http.Request) {
			v := d
			v.Name = "../PRIVATE"
			r.Header.Set("X-Mybuilds-Artifact", base64.RawURLEncoding.EncodeToString([]byte(encodeMessage(t, v))))
		}},
		{"size", 413, func(r *http.Request) {
			v := d
			v.Size = (1 << 30) + 1
			r.Header.Set("X-Mybuilds-Artifact", base64.RawURLEncoding.EncodeToString([]byte(encodeMessage(t, v))))
		}},
		{"fence", 409, func(r *http.Request) {
			v := d
			v.Ref.Epoch++
			r.Header.Set("X-Mybuilds-Artifact", base64.RawURLEncoding.EncodeToString([]byte(encodeMessage(t, v))))
		}},
		{"unknown", 400, func(r *http.Request) {
			raw := encodeMessage(t, d)
			raw = raw[:len(raw)-1] + `,"unknown":"PRIVATE"}`
			r.Header.Set("X-Mybuilds-Artifact", base64.RawURLEncoding.EncodeToString([]byte(raw)))
		}},
		{"noncanonical", 400, func(r *http.Request) {
			r.Header.Set("X-Mybuilds-Artifact", base64.RawURLEncoding.EncodeToString([]byte(" "+encodeMessage(t, d))))
		}},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			r, e := http.NewRequest("PUT", h.URL+"/api/agent/artifacts/"+d.ID, bytes.NewReader(data))
			if e != nil {
				t.Fatal(e)
			}
			r.Header.Set("Authorization", "Bearer "+token)
			r.Header.Set("X-Mybuilds-Artifact", base64.RawURLEncoding.EncodeToString([]byte(encodeMessage(t, d))))
			item.change(r)
			res, e := h.Client().Do(r)
			if e != nil {
				t.Fatal(e)
			}
			out, e := io.ReadAll(res.Body)
			res.Body.Close()
			if e != nil || res.StatusCode != item.code || strings.Contains(string(out), "PRIVATE") {
				t.Fatal(res.StatusCode, string(out), e)
			}
		})
	}
	admin, _ := st.Authenticate(context.Background(), adminToken)
	files, e := st.ListArtifacts(context.Background(), admin, d.Ref.BuildID, store.Page{Limit: 200})
	if e != nil || len(files) != 0 {
		t.Fatal("非法上传有确认记录", files, e)
	}
}
func TestActualArtifactAuthorityLossInterruptsSlowBody(t *testing.T) {
	s, st, h, token, d, data := artifactHTTPFixture(t)
	address := strings.TrimPrefix(h.URL, "http://")
	conn, e := net.Dial("tcp", address)
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	header := base64.RawURLEncoding.EncodeToString([]byte(encodeMessage(t, d)))
	_, e = fmt.Fprintf(conn, "PUT /api/agent/artifacts/%s HTTP/1.1\r\nHost: %s\r\nAuthorization: Bearer %s\r\nX-Mybuilds-Artifact: %s\r\nContent-Length: %d\r\n\r\n", d.ID, address, token, header, len(data))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = conn.Write(data[:1]); e != nil {
		t.Fatal(e)
	}
	time.Sleep(50 * time.Millisecond)
	code, out := request(t, h, "POST", "/api/nodes/worker/disable", adminToken, "")
	if code != 204 {
		t.Fatal(code, out)
	}
	res, e := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "PUT"})
	if e != nil {
		t.Fatal("失权慢body未有界停止", e)
	}
	defer res.Body.Close()
	if res.StatusCode != 401 && res.StatusCode != 409 {
		t.Fatal("失权仍ACK", res.StatusCode)
	}
	admin, _ := st.Authenticate(context.Background(), adminToken)
	files, e := st.ListArtifacts(context.Background(), admin, d.Ref.BuildID, store.Page{Limit: 200})
	if e != nil || len(files) != 0 {
		t.Fatal("失权文件确认", files, e)
	}
	entries, e := os.ReadDir(filepath.Join(s.config.DataDir, "artifacts"))
	if e != nil || len(entries) != 0 {
		t.Fatal("失权stage残留", entries, e)
	}
}
