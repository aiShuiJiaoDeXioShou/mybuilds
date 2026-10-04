//go:build darwin || linux

package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"testing"
	"time"
)

// 使用真实子进程写入两条 OS 管道，二进制内容可逐字节证明没有遗漏。
func TestPipeProcessHelper(t *testing.T) {
	mode := os.Getenv("MYBUILDS_PIPE_HELPER")
	if mode == "" {
		return
	}
	size := 16 * 1024
	if mode == "buffers" || mode == "combined" {
		size = 256 * 1024
	}
	if mode == "combined" {
		var writers sync.WaitGroup
		for _, stream := range []struct {
			file *os.File
			unit []byte
		}{{os.Stdout, []byte{0, 255, 'o', '\n'}}, {os.Stderr, []byte{1, 254, 'e', '\n'}}} {
			writers.Add(1)
			go func(file *os.File, unit []byte) {
				defer writers.Done()
				if _, err := file.Write(bytes.Repeat(unit, size/4)); err != nil {
					os.Exit(4)
				}
			}(stream.file, stream.unit)
		}
		writers.Wait()
		os.Exit(0)
	}
	if _, err := os.Stdout.Write(bytes.Repeat([]byte{0, 255, 'o', '\n'}, size/4)); err != nil {
		os.Exit(2)
	}
	if _, err := os.Stderr.Write(bytes.Repeat([]byte{1, 254, 'e', '\n'}, size/4)); err != nil {
		os.Exit(3)
	}
	os.Exit(0)
}

type slowPipeOutput struct {
	bytes.Buffer
	once  sync.Once
	delay time.Duration
}

func (output *slowPipeOutput) Write(data []byte) (int, error) {
	output.once.Do(func() { time.Sleep(output.delay) })
	return output.Buffer.Write(data)
}

func TestRunSlowWriterPreservesBothPipes(t *testing.T) {
	for _, mode := range []string{"slow", "buffers"} {
		t.Run(mode, func(t *testing.T) {
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			stdout := &slowPipeOutput{delay: 800 * time.Millisecond}
			stderr := &slowPipeOutput{delay: 800 * time.Millisecond}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			result := Run(ctx, Command{Path: executable, Args: []string{"-test.run=^TestPipeProcessHelper$"}, Env: []string{"MYBUILDS_PIPE_HELPER=" + mode}}, stdout, stderr)
			if result.Reason != "" || result.ExitCode != 0 || !result.Started || result.CleanupFailed {
				t.Fatalf("真实成功进程被慢日志误判：%+v", result)
			}
			size := 16 * 1024
			if mode == "buffers" {
				size = 256 * 1024
			}
			for _, item := range []struct {
				output *slowPipeOutput
				unit   []byte
				stream string
			}{{stdout, []byte{0, 255, 'o', '\n'}, "stdout"}, {stderr, []byte{1, 254, 'e', '\n'}, "stderr"}} {
				if !bytes.Equal(item.output.Bytes(), bytes.Repeat(item.unit, size/4)) {
					t.Fatal(fmt.Sprintf("%s 数据不完整：got=%d want=%d", item.stream, item.output.Len(), size))
				}
			}
		})
	}
}

// writer 正在处理已读取的数据时不能被误当成管道读取 idle。
func TestDrainPipeIdleDeadlineExcludesWriterTime(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	if _, err := writer.Write([]byte("buffered-log")); err != nil {
		t.Fatal(err)
	}
	cleaned := make(chan struct{})
	close(cleaned)
	output := &slowPipeOutput{delay: 800 * time.Millisecond}
	before := time.Now()
	err = drainPipe(reader, output, cleaned)
	if !errors.Is(err, os.ErrDeadlineExceeded) || output.String() != "buffered-log" {
		t.Fatal("持写端管道未明确拒绝或缓冲丢失", err, output.String())
	}
	if elapsed := time.Since(before); elapsed < 1200*time.Millisecond || elapsed > 3*time.Second {
		t.Fatal("读取 idle 窗口包含了 writer 时间或无限等待", elapsed)
	}
}

type blockedPipeOutput struct {
	ready   *readyOutput
	release <-chan struct{}
}

func (output blockedPipeOutput) Write(data []byte) (int, error) {
	n, err := output.ready.Write(data)
	<-output.release
	return n, err
}

func TestRunReapsBackgroundBeforeSlowWriterReturns(t *testing.T) {
	unrelated := exec.Command("sleep", "30")
	if err := unrelated.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unrelated.Process.Kill(); _ = unrelated.Wait() }()
	// 等后台实际打印 READY，再令 leader 退出；stderr 仍是本组继承的真实管道。
	command := shellTestCommand(t, processHelper+" >helper-ready & while [ ! -s helper-ready ]; do sleep 0.01; done; cat helper-ready; exit 0")
	ready := newReadyOutput()
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	done := make(chan Result, 1)
	go func() { done <- Run(context.Background(), command, blockedPipeOutput{ready, release}, io.Discard) }()
	pid := awaitReady(t, ready)
	assertProcessStopped(t, pid)
	if err := unrelated.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatal("清理误杀无关进程", err)
	}
	select {
	case result := <-done:
		t.Fatal("writer 未返回却提前声称日志完整", result)
	default:
	}
	unblock()
	result := awaitShellResult(t, done)
	if result.Reason != "" || result.ExitCode != 0 || result.CleanupFailed {
		t.Fatal(result)
	}
}

func TestRunSharedWriterSerializesBothPipes(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var combined bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result := Run(ctx, Command{Path: executable, Args: []string{"-test.run=^TestPipeProcessHelper$"}, Env: []string{"MYBUILDS_PIPE_HELPER=combined"}}, &combined, &combined)
	if result.Reason != "" || result.ExitCode != 0 || result.CleanupFailed || combined.Len() != 512*1024 {
		t.Fatal("共用 writer 丢失或误判", result, combined.Len())
	}
	for _, value := range []byte{0, 255, 'o', 1, 254, 'e'} {
		if count := bytes.Count(combined.Bytes(), []byte{value}); count != 64*1024 {
			t.Fatalf("共用双流字节%d不完整：%d", value, count)
		}
	}
}
