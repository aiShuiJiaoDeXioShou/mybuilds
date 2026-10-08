//go:build darwin || linux

package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
)

func TestReportLargeJournalPersistenceAndStartupParsing(t *testing.T) {
	execution, p := reportJournalFixture(t)
	e := copyReportEvidence(*p.Reports)
	e.Files = []protocol.ReportFile{}
	for i := 0; i < config.MaximumJUnitMaxFiles; i++ {
		f := p.Reports.Files[0]
		f.Path = strings.Repeat(strings.Repeat(">", 200)+"/", 4) + strings.Repeat(">", 212) + fmt.Sprintf("%04d.xml", i)
		key := sha256.Sum256([]byte(f.Path))
		f.Key = hex.EncodeToString(key[:])
		f.ArtifactID = uuid.NewString()
		e.Files = append(e.Files, f)
	}
	e.Counts.Tests = int64(len(e.Files))
	j := execution.journal
	j.state.Reports = &reportCheckpoint{Evidence: e}
	if err := j.save(); err != nil {
		t.Fatal(err)
	}
	data, _, err := j.lock.readJournalFile(j.name)
	if err != nil || len(data) <= 1<<20 {
		t.Fatal("大journal写读失败", len(data), err)
	}
	nodes := 0
	if err := journalJSONValue(json.NewDecoder(bytes.NewReader(data)), 0, &nodes, maxJournalNodes); err != nil || nodes <= 10000 {
		t.Fatal("节点容量未更新", nodes, err)
	}
	if err := closeRecoveredIOSResources(context.Background(), j.lock); err != nil {
		t.Fatal("非iOS大journal启动扫描失败", err)
	}
	before := j.info
	j.state.ResultDir = strings.Repeat("x", maxJournalBytes)
	if err := j.save(); err == nil {
		t.Fatal("journal超限未拒绝")
	}
	after, err := j.lock.root.Stat("journal/" + j.name)
	if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() {
		t.Fatal("超限写入破坏旧journal")
	}
}

// 真实私有锁/原子journal核验本地接点，中央成功仍由HTTP整链测试证明。
func reportJournalFixture(t *testing.T) (*taskExecution, protocol.ExecutionProgress) {
	t.Helper()
	lock, err := lockDataDir(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { lock.Close() })
	if err = lock.prepareJournal(); err != nil {
		t.Fatal(err)
	}
	journal, err := newJournal(lock, uuid.NewString(), uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	ref := protocol.LeaseRef{NodeID: uuid.NewString(), SessionID: journal.state.SessionID, BuildID: uuid.NewString(), AttemptID: uuid.NewString(), LeaseID: uuid.NewString(), Epoch: 1}
	if err = journal.setLease(ref, nil, 1000000000); err != nil {
		t.Fatal(err)
	}
	result, err := lock.resultParent()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(result, "mybuilds-")
	if err != nil {
		t.Fatal(err)
	}
	snapshot := "reports/" + uuid.NewString() + "/" + uuid.NewString() + ".xml"
	data := []byte(`<testsuite><testcase/></testsuite>`)
	if err = os.MkdirAll(filepath.Dir(filepath.Join(dir, snapshot)), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, snapshot), data, 0600); err != nil {
		t.Fatal(err)
	}
	key := sha256.Sum256([]byte("result.xml"))
	hash := sha256.Sum256(data)
	file := protocol.ReportFile{Key: hex.EncodeToString(key[:]), Path: "result.xml", ArtifactID: uuid.NewString(), SourceIndex: 1, SourceStep: "tests", Size: int64(len(data)), SHA256: hex.EncodeToString(hash[:]), Counts: protocol.JUnitCounts{Tests: 1}}
	evidence := protocol.ReportEvidence{Revision: 1, Outcome: "pending", Required: true, Counts: file.Counts, Diagnostics: []protocol.JUnitDiagnostic{}, Files: []protocol.ReportFile{file}}
	return &taskExecution{journal: journal, task: &protocol.TaskSnapshot{Definition: config.Build{Reports: &config.Reports{JUnit: &config.JUnitReport{Paths: []string{"result.xml"}}}}}}, protocol.ExecutionProgress{Kind: "reports_checked", Phase: "ordinary", Index: 1, Name: "tests", StepKind: "run", ExitCode: -1, Reports: &evidence, LocalReports: []protocol.CollectedReport{{File: file, SnapshotPath: snapshot}}, LocalResultDir: dir}
}

func TestReportCheckpointPrivateAndFinalOnlyDeclaration(t *testing.T) {
	execution, p := reportJournalFixture(t)
	if err := execution.prepareReports(&p); err != nil {
		t.Fatal(err)
	}
	if len(execution.journal.state.Artifacts) != 0 || execution.journal.state.Reports == nil {
		t.Fatal("非final不能声明XML")
	}
	data, err := os.ReadFile(filepath.Join(execution.journal.lock.root.Name(), "journal", execution.journal.name))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), p.LocalReports[0].SnapshotPath) {
		t.Fatal("未最终上传不能新增无消费者私有路径")
	}
	// caller的可变slice不应改写已经保存的checkpoint。
	p.Reports.Files[0].SourceIndex = 2
	if execution.journal.state.Reports.Evidence.Files[0].SourceIndex != 1 {
		t.Fatal("checkpoint共享了caller slice")
	}
	p.Reports.Files[0].SourceIndex = 1
	p.Reports.Revision = 2
	p.Reports.Outcome = "passed"
	p.Phase = ""
	p.Index = 0
	p.Name = ""
	p.StepKind = ""
	if err = execution.prepareReports(&p); err != nil {
		t.Fatal(err)
	}
	artifacts := execution.journal.state.Artifacts
	if len(artifacts) != 1 || artifacts[0].Declaration.ID != p.Reports.Files[0].ArtifactID || artifacts[0].Declaration.Seq != 1 || artifacts[0].Declaration.Purpose != "junit" || artifacts[0].Declaration.ReportRevision != 2 || len(execution.journal.state.ArtifactSteps) != 0 {
		t.Fatal("最终声明应沿原cursor且不伪造artifactStep")
	}
	event, err := execution.journal.prepareEvent(p)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if len(event.Progress.LocalReports) != 0 || strings.Contains(string(wire), p.LocalReports[0].SnapshotPath) {
		t.Fatal("网络摘要前未清私有LocalReports")
	}
}

func TestReportCheckpointRejectsIncompleteSealAndFalseTerminal(t *testing.T) {
	execution, p := reportJournalFixture(t)
	p.Reports.Outcome = "passed"
	p.Phase = ""
	p.Index = 0
	p.Name = ""
	p.StepKind = ""
	if err := execution.prepareReports(&p); err != nil {
		t.Fatal(err)
	}
	seal := copyReportEvidence(*p.Reports)
	seal.Sealed = true
	if err := execution.prepareReports(&protocol.ExecutionProgress{Kind: "reports_sealed", ExitCode: -1, Reports: &seal}); err == nil {
		t.Fatal("未上传确认XML仍可seal")
	}
	if execution.journal.state.Reports.SealDigest != "" {
		t.Fatal("未确认产生可信seal")
	}
	if err := execution.prepareReports(&protocol.ExecutionProgress{Kind: "build_finished", ReportManifest: &protocol.ReportManifest{SealDigest: strings.Repeat("a", 64), IDs: []string{seal.Files[0].ArtifactID}}}); err == nil {
		t.Fatal("未封存报告伪造终态")
	}
}

func TestReportCheckpointRejectsForgedConfirmedMetadata(t *testing.T) {
	for _, field := range []string{"phase", "ref", "name", "purpose", "revision", "key"} {
		t.Run(field, func(t *testing.T) {
			execution, p := reportJournalFixture(t)
			p.Reports.Outcome = "passed"
			p.Phase = ""
			p.Index = 0
			p.Name = ""
			p.StepKind = ""
			if err := execution.prepareReports(&p); err != nil {
				t.Fatal(err)
			}
			a := &execution.journal.state.Artifacts[0]
			// 模拟已持久本地记录损坏；不得仅信任Confirmed布尔或同一SHA。
			a.Confirmed = true
			switch field {
			case "phase":
				a.Declaration.Phase = "always"
			case "ref":
				a.Declaration.Ref.AttemptID = uuid.NewString()
			case "name":
				a.Declaration.Name = "other.xml"
			case "purpose":
				a.Declaration.Purpose = ""
			case "revision":
				a.Declaration.ReportRevision++
			case "key":
				a.Declaration.ReportKey = strings.Repeat("b", 64)
			}
			if err := execution.journal.save(); err != nil {
				t.Fatal(err)
			}
			seal := copyReportEvidence(*p.Reports)
			seal.Sealed = true
			if err := execution.prepareReports(&protocol.ExecutionProgress{Kind: "reports_sealed", ExitCode: -1, Reports: &seal}); err == nil {
				t.Fatal("损坏的完整身份仍可seal")
			}
		})
	}
}

func TestReportCheckpointTerminalRequiresCanonicalAndConfirmedMetadata(t *testing.T) {
	for _, field := range []string{"counts", "file", "declaration", "missing_checkpoint", "extra_junit"} {
		t.Run(field, func(t *testing.T) {
			execution, p := reportJournalFixture(t)
			p.Reports.Outcome = "passed"
			p.Phase = ""
			p.Index = 0
			p.Name = ""
			p.StepKind = ""
			if err := execution.prepareReports(&p); err != nil {
				t.Fatal(err)
			}
			// 这里只检验本地记录损坏后的拒绝；中央确认正例由真实HTTP整链证明。
			execution.journal.state.Artifacts[0].Confirmed = true
			seal := copyReportEvidence(*p.Reports)
			seal.Sealed = true
			if err := execution.prepareReports(&protocol.ExecutionProgress{Kind: "reports_sealed", ExitCode: -1, Reports: &seal}); err != nil {
				t.Fatal(err)
			}
			checkpoint := execution.journal.state.Reports
			terminal := protocol.ExecutionProgress{Kind: "build_finished", ReportManifest: &protocol.ReportManifest{SealDigest: checkpoint.SealDigest, IDs: []string{seal.Files[0].ArtifactID}}}
			switch field {
			case "counts":
				checkpoint.Evidence.Counts.Tests++
			case "file":
				checkpoint.Evidence.Files[0].Size++
			case "declaration":
				execution.journal.state.Artifacts[0].Declaration.Size++
			case "missing_checkpoint":
				execution.journal.state.Reports = nil
				terminal.ReportManifest = nil
			case "extra_junit":
				a := execution.journal.state.Artifacts[0]
				a.Declaration.ID = uuid.NewString()
				a.Declaration.Seq++
				execution.journal.state.Artifacts = append(execution.journal.state.Artifacts, a)
			}
			if err := execution.journal.save(); err != nil {
				t.Fatal(err)
			}
			if err := execution.prepareReports(&terminal); err == nil {
				t.Fatal("损坏的checkpoint不能成为终态/只读恢复停止证据")
			}
		})
	}
}

// 纯私有元数据校验：历史XML必须由原审批摘要及完整Ref链证明，不能仅凭低revision放行。
func TestApprovalReportHistoryOnlyAcceptsCheckpointFiles(t *testing.T) {
	e, p := reportJournalFixture(t)
	p.Reports.Outcome, p.Phase, p.Index, p.Name, p.StepKind = "passed", "", 0, "", ""
	if err := e.prepareReports(&p); err != nil {
		t.Fatal(err)
	}
	state := &e.journal.state
	state.Artifacts[0].Confirmed = true
	old := *state.Ref
	progress := protocol.ExecutionProgress{Kind: "approval_checkpoint", Phase: "ordinary", Index: 2, Name: "review", StepKind: "approval", PostPhase: "none", StopConfirmed: true, ExitCode: -1, At: time.Now().UTC(), ArtifactSteps: []protocol.ArtifactExpectation{}, Approval: &protocol.ApprovalCheckpointEvidence{ID: uuid.NewString(), Revision: 1, SnapshotDigest: strings.Repeat("a", 64), WorkspaceID: uuid.NewString(), ResultID: uuid.NewString(), NextOrdinaryIndex: 3, Artifacts: []protocol.ArtifactExpectation{}, PublishIntents: []protocol.PublishExpectation{}, SystemResourcesClosed: true, Reports: p.Reports}}
	digest, err := protocol.ApprovalCheckpointDigest(old, 5, progress)
	if err != nil {
		t.Fatal(err)
	}
	progress.Approval.CheckpointDigest = digest
	encoded, _ := json.Marshal(progress)
	next := old
	next.Epoch++
	next.SessionID, next.LeaseID = uuid.NewString(), uuid.NewString()
	state.Ref = &next
	state.ApprovalHistory = []approvalRefTransition{{Event: protocol.ExecutionEvent{Ref: old, Seq: 5, Progress: progress, Digest: journalProgressDigest(encoded)}, ResumeRef: next}}
	file := p.Reports.Files[0]
	file.ArtifactID = uuid.NewString()
	evidence := copyReportEvidence(*p.Reports)
	evidence.Revision = 2
	evidence.Files = []protocol.ReportFile{file}
	current := state.Artifacts[0]
	current.Declaration.ID, current.Declaration.Seq, current.Declaration.Ref, current.Declaration.ReportRevision = file.ArtifactID, 2, next, 2
	state.Artifacts = append(state.Artifacts, current)
	if !confirmedReportFiles(state, evidence) {
		t.Fatal("精确原审批XML历史被拒绝")
	}
	state.Artifacts[0].Declaration.Size++
	if confirmedReportFiles(state, evidence) {
		t.Fatal("历史XML元数据篡改被接受")
	}
	state.Artifacts[0].Declaration.Size--
	state.ApprovalHistory = nil
	if confirmedReportFiles(state, evidence) {
		t.Fatal("无审批链的集合外XML被接受")
	}
}
