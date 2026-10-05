package agent

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"path"
	"slices"
	"strings"

	"mybuilds/internal/protocol"
)

// checkpoint仅保存本次实际结论；最终私有路径沿既有Artifacts持久化。
type reportCheckpoint struct {
	Evidence   protocol.ReportEvidence `json:"evidence"`
	Final      bool                    `json:"final"`
	SealDigest string                  `json:"seal_digest,omitempty"`
}

func copyReportEvidence(e protocol.ReportEvidence) protocol.ReportEvidence {
	e.Files = slices.Clone(e.Files)
	e.Diagnostics = slices.Clone(e.Diagnostics)
	return e
}
func sameReportEvidence(a, b protocol.ReportEvidence) bool {
	left, err := json.Marshal(a)
	if err != nil {
		return false
	}
	right, err := json.Marshal(b)
	return err == nil && bytes.Equal(left, right)
}

// prepareReports只消费唯一Run的真实checkpoint，不创建报告步骤或重采源文件。
func (execution *taskExecution) prepareReports(p *protocol.ExecutionProgress) error {
	switch p.Kind {
	case "reports_checked":
		return execution.declareReports(p)
	case "reports_sealed", "build_finished":
		return execution.checkSealed(p)
	default:
		return nil
	}
}
func (execution *taskExecution) declareReports(p *protocol.ExecutionProgress) error {
	journal := execution.journal
	journal.mu.Lock()
	defer journal.mu.Unlock()
	e := p.Reports
	previous := journal.state.Reports
	if e == nil || e.Sealed || e.Revision < 1 || journal.state.Ref == nil || len(e.Files) > 64 || len(e.Files) != len(p.LocalReports) || previous != nil && (previous.Final || e.Revision != previous.Evidence.Revision+1) || previous == nil && e.Revision != 1 {
		return failure("report_invalid")
	}
	final := p.Index == 0
	if final && (p.Phase != "" || p.Name != "" || p.StepKind != "" || e.Outcome == "pending") || !final && (p.Phase != "ordinary" || p.StepKind != "run" || p.Index < 1 || p.Name == "" || e.Outcome != "pending" && e.Outcome != "failed") {
		return failure("report_invalid")
	}
	var total, reportTotal int64
	for _, a := range journal.state.Artifacts {
		total += a.Declaration.Size
	}
	pending := make([]localArtifact, 0, len(e.Files))
	ids := map[string]bool{}
	lastPath := ""
	for i, file := range e.Files {
		local := p.LocalReports[i]
		if local.File != file || !exactUUID(file.ArtifactID) || ids[file.ArtifactID] || file.Path <= lastPath || file.Size < 1 || file.Size > 8<<20 || file.SourceIndex < 1 || file.SourceStep == "" || !fs.ValidPath(local.SnapshotPath) || strings.Contains(local.SnapshotPath, "\\") || path.Clean(local.SnapshotPath) != local.SnapshotPath || !strings.HasPrefix(local.SnapshotPath, "reports/") {
			return failure("report_invalid")
		}
		ids[file.ArtifactID] = true
		lastPath = file.Path
		reportTotal += file.Size
		if reportTotal > 64<<20 {
			return failure("report_invalid")
		}
		if !final {
			continue
		}
		found := false
		for _, a := range journal.state.Artifacts {
			if a.Declaration.ID == file.ArtifactID {
				d := a.Declaration
				if !a.Confirmed || !approvalRefKnown(journal.state, d.Ref) || d.Purpose != "junit" || d.ReportRevision > e.Revision || d.ReportKey != file.Key || d.Size != file.Size || d.SHA256 != file.SHA256 || d.Index != file.SourceIndex || d.Step != file.SourceStep {
					return failure("report_invalid")
				}
				found = true
			}
		}
		if found {
			continue
		}
		total += file.Size
		if len(journal.state.Artifacts)+len(pending) >= 128 || total > 4<<30 {
			return failure("artifact_limit")
		}
		declaration := protocol.ArtifactDeclaration{Ref: *journal.state.Ref, ID: file.ArtifactID, Seq: int64(len(journal.state.Artifacts) + len(pending) + 1), Phase: "ordinary", Index: file.SourceIndex, Step: file.SourceStep, Name: path.Base(file.Path), Size: file.Size, SHA256: file.SHA256, Purpose: "junit", ReportRevision: e.Revision, ReportKey: file.Key}
		pending = append(pending, localArtifact{Declaration: declaration, SnapshotPath: local.SnapshotPath})
	}
	journal.state.Reports = &reportCheckpoint{Evidence: copyReportEvidence(*e), Final: final}
	journal.state.Artifacts = append(journal.state.Artifacts, pending...)
	if p.LocalResultDir != "" {
		journal.state.ResultDir = p.LocalResultDir
	}
	return journal.saveLocked()
}

func (execution *taskExecution) checkSealed(p *protocol.ExecutionProgress) error {
	journal := execution.journal
	journal.mu.Lock()
	defer journal.mu.Unlock()
	checkpoint := journal.state.Reports
	if p.Kind == "build_finished" {
		if checkpoint == nil {
			if p.ReportManifest != nil {
				return failure("report_invalid")
			}
			for _, artifact := range journal.state.Artifacts {
				if artifact.Declaration.Purpose == "junit" {
					return failure("report_invalid")
				}
			}
			return nil
		}
		if !checkpoint.Final || checkpoint.SealDigest == "" || !checkpoint.Evidence.Sealed || checkpoint.Evidence.Files == nil || checkpoint.Evidence.Diagnostics == nil || p.ReportManifest == nil || p.ReportManifest.IDs == nil || p.ReportManifest.SealDigest != checkpoint.SealDigest || len(p.ReportManifest.IDs) != len(checkpoint.Evidence.Files) {
			return failure("report_invalid")
		}
		canonical, err := json.Marshal(checkpoint.Evidence)
		if err != nil {
			return failure("report_invalid")
		}
		hash := sha256.Sum256(canonical)
		if hex.EncodeToString(hash[:]) != checkpoint.SealDigest || !confirmedReportFiles(&journal.state, checkpoint.Evidence) {
			return failure("report_invalid")
		}
		for i, file := range checkpoint.Evidence.Files {
			if p.ReportManifest.IDs[i] != file.ArtifactID {
				return failure("report_invalid")
			}
		}
		return nil
	}
	if checkpoint == nil || !checkpoint.Final || checkpoint.SealDigest != "" || p.Reports == nil || !p.Reports.Sealed || p.Index != 0 || p.Phase != "" || p.Name != "" || p.StepKind != "" {
		return failure("report_invalid")
	}
	expected := copyReportEvidence(checkpoint.Evidence)
	expected.Sealed = true
	if !sameReportEvidence(expected, *p.Reports) {
		return failure("report_invalid")
	}
	if !confirmedReportFiles(&journal.state, expected) {
		return failure("report_invalid")
	}
	data, err := json.Marshal(expected)
	if err != nil {
		return failure("persistence_error")
	}
	digest := sha256.Sum256(data)
	checkpoint.Evidence = expected
	checkpoint.SealDigest = hex.EncodeToString(digest[:])
	return journal.saveLocked()
}

// 封存与终态/只读恢复都核同一份完整声明；Confirmed不能替代归属与元数据。
func confirmedReportFiles(state *journalState, evidence protocol.ReportEvidence) bool {
	if state.Ref == nil || len(evidence.Files) > 64 || len(state.Artifacts) > 128 {
		return false
	}
	seen := map[string]bool{}
	for _, file := range evidence.Files {
		if seen[file.ArtifactID] {
			return false
		}
		seen[file.ArtifactID] = true
		found := false
		for index, artifact := range state.Artifacts {
			d := artifact.Declaration
			if approvalRefKnown(*state, d.Ref) && d.ID == file.ArtifactID && d.Seq == int64(index+1) && artifact.Confirmed && d.Phase == "ordinary" && d.Name == path.Base(file.Path) && d.Purpose == "junit" && d.ReportRevision <= evidence.Revision && d.ReportKey == file.Key && d.Size == file.Size && d.SHA256 == file.SHA256 && d.Index == file.SourceIndex && d.Step == file.SourceStep {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	// 集合外XML只能是先前实际审批封存后被新检查替换的历史文件。
	for index, artifact := range state.Artifacts {
		if artifact.Declaration.Seq != int64(index+1) {
			return false
		}
		if artifact.Declaration.Purpose == "junit" && !seen[artifact.Declaration.ID] && !approvalReportArtifactKnown(state, artifact, evidence.Revision) {
			return false
		}
	}
	return true
}

func approvalReportArtifactKnown(state *journalState, artifact localArtifact, revision int64) bool {
	d := artifact.Declaration
	if !artifact.Confirmed || d.ReportRevision < 1 || d.ReportRevision >= revision || !approvalRefKnown(*state, d.Ref) {
		return false
	}
	matches := func(event protocol.ExecutionEvent) bool {
		p := event.Progress.Approval
		if p == nil || p.Reports == nil || event.Ref != d.Ref || p.Reports.Revision < d.ReportRevision {
			return false
		}
		digest, err := protocol.ApprovalCheckpointDigest(event.Ref, event.Seq, event.Progress)
		encoded, marshalErr := json.Marshal(event.Progress)
		if err != nil || marshalErr != nil || digest != p.CheckpointDigest || journalProgressDigest(encoded) != event.Digest {
			return false
		}
		for _, file := range p.Reports.Files {
			if d.ID == file.ArtifactID && d.Phase == "ordinary" && d.Name == path.Base(file.Path) && d.ReportKey == file.Key && d.Size == file.Size && d.SHA256 == file.SHA256 && d.Index == file.SourceIndex && d.Step == file.SourceStep {
				return true
			}
		}
		return false
	}
	for _, proof := range state.ApprovalHistory {
		if matches(proof.Event) {
			return true
		}
	}
	return false
}
