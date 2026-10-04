package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

func encodeMessage(t *testing.T, v any) string {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}
func messageDigest(t *testing.T, v any) string {
	b := []byte(encodeMessage(t, v))
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func executionHTTPFixture(t *testing.T) (*Server, *store.Store, *httptest.Server, string, protocol.ClaimRequest) {
	return executionHTTPBuildFixture(t, config.Build{Steps: []config.Step{{Name: "shell", Kind: "run", Run: "echo PRIVATE_SCRIPT"}}})
}
func executionHTTPBuildFixture(t *testing.T, definition config.Build) (*Server, *store.Store, *httptest.Server, string, protocol.ClaimRequest) {
	t.Helper()
	s, st, h := serverFixture(t)
	if e := os.Chmod(s.config.DataDir, 0700); e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	admin, e := st.Authenticate(ctx, adminToken)
	if e != nil {
		t.Fatal(e)
	}
	node, e := st.CreateNode(ctx, admin, store.NodeInput{Name: "worker", Capacity: 1})
	if e != nil {
		t.Fatal(e)
	}
	p, e := st.CreateProject(ctx, admin, store.ProjectInput{Name: "app", Repository: "https://example.org/repo.git", AllowedNodes: []string{"worker"}, DefaultNode: "worker"})
	if e != nil {
		t.Fatal(e)
	}
	_, e = st.Enqueue(ctx, store.EnqueueInput{Actor: admin, ProjectID: p.ID, ProjectVersion: p.PolicyVersion, Key: "request", RequestDigest: strings.Repeat("a", 64), SHA: strings.Repeat("b", 40), Branch: "main", Source: "repo", File: "mybuilds.yml", SourceDigest: strings.Repeat("c", 64), Builds: []store.PreparedBuild{{Name: "compile", Status: "queued", Snapshot: store.BuildSnapshot{Definition: definition, Params: map[string]string{}, Facts: map[string]string{}, Condition: "ready", AllowedNodes: p.AllowedNodes, DefaultNode: p.DefaultNode}, PostBudgetNS: int64(2 * time.Minute), Steps: []store.StepProgress{{Phase: "ordinary", Index: 1, Name: definition.Steps[0].Name, Kind: definition.Steps[0].Kind, Condition: "ready", Status: "pending"}}}}})
	if e != nil {
		t.Fatal(e)
	}
	session := protocol.SessionRequest{SessionID: uuid.NewString(), HeartbeatNS: int64(5 * time.Second), LeaseNS: int64(30 * time.Second), Report: protocol.NodeReport{OS: "linux", Arch: "amd64", Capacity: 1, Tools: []protocol.ToolCheck{{Name: "shell", Status: "passed"}, {Name: "git", Status: "passed"}, {Name: "node_journal", Status: "passed"}}}}
	code, body := request(t, h, "POST", "/api/agent/session", node.Token, encodeMessage(t, session))
	if code != 200 {
		t.Fatal(code, body)
	}
	return s, st, h, node.Token, protocol.ClaimRequest{SessionID: session.SessionID, ClaimKey: uuid.NewString()}
}
func claimHTTP(t *testing.T, h *httptest.Server, token string, in protocol.ClaimRequest) protocol.LeaseGrant {
	t.Helper()
	code, body := request(t, h, "POST", "/api/agent/claim", token, encodeMessage(t, in))
	if code != 200 {
		t.Fatal(code, body)
	}
	var g protocol.LeaseGrant
	if e := json.Unmarshal([]byte(body), &g); e != nil || g.Task == nil || g.TTLNS <= 0 {
		t.Fatal(body, e)
	}
	return g
}
func TestActualExecutionHTTPClaimRenewAndEvent(t *testing.T) {
	_, st, h, token, in := executionHTTPFixture(t)
	g := claimHTTP(t, h, token, in)
	replay := claimHTTP(t, h, token, in)
	if replay.Ref != g.Ref {
		t.Fatal("领取重放更换执行")
	}
	in.ClaimKey = uuid.NewString()
	code, body := request(t, h, "POST", "/api/agent/claim", token, encodeMessage(t, in))
	if code != 204 {
		t.Fatal(code, body)
	}
	code, body = request(t, h, "POST", "/api/agent/renew", token, encodeMessage(t, g.Ref))
	var renewed protocol.LeaseGrant
	json.Unmarshal([]byte(body), &renewed)
	if code != 200 || renewed.Task != nil || renewed.Ref != g.Ref {
		t.Fatal(code, body)
	}
	p := protocol.ExecutionProgress{Kind: "intent", Phase: "ordinary", Index: 1, Name: "shell", StepKind: "run", At: time.Now().UTC(), RemainingPostBudgetNS: g.RemainingPostBudgetNS, ArtifactSteps: []protocol.ArtifactExpectation{}}
	ev := protocol.ExecutionEvent{Ref: g.Ref, Seq: 1, Digest: messageDigest(t, p), Progress: p}
	code, body = request(t, h, "POST", "/api/agent/events", token, encodeMessage(t, ev))
	if code != 200 {
		t.Fatal(code, body)
	}
	code, repeated := request(t, h, "POST", "/api/agent/events", token, encodeMessage(t, ev))
	if code != 200 || repeated != body {
		t.Fatal("事件原回执不一致", code, repeated)
	}
	ev.Progress.At = ev.Progress.At.Add(time.Nanosecond)
	ev.Digest = messageDigest(t, ev.Progress)
	code, _ = request(t, h, "POST", "/api/agent/events", token, encodeMessage(t, ev))
	if code != 409 {
		t.Fatal("冲突事件接受", code)
	}
	for _, path := range []string{"claim", "renew", "events", "logs"} {
		code, _ = request(t, h, "POST", "/api/agent/"+path, adminToken, "{}")
		if code != 401 {
			t.Fatal("用户进入执行接口", path, code)
		}
	}
	wrong := g.Ref
	wrong.Epoch++
	code, _ = request(t, h, "POST", "/api/agent/renew", token, encodeMessage(t, wrong))
	if code != 409 {
		t.Fatal("旧fence接受", code)
	}
	view, e := st.GetBuild(context.Background(), g.Ref.BuildID)
	if e != nil || !view.Steps[0].Intent || view.Steps[0].Started {
		t.Fatal("HTTP伪造启动证据", view, e)
	}
}
func TestExecutionHTTPStrictAndLockLost(t *testing.T) {
	s, _, h, token, in := executionHTTPFixture(t)
	for _, body := range []string{`{"session_id":null}`, `{"SessionID":"PRIVATE"}`, `{"session_id":"x","session_id":"y"}`, `{"unknown":"PRIVATE"}`} {
		code, out := request(t, h, "POST", "/api/agent/claim", token, body)
		if code != 400 || strings.Contains(out, "PRIVATE") {
			t.Fatal(code, out)
		}
	}
	loseServerLock(t, s)
	code, body := request(t, h, "POST", "/api/agent/claim", token, encodeMessage(t, in))
	if code != 503 {
		t.Fatal(code, body)
	}
}

func loseServerLock(t *testing.T, s *Server) {
	t.Helper()
	p := filepath.Join(s.config.DataDir, "control.db.lock")
	if e := os.Rename(p, p+".owned-old"); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(p, []byte("replacement"), 0600); e != nil {
		t.Fatal(e)
	}
}
