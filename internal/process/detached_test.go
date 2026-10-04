//go:build darwin || linux

package process

import (
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
	"syscall"
	"testing"
	"time"
)

// 用真实setsid子进程复现Gradle单次daemon；不依赖Gradle安装或模拟执行器。
func TestDetachedProcessHelper(t *testing.T) {
	mode := os.Getenv("MYBUILDS_DETACHED_HELPER")
	if mode == "" {
		return
	}
	if mode == "child" {
		signal.Ignore(syscall.SIGTERM)
		if err := os.WriteFile(os.Getenv("OWN_READY"), []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
			os.Exit(3)
		}
		until := time.Now().Add(20 * time.Second)
		for time.Now().Before(until) {
			time.Sleep(time.Second)
		}
		os.Exit(0)
	}
	exe, err := os.Executable()
	if err != nil {
		os.Exit(4)
	}
	child := exec.Command(exe, "-test.run=^TestDetachedProcessHelper$")
	child.Env = append([]string{}, os.Environ()...)
	for i, value := range child.Env {
		if strings.HasPrefix(value, "MYBUILDS_DETACHED_HELPER=") {
			child.Env[i] = "MYBUILDS_DETACHED_HELPER=child"
			if mode == "double" {
				child.Env[i] = "MYBUILDS_DETACHED_HELPER=middle"
			}
		}
	}
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		os.Exit(5)
	}
	child.Stdin, child.Stdout, child.Stderr = null, null, null
	if child.Start() != nil {
		os.Exit(6)
	}
	limit := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(os.Getenv("OWN_READY")); err == nil {
			break
		}
		if time.Now().After(limit) {
			os.Exit(7)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if mode == "middle" {
		os.Exit(0)
	}
	ready, err := os.ReadFile(os.Getenv("OWN_READY"))
	if err != nil {
		os.Exit(8)
	}
	pid, err := strconv.Atoi(string(ready))
	if err != nil {
		os.Exit(9)
	}
	fmt.Printf("READY %d\n", pid)
	if mode == "exit" {
		os.Exit(0)
	}
	for {
		time.Sleep(time.Second)
	}
}
func TestRunDetachedMembersStopBeforeResult(t *testing.T) {
	for _, mode := range []string{"exit", "cancel", "timeout", "double"} {
		t.Run(mode, func(t *testing.T) {
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			ready := filepath.Join(t.TempDir(), "ready")
			launchMode := "wait"
			if mode == "exit" || mode == "double" {
				launchMode = mode
			}
			command := Command{Path: exe, Args: []string{"-test.run=^TestDetachedProcessHelper$"}, Dir: t.TempDir(), Env: []string{"MYBUILDS_DETACHED_HELPER=" + launchMode, "OWN_READY=" + ready}}
			unrelated := exec.Command("sleep", "30")
			if err = unrelated.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { unrelated.Process.Kill(); unrelated.Wait() })
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "timeout" {
				ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
			}
			output := newReadyOutput()
			done := make(chan Result, 1)
			go func() { done <- Run(ctx, command, output, io.Discard) }()
			pid := awaitReady(t, output)
			pgid, err := syscall.Getpgid(pid)
			if err != nil || pgid != pid {
				t.Fatal("helper未真正setsid", pgid, err)
			}
			if mode == "cancel" || mode == "double" {
				cancel()
			}
			result := awaitShellResult(t, done)
			expected := ""
			if mode == "cancel" || mode == "double" {
				expected = "cancelled"
			}
			if mode == "timeout" {
				expected = "timeout"
			}
			if !result.Started || result.CleanupFailed || result.Reason != expected {
				t.Fatal("实际回收未确认", result)
			}
			if err = syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
				t.Fatal("Result返回后脱离daemon仍存在", pid, err)
			}
			if err = syscall.Kill(unrelated.Process.Pid, 0); err != nil {
				t.Fatal("无关同用户进程被停止", err)
			}
		})
	}
}

func TestScopeBirthMismatchDoesNotSignalUnrelatedProcess(t *testing.T) {
	child := exec.Command("sleep", "20")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { child.Process.Kill(); child.Wait() })
	birth, live, err := readProcessBirth(child.Process.Pid)
	if err != nil || !live {
		t.Fatal("真实birth", err)
	}
	wrong := birth
	wrong.first++
	stopped, err := signalOwnedProcess(child.Process.Pid, wrong, syscall.SIGKILL)
	if err != nil || !stopped {
		t.Fatal("不同birth不应产生信号", err)
	}
	if err = syscall.Kill(child.Process.Pid, 0); err != nil {
		t.Fatal("无关birth被停止", err)
	}
}
func TestScopeOutputRedactsPrivateValueAcrossChunks(t *testing.T) {
	scope, err := newProcessScope()
	if err != nil {
		t.Fatal(err)
	}
	var buffer strings.Builder
	output := &scopeOutput{writer: &buffer, token: []byte(scope.token)}
	for _, part := range []string{"plain\n", scope.token[:1], scope.token[1:17], scope.token[17:], "end\n"} {
		if _, err = output.Write([]byte(part)); err != nil {
			t.Fatal(err)
		}
	}
	if err = output.flush(); err != nil {
		t.Fatal(err)
	}
	if buffer.String() != "plain\n[REDACTED]end\n" {
		t.Fatal("私有标记或正文处理失败")
	}
	command := shellTestCommand(t, `printf '%s' "$MYBUILDS_PROCESS_SCOPE"; printf '%s' "$MYBUILDS_PROCESS_SCOPE" >&2`)
	var stdout, stderr strings.Builder
	result := Run(context.Background(), command, &stdout, &stderr)
	if result.Reason != "" || result.CleanupFailed || stdout.String() != "[REDACTED]" || stderr.String() != "[REDACTED]" {
		t.Fatal("内部标记进入真实输出", result)
	}
}
func TestDetachedParallelRunHasSeparateScope(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var results [2]chan Result
	var cancels [2]context.CancelFunc
	var pids [2]int
	for index := 0; index < 2; index++ {
		ctx, cancel := context.WithCancel(context.Background())
		cancels[index] = cancel
		defer cancel()
		output := newReadyOutput()
		ready := filepath.Join(t.TempDir(), "ready")
		command := Command{Path: exe, Args: []string{"-test.run=^TestDetachedProcessHelper$"}, Dir: t.TempDir(), Env: []string{"MYBUILDS_DETACHED_HELPER=wait", "OWN_READY=" + ready}}
		results[index] = make(chan Result, 1)
		done := results[index]
		go func() { done <- Run(ctx, command, output, io.Discard) }()
		pids[index] = awaitReady(t, output)
	}
	cancels[0]()
	first := awaitShellResult(t, results[0])
	assertProcessStopped(t, pids[0])
	if first.CleanupFailed || first.Reason != "cancelled" {
		t.Fatal(first)
	}
	if err = syscall.Kill(pids[1], 0); err != nil {
		t.Fatal("另一Run被误停止", err)
	}
	cancels[1]()
	second := awaitShellResult(t, results[1])
	assertProcessStopped(t, pids[1])
	if second.CleanupFailed || second.Reason != "cancelled" {
		t.Fatal(second)
	}
}
