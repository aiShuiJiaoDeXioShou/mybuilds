package pipeline

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mybuilds/internal/protocol"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const logChunkSize = 4096

var (
	errLogOutput = errors.New("日志输出失败")
	errLogClosed = errors.New("日志流已关闭")
)

type runLogger struct {
	output  io.Writer
	secrets [][]byte
	mu      sync.Mutex
	err     error
	root    *os.Root
	files   map[string]*os.File
	closed  bool
	remote  *remoteRun
}

type logStream struct {
	logger              *runLogger
	build, step, source string
	mu                  sync.Mutex
	pending, line       []byte
	closed              bool
	phase               string
	index               int
}

func newRunLogger(output io.Writer, secrets []string) *runLogger {
	if output == nil {
		output = io.Discard
	}
	logger := &runLogger{output: output}
	seen := make(map[string]bool)
	for _, value := range secrets {
		if value != "" && !seen[value] {
			seen[value] = true
			logger.secrets = append(logger.secrets, []byte(value))
		}
	}
	sort.SliceStable(logger.secrets, func(i, j int) bool { return len(logger.secrets[i]) > len(logger.secrets[j]) })
	return logger
}

func (logger *runLogger) stream(build, step, stream string) io.WriteCloser {
	return &logStream{logger: logger, build: build, step: step, source: stream}
}

func (logger *runLogger) stepStream(build string, step preparedStep, source string) io.WriteCloser {
	return &logStream{logger: logger, build: build, step: step.step.Name, source: source, phase: step.phase, index: step.index}
}

func (logger *runLogger) failure() error {
	logger.mu.Lock()
	defer logger.mu.Unlock()
	if logger.closed && logger.err == nil {
		return errLogClosed
	}
	return logger.err
}

func (logger *runLogger) logPath(build, step string) string {
	logger.mu.Lock()
	defer logger.mu.Unlock()
	name := path.Join("logs", build, step+".log")
	if logger.files[name] != nil {
		return name
	}
	return ""
}

func (logger *runLogger) close() error {
	logger.mu.Lock()
	defer logger.mu.Unlock()
	if logger.closed {
		return logger.err
	}
	logger.closed = true
	// 即使先前写入失败，也尝试关闭全部自身文件；Root归Run管理。
	for _, file := range logger.files {
		if err := file.Close(); err != nil {
			logger.err = errLogOutput
		}
	}
	return logger.err
}

func (stream *logStream) Write(data []byte) (int, error) {
	stream.mu.Lock()
	defer stream.mu.Unlock()
	if stream.closed {
		return 0, errLogClosed
	}
	if err := stream.logger.failure(); err != nil {
		return 0, err
	}
	accepted := 0
	for accepted < len(data) {
		size := min(logChunkSize, len(data)-accepted)
		stream.pending = append(stream.pending, data[accepted:accepted+size]...)
		accepted += size
		if err := stream.redact(false); err != nil {
			return accepted, err
		}
	}
	return accepted, nil
}

func (stream *logStream) Close() error {
	stream.mu.Lock()
	defer stream.mu.Unlock()
	if stream.closed {
		return stream.logger.failure()
	}
	stream.closed = true
	if err := stream.logger.failure(); err != nil {
		return err
	}
	if err := stream.redact(true); err != nil {
		return err
	}
	if len(stream.line) > 0 {
		return stream.flush()
	}
	return nil
}

func (stream *logStream) redact(final bool) error {
	consumed := 0
	for consumed < len(stream.pending) {
		remaining := stream.pending[consumed:]
		matched, undecided := 0, false
		// ponytail: 逐项匹配显式密钥，引用数量显著增多时再采用多模式匹配。
		for _, secret := range stream.logger.secrets {
			if bytes.HasPrefix(remaining, secret) {
				matched = len(secret)
				break
			}
			if !final && len(remaining) < len(secret) && bytes.HasPrefix(secret, remaining) {
				undecided = true
				break
			}
		}
		if undecided {
			break
		}
		if matched > 0 {
			for _, char := range []byte("[REDACTED]") {
				if err := stream.appendByte(char); err != nil {
					return err
				}
			}
			consumed += matched
		} else {
			if err := stream.appendByte(remaining[0]); err != nil {
				return err
			}
			consumed++
		}
	}
	copy(stream.pending, stream.pending[consumed:])
	stream.pending = stream.pending[:len(stream.pending)-consumed]
	return nil
}

func (stream *logStream) appendByte(char byte) error {
	if char == '\n' {
		return stream.flush()
	}
	stream.line = append(stream.line, char)
	// 多字节字符保持完整；无效字节最多额外缓冲一个 UTF-8 字符。
	if len(stream.line) >= logChunkSize && (utf8.Valid(stream.line) || len(stream.line) >= logChunkSize+utf8.UTFMax) {
		return stream.flush()
	}
	return nil
}

func (stream *logStream) flush() error {
	logger := stream.logger
	logger.mu.Lock()
	defer logger.mu.Unlock()
	defer func() {
		if logger.err != nil && logger.remote != nil {
			logger.remote.fail("log_error")
		}
	}()
	if logger.err != nil {
		return logger.err
	}
	if logger.closed {
		return errLogClosed
	}
	now := time.Now().UTC()
	if logger.remote != nil {
		p := protocol.LogRecord{UTC: now, Build: stream.build, Phase: stream.phase, Step: stream.step, Index: stream.index, Stream: stream.source, Text: strings.ToValidUTF8(string(stream.line), "�")}
		if err := logger.remote.options.Log(logger.remote.authority, p); err != nil {
			logger.remote.fail("log_error")
			logger.err = errLogOutput
			return logger.err
		}
	}
	record := fmt.Sprintf("[%s] [build=%s] [step=%s] [stream=%s] %s\n", now.Format(time.RFC3339Nano), stream.build, stream.step, stream.source, stream.line)
	written, err := io.WriteString(logger.output, record)
	if err != nil || written != len(record) {
		logger.err = errLogOutput
		return logger.err
	}
	if logger.root != nil {
		name := path.Join("logs", stream.build, stream.step+".log")
		file := logger.files[name]
		if file == nil {
			if err := logger.root.MkdirAll(path.Dir(name), 0700); err != nil {
				logger.err = errLogOutput
				return logger.err
			}
			file, err = logger.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if err != nil {
				logger.err = errLogOutput
				return logger.err
			}
			if logger.files == nil {
				logger.files = make(map[string]*os.File)
			}
			logger.files[name] = file
		}
		if written, err := io.WriteString(file, record); err != nil || written != len(record) {
			logger.err = errLogOutput
			return logger.err
		}
	}
	stream.line = stream.line[:0]
	return nil
}
