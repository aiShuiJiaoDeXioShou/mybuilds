package pipeline

import (
	"bytes"
	"errors"
	"io"
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
