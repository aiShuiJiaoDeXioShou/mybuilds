package server

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
	"mybuilds/internal/store"
)

// 以真实Store/HTTP领取、run事件及final检查声明报告，不伪造artifact步骤。
func reportHTTPFixture(t *testing.T, data []byte, budget *int64) (*Server, *store.Store, *httptest.Server, string, protocol.ArtifactDeclaration, protocol.ReportEvidence) {
	t.Helper()
	definition := config.Build{Steps: []config.Step{{Name: "tests", Kind: "run", Run: "printf test"}}, Reports: &config.Reports{JUnit: &config.JUnitReport{Paths: []string{"results/*.xml"}}}}
	var s *Server
	var st *store.Store
	var api *httptest.Server
	var token string
	var claim protocol.ClaimRequest
	if budget == nil {
		s, st, api, token, claim = executionHTTPBuildFixture(t, definition)
	} else {
		s, st, api = serverFixture(t)
		if err := os.Chmod(s.config.DataDir, 0700); err != nil {
			t.Fatal(err)
		}
		admin, err := st.Authenticate(context.Background(), adminToken)
		if err != nil {
			t.Fatal(err)
		}
		node, err := st.CreateNode(context.Background(), admin, store.NodeInput{Name: "worker", Capacity: 1})
		if err != nil {
			t.Fatal(err)
		}
		project, err := st.CreateProject(context.Background(), admin, store.ProjectInput{Name: "app", Repository: "https://example.org/repo.git", AllowedNodes: []string{"worker"}, DefaultNode: "worker"})
		if err != nil {
			t.Fatal(err)
		}
		definition.Timeout = time.Duration(*budget).String()
		_, err = st.Enqueue(context.Background(), store.EnqueueInput{Actor: admin, ProjectID: project.ID, ProjectVersion: project.PolicyVersion, Key: "request", RequestDigest: strings.Repeat("a", 64), SHA: strings.Repeat("b", 40), Branch: "main", Source: "repo", File: "mybuilds.yml", SourceDigest: strings.Repeat("c", 64), Builds: []store.PreparedBuild{{Name: "compile", Status: "queued", InitialBudgetNS: budget, PostBudgetNS: int64(2 * time.Minute), Snapshot: store.BuildSnapshot{Definition: definition, Params: map[string]string{}, Facts: map[string]string{}, Condition: "ready", AllowedNodes: project.AllowedNodes, DefaultNode: project.DefaultNode}, Steps: []store.StepProgress{{Phase: "ordinary", Index: 1, Name: "tests", Kind: "run", Condition: "ready", Status: "pending"}}}}})
		if err != nil {
			t.Fatal(err)
		}
		session := protocol.SessionRequest{SessionID: uuid.NewString(), HeartbeatNS: int64(5 * time.Second), LeaseNS: int64(30 * time.Second), Report: protocol.NodeReport{OS: "linux", Arch: "amd64", Capacity: 1, Tools: []protocol.ToolCheck{{Name: "shell", Status: "passed"}, {Name: "git", Status: "passed"}, {Name: "node_journal", Status: "passed"}}}}
		code, out := request(t, api, "POST", "/api/agent/session", node.Token, encodeMessage(t, session))
		if code != 200 {
			t.Fatal(code, out)
		}
		token = node.Token
		claim = protocol.ClaimRequest{SessionID: session.SessionID, ClaimKey: uuid.NewString()}
	}
	grant := claimHTTP(t, api, token, claim)
	sum := sha256.Sum256(data)
	pathKey := sha256.Sum256([]byte("results/result.xml"))
	declaration := protocol.ArtifactDeclaration{Ref: grant.Ref, ID: uuid.NewString(), Seq: 1, Phase: "ordinary", Index: 1, Step: "tests", Name: "result.xml", Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:]), Purpose: "junit", ReportRevision: 2, ReportKey: hex.EncodeToString(pathKey[:])}
	for i, kind := range []string{"intent", "started", "finished"} {
		p := protocol.ExecutionProgress{Kind: kind, Phase: "ordinary", Index: 1, Name: "tests", StepKind: "run", At: time.Now().UTC(), ExitCode: -1, RemainingBudgetNS: budget, RemainingPostBudgetNS: grant.RemainingPostBudgetNS, ArtifactSteps: []protocol.ArtifactExpectation{}}
		if kind != "intent" {
			p.Started = true
		}
		if kind == "finished" {
			p.Status = "succeeded"
			p.StopConfirmed = true
			p.ExitCode = 0
		}
		postReportEvent(t, api, token, declaration.Ref, int64(i+1), p, 200)
	}
	// 恶意XML测试也只声明1个用例，服务端必须重新解析，不能相信这份自报摘要。
	counts := protocol.JUnitCounts{Tests: 1}
	evidence := protocol.ReportEvidence{Revision: 1, Outcome: "pending", Required: true, Counts: counts, Diagnostics: []protocol.JUnitDiagnostic{}, Files: []protocol.ReportFile{{Key: declaration.ReportKey, Path: "results/result.xml", ArtifactID: declaration.ID, SourceIndex: 1, SourceStep: "tests", Size: declaration.Size, SHA256: declaration.SHA256, Counts: counts}}}
	p := protocol.ExecutionProgress{Kind: "reports_checked", Phase: "ordinary", Index: 1, Name: "tests", StepKind: "run", At: time.Now().UTC(), ExitCode: -1, RemainingBudgetNS: budget, RemainingPostBudgetNS: grant.RemainingPostBudgetNS, ArtifactSteps: []protocol.ArtifactExpectation{}, Reports: &evidence}
	postReportEvent(t, api, token, declaration.Ref, 4, p, 200)
	evidence.Revision = 2
	evidence.Outcome = "passed"
	p.Phase = ""
	p.Index = 0
	p.Name = ""
	p.StepKind = ""
	p.At = time.Now().UTC()
	postReportEvent(t, api, token, declaration.Ref, 5, p, 200)
	return s, st, api, token, declaration, evidence
}

func postReportEvent(t *testing.T, api *httptest.Server, token string, ref protocol.LeaseRef, seq int64, p protocol.ExecutionProgress, want int) {
	t.Helper()
	ev := protocol.ExecutionEvent{Ref: ref, Seq: seq, Digest: messageDigest(t, p), Progress: p}
	code, out := request(t, api, "POST", "/api/agent/events", token, encodeMessage(t, ev))
	if code != want {
		t.Fatalf("事件%s seq%d=%d，%s", p.Kind, seq, code, out)
	}
}
func sealReportHTTP(t *testing.T, api *httptest.Server, token string, d protocol.ArtifactDeclaration, evidence protocol.ReportEvidence, want int) string {
	t.Helper()
	evidence.Sealed = true
	p := protocol.ExecutionProgress{Kind: "reports_sealed", At: time.Now().UTC(), ExitCode: -1, RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}, Reports: &evidence}
	postReportEvent(t, api, token, d.Ref, 6, p, want)
	return messageDigest(t, evidence)
}

func TestReportHTTPSealedOriginalAndRoleBoundaries(t *testing.T) {
	data := []byte(`<testsuite tests="1"><testcase name="actual"/></testsuite>`)
	s, st, api, token, d, evidence := reportHTTPFixture(t, data, nil)
	sealReportHTTP(t, api, token, d, evidence, 409)
	code, body := uploadArtifactHTTP(t, api, token, d, data)
	if code != 200 {
		t.Fatal(code, string(body))
	}
	var meta protocol.ArtifactView
	if err := json.Unmarshal(body, &meta); err != nil || meta.Purpose != "junit" || meta.ReportRevision != 2 || meta.ReportKey != d.ReportKey {
		t.Fatal("缺用途meta", err, string(body))
	}
	code, repeated := uploadArtifactHTTP(t, api, token, d, data)
	if code != 200 || !bytes.Equal(body, repeated) {
		t.Fatal("同ID不幂等", code, string(repeated))
	}
	for _, path := range []string{"/api/artifacts/" + d.ID, "/api/artifacts/" + d.ID + "/" + d.Name} {
		code, out := request(t, api, "GET", path, adminToken, "")
		if code != 404 {
			t.Fatal("未seal文件可见", code, out)
		}
	}
	code, out := request(t, api, "GET", "/api/builds/"+d.Ref.BuildID, adminToken, "")
	if code != 200 || strings.Contains(out, `"reports"`) {
		t.Fatal("未seal公开摘要", code, out)
	}
	sealDigest := sealReportHTTP(t, api, token, d, evidence, 200)
	actor, err := st.Authenticate(context.Background(), adminToken)
	if err != nil {
		t.Fatal(err)
	}
	approver, err := st.CreateToken(context.Background(), actor, "approver")
	if err != nil {
		t.Fatal(err)
	}
	trigger, err := st.CreateToken(context.Background(), actor, "trigger")
	if err != nil {
		t.Fatal(err)
	}
	code, out = request(t, api, "POST", "/api/nodes/worker/disable", adminToken, "")
	if code != 204 {
		t.Fatal(code, out)
	}
	for _, credential := range []string{adminToken, approver.Token} {
		code, out = request(t, api, "GET", "/api/builds/"+d.Ref.BuildID, credential, "")
		var view store.BuildView
		if err = json.Unmarshal([]byte(out), &view); err != nil || code != 200 || view.Reports == nil || !view.Reports.Sealed || view.Reports.Counts.Tests != 1 || view.ReportSealDigest != sealDigest {
			t.Fatal("中央封存摘要不符", code, out, err)
		}
		r, err := http.NewRequest("GET", api.URL+"/api/artifacts/"+d.ID+"/"+d.Name, nil)
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Authorization", "Bearer "+credential)
		response, err := api.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		got, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil || response.StatusCode != 200 || !bytes.Equal(got, data) || response.ContentLength != d.Size || response.Header.Get("X-Content-SHA256") != d.SHA256 {
			t.Fatal("离线中央原XML错误", readErr, response.StatusCode)
		}
		code, out = request(t, api, "GET", "/api/builds/"+d.Ref.BuildID+"/artifacts", credential, "")
		if code != 200 || !strings.Contains(out, d.ID) || strings.Contains(out, "storage_id") || strings.Contains(out, s.config.DataDir) {
			t.Fatal("列表用途或隐私错误", code, out)
		}
	}
	for _, tc := range []struct {
		token string
		code  int
	}{{trigger.Token, 403}, {token, 401}} {
		for _, path := range []string{"/api/builds/" + d.Ref.BuildID, "/api/artifacts/" + d.ID, "/api/artifacts/" + d.ID + "/" + d.Name} {
			code, out = request(t, api, "GET", path, tc.token, "")
			if code != tc.code {
				t.Fatal("证据权限越界", code, out)
			}
		}
	}
	stored, err := st.GetArtifact(context.Background(), actor, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(s.config.DataDir, "artifacts", stored.StorageID), bytes.Repeat([]byte("x"), len(data)), 0600); err != nil {
		t.Fatal(err)
	}
	code, out = request(t, api, "GET", "/api/artifacts/"+d.ID+"/"+d.Name, adminToken, "")
	if code != 500 {
		t.Fatal("坏SHA原XML可下载", code, out)
	}
}

func TestReportHTTPRejectsInvalidXMLAndForgedSummary(t *testing.T) {
	for _, tc := range []struct {
		name, xml string
		want      int
		published bool
	}{
		{"非法XML", `<testsuite>PRIVATE_UNCLOSED`, 400, false},
		{"外部指令", `<!DOCTYPE testsuite><testsuite/>`, 400, false},
		{"伪计数", `<testsuite tests="2"><testcase/><testcase/></testsuite>`, 409, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := []byte(tc.xml)
			s, st, api, token, d, evidence := reportHTTPFixture(t, data, nil)
			code, body := uploadArtifactHTTP(t, api, token, d, data)
			if code != tc.want || strings.Contains(string(body), "PRIVATE_UNCLOSED") {
				t.Fatal("非法原XML或伪摘要未拒绝", code, string(body))
			}
			actor, err := st.Authenticate(context.Background(), adminToken)
			if err != nil {
				t.Fatal(err)
			}
			items, err := st.ListArtifacts(context.Background(), actor, d.Ref.BuildID, store.Page{Limit: 100})
			if err != nil || len(items) != 0 {
				t.Fatal("失败上传公开", items, err)
			}
			sealReportHTTP(t, api, token, d, evidence, 409)
			entries, err := os.ReadDir(filepath.Join(s.config.DataDir, "artifacts"))
			if err != nil {
				t.Fatal(err)
			}
			if (!tc.published && len(entries) != 0) || (tc.published && len(entries) != 1) {
				t.Fatal("解析/短提交边界错误", len(entries))
			}
		})
	}
}
