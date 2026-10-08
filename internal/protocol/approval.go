package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"regexp"
)

// 审批证据只有实际进度与中性ID，节点私有目录和材料不进入wire。
type ApprovalCheckpointEvidence struct {
	ID                    string                `json:"id"`
	Revision              int64                 `json:"revision"`
	SnapshotDigest        string                `json:"snapshot_digest"`
	WorkspaceID           string                `json:"workspace_id"`
	ResultID              string                `json:"result_id"`
	NextOrdinaryIndex     int                   `json:"next_ordinary_index"`
	CheckpointDigest      string                `json:"checkpoint_digest"`
	Artifacts             []ArtifactExpectation `json:"artifacts"`
	Reports               *ReportEvidence       `json:"reports,omitempty"`
	ReportManifest        *ReportManifest       `json:"report_manifest,omitempty"`
	PublishIntents        []PublishExpectation  `json:"publish_intents"`
	SystemResourcesClosed bool                  `json:"system_resources_closed"`
	ResourceID            string                `json:"resource_id,omitempty"`
	OwnershipDigest       string                `json:"ownership_digest,omitempty"`
	IOSResourceDigest     string                `json:"ios_resource_digest,omitempty"`
}
type ApprovalCheckpointLookup struct {
	Ref              LeaseRef `json:"ref"`
	ApprovalID       string   `json:"approval_id"`
	Revision         int64    `json:"revision"`
	Seq              int64    `json:"seq"`
	Digest           string   `json:"digest"`
	CheckpointDigest string   `json:"checkpoint_digest"`
}
type ApprovalCheckpointReceipt struct {
	Ref                   LeaseRef `json:"ref"`
	ApprovalID            string   `json:"approval_id"`
	Revision              int64    `json:"revision"`
	Seq                   int64    `json:"seq"`
	Digest                string   `json:"digest"`
	CheckpointDigest      string   `json:"checkpoint_digest"`
	State                 string   `json:"state"`
	NodeName              string   `json:"node_name"`
	StopConfirmed         bool     `json:"stop_confirmed"`
	SystemResourcesClosed bool     `json:"system_resources_closed"`
}

// 原步骤ledger只保存已确认事实，不携带脚本/参数或私有日志路径。
type ApprovalStepLedger struct {
	Phase         string   `json:"phase"`
	Index         int      `json:"index"`
	Name          string   `json:"name"`
	Kind          string   `json:"kind"`
	Condition     string   `json:"condition"`
	Status        string   `json:"status"`
	Reasons       []string `json:"reasons"`
	ElapsedNS     int64    `json:"elapsed_ns"`
	Intent        bool     `json:"intent"`
	Started       bool     `json:"started"`
	StopConfirmed bool     `json:"stop_confirmed"`
	CleanupFailed bool     `json:"cleanup_failed"`
	Reason        string   `json:"reason,omitempty"`
	ExitCode      int      `json:"exit_code"`
}
type ApprovalResumeEvidence struct {
	IOSResourceDigest string                `json:"ios_resource_digest,omitempty"`
	ResourceID        string                `json:"resource_id,omitempty"`
	OwnershipDigest   string                `json:"ownership_digest,omitempty"`
	ApprovalID        string                `json:"approval_id"`
	Revision          int64                 `json:"revision"`
	CheckpointDigest  string                `json:"checkpoint_digest"`
	CheckpointRef     LeaseRef              `json:"checkpoint_ref"`
	SnapshotDigest    string                `json:"snapshot_digest"`
	WorkspaceID       string                `json:"workspace_id"`
	ResultID          string                `json:"result_id"`
	NextOrdinaryIndex int                   `json:"next_ordinary_index"`
	Steps             []ApprovalStepLedger  `json:"steps"`
	EventSeq          int64                 `json:"event_seq"`
	LastLogSeq        int64                 `json:"last_log_seq"`
	LastLogOffset     int64                 `json:"last_log_offset"`
	LastArtifactSeq   int64                 `json:"last_artifact_seq"`
	Artifacts         []ArtifactExpectation `json:"artifacts"`
	Reports           *ReportEvidence       `json:"reports,omitempty"`
	ReportManifest    *ReportManifest       `json:"report_manifest,omitempty"`
	PublishIntents    []PublishExpectation  `json:"publish_intents"`
}

var approvalHex = regexp.MustCompile(`^[a-f0-9]{64}$`)

func approvalUUID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.String() == value
}
func approvalShape(ref LeaseRef, seq int64, p ExecutionProgress) bool {
	if seq < 1 || ref.Epoch < 1 || !approvalUUID(ref.NodeID) || !approvalUUID(ref.SessionID) || !approvalUUID(ref.BuildID) || !approvalUUID(ref.AttemptID) || !approvalUUID(ref.LeaseID) {
		return false
	}
	a := p.Approval
	if a == nil || !approvalUUID(a.ID) || a.Revision < 1 || !approvalHex.MatchString(a.SnapshotDigest) || !approvalUUID(a.WorkspaceID) || !approvalUUID(a.ResultID) || a.Artifacts == nil || a.PublishIntents == nil || !a.SystemResourcesClosed {
		return false
	}
	_, offset := p.At.Zone()
	if p.At.IsZero() || offset != 0 || p.Kind != "approval_checkpoint" || p.Phase != "ordinary" || p.StepKind != "approval" || p.Index < 1 || a.NextOrdinaryIndex != p.Index+1 || p.Name == "" || p.Status != "" || p.Reason != "" || p.PostPhase != "none" || p.Started || !p.StopConfirmed || p.CleanupFailed || p.ExitCode != -1 || p.ElapsedNS < 0 || p.RemainingPostBudgetNS < 0 || p.RemainingBudgetNS != nil && *p.RemainingBudgetNS < 0 || p.ArtifactSteps == nil || p.LastLogSeq < 0 || p.LastLogOffset < 0 || p.LastArtifactSeq < 0 {
		return false
	}
	if a.Reports != nil && (a.Reports.Files == nil || a.Reports.Diagnostics == nil || a.Reports.Revision < 1) {
		return false
	}
	if a.ReportManifest != nil && a.ReportManifest.IDs == nil {
		return false
	}
	return true
}

// Checkpoint摘要绑定旧完整Ref、原seq、预算与游标；自身摘要字段不参与循环。
func ApprovalCheckpointDigest(ref LeaseRef, seq int64, p ExecutionProgress) (string, error) {
	if !approvalShape(ref, seq, p) {
		return "", errors.New("approval_invalid")
	}
	evidence := *p.Approval
	evidence.CheckpointDigest = ""
	p.Approval = &evidence
	data, err := json.Marshal(struct {
		Ref      LeaseRef          `json:"ref"`
		Seq      int64             `json:"seq"`
		Progress ExecutionProgress `json:"progress"`
	}{ref, seq, p})
	if err != nil || len(data) > MaxReportMessageBytes {
		return "", errors.New("approval_invalid")
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// Snapshot摘要只绑定原冻结输入，不把恢复证明纳入递归摘要。
func ApprovalSnapshotDigest(task TaskSnapshot) (string, error) {
	task.Resume = nil
	data, err := json.Marshal(task)
	if err != nil || len(data) > 1<<20 {
		return "", errors.New("approval_invalid")
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
