package pipeline

import (
	"context"
	"io"

	"mybuilds/internal/mobile"
	"mybuilds/internal/process"
	"mybuilds/internal/protocol"
)

// RunOptions 增加本地工作区与日志输出，选择与参数规则沿用预览。
type RunOptions struct {
	Resume          *ApprovalResume
	ConfirmApproval func(context.Context, ApprovalPrompt) (bool, error)
	PreviewOptions
	Workspace string
	Output    io.Writer
	Remote    *RemoteOptions
}

// RemoteOptions只供Agent同一Run的实际持久化消费者，nil保持本地行为。
type RemoteOptions struct {
	Publish               func(context.Context, PublishInput) (protocol.PublishReceipt, error)
	AuthorityContext      context.Context
	Facts, Secrets        map[string]string
	ResultParent          string
	ResultCreated         func(context.Context, string) error
	RemainingBudgetNS     *int64
	RemainingPostBudgetNS int64
	IOSCheckpoint         func(mobile.IOSResourceOwnership) error
	Progress              func(context.Context, protocol.ExecutionProgress) error
	Log                   func(context.Context, protocol.LogRecord) error
}

type RunResult struct {
	Paused    *ApprovalPause `json:"paused,omitempty"`
	Builds    []BuildRun     `json:"builds"`
	ResultDir string         `json:"result_dir,omitempty"`
}

type BuildRun struct {
	paused              *ApprovalPause
	iosResourceDigest   string
	iosTeamID           string
	CleanupFailed       bool                     `json:"cleanup_failed,omitempty"`
	IOSCleanupConfirmed bool                     `json:"ios_cleanup_confirmed,omitempty"`
	Reports             *protocol.ReportEvidence `json:"reports,omitempty"`
	ReportSealDigest    string                   `json:"report_seal_digest,omitempty"`
	Name                string                   `json:"name"`
	Status              string                   `json:"status"`
	Reason              string                   `json:"reason,omitempty"`
	DurationMS          int64                    `json:"duration_ms"`
	Steps               []StepRun                `json:"steps"`
	Post                []StepRun                `json:"post,omitempty"`
}

type StepRun struct {
	Name          string           `json:"name"`
	Kind          string           `json:"kind"`
	Status        string           `json:"status"`
	Reason        string           `json:"reason,omitempty"`
	ExitCode      int              `json:"exit_code"`
	DurationMS    int64            `json:"duration_ms"`
	CleanupFailed bool             `json:"cleanup_failed,omitempty"`
	Artifacts     []ArtifactRecord `json:"artifacts,omitempty"`
	LogPath       string           `json:"log_path,omitempty"`
	started       bool
}

// ArtifactRecord 记录独立快照的实际大小和内容摘要，不引用可变源文件作为证据。
type ArtifactRecord struct {
	SourcePath   string `json:"source_path"`
	SnapshotPath string `json:"snapshot_path"`
	Size         int64  `json:"size"`
	SHA256       string `json:"sha256"`
}

type shellCommand = process.Command

// Resume只由Agent从原私有journal与中央已批准grant共同恢复。
type ApprovalResume struct {
	Evidence protocol.ApprovalResumeEvidence
	Local    protocol.ApprovalLocalCheckpoint
}
type ApprovalPrompt struct {
	Build, Step string
	Index       int
}
type ApprovalPause struct {
	Index int                              `json:"index"`
	Step  string                           `json:"step"`
	Local protocol.ApprovalLocalCheckpoint `json:"-"`
}
