//go:build !darwin && !linux

package process

import (
	"context"
	"io"
)

// 其他平台保留客户端编译；本地执行由整批预检查明确拒绝。
func Run(context.Context, Command, io.Writer, io.Writer) Result {
	return Result{ExitCode: -1, Reason: "start_error"}
}
