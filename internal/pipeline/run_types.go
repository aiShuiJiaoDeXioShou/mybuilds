package pipeline

import (
	"io"

	"mybuilds/internal/process"
)

// RunOptions 增加本地工作区与日志输出，选择与参数规则沿用预览。
type RunOptions struct {
	PreviewOptions
	Workspace string
	Output    io.Writer
}

type RunResult struct {
	Builds    []BuildRun `json:"builds"`
	ResultDir string     `json:"result_dir,omitempty"`
}

type BuildRun struct {
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	Reason     string    `json:"reason,omitempty"`
	DurationMS int64     `json:"duration_ms"`
	Steps      []StepRun `json:"steps"`
	Post       []StepRun `json:"post,omitempty"`
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
