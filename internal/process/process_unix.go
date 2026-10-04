//go:build darwin || linux

package process

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const processTermGrace = 500 * time.Millisecond

// Run 只执行已准备的命令；等待、清理及诊断均不包含原始配置。
func Run(ctx context.Context, command Command, stdout, stderr io.Writer) (result Result) {
	started := time.Now()
	result.ExitCode = -1
	defer func() { result.Duration = time.Since(started) }()
	if ctx.Err() != nil {
		result.Reason = contextReason(ctx)
		return result
	}
	runContext, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(runContext, command.Path, command.Args...)
	cmd.Dir = command.Dir
	// 非 nil 空切片阻止 os/exec 自动继承完整宿主环境。
	cmd.Env = append([]string{}, command.Env...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// 管道由后台进程持有时有限结束复制；整个进程组仍由 cleanup 负责。
	cmd.WaitDelay = processTermGrace
	var logFailed atomic.Bool
	cmd.Stdout = cancelOnWriteError{writer: stdout, cancel: cancel, failed: &logFailed}
	cmd.Stderr = cancelOnWriteError{writer: stderr, cancel: cancel, failed: &logFailed}
	var cleanupOnce sync.Once
	cleanup := func() { cleanupOnce.Do(func() { result.CleanupFailed = !stopProcessGroup(cmd.Process.Pid) }) }
	cmd.Cancel = func() error {
		cleanup()
		if result.CleanupFailed {
			return errors.New("本次进程组停止未确认")
		}
		return nil
	}
	if err := cmd.Start(); err != nil {
		result.Reason = "start_error"
		if ctx.Err() != nil {
			result.Reason = contextReason(ctx)
		}
		return result
	}
	result.Started = true
	// Start 成功后仅调用一次 Wait，确保直接子进程及复制 goroutine 被回收。
	waitError := cmd.Wait()
	cleanup()
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	switch {
	case ctx.Err() != nil:
		result.Reason = contextReason(ctx)
	case logFailed.Load():
		result.Reason = "log_error"
	case waitError != nil:
		result.Reason = "exit"
	}
	if result.CleanupFailed && result.Reason == "" {
		result.Reason = "cleanup_error"
	}
	return result
}

func contextReason(ctx context.Context) string {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "timeout"
	}
	return "cancelled"
}

// stopProcessGroup 也用于正常退出后的后台清理，不按进程名或系统用户杀进程。
func stopProcessGroup(pgid int) bool {
	if pgid <= 1 {
		return false
	}
	if err := signalProcessGroup(pgid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.EPERM) {
		return errors.Is(err, syscall.ESRCH)
	}
	if waitProcessGroupGone(pgid, processTermGrace) {
		return true
	}
	if err := signalProcessGroup(pgid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.EPERM) {
		return errors.Is(err, syscall.ESRCH)
	}
	// 信号发送成功不等于停止已确认；只等待本组消失，孙进程由其父/系统回收。
	return waitProcessGroupGone(pgid, processTermGrace)
}

func waitProcessGroupGone(pgid int, budget time.Duration) bool {
	deadline := time.Now().Add(budget)
	for {
		err := signalProcessGroup(pgid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return true
		}
		// Darwin 退出/回收边界可能暂时返回 EPERM；它不证明停止，只能在原窗口内复查。
		if (err != nil && !errors.Is(err, syscall.EPERM)) || !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(10 * time.Millisecond)
	}
}

type cancelOnWriteError struct {
	writer io.Writer
	cancel context.CancelFunc
	failed *atomic.Bool
}

func (output cancelOnWriteError) Write(data []byte) (int, error) {
	if output.writer == nil {
		return len(data), nil
	}
	n, err := output.writer.Write(data)
	if n < len(data) && err == nil {
		err = io.ErrShortWrite
	}
	if err != nil {
		output.failed.Store(true)
		output.cancel()
	}
	return n, err
}

func signalProcessGroup(pgid int, signal syscall.Signal) error {
	for {
		err := syscall.Kill(-pgid, signal)
		if !errors.Is(err, syscall.EINTR) {
			return err
		}
	}
}
