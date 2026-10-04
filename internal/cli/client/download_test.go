package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"mybuilds/internal/protocol"
)

func clientArtifactFixture(t *testing.T) (string, string, []byte) {
	t.Helper()
	_, api, token, g := clientEvidenceFixture(t, "artifact")
	id := uuid.NewString()
	p := protocol.ExecutionProgress{Kind: "intent", Phase: "ordinary", Name: "compile", StepKind: "artifact", Index: 1, At: time.Now().UTC(), RemainingPostBudgetNS: g.RemainingPostBudgetNS, ArtifactSteps: []protocol.ArtifactExpectation{}}
	for i, kind := range []string{"intent", "started", "finished"} {
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
		postEvidence(t, api, token, "/api/agent/events", protocol.ExecutionEvent{Ref: g.Ref, Seq: int64(i + 1), Progress: p, Digest: evidenceDigest(t, p)})
	}
	data := []byte("real binary\x00\xff")
	sum := sha256.Sum256(data)
	decl := protocol.ArtifactDeclaration{Ref: g.Ref, ID: id, Seq: 1, Phase: "ordinary", Step: "compile", Index: 1, Name: "app.apk", Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}
	encoded, e := json.Marshal(decl)
	if e != nil {
		t.Fatal(e)
	}
	r, e := http.NewRequest("PUT", api.URL+"/api/agent/artifacts/"+id, bytes.NewReader(data))
	if e != nil {
		t.Fatal(e)
	}
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("X-Mybuilds-Artifact", base64.RawURLEncoding.EncodeToString(encoded))
	res, e := api.Client().Do(r)
	if e != nil {
		t.Fatal(e)
	}
	body, e := io.ReadAll(res.Body)
	res.Body.Close()
	if e != nil || res.StatusCode != 200 {
		t.Fatal(res.StatusCode, string(body), e)
	}
	return api.URL, id, data
}
func TestActualClientArtifactDownloadAndExclusiveOutput(t *testing.T) {
	address, id, data := clientArtifactFixture(t)
	dir := t.TempDir()
	if e := os.Chmod(dir, 0700); e != nil {
		t.Fatal(e)
	}
	output := filepath.Join(dir, "download.apk")
	out, e := executeRemote(t, "--server-url", address, "artifact", "download", id, "--output", output)
	if e != nil {
		t.Fatal(out, e)
	}
	actual, e := os.ReadFile(output)
	info, se := os.Lstat(output)
	if e != nil || se != nil || !bytes.Equal(actual, data) || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		t.Fatal(actual, info, e, se)
	}
	entries, e := os.ReadDir(dir)
	if e != nil || len(entries) != 1 {
		t.Fatal("下载stage残留", entries, e)
	}
	if _, e = executeRemote(t, "--server-url", address, "artifact", "download", id, "--output", output); e == nil {
		t.Fatal("覆盖现有输出")
	}
	actual, _ = os.ReadFile(output)
	if !bytes.Equal(actual, data) {
		t.Fatal("原目标被修改")
	}
	for _, args := range [][]string{{"artifact", "download", id}, {"artifact", "download", id, "--output", output, "--force"}, {"artifact", "retry", id}, {"artifact", "ls", id, "--limit", "201"}} {
		if _, e := executeRemote(t, args...); e == nil {
			t.Fatal("非法产物选项接受", args)
		}
	}
}

type blockedArtifactBody struct {
	io.ReadCloser
	ctx    context.Context
	prefix bool
}

func (body *blockedArtifactBody) Read(buffer []byte) (int, error) {
	if !body.prefix {
		body.prefix = true
		return body.ReadCloser.Read(buffer[:min(2, len(buffer))])
	}
	<-body.ctx.Done()
	return 0, body.ctx.Err()
}
func TestActualClientDownloadShortDigestAndCancellation(t *testing.T) {
	address, id, _ := clientArtifactFixture(t)
	destination, e := url.Parse(address)
	if e != nil {
		t.Fatal(e)
	}
	for _, mode := range []string{"short", "digest", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			proxy := httputil.NewSingleHostReverseProxy(destination)
			proxy.FlushInterval = -1
			proxy.ErrorLog = log.New(io.Discard, "", 0)
			proxy.ModifyResponse = func(response *http.Response) error {
				if response.Header.Get("Content-Type") != "application/octet-stream" {
					return nil
				}
				switch mode {
				case "short":
					response.Body = struct {
						io.Reader
						io.Closer
					}{io.LimitReader(response.Body, 2), response.Body}
				case "digest":
					response.Header.Set("X-Content-SHA256", strings.Repeat("0", 64))
				case "cancel":
					response.Body = &blockedArtifactBody{ReadCloser: response.Body, ctx: response.Request.Context()}
				}
				return nil
			}
			api := httptest.NewServer(proxy)
			defer api.Close()
			dir := t.TempDir()
			if e = os.Chmod(dir, 0700); e != nil {
				t.Fatal(e)
			}
			output := filepath.Join(dir, "must-not-publish.apk")
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			cmd := NewCommand()
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SetArgs([]string{"--server-url", api.URL, "artifact", "download", id, "--output", output})
			done := make(chan error, 1)
			go func() { done <- cmd.ExecuteContext(ctx) }()
			if mode == "cancel" {
				until := time.Now().Add(2 * time.Second)
				for {
					entries, e := os.ReadDir(dir)
					if e != nil {
						t.Fatal(e)
					}
					if len(entries) > 0 {
						break
					}
					if time.Now().After(until) {
						t.Fatal("真实慢流未创建stage")
					}
					time.Sleep(10 * time.Millisecond)
				}
				cancel()
			}
			select {
			case e := <-done:
				if e == nil {
					t.Fatal("未完整/坏digest/取消下载成功")
				}
			case <-time.After(4 * time.Second):
				t.Fatal("下载未有界停止")
			}
			if _, e = os.Lstat(output); !os.IsNotExist(e) {
				t.Fatal("失败输出被发布", e)
			}
			entries, e := os.ReadDir(dir)
			if e != nil || len(entries) != 0 {
				t.Fatal("失败stage残留", entries, e)
			}
		})
	}
}
