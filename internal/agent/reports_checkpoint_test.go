//go:build darwin || linux

package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"mybuilds/internal/protocol"
)

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
	return &taskExecution{journal: journal}, protocol.ExecutionProgress{Kind: "reports_checked", Phase: "ordinary", Index: 1, Name: "tests", StepKind: "run", ExitCode: -1, Reports: &evidence, LocalReports: []protocol.CollectedReport{{File: file, SnapshotPath: snapshot}}, LocalResultDir: dir}
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
