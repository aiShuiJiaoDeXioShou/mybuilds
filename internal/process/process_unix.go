//go:build darwin || linux

package process

import (
	"context"
	"errors"
	"io"
	"os"
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
	// 自持两条管道，让 Wait 只等待真实子进程，不用 WaitDelay 截断慢日志。
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		result.Reason = "start_error"
		return result
	}
	defer stdoutRead.Close()
	defer stdoutWrite.Close()
	stderrRead, stderrWrite, err := os.Pipe()
	if err != nil {
		result.Reason = "start_error"
		return result
	}
	defer stderrRead.Close()
	defer stderrWrite.Close()
	// OS 管道必须支持读取期限；不支持时不启动任何用户动作。
	if stdoutRead.SetReadDeadline(time.Time{}) != nil || stderrRead.SetReadDeadline(time.Time{}) != nil {
		result.Reason = "start_error"
		return result
	}
	cmd.Stdout, cmd.Stderr = stdoutWrite, stderrWrite
	var logFailed, cleanupFailed atomic.Bool
	var cleanupOnce sync.Once
	cleaned := make(chan struct{})
	cleanup := func() {
		cleanupOnce.Do(func() {
			cleanupFailed.Store(!stopProcessGroup(cmd.Process.Pid))
			close(cleaned)
		})
	}
	cmd.Cancel = func() error {
		cleanup()
		if cleanupFailed.Load() {
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
	// 父进程不得保留写端，否则真实 EOF 永远无法出现。
	stdoutWrite.Close()
	stderrWrite.Close()
	var copies sync.WaitGroup
	// 两条流可共用同一 writer；只串行实际 Write，不锁读取或清理。
	var writeMutex sync.Mutex
	for _, pipe := range []struct {
		reader *os.File
		writer io.Writer
	}{{stdoutRead, stdout}, {stderrRead, stderr}} {
		copies.Add(1)
		go func(reader *os.File, writer io.Writer) {
			defer copies.Done()
			output := cancelOnWriteError{writer: writer, cancel: cancel, failed: &logFailed, mutex: &writeMutex}
			if err := drainPipe(reader, output, cleaned); err != nil {
				logFailed.Store(true)
				cancel()
			}
		}(pipe.reader, pipe.writer)
	}
	// 仅一次 Wait。leader 退出后先回收本组，再等日志完整 EOF。
	waitError := cmd.Wait()
	cleanup()
	copies.Wait()
	result.CleanupFailed = cleanupFailed.Load()
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

// drainPipe 的期限只限制无数据读取；同步 writer 的耗时不算读取 idle。
func drainPipe(reader *os.File, writer io.Writer, cleaned <-chan struct{}) error {
	buffer := make([]byte, 32*1024)
	for {
		if err := reader.SetReadDeadline(time.Now().Add(processTermGrace)); err != nil {
			return err
		}
		n, err := reader.Read(buffer)
		if n > 0 {
			if _, writeErr := writer.Write(buffer[:n]); writeErr != nil {
				return writeErr
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if errors.Is(err, os.ErrDeadlineExceeded) {
			select {
			case <-cleaned:
				// 已回收本组仍无 EOF，不把未完成输出报告为成功。
				return err
			default:
				continue
			}
		}
		if err != nil {
			return err
		}
	}
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
	mutex  *sync.Mutex
	writer io.Writer
	cancel context.CancelFunc
	failed *atomic.Bool
}

func (output cancelOnWriteError) Write(data []byte) (int, error) {
	output.mutex.Lock()
	defer output.mutex.Unlock()
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
