// Package protocol 定义当前控制端与节点实际消费者的具体消息。
package protocol

import (
	"mybuilds/internal/config"
	"time"
)

type LeaseRef struct {
	NodeID    string `json:"node_id"`
	SessionID string `json:"session_id"`
	BuildID   string `json:"build_id"`
	AttemptID string `json:"attempt_id"`
	LeaseID   string `json:"lease_id"`
	Epoch     int64  `json:"epoch"`
}

// TerminalReceipt 仅核对已经确认的终态，不授予旧执行权。
type TerminalReceiptRequest struct {
	Ref    LeaseRef `json:"ref"`
	Seq    int64    `json:"seq"`
	Digest string   `json:"digest"`
}
type TerminalReceipt struct {
	Ref       LeaseRef `json:"ref"`
	Seq       int64    `json:"seq"`
	Digest    string   `json:"digest"`
	Status    string   `json:"status"`
	StopKnown bool     `json:"stop_known"`
	NodeName  string   `json:"node_name"`
}
type ToolCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Version string `json:"version,omitempty"`
	Reason  string `json:"reason,omitempty"`
}
type NodeReport struct {
	OS       string      `json:"os"`
	Arch     string      `json:"arch"`
	Capacity int         `json:"capacity"`
	Tools    []ToolCheck `json:"tools"`
}
type SessionRequest struct {
	SessionID   string     `json:"session_id"`
	Report      NodeReport `json:"report"`
	HeartbeatNS int64      `json:"heartbeat_ns"`
	LeaseNS     int64      `json:"lease_ns"`
}
type SessionGrant struct {
	NodeID      string `json:"node_id"`
	NodeName    string `json:"node_name"`
	SessionID   string `json:"session_id"`
	HeartbeatNS int64  `json:"heartbeat_ns"`
	LeaseNS     int64  `json:"lease_ns"`
}
type HeartbeatRequest struct {
	SessionID string     `json:"session_id"`
	Report    NodeReport `json:"report"`
}
type ClaimRequest struct {
	SessionID string `json:"session_id"`
	ClaimKey  string `json:"claim_key"`
}
type TaskSnapshot struct {
	Project      string            `json:"project"`
	BuildName    string            `json:"build_name"`
	Number       int64             `json:"number"`
	Repository   string            `json:"repository"`
	Branch       string            `json:"branch"`
	SHA          string            `json:"sha"`
	SourceDigest string            `json:"source_digest"`
	Definition   config.Build      `json:"definition"`
	Parameters   map[string]string `json:"parameters"`
	Facts        map[string]string `json:"facts"`
}
type LeaseGrant struct {
	Ref                   LeaseRef      `json:"ref"`
	TTLNS                 int64         `json:"ttl_ns"`
	CancelRequested       bool          `json:"cancel_requested"`
	Task                  *TaskSnapshot `json:"task,omitempty"`
	RemainingBudgetNS     *int64        `json:"remaining_budget_ns,omitempty"`
	RemainingPostBudgetNS int64         `json:"remaining_post_budget_ns"`
}
type ArtifactExpectation struct {
	Phase string   `json:"phase"`
	Index int      `json:"index"`
	Count int      `json:"count"`
	IDs   []string `json:"ids"`
}

// CollectedArtifact 仅在本地把真实收集结果交给Agent，路径不进入网络事件。
type CollectedArtifact struct {
	SnapshotPath, Name, SHA256 string
	Size                       int64
}
type ExecutionProgress struct {
	Kind                  string                `json:"kind"`
	Phase                 string                `json:"phase,omitempty"`
	Name                  string                `json:"name,omitempty"`
	StepKind              string                `json:"step_kind,omitempty"`
	Status                string                `json:"status,omitempty"`
	Reason                string                `json:"reason,omitempty"`
	PostPhase             string                `json:"post_phase,omitempty"`
	Index                 int                   `json:"index"`
	Started               bool                  `json:"started"`
	StopConfirmed         bool                  `json:"stop_confirmed"`
	CleanupFailed         bool                  `json:"cleanup_failed"`
	ExitCode              int                   `json:"exit_code"`
	ElapsedNS             int64                 `json:"elapsed_ns"`
	RemainingBudgetNS     *int64                `json:"remaining_budget_ns,omitempty"`
	RemainingPostBudgetNS int64                 `json:"remaining_post_budget_ns"`
	At                    time.Time             `json:"at"`
	ArtifactIDs           []string              `json:"artifact_ids,omitempty"`
	LastLogSeq            int64                 `json:"last_log_seq"`
	LastLogOffset         int64                 `json:"last_log_offset"`
	LastArtifactSeq       int64                 `json:"last_artifact_seq"`
	ArtifactSteps         []ArtifactExpectation `json:"artifact_steps"`
	PID                   int                   `json:"-"`
	PGID                  int                   `json:"-"`
	LocalResultDir        string                `json:"-"`
	LocalArtifacts        []CollectedArtifact   `json:"-"`
}
type ExecutionEvent struct {
	Ref      LeaseRef          `json:"ref"`
	Seq      int64             `json:"seq"`
	Digest   string            `json:"digest"`
	Progress ExecutionProgress `json:"progress"`
}
type EventAck struct {
	Seq    int64  `json:"seq"`
	Digest string `json:"digest"`
}
type LogRecord struct {
	UTC    time.Time `json:"utc"`
	Build  string    `json:"build"`
	Phase  string    `json:"phase"`
	Step   string    `json:"step"`
	Stream string    `json:"stream"`
	Text   string    `json:"text"`
	Index  int       `json:"index"`
}
type LogChunk struct {
	Ref     LeaseRef    `json:"ref"`
	Seq     int64       `json:"seq"`
	Offset  int64       `json:"offset"`
	Digest  string      `json:"digest"`
	Records []LogRecord `json:"records"`
}
type LogAck struct {
	Seq        int64  `json:"seq"`
	NextOffset int64  `json:"next_offset"`
	Digest     string `json:"digest"`
}
type ArtifactDeclaration struct {
	Ref    LeaseRef `json:"ref"`
	ID     string   `json:"id"`
	Seq    int64    `json:"seq"`
	Phase  string   `json:"phase"`
	Step   string   `json:"step"`
	Name   string   `json:"name"`
	Index  int      `json:"index"`
	Size   int64    `json:"size"`
	SHA256 string   `json:"sha256"`
}
type ArtifactView struct {
	ID          string    `json:"id"`
	BuildID     string    `json:"build_id"`
	AttemptID   string    `json:"attempt_id"`
	BuildName   string    `json:"build_name"`
	Phase       string    `json:"phase"`
	Step        string    `json:"step"`
	Name        string    `json:"name"`
	Index       int       `json:"index"`
	Size        int64     `json:"size"`
	SHA256      string    `json:"sha256"`
	CompletedAt time.Time `json:"completed_at"`
}
type StopConfirmation struct {
	Ref          LeaseRef `json:"ref"`
	EvidenceCode string   `json:"evidence_code"`
	Note         string   `json:"note"`
}
