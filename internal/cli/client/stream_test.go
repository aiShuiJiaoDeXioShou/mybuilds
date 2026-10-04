package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
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
	"mybuilds/internal/server"
	"mybuilds/internal/store"
)

func clientEvidenceFixture(t *testing.T, kind string) (*store.Store, *httptest.Server, string, protocol.LeaseGrant) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	if e := os.Chmod(dir, 0700); e != nil {
		t.Fatal(e)
	}
	st, e := store.Open(ctx, store.Options{Driver: "sqlite", DSN: filepath.Join(dir, "control.db")})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { st.Close() })
	if e = st.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	admin := store.Actor{ID: "local-admin", Role: "admin"}
	created, e := st.CreateToken(ctx, admin, "admin")
	if e != nil {
		t.Fatal(e)
	}
	t.Setenv("MYBUILDS_CLIENT_TOKEN", created.Token)
	node, e := st.CreateNode(ctx, admin, store.NodeInput{Name: "worker", Capacity: 1})
	if e != nil {
		t.Fatal(e)
	}
	actor, e := st.AuthenticateNode(ctx, node.Token)
	if e != nil {
		t.Fatal(e)
	}
	policy := store.LeasePolicy{Concurrency: 1, Heartbeat: 5 * time.Second, Duration: 30 * time.Second}
	session, e := st.OpenNodeSession(ctx, actor, protocol.SessionRequest{SessionID: uuid.NewString(), HeartbeatNS: int64(policy.Heartbeat), LeaseNS: int64(policy.Duration), Report: protocol.NodeReport{OS: "linux", Arch: "amd64", Capacity: 1, Tools: []protocol.ToolCheck{{Name: "shell", Status: "passed"}, {Name: "git", Status: "passed"}, {Name: "node_journal", Status: "passed"}}}}, policy)
	if e != nil {
		t.Fatal(e)
	}
	p, e := st.CreateProject(ctx, admin, store.ProjectInput{Name: "app", Repository: "https://example.org/repo.git", AllowedNodes: []string{"worker"}, DefaultNode: "worker"})
	if e != nil {
		t.Fatal(e)
	}
	step := config.Step{Name: "compile", Kind: kind}
	if kind == "run" {
		step.Run = ":"
	} else {
		step.Paths = []string{"out/*"}
	}
	_, e = st.Enqueue(ctx, store.EnqueueInput{Actor: admin, ProjectID: p.ID, ProjectVersion: p.PolicyVersion, Key: "evidence", RequestDigest: strings.Repeat("a", 64), SHA: strings.Repeat("b", 40), Branch: "main", Source: "repo", File: "mybuilds.yml", SourceDigest: strings.Repeat("c", 64), Builds: []store.PreparedBuild{{Name: "app", Status: "queued", PostBudgetNS: int64(2 * time.Minute), Snapshot: store.BuildSnapshot{Definition: config.Build{Steps: []config.Step{step}}, Condition: "ready", AllowedNodes: p.AllowedNodes, DefaultNode: p.DefaultNode}, Steps: []store.StepProgress{{Phase: "ordinary", Index: 1, Name: step.Name, Kind: step.Kind, Condition: "ready", Status: "pending"}}}}})
	if e != nil {
		t.Fatal(e)
	}
	grant, e := st.Claim(ctx, actor, protocol.ClaimRequest{SessionID: session.SessionID, ClaimKey: uuid.NewString()}, policy)
	if e != nil || grant == nil {
		t.Fatal(e)
	}
	api := httptest.NewServer(server.New(st, config.ServerConfig{DataDir: dir, Concurrency: 1, HeartbeatInterval: policy.Heartbeat, LeaseDuration: policy.Duration}).Handler())
	t.Cleanup(api.Close)
	return st, api, node.Token, *grant
}
func evidenceDigest(t *testing.T, v any) string {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func postEvidence(t *testing.T, api *httptest.Server, token, path string, v any) {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	r, e := http.NewRequest("POST", api.URL+path, bytes.NewReader(b))
	if e != nil {
		t.Fatal(e)
	}
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	res, e := api.Client().Do(r)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	body, e := io.ReadAll(res.Body)
	if e != nil || res.StatusCode != 200 {
		t.Fatal(res.StatusCode, string(body), e)
	}
}
func TestActualClientSSEMaximumLegalChunkAndCancellation(t *testing.T) {
	_, api, token, g := clientEvidenceFixture(t, "run")
	record := protocol.LogRecord{UTC: time.Now().UTC(), Build: "app", Phase: "ordinary", Index: 1, Step: "compile", Stream: "stdout", Text: ""}
	chunk := protocol.LogChunk{Ref: g.Ref, Seq: 1, Records: make([]protocol.LogRecord, 16)}
	low, high := 0, 8192
	for low < high {
		n := (low + high + 1) / 2
		record.Text = strings.Repeat("x", n) + "\x1b"
		for i := range chunk.Records {
			chunk.Records[i] = record
		}
		chunk.Digest = evidenceDigest(t, chunk.Records)
		body, _ := json.Marshal(chunk)
		if len(body) <= 64*1024 {
			low = n
		} else {
			high = n - 1
		}
	}
	record.Text = strings.Repeat("x", low) + "\x1b"
	for i := range chunk.Records {
		chunk.Records[i] = record
	}
	chunk.Digest = evidenceDigest(t, chunk.Records)
	postEvidence(t, api, token, "/api/agent/logs", chunk)
	out, e := executeRemote(t, "--server-url", api.URL, "--timeout", "50ms", "logs", g.Ref.BuildID, "--follow", "--stream-timeout", "1s")
	if e != nil || strings.Contains(out, "\x1b") || strings.Count(out, "\\u001b") != 16 {
		t.Fatal("合法最大chunk被丢失或控制码直出", len(out), e)
	}
	history, e := executeRemote(t, "--server-url", api.URL, "logs", g.Ref.BuildID, "--json")
	var page server.LogPage
	if e != nil || json.Unmarshal([]byte(history), &page) != nil || page.NextSeq != 1 || len(page.Records) != 16 {
		t.Fatal(len(history), e)
	}
	for _, args := range [][]string{{"--follow", "--json"}, {"--after-seq", "-1"}, {"--limit", "201"}, {"--stream-timeout", "0s"}} {
		_, e := executeRemote(t, append([]string{"logs", g.Ref.BuildID}, args...)...)
		if e == nil {
			t.Fatal("非法流选项", args)
		}
	}
}
func TestClientSSERejectsOversizedOrTruncatedEvents(t *testing.T) {
	for _, data := range []string{"data: " + strings.Repeat("x", 64*1024) + "\n\n", "event: log\nid: 1\ndata: {}", "event: log\nid: 1\nid: 1\ndata: {}\n\n", "event: end\ndata: null\n\n"} {
		cmd := NewCommand()
		cmd.SetOut(io.Discard)
		last := int64(0)
		if _, e := readLogEvents(context.Background(), cmd, strings.NewReader(data), &last); e == nil {
			t.Fatal("非法SSE接受")
		}
	}
}
