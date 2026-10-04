package pipeline

import (
	"io"
	"time"
)

// RunOptions 增加本地工作区与日志输出，选择与参数规则沿用预览。
type RunOptions struct {
	PreviewOptions
	Workspace string
	Output    io.Writer
}

type RunResult struct {
	Builds []BuildRun `json:"builds"`
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
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	Status        string `json:"status"`
	Reason        string `json:"reason,omitempty"`
	ExitCode      int    `json:"exit_code"`
	DurationMS    int64  `json:"duration_ms"`
	CleanupFailed bool   `json:"cleanup_failed,omitempty"`
	started       bool
}

// shellCommand 只保存已准备的命令，不自行读取宿主环境或配置。
type shellCommand struct {
	Path string
	Args []string
	Dir  string
	Env  []string
}

type shellResult struct {
	Started       bool
	ExitCode      int
	Reason        string
	Duration      time.Duration
	CleanupFailed bool
}
