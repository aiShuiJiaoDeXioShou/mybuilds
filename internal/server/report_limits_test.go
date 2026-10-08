package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
)

func TestReportMaximumLongPathsHTTPAndApprovalDigest(t *testing.T) {
	limit := config.MaximumJUnitMaxFiles
	definition := config.Build{Steps: []config.Step{{Name: "tests", Kind: "run", Run: "printf test"}}, Reports: &config.Reports{JUnit: &config.JUnitReport{Paths: []string{"**/*.xml"}, MaxFiles: &limit}}}
	_, _, api, token, claim := executionHTTPBuildFixture(t, definition)
	g := claimHTTP(t, api, token, claim)
	for i, kind := range []string{"intent", "started", "finished"} {
		p := protocol.ExecutionProgress{Kind: kind, Phase: "ordinary", Index: 1, Name: "tests", StepKind: "run", At: time.Now().UTC(), ExitCode: -1, RemainingPostBudgetNS: g.RemainingPostBudgetNS, ArtifactSteps: []protocol.ArtifactExpectation{}}
		if kind != "intent" {
			p.Started = true
		}
		if kind == "finished" {
			p.Status = "succeeded"
			p.StopConfirmed = true
			p.ExitCode = 0
		}
		postReportEvent(t, api, token, g.Ref, int64(i+1), p, 200)
	}
	e := protocol.ReportEvidence{Revision: 1, Outcome: "pending", Required: true, Counts: protocol.JUnitCounts{Tests: int64(limit)}, Files: []protocol.ReportFile{}, Diagnostics: []protocol.JUnitDiagnostic{}}
	for i := 0; i < limit; i++ {
		name := strings.Repeat(strings.Repeat(">", 200)+"/", 4) + strings.Repeat(">", 212) + fmt.Sprintf("%04d.xml", i)
		key := sha256.Sum256([]byte(name))
		e.Files = append(e.Files, protocol.ReportFile{Path: name, Key: hex.EncodeToString(key[:]), ArtifactID: uuid.NewString(), SourceIndex: 1, SourceStep: "tests", Size: 1, SHA256: strings.Repeat("a", 64), Counts: protocol.JUnitCounts{Tests: 1}})
	}
	p := protocol.ExecutionProgress{Kind: "reports_checked", Phase: "ordinary", Index: 1, Name: "tests", StepKind: "run", At: time.Now().UTC(), ExitCode: -1, RemainingPostBudgetNS: g.RemainingPostBudgetNS, ArtifactSteps: []protocol.ArtifactExpectation{}, Reports: &e}
	wire, err := json.Marshal(p)
	if err != nil || len(wire) <= 1<<20 || len(wire) >= protocol.MaxReportMessageBytes {
		t.Fatal("未形成合法大消息", len(wire), err)
	}
	postReportEvent(t, api, token, g.Ref, 4, p, 200)
	p.Kind = "approval_checkpoint"
	p.Index = 2
	p.Name = "review"
	p.StepKind = "approval"
	p.PostPhase = "none"
	p.StopConfirmed = true
	p.Reports = nil
	p.Approval = &protocol.ApprovalCheckpointEvidence{ID: uuid.NewString(), Revision: 1, SnapshotDigest: strings.Repeat("b", 64), WorkspaceID: uuid.NewString(), ResultID: uuid.NewString(), NextOrdinaryIndex: 3, Artifacts: []protocol.ArtifactExpectation{}, PublishIntents: []protocol.PublishExpectation{}, Reports: &e, SystemResourcesClosed: true}
	if _, err := protocol.ApprovalCheckpointDigest(g.Ref, 5, p); err != nil {
		t.Fatal("大报告审批摘要失败", err)
	}
	code, _ := request(t, api, "POST", "/api/agent/events", token, strings.Repeat(" ", protocol.MaxReportMessageBytes+1))
	if code != 413 {
		t.Fatal("大于8MiB请求未拒绝", code)
	}
}
