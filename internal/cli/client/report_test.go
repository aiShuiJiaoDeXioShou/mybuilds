package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"mybuilds/internal/server"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/pipeline"
	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
)

func TestReportLargeBuildResponseBudget(t *testing.T) {
	e := &protocol.ReportEvidence{Revision: 1, Sealed: true, Outcome: "passed", Required: true, Counts: protocol.JUnitCounts{Tests: 1024}, Files: []protocol.ReportFile{}, Diagnostics: []protocol.JUnitDiagnostic{}}
	for i := 0; i < 1024; i++ {
		e.Files = append(e.Files, protocol.ReportFile{Path: strings.Repeat("x/", 500) + fmt.Sprintf("%04d.xml", i), ArtifactID: uuid.NewString(), SourceStep: "tests", SourceIndex: 1, Counts: protocol.JUnitCounts{Tests: 1}})
	}
	body, err := json.Marshal(store.BuildView{Reports: e})
	if err != nil || len(body) <= config.MaxConfigBytes {
		t.Fatal("未形成大报告响应", len(body), err)
	}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	defer api.Close()
	cfg := config.ClientConfig{Server: api.URL, RuntimeToken: strings.Repeat("t", 32), Timeout: time.Second}
	for _, path := range []string{"/api/builds/fixture", "/api/approvals/fixture"} {
		var result store.BuildView
		if err := requestJSON(context.Background(), cfg, http.MethodGet, path, nil, &result, ""); err != nil || result.Reports == nil || len(result.Reports.Files) != 1024 {
			t.Fatal("完整大响应未读取", err)
		}
	}
	body = []byte(strings.Repeat(" ", protocol.MaxReportMessageBytes+1))
	if err := requestJSON(context.Background(), cfg, http.MethodGet, "/api/builds/fixture", nil, &store.BuildView{}, ""); err == nil {
		t.Fatal("超响应预算未拒绝")
	}
}

func TestReportsLocalCLIJSONKeepsOriginalFailureAndSnapshot(t *testing.T) {
	requireLocalShell(t)
	t.Chdir(t.TempDir())
	xml := `<testsuite tests="1" failures="1"><testcase name="failed" time="0.123456789"><failure message="safe mismatch"/></testcase></testsuite>`
	writeConfig(t, "mybuilds.yml", `version: 1
reports: {junit: {paths: ['results/*.xml']}}
steps:
 - kind: run
   name: tests
   run: |
     mkdir -p results
     printf '%s' '`+xml+`' > results/result.xml
     exit 7
 - {kind: run, name: must-not-run, run: 'touch unexpected'}
post:
 failure:
  - kind: run
    name: rewrite
    run: printf '<testsuite/>' > results/result.xml
`)
	out, logs, err := localExecute(t, context.Background(), "run", "--file", "mybuilds.yml")
	if err == nil {
		t.Fatal("exit7未失败")
	}
	var result pipeline.RunResult
	if err = json.Unmarshal([]byte(out), &result); err != nil || len(result.Builds) != 1 {
		t.Fatal("无真实本地结果", err, out, logs)
	}
	build := result.Builds[0]
	if build.Status != "failed" || build.Reason != "exit" || build.Steps[0].ExitCode != 7 || build.Reports == nil || !build.Reports.Sealed || build.Reports.Counts.Tests != 1 || build.Reports.Counts.Failures != 1 || build.Reports.Counts.DurationNS != 123456789 || build.Reports.Reason != "report_failed" || len(build.Reports.Files) != 1 || build.ReportSealDigest == "" {
		t.Fatal("CLI丢报告或覆盖原失败", out, logs)
	}
	if strings.Contains(out, "snapshot_path") || strings.Contains(out, "local_reports") || strings.Contains(out, "<testsuite") {
		t.Fatal("报告JSON泄漏私有快照或原XML", out)
	}
	file := build.Reports.Files[0]
	sum := sha256.Sum256([]byte(xml))
	if file.Size != int64(len(xml)) || file.SHA256 != hex.EncodeToString(sum[:]) || file.SourceIndex != 1 || file.SourceStep != "tests" {
		t.Fatal("原XML元数据失真", file)
	}
	found := false
	err = filepath.WalkDir(result.ResultDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Name() == file.ArtifactID+".xml" {
			actual, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if !bytes.Equal(actual, []byte(xml)) {
				t.Fatal("本地快照非原XML")
			}
			found = true
		}
		return nil
	})
	if err != nil || !found {
		t.Fatal("无真实独立XML快照", err)
	}
	current, err := os.ReadFile("results/result.xml")
	if err != nil || string(current) != "<testsuite/>" {
		t.Fatal("真实post没有改写工作树", err)
	}
	if _, err = os.Stat("unexpected"); !os.IsNotExist(err) {
		t.Fatal("失败后启动下一ordinary")
	}
}

// 实际Store领取/run/check/final、真实HTTP stage解析与seal，再由原CLI读取。
func realReportRemoteCLI(t *testing.T) (*store.Store, string, protocol.ArtifactDeclaration, []byte) {
	t.Helper()
	ctx := context.Background()
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(ctx, store.Options{Driver: "sqlite", DSN: filepath.Join(directory, "control.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err = st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	created, err := st.CreateToken(ctx, store.Actor{ID: "local-admin", Role: "admin"}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("MYBUILDS_CLIENT_TOKEN", created.Token)
	api := httptest.NewServer(server.New(st, config.ServerConfig{DataDir: directory, Concurrency: 1, HeartbeatInterval: 5 * time.Second, LeaseDuration: 30 * time.Second}).Handler())
	t.Cleanup(api.Close)
	address := api.URL
	admin, err := st.Authenticate(ctx, os.Getenv("MYBUILDS_CLIENT_TOKEN"))
	if err != nil {
		t.Fatal(err)
	}
	node, err := st.CreateNode(ctx, admin, store.NodeInput{Name: "report-worker", Capacity: 1})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := st.AuthenticateNode(ctx, node.Token)
	if err != nil {
		t.Fatal(err)
	}
	policy := store.LeasePolicy{Concurrency: 1, Heartbeat: 5 * time.Second, Duration: 30 * time.Second}
	session, err := st.OpenNodeSession(ctx, actor, protocol.SessionRequest{SessionID: uuid.NewString(), HeartbeatNS: int64(policy.Heartbeat), LeaseNS: int64(policy.Duration), Report: protocol.NodeReport{OS: "linux", Arch: "amd64", Capacity: 1, Tools: []protocol.ToolCheck{{Name: "shell", Status: "passed"}, {Name: "git", Status: "passed"}, {Name: "node_journal", Status: "passed"}}}}, policy)
	if err != nil {
		t.Fatal(err)
	}
	project, err := st.CreateProject(ctx, admin, store.ProjectInput{Name: "report-app", Repository: "https://example.org/repo.git", AllowedNodes: []string{"report-worker"}, DefaultNode: "report-worker"})
	if err != nil {
		t.Fatal(err)
	}
	definition := config.Build{Steps: []config.Step{{Name: "tests", Kind: "run", Run: "PRIVATE_REPORT_SCRIPT"}}, Reports: &config.Reports{JUnit: &config.JUnitReport{Paths: []string{"results/*.xml"}}}}
	_, err = st.Enqueue(ctx, store.EnqueueInput{Actor: admin, ProjectID: project.ID, ProjectVersion: project.PolicyVersion, Key: "report", RequestDigest: strings.Repeat("a", 64), SHA: strings.Repeat("b", 40), Branch: "main", Source: "repo", File: "mybuilds.yml", SourceDigest: strings.Repeat("c", 64), Builds: []store.PreparedBuild{{Name: "tests", Status: "queued", PostBudgetNS: int64(2 * time.Minute), Snapshot: store.BuildSnapshot{Definition: definition, Params: map[string]string{}, Condition: "ready", AllowedNodes: project.AllowedNodes, DefaultNode: project.DefaultNode}, Steps: []store.StepProgress{{Phase: "ordinary", Index: 1, Name: "tests", Kind: "run", Condition: "ready", Status: "pending"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := st.Claim(ctx, actor, protocol.ClaimRequest{SessionID: session.SessionID, ClaimKey: uuid.NewString()}, policy)
	if err != nil || grant == nil {
		t.Fatal(err)
	}
	apply := func(seq int64, p protocol.ExecutionProgress) {
		t.Helper()
		p.At = time.Now().UTC()
		p.RemainingPostBudgetNS = int64(2 * time.Minute)
		p.ArtifactSteps = []protocol.ArtifactExpectation{}
		raw, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(raw)
		_, err = st.ApplyEvent(ctx, actor, protocol.ExecutionEvent{Ref: grant.Ref, Seq: seq, Digest: hex.EncodeToString(hash[:]), Progress: p})
		if err != nil {
			t.Fatal("真实事件", p.Kind, err)
		}
	}
	for i, kind := range []string{"intent", "started", "finished"} {
		p := protocol.ExecutionProgress{Kind: kind, Phase: "ordinary", Name: "tests", StepKind: "run", Index: 1, ExitCode: -1}
		if kind != "intent" {
			p.Started = true
		}
		if kind == "finished" {
			p.Status = "succeeded"
			p.StopConfirmed = true
			p.ExitCode = 0
		}
		apply(int64(i+1), p)
	}
	data := []byte(`<testsuite tests="1"><testcase name="actual"/></testsuite>`)
	sum := sha256.Sum256(data)
	key := sha256.Sum256([]byte("results/result.xml"))
	d := protocol.ArtifactDeclaration{Ref: grant.Ref, ID: uuid.NewString(), Seq: 1, Phase: "ordinary", Index: 1, Step: "tests", Name: "result.xml", Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:]), Purpose: "junit", ReportRevision: 2, ReportKey: hex.EncodeToString(key[:])}
	counts := protocol.JUnitCounts{Tests: 1}
	evidence := protocol.ReportEvidence{Revision: 1, Outcome: "pending", Required: true, Counts: counts, Diagnostics: []protocol.JUnitDiagnostic{}, Files: []protocol.ReportFile{{Key: d.ReportKey, Path: "results/result.xml", ArtifactID: d.ID, SourceIndex: 1, SourceStep: "tests", Size: d.Size, SHA256: d.SHA256, Counts: counts}}}
	apply(4, protocol.ExecutionProgress{Kind: "reports_checked", Phase: "ordinary", Index: 1, Name: "tests", StepKind: "run", ExitCode: -1, Reports: &evidence})
	evidence.Revision = 2
	evidence.Outcome = "passed"
	apply(5, protocol.ExecutionProgress{Kind: "reports_checked", ExitCode: -1, Reports: &evidence})
	out, err := executeRemote(t, "--server-url", address, "build", "show", d.Ref.BuildID, "--json")
	if err != nil || strings.Contains(out, `"reports"`) {
		t.Fatal("未seal报告被CLI读取", err, out)
	}
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	r, err := http.NewRequest("PUT", address+"/api/agent/artifacts/"+d.ID, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Authorization", "Bearer "+node.Token)
	r.Header.Set("X-Mybuilds-Artifact", base64.RawURLEncoding.EncodeToString(raw))
	response, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 {
		t.Fatal("真实报告上传", err, response.StatusCode, string(body))
	}
	evidence.Sealed = true
	apply(6, protocol.ExecutionProgress{Kind: "reports_sealed", ExitCode: -1, Reports: &evidence})
	return st, address, d, data
}

func TestReportsRemoteCLIShowsSealedSummaryAndPurpose(t *testing.T) {
	_, address, d, _ := realReportRemoteCLI(t)
	out, err := executeRemote(t, "--server-url", address, "build", "show", d.Ref.BuildID, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var view store.BuildView
	if err = json.Unmarshal([]byte(out), &view); err != nil || view.Reports == nil || !view.Reports.Sealed || view.Reports.Counts.Tests != 1 || view.ReportSealDigest == "" {
		t.Fatal("CLI丢sealed报告", err, out)
	}
	out, err = executeRemote(t, "--server-url", address, "build", "show", d.Ref.BuildID)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"report_tests", "report_duration_ns", "report_outcome", "report_sealed", "report_seal_digest", d.ID, d.SHA256} {
		if !strings.Contains(out, key) {
			t.Errorf("真实build详情缺%s：%s", key, out)
		}
	}
	out, err = executeRemote(t, "--server-url", address, "artifact", "ls", d.Ref.BuildID)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"PURPOSE", "junit", "REPORT_REVISION", "REPORT_KEY", d.ID, d.ReportKey} {
		if !strings.Contains(out, key) {
			t.Errorf("真实artifact列表缺%s：%s", key, out)
		}
	}
	if strings.Contains(out, "PRIVATE_REPORT_SCRIPT") || strings.Contains(out, "snapshot_path") {
		t.Fatal("列表泄露原定义或私有路径")
	}
}
