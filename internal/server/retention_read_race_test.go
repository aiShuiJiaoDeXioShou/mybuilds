//go:build darwin || linux

package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
)

// 返回真实上传/终态后、尚未退役的资源，便于真实socket与fd跨退役竞争。
func retentionLiveReadFixture(t *testing.T, kind string) (*Server, *httptest.Server, store.Actor, string, string, string, string, []byte) {
	t.Helper()
	ctx := context.Background()
	var s *Server
	var st *store.Store
	var h *httptest.Server
	var token string
	var d protocol.ArtifactDeclaration
	var evidence protocol.ReportEvidence
	var data []byte
	if kind == "junit" {
		data = []byte(`<testsuite tests="1"><testcase name="actual-read"/></testsuite>`)
		s, st, h, token, d, evidence = reportHTTPFixture(t, data, nil)
	} else {
		s, st, h, token, d, data = artifactHTTPFixture(t)
	}
	if code, body := uploadArtifactHTTP(t, h, token, d, data); code != 200 {
		t.Fatal(code, string(body))
	}
	registration := protocol.NodeResourceRegistration{Ref: d.Ref, ID: uuid.NewString(), OwnershipDigest: strings.Repeat("a", 64), HasWorkspace: true, HasResults: true}
	if code, body := request(t, h, "POST", "/api/agent/resources", token, encodeMessage(t, registration)); code != 204 {
		t.Fatal(code, body)
	}
	var logSeq, logOffset int64
	if kind == "log" {
		chunk := logHTTPMessage(t, d.Ref)
		chunk.Records[0].Step = "collect"
		chunk.Records[0].Stream = "system"
		chunk.Digest = messageDigest(t, chunk.Records)
		if code, body := request(t, h, "POST", "/api/agent/logs", token, encodeMessage(t, chunk)); code != 200 {
			t.Fatal(code, body)
		}
		logSeq = 1
		data = []byte(encodeMessage(t, chunk.Records))
		logOffset = int64(len(data))
	}
	seq := int64(4)
	var manifest *protocol.ReportManifest
	if kind == "junit" {
		seal := sealReportHTTP(t, h, token, d, evidence, 200)
		seq = 7
		manifest = &protocol.ReportManifest{SealDigest: seal, IDs: []string{d.ID}}
	}
	postReportEvent(t, h, token, d.Ref, seq, protocol.ExecutionProgress{Kind: "post_selected", PostPhase: "success", At: time.Now().UTC(), RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}, 200)
	expected := []protocol.ArtifactExpectation{{Phase: "ordinary", Index: 1, Count: 1, IDs: []string{d.ID}}}
	if kind == "junit" {
		expected = []protocol.ArtifactExpectation{}
	}
	postReportEvent(t, h, token, d.Ref, seq+1, protocol.ExecutionProgress{Kind: "build_finished", Status: "succeeded", Started: true, StopConfirmed: true, At: time.Now().UTC(), RemainingPostBudgetNS: int64(2 * time.Minute), LastLogSeq: logSeq, LastLogOffset: logOffset, LastArtifactSeq: 1, ArtifactSteps: expected, ReportManifest: manifest}, 200)
	actor, err := st.Authenticate(ctx, adminToken)
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.GetProject(ctx, "app")
	if err != nil {
		t.Fatal(err)
	}
	newer := store.PreparedBuild{Name: "newer-read", Status: "skipped", Snapshot: store.BuildSnapshot{Definition: config.Build{Steps: []config.Step{{Name: "shell", Kind: "run", Run: "true"}}}, Condition: "skipped", Params: map[string]string{}, Facts: map[string]string{}}, Steps: []store.StepProgress{{Phase: "ordinary", Index: 1, Name: "shell", Kind: "run", Condition: "ready", Status: "pending"}}}
	if _, err = st.Enqueue(ctx, store.EnqueueInput{Actor: actor, ProjectID: p.ID, ProjectVersion: p.PolicyVersion, Key: "read-newer", RequestDigest: strings.Repeat("d", 64), SHA: strings.Repeat("b", 40), Branch: "main", Source: "repo", File: "mybuilds.yml", SourceDigest: strings.Repeat("c", 64), Builds: []store.PreparedBuild{newer}}); err != nil {
		t.Fatal(err)
	}
	if err = st.SyncGlobalRetention(ctx, config.Retention{Builds: 1, Days: 30}); err != nil {
		t.Fatal(err)
	}
	id, url, path := d.ID, "/api/artifacts/"+d.ID+"/"+d.Name, ""
	if kind == "log" {
		rows, err := st.ListLogChunks(ctx, actor, d.Ref.BuildID, 0, store.Page{})
		if err != nil || len(rows) != 1 {
			t.Fatal(err, len(rows))
		}
		id = rows[0].ID
		path = filepath.Join(s.config.DataDir, "logs", rows[0].StorageID)
		url = "/api/builds/" + d.Ref.BuildID + "/log?follow=1"
	} else {
		file, err := st.GetArtifact(ctx, actor, id)
		if err != nil {
			t.Fatal(err)
		}
		path = filepath.Join(s.config.DataDir, "artifacts", file.StorageID)
	}
	return s, h, actor, p.ID, id, url, path, data
}

func TestRetentionActualSocketReadRetireRace100(t *testing.T) {
	for _, kind := range []string{"artifact", "junit", "log"} {
		t.Run(kind, func(t *testing.T) {
			s, h, actor, pid, id, url, path, original := retentionLiveReadFixture(t, kind)
			readKind := kind
			if kind == "junit" {
				readKind = "artifact"
			}
			held, err := s.openRetentionEvidence(context.Background(), actor, readKind, id)
			if err != nil {
				t.Fatal(err)
			}
			defer held.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			type outcome struct {
				status int
				data   []byte
				digest string
				err    error
			}
			results := make(chan outcome, 100)
			start := make(chan struct{})
			var workers sync.WaitGroup
			for range 100 {
				workers.Add(1)
				go func() {
					defer workers.Done()
					<-start
					r, e := http.NewRequestWithContext(ctx, http.MethodGet, h.URL+url, nil)
					if e != nil {
						results <- outcome{err: e}
						return
					}
					r.Header.Set("Authorization", "Bearer "+adminToken)
					response, e := h.Client().Do(r)
					if e != nil {
						results <- outcome{err: e}
						return
					}
					data, e := io.ReadAll(response.Body)
					response.Body.Close()
					results <- outcome{status: response.StatusCode, data: data, digest: response.Header.Get("X-Content-SHA256"), err: e}
				}()
			}
			close(start)
			if _, err = s.store.ScheduleRetention(ctx, actor, pid, 100); err != nil {
				cancel()
				workers.Wait()
				t.Fatal(err)
			}
			if err = s.advanceCentralRetention(ctx, pid, 100); !errors.Is(err, store.ErrRetentionReadersActive) {
				cancel()
				workers.Wait()
				t.Fatal("实际held SH期间必须等待", err)
			}
			data, e := os.ReadFile(path)
			if e != nil || !bytes.Equal(data, original) {
				cancel()
				workers.Wait()
				t.Fatal("活动fd对应原文件未保持", e)
			}
			workers.Wait()
			close(results)
			for result := range results {
				if result.err != nil {
					t.Fatal("真实socket读取失败", result.err)
				}
				if result.status == 410 {
					if !bytes.Contains(result.data, []byte("retention_retired")) {
						t.Fatal("非退役固定错误")
					}
					continue
				}
				if result.status != 200 {
					t.Fatal("竞争产生不明确成功", result.status, string(result.data))
				}
				if kind == "log" {
					if !bytes.Contains(result.data, []byte("event: log\n")) || !bytes.Contains(result.data, []byte(`"next_seq":1`)) {
						t.Fatal("真实SSE未完成原chunk", string(result.data))
					}
				} else {
					hash := sha256.Sum256(result.data)
					if !bytes.Equal(result.data, original) || hex.EncodeToString(hash[:]) != result.digest {
						t.Fatal("成功下载不是完整原字节")
					}
				}
			}
			if err = held.Close(); err != nil {
				t.Fatal(err)
			}
			if err = s.advanceCentralRetention(ctx, pid, 100); err != nil {
				t.Fatal("所有读者已真实结束后清理", err)
			}
			if _, err = os.Lstat(path); !os.IsNotExist(err) {
				t.Fatal("原文件实际未删除", err)
			}
			for range 3 {
				r, _ := http.NewRequestWithContext(ctx, http.MethodGet, h.URL+url, nil)
				r.Header.Set("Authorization", "Bearer "+adminToken)
				response, e := h.Client().Do(r)
				if e != nil {
					t.Fatal(e)
				}
				data, e = io.ReadAll(response.Body)
				response.Body.Close()
				if e != nil || response.StatusCode != 410 || !bytes.Contains(data, []byte("retention_retired")) {
					t.Fatal("退役后新socket读", response.StatusCode, e)
				}
			}
		})
	}
}

func TestRetentionActualReadOwnerRestartRequiresSameFDReleased(t *testing.T) {
	s, h, actor, pid, id, _, path, original := retentionLiveReadFixture(t, "artifact")
	held, err := s.openRetentionEvidence(context.Background(), actor, "artifact", id)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	if _, err = s.store.ScheduleRetention(context.Background(), actor, pid, 100); err != nil {
		t.Fatal(err)
	}
	h.Close()
	if err = s.store.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := store.Open(context.Background(), store.Options{Driver: "sqlite", DSN: filepath.Join(s.config.DataDir, "control.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	current := New(next, s.config)
	if current.evidenceReadError != nil || current.evidenceReadOwner == s.evidenceReadOwner {
		t.Fatal("真实重开控制实例", current.evidenceReadError)
	}
	if err = current.advanceCentralRetention(context.Background(), pid, 100); !errors.Is(err, store.ErrRetentionReadersActive) {
		t.Fatal("新owner不能代替原同fd EX", err)
	}
	data, err := io.ReadAll(held.File)
	if err != nil || !bytes.Equal(data, original) {
		t.Fatal("原held fd字节", err)
	}
	if err = held.Close(); err == nil {
		t.Fatal("旧已关闭Store不能清登记")
	}
	states := retentionEvidenceRows(t, current)
	if states[held.Read.ID].state != "active" {
		t.Fatal("旧登记被提前清", states)
	}
	if err = current.advanceCentralRetention(context.Background(), pid, 100); err != nil {
		t.Fatal("实际旧fd释放后同inode EX修复", err)
	}
	states = retentionEvidenceRows(t, current)
	if states[held.Read.ID].state != "closed" || states[held.Read.ID].owner != s.evidenceReadOwner {
		t.Fatal("实际EX绑定原身份/owner", states)
	}
	if _, err = os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("恢复实际清理", err)
	}
}
