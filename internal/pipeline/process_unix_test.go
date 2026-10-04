//go:build darwin || linux

package pipeline

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// 当前测试二进制兼作真实子进程，不需要外部辅助程序或构建框架。
func TestShellProcessHelper(t *testing.T) {
	mode := os.Getenv("MYBUILDS_PROCESS_HELPER")
	if mode == "" {
		return
	}
	if mode == "ignore" {
		signal.Ignore(syscall.SIGTERM)
	}
	fmt.Printf("READY %d\n", os.Getpid())
	for {
		time.Sleep(time.Second)
	}
}

func shellTestCommand(t *testing.T, script string) shellCommand {
	t.Helper()
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return shellCommand{Path: shell, Args: []string{"-e", "-c", script}, Dir: t.TempDir(), Env: []string{"PATH=" + os.Getenv("PATH"), "HELPER=" + executable}}
}

const processHelper = `MYBUILDS_PROCESS_HELPER=ignore "$HELPER" -test.run='^TestShellProcessHelper$'`

// 捕获输出与 ready 同步，避免按固定 sleep 猜测子进程是否启动。
type readyOutput struct {
	mutex sync.Mutex
	bytes bytes.Buffer
	ready chan int
	once  sync.Once
}

func newReadyOutput() *readyOutput { return &readyOutput{ready: make(chan int, 1)} }
func (output *readyOutput) Write(data []byte) (int, error) {
	output.mutex.Lock()
	defer output.mutex.Unlock()
	n, err := output.bytes.Write(data)
	for _, line := range strings.Split(output.bytes.String(), "\n") {
		if strings.HasPrefix(line, "READY ") {
			if pid, parseErr := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "READY "))); parseErr == nil {
				output.once.Do(func() { output.ready <- pid })
			}
		}
	}
	return n, err
}
func (output *readyOutput) String() string {
	output.mutex.Lock()
	defer output.mutex.Unlock()
	return output.bytes.String()
}
func awaitReady(t *testing.T, output *readyOutput) int {
	t.Helper()
	select {
	case pid := <-output.ready:
		return pid
	case <-time.After(10 * time.Second):
		t.Fatal("辅助进程未准备就绪")
		return 0
	}
}
func assertProcessStopped(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("本次进程仍存在，PID=%d", pid)
}
func awaitShellResult(t *testing.T, result <-chan shellResult) shellResult {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("进程结束无限等待")
		return shellResult{}
	}
}

func TestRunShellExitAndEnvironment(t *testing.T) {
	for _, tc := range []struct {
		script string
		code   int
		reason string
	}{{"printf out; printf err >&2", 0, ""}, {"exit 7", 7, "exit"}} {
		var stdout, stderr bytes.Buffer
		result := runShell(context.Background(), shellTestCommand(t, tc.script), &stdout, &stderr)
		if !result.Started || result.ExitCode != tc.code || result.Reason != tc.reason || result.CleanupFailed || result.Duration <= 0 {
			t.Fatalf("退出结果错误: %+v", result)
		}
		if tc.code == 0 && (stdout.String() != "out" || stderr.String() != "err") {
			t.Fatal("标准输出未完整返回")
		}
	}
	t.Setenv("HOST_SECRET", "DO_NOT_INHERIT")
	cmd := shellTestCommand(t, `printf '%s' "${HOST_SECRET-unset}"`)
	cmd.Env = nil
	var output bytes.Buffer
	result := runShell(context.Background(), cmd, &output, io.Discard)
	if result.Reason != "" || output.String() != "unset" {
		t.Fatal("nil Env 继承了宿主环境", result, output.String())
	}
}

func TestRunShellBeforeStartCancelledAndSafeFailure(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		var ctx context.Context
		var cancel context.CancelFunc
		want := "cancelled"
		if timeout {
			ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			want = "timeout"
		} else {
			ctx, cancel = context.WithCancel(context.Background())
			cancel()
		}
		defer cancel()
		cmd := shellTestCommand(t, "touch marker")
		result := runShell(ctx, cmd, io.Discard, io.Discard)
		if result.Started || result.Reason != want || result.ExitCode != -1 || result.CleanupFailed {
			t.Fatal(result)
		}
		if _, err := os.Stat(filepath.Join(cmd.Dir, "marker")); !os.IsNotExist(err) {
			t.Fatal("已取消的命令仍被启动")
		}
	}
	cmd := shellTestCommand(t, "echo SECRET_SCRIPT")
	cmd.Dir = filepath.Join(cmd.Dir, "SECRET_PATH")
	result := runShell(context.Background(), cmd, io.Discard, io.Discard)
	if result.Started || result.Reason != "start_error" || strings.Contains(fmt.Sprint(result), "SECRET") {
		t.Fatal("启动错误泄露或误分类", result)
	}
}

func TestRunShellCancelGroupAndKeepUnrelated(t *testing.T) {
	unrelated := exec.Command("sleep", "30")
	if err := unrelated.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unrelated.Process.Kill(); _ = unrelated.Wait() }()
	for _, tc := range []struct{ name, script string }{{"foreground", processHelper}, {"background", processHelper + " & wait"}, {"leader-exits", processHelper + " & exit 0"}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			output := newReadyOutput()
			done := make(chan shellResult, 1)
			go func() { done <- runShell(ctx, shellTestCommand(t, tc.script), output, io.Discard) }()
			pid := awaitReady(t, output)
			cancel()
			result := awaitShellResult(t, done)
			if result.Reason != "cancelled" || result.CleanupFailed {
				t.Fatal("取消结果错误", result)
			}
			assertProcessStopped(t, pid)
			if err := unrelated.Process.Signal(syscall.Signal(0)); err != nil {
				t.Fatal("误杀无关进程", err)
			}
		})
	}
}

func TestRunShellNormalExitCleansBackground(t *testing.T) {
	for _, redirect := range []bool{false, true} {
		t.Run(fmt.Sprint(redirect), func(t *testing.T) {
			script := processHelper + " & exit 0"
			if redirect {
				script = processHelper + " >/dev/null 2>&1 & echo READY $!; exit 0"
			}
			cmd := shellTestCommand(t, script)
			output := newReadyOutput()
			done := make(chan shellResult, 1)
			go func() { done <- runShell(context.Background(), cmd, output, io.Discard) }()
			pid := awaitReady(t, output)
			result := awaitShellResult(t, done)
			if result.CleanupFailed {
				t.Fatal("正常退出未清理后台", result)
			}
			if redirect && result.Reason != "" {
				t.Fatal("重定向后台导致错误", result)
			}
			assertProcessStopped(t, pid)
		})
	}
}

func TestRunShellTimeoutKillsIgnoringTerm(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	output := newReadyOutput()
	done := make(chan shellResult, 1)
	cmd := shellTestCommand(t, processHelper+" & wait")
	started := time.Now()
	go func() { done <- runShell(ctx, cmd, output, io.Discard) }()
	pid := awaitReady(t, output)
	result := awaitShellResult(t, done)
	if result.Reason != "timeout" || result.CleanupFailed {
		t.Fatal("超时结果错误", result)
	}
	if time.Since(started) > 3*time.Second {
		t.Fatal("超时清理超过有限宽限")
	}
	assertProcessStopped(t, pid)
}

type failOutput struct{}

func (failOutput) Write([]byte) (int, error) { return 0, errors.New("DO_NOT_LEAK_LOG_ERROR") }

type shortOutput struct{}

func (shortOutput) Write([]byte) (int, error) { return 0, nil }
func TestRunShellLogFailureCancels(t *testing.T) {
	for _, writer := range []io.Writer{failOutput{}, shortOutput{}} {
		for _, stderr := range []bool{false, true} {
			cmd := shellTestCommand(t, "printf log; sleep 30")
			stdout, errorOutput := writer, io.Writer(io.Discard)
			if stderr {
				cmd.Args[2] = "printf log >&2; sleep 30"
				stdout, errorOutput = io.Discard, writer
			}
			done := make(chan shellResult, 1)
			go func() { done <- runShell(context.Background(), cmd, stdout, errorOutput) }()
			result := awaitShellResult(t, done)
			if result.Reason != "log_error" || result.CleanupFailed || strings.Contains(fmt.Sprint(result), "DO_NOT_LEAK") {
				t.Fatal("日志错误未安全停止", result)
			}
		}
	}
}
