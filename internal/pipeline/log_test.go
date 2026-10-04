package pipeline

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

func logPayload(t *testing.T, output string) string {
	t.Helper()
	var result strings.Builder
	for _, line := range strings.Split(strings.TrimSuffix(output, "\n"), "\n") {
		parts := strings.SplitN(line, "] ", 5)
		if len(parts) != 5 {
			t.Fatalf("日志缺少来源前缀：%q", line)
		}
		stamp := strings.TrimPrefix(parts[0], "[")
		if _, err := time.Parse(time.RFC3339Nano, stamp); err != nil || !strings.HasSuffix(stamp, "Z") {
			t.Fatalf("日志时间不是 UTC：%q", stamp)
		}
		result.WriteString(parts[4])
	}
	return result.String()
}

func TestLogRedactsFragmentsLongestMatchAndFinalTail(t *testing.T) {
	var output bytes.Buffer
	logger := newRunLogger(&output, []string{"abc", "abcde", "secret-value", "", "abc"})
	stream := logger.stream("demo", "package", "stdout")
	for _, fragment := range []string{"start a", "bc", "de / sec", "ret-", "value / abc / end se"} {
		if _, err := io.WriteString(stream, fragment); err != nil {
			t.Fatal(err)
		}
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	payload := logPayload(t, output.String())
	if payload != "start [REDACTED] / [REDACTED] / [REDACTED] / end se" {
		t.Fatalf("增量脱敏错误：%q", payload)
	}
	if !strings.Contains(output.String(), "[build=demo] [step=package] [stream=stdout]") {
		t.Fatalf("日志来源缺失：%q", output.String())
	}
}

func TestLogConcurrentStreamsAndLongLines(t *testing.T) {
	var output bytes.Buffer
	logger := newRunLogger(&output, []string{"private-secret"})
	var group sync.WaitGroup
	for _, name := range []string{"stdout", "stderr"} {
		group.Go(func() {
			stream := logger.stream("demo", "test", name)
			if _, err := io.WriteString(stream, strings.Repeat("x", 200000)+"private-secret\n"); err != nil {
				t.Error(err)
			}
			if err := stream.Close(); err != nil {
				t.Error(err)
			}
		})
	}
	group.Wait()
	if strings.Contains(output.String(), "private-secret") {
		t.Fatal("长行泄露密钥")
	}
	payload := logPayload(t, output.String())
	if strings.Count(payload, "x") != 400000 || strings.Count(payload, "[REDACTED]") != 2 {
		t.Fatal("长行内容丢失")
	}
	if !strings.Contains(output.String(), "[stream=stdout]") || !strings.Contains(output.String(), "[stream=stderr]") {
		t.Fatal("两流没有分别标记")
	}
}

type failingLogWriter struct{ short bool }

func (w failingLogWriter) Write(data []byte) (int, error) {
	if w.short {
		return len(data) - 1, nil
	}
	return 0, errors.New("secret-writer-error")
}

func TestLogSafeWriteAndCloseErrors(t *testing.T) {
	for _, short := range []bool{false, true} {
		for _, duringWrite := range []bool{false, true} {
			logger := newRunLogger(failingLogWriter{short: short}, nil)
			stream := logger.stream("demo", "test", "stdout")
			data := "tail"
			if duringWrite {
				data += "\n"
			}
			_, err := io.WriteString(stream, data)
			if !duringWrite {
				err = stream.Close()
			}
			if err == nil || strings.Contains(err.Error(), "secret-writer-error") {
				t.Fatalf("日志错误不安全或缺失：%v", err)
			}
			if _, err := io.WriteString(logger.stream("demo", "test", "stderr"), "next\n"); err == nil {
				t.Fatal("共享 writer 失败后另一个流没有报错")
			}
		}
	}
}

func TestLogStreamBuffersIndependently(t *testing.T) {
	var output bytes.Buffer
	logger := newRunLogger(&output, []string{"abcd"})
	stdout, stderr := logger.stream("demo", "test", "stdout"), logger.stream("demo", "test", "stderr")
	io.WriteString(stdout, "ab")
	io.WriteString(stderr, "cd\n")
	stdout.Close()
	stderr.Close()
	if payload := logPayload(t, output.String()); strings.Contains(payload, "[REDACTED]") || !strings.Contains(payload, "ab") || !strings.Contains(payload, "cd") {
		t.Fatalf("两流错误合并了秘密后缀：%q", payload)
	}
}

func TestLogLongUnicodeLineKeepsValidUTF8(t *testing.T) {
	var output bytes.Buffer
	stream := newRunLogger(&output, nil).stream("demo", "test", "stdout")
	text := strings.Repeat("中文", 20000)
	if _, err := io.WriteString(stream, text); err != nil {
		t.Fatal(err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	if !utf8.ValidString(output.String()) || logPayload(t, output.String()) != text {
		t.Fatal("长行分块破坏了 UTF-8 文本")
	}
}

func mirrorLogger(t *testing.T, output io.Writer, secrets []string) (*runLogger, string) {
	t.Helper()
	directory := t.TempDir()
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	logger := newRunLogger(output, secrets)
	logger.root = root
	return logger, directory
}

func TestLogRootMirrorPermissionsAndSharedStreams(t *testing.T) {
	var output bytes.Buffer
	logger, directory := mirrorLogger(t, &output, []string{"private-mirror-secret"})
	if path := logger.logPath("demo", "test"); path != "" {
		t.Fatalf("尚未创建的日志不能伪造路径：%q", path)
	}
	var group sync.WaitGroup
	for _, name := range []string{"stdout", "stderr"} {
		group.Go(func() {
			stream := logger.stream("demo", "test", name)
			for _, text := range []string{strings.Repeat("中文", 2000) + "private-", "mirror-secret\n"} {
				if _, err := io.WriteString(stream, text); err != nil {
					t.Error(err)
				}
			}
			if err := stream.Close(); err != nil {
				t.Error(err)
			}
		})
	}
	group.Wait()
	if err := logger.close(); err != nil {
		t.Fatal(err)
	}
	name := logger.logPath("demo", "test")
	if name != "logs/demo/test.log" {
		t.Fatalf("实际日志路径错误：%q", name)
	}
	data, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(name)))
	if err != nil || string(data) != output.String() || strings.Contains(string(data), "private-mirror-secret") || !utf8.Valid(data) {
		t.Fatalf("镜像内容/脱敏/字符错误：%v", err)
	}
	logPayload(t, string(data))
	if runtime.GOOS != "windows" {
		for _, item := range []struct {
			name string
			mode os.FileMode
		}{{"logs", 0700}, {"logs/demo", 0700}, {name, 0600}} {
			info, err := logger.root.Stat(item.name)
			if err != nil || info.Mode().Perm() != item.mode {
				t.Fatalf("日志权限错误：%s，%v", item.name, err)
			}
		}
	}
	if _, err := logger.root.Stat(name); err != nil {
		t.Fatalf("logger关闭了不归自身管理的Root：%v", err)
	}
}

func TestLogRootRejectsExternalSymlinksWithoutTouchingTarget(t *testing.T) {
	for _, directoryLink := range []bool{false, true} {
		logger, directory := mirrorLogger(t, io.Discard, nil)
		outside := t.TempDir()
		original := filepath.Join(outside, "outside-file")
		if err := os.WriteFile(original, []byte("原文件"), 0600); err != nil {
			t.Fatal(err)
		}
		link, target := filepath.Join(directory, "logs"), outside
		if !directoryLink {
			if err := logger.root.MkdirAll("logs/demo", 0700); err != nil {
				t.Fatal(err)
			}
			link, target = filepath.Join(directory, "logs/demo/test.log"), original
		}
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		stream := logger.stream("demo", "test", "stdout")
		_, err := io.WriteString(stream, "安全记录\n")
		if err == nil || strings.Contains(err.Error(), outside) || logger.logPath("demo", "test") != "" {
			t.Fatalf("外链未安全拒绝：%v", err)
		}
		if err := logger.close(); err == nil {
			t.Fatal("存储失败没有保持粘性错误")
		}
		data, err := os.ReadFile(original)
		if err != nil || string(data) != "原文件" {
			t.Fatal("外链目标被修改")
		}
		entries, err := os.ReadDir(outside)
		if err != nil || len(entries) != 1 {
			t.Fatal("外链根新增了文件")
		}
	}
}

func TestLogRootWriteAndCloseErrorsCloseEveryFile(t *testing.T) {
	for _, failWrite := range []bool{false, true} {
		logger, _ := mirrorLogger(t, io.Discard, nil)
		for _, name := range []string{"first", "second"} {
			stream := logger.stream("demo", name, "stdout")
			if _, err := io.WriteString(stream, "记录\n"); err != nil {
				t.Fatal(err)
			}
			if err := stream.Close(); err != nil {
				t.Fatal(err)
			}
		}
		file := logger.files["logs/demo/first.log"]
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		if failWrite {
			_, err := io.WriteString(logger.stream("demo", "first", "stderr"), "后续记录\n")
			if err == nil || strings.Contains(err.Error(), "first.log") {
				t.Fatalf("文件写错误不安全：%v", err)
			}
		}
		if err := logger.close(); err == nil || strings.Contains(err.Error(), "first.log") {
			t.Fatalf("关闭错误缺失或不安全：%v", err)
		}
		if _, err := logger.files["logs/demo/second.log"].Stat(); !errors.Is(err, os.ErrClosed) {
			t.Fatalf("未关闭剩余日志文件：%v", err)
		}
		if err := logger.close(); err == nil {
			t.Fatal("重复close丢失错误")
		}
	}
}
