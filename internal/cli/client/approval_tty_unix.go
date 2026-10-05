//go:build darwin || linux

package client

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"mybuilds/internal/pipeline"
	"os"
	"strings"
)

// 只从实际终端读取有限输入，poll让取消无需遗留阻塞reader。
func confirmLocalApproval(input io.Reader, output io.Writer) func(context.Context, pipeline.ApprovalPrompt) (bool, error) {
	return func(ctx context.Context, _ pipeline.ApprovalPrompt) (bool, error) {
		f, ok := input.(*os.File)
		if !ok {
			return false, errors.New("approval_requires_tty")
		}
		fd := int(f.Fd())
		if _, err := unix.IoctlGetTermios(fd, approvalTermiosRequest); err != nil {
			return false, errors.New("approval_requires_tty")
		}
		if _, err := fmt.Fprint(output, "本地审批：输入 yes 批准，或 no 拒绝：\n"); err != nil {
			return false, errors.New("approval_input_error")
		}
		data := make([]byte, 0, 256)
		for {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			poll := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
			_, err := unix.Poll(poll, 100)
			if err == unix.EINTR {
				continue
			}
			if err != nil {
				return false, errors.New("approval_input_error")
			}
			if poll[0].Revents == 0 {
				continue
			}
			if poll[0].Revents&(unix.POLLERR|unix.POLLNVAL) != 0 {
				return false, errors.New("approval_input_error")
			}
			var b [1]byte
			n, err := unix.Read(fd, b[:])
			if err == unix.EINTR {
				continue
			}
			if err != nil || n == 0 {
				return false, errors.New("approval_input_error")
			}
			if b[0] == '\n' || b[0] == '\r' {
				switch strings.TrimSpace(string(data)) {
				case "yes":
					return true, nil
				case "no":
					return false, nil
				default:
					return false, errors.New("approval_input_error")
				}
			}
			if len(data) >= 256 || b[0] < 32 || b[0] == 127 {
				return false, errors.New("approval_input_error")
			}
			data = append(data, b[0])
		}
	}
}
