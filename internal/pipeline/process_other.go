//go:build !darwin && !linux

package pipeline

import (
	"context"
	"io"
)

// 其他平台保留客户端编译；本地执行由整批预检查明确拒绝。
func runShell(context.Context, shellCommand, io.Writer, io.Writer) shellResult {
	return shellResult{ExitCode: -1, Reason: "start_error"}
}
