// Package reports 只解析有界的本次JUnit字节，不访问路径或运行命令。
package reports

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"mybuilds/internal/protocol"
)

var (
	ErrInvalid   = errors.New("junit_invalid")
	ErrLimit     = errors.New("junit_limit")
	ErrSecret    = errors.New("junit_secret")
	ErrCancelled = errors.New("junit_cancelled")
)

const (
	maxXMLBytes              = 8 << 20
	maxCases           int64 = 100000
	maxDuration        int64 = 365 * 24 * 60 * 60 * 1e9
	maxCaseDuration    int64 = 24 * 60 * 60 * 1e9
	maxDiagnostics           = 20
	maxDiagnosticBytes       = 20 << 10
)

type junitFrame struct {
	name                       string
	attrs                      map[string]string
	counts                     protocol.JUnitCounts
	outcome, message, caseName string
	text                       strings.Builder
}

func hasSecret(ctx context.Context, value string, secrets []string) bool {
	for _, secret := range secrets {
		if ctx.Err() != nil {
			return false
		}
		found := secret != "" && strings.Contains(value, secret)
		if ctx.Err() != nil {
			return false
		}
		if found {
			return true
		}
	}
	return false
}
func decimalInteger(value string, limit int64) (int64, error) {
	if value == "" {
		return 0, ErrInvalid
	}
	var n int64
	for _, c := range value {
		if c < '0' || c > '9' {
			return 0, ErrInvalid
		}
		d := int64(c - '0')
		if n > (limit-d)/10 {
			return 0, ErrLimit
		}
		n = n*10 + d
	}
	return n, nil
}
func durationNS(value string, limit int64) (int64, error) {
	parts := strings.Split(value, ".")
	if len(parts) > 2 {
		return 0, ErrInvalid
	}
	seconds, err := decimalInteger(parts[0], limit/1e9)
	if err != nil {
		return 0, err
	}
	ns := seconds * 1e9
	if len(parts) == 2 {
		if len(parts[1]) == 0 || len(parts[1]) > 9 {
			return 0, ErrInvalid
		}
		fraction, err := decimalInteger(parts[1], 999999999)
		if err != nil {
			return 0, err
		}
		for i := len(parts[1]); i < 9; i++ {
			fraction *= 10
		}
		if ns > limit-fraction {
			return 0, ErrLimit
		}
		ns += fraction
	}
	return ns, nil
}
func validCounts(c protocol.JUnitCounts) error {
	if c.Tests < 0 || c.Failures < 0 || c.Errors < 0 || c.Skipped < 0 || c.DurationNS < 0 {
		return ErrInvalid
	}
	if c.Tests > maxCases || c.DurationNS > maxDuration {
		return ErrLimit
	}
	if c.Failures > c.Tests || c.Errors > c.Tests-c.Failures || c.Skipped > c.Tests-c.Failures-c.Errors {
		return ErrInvalid
	}
	return nil
}
func addCounts(a, b protocol.JUnitCounts) (protocol.JUnitCounts, error) {
	if err := validCounts(a); err != nil {
		return a, err
	}
	if err := validCounts(b); err != nil {
		return a, err
	}
	if a.Tests > maxCases-b.Tests || a.DurationNS > maxDuration-b.DurationNS {
		return a, ErrLimit
	}
	a.Tests += b.Tests
	a.Failures += b.Failures
	a.Errors += b.Errors
	a.Skipped += b.Skipped
	a.DurationNS += b.DurationNS
	return a, nil
}
func cleanDiagnostic(value string, max int) string {
	value = strings.Join(strings.FieldsFunc(value, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }), " ")
	if len(value) <= max {
		return value
	}
	value = value[:max]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
func diagnosticBytes(d protocol.JUnitDiagnostic) int {
	return len(d.PathKey) + len(d.Case) + len(d.Outcome) + len(d.Message)
}
func appendDiagnostic(out *[]protocol.JUnitDiagnostic, d protocol.JUnitDiagnostic, used *int) {
	if len(*out) >= maxDiagnostics {
		return
	}
	size := diagnosticBytes(d)
	if size > maxDiagnosticBytes-*used {
		return
	}
	*out = append(*out, d)
	*used += size
}

// MergeJUnit 的两个实际消费者先按报告Path排序并补PathKey；显示截断不跳过后续验证。
func MergeJUnit(inputs []protocol.JUnitResult) (protocol.JUnitResult, error) {
	result := protocol.JUnitResult{Diagnostics: []protocol.JUnitDiagnostic{}}
	used := 0
	for _, input := range inputs {
		counts, err := addCounts(result.Counts, input.Counts)
		if err != nil {
			return protocol.JUnitResult{}, err
		}
		result.Counts = counts
		if len(input.Diagnostics) > maxDiagnostics {
			return protocol.JUnitResult{}, ErrLimit
		}
		for _, d := range input.Diagnostics {
			if !utf8.ValidString(d.PathKey) || !utf8.ValidString(d.Case) || !utf8.ValidString(d.Message) || len(d.PathKey) > 64 || len(d.Case) > 512 || len(d.Message) > 1024 || strings.ContainsFunc(d.PathKey+d.Case+d.Message, unicode.IsControl) || d.Outcome != "failure" && d.Outcome != "error" {
				return protocol.JUnitResult{}, ErrInvalid
			}
			appendDiagnostic(&result.Diagnostics, d, &used)
		}
	}
	return result, nil
}
func allowedChild(parent, child string) bool {
	switch parent {
	case "testsuites":
		return child == "testsuite"
	case "testsuite":
		return child == "testsuite" || child == "testcase" || child == "properties" || child == "system-out" || child == "system-err"
	case "testcase":
		return child == "failure" || child == "error" || child == "skipped" || child == "properties" || child == "system-out" || child == "system-err"
	case "properties":
		return child == "property"
	}
	return false
}
func container(name string) bool {
	return name == "testsuite" || name == "testsuites" || name == "testcase" || name == "properties"
}
func xmlDeclaration(instr []byte) bool {
	decoder := xml.NewDecoder(bytes.NewReader(append(append([]byte("<declaration "), instr...), []byte("/>")...)))
	token, err := decoder.Token()
	if err != nil {
		return false
	}
	start, ok := token.(xml.StartElement)
	if !ok || start.Name.Local != "declaration" || len(start.Attr) < 1 || len(start.Attr) > 3 {
		return false
	}
	if start.Attr[0].Name != (xml.Name{Local: "version"}) || start.Attr[0].Value != "1.0" {
		return false
	}
	last := 0
	for _, a := range start.Attr[1:] {
		if a.Name.Space != "" {
			return false
		}
		switch a.Name.Local {
		case "encoding":
			if last != 0 || !strings.EqualFold(a.Value, "UTF-8") {
				return false
			}
			last = 1
		case "standalone":
			if last == 2 || a.Value != "yes" && a.Value != "no" {
				return false
			}
			last = 2
		default:
			return false
		}
	}
	token, err = decoder.Token()
	if err != nil {
		return false
	}
	if _, ok = token.(xml.EndElement); !ok {
		return false
	}
	_, err = decoder.Token()
	return err == io.EOF
}

// ParseJUnit 只接受稳定普通文件/stage的有限Reader；不会弃置后台读取goroutine。
func ParseJUnit(ctx context.Context, input io.Reader, secrets []string) (protocol.JUnitResult, error) {
	empty := protocol.JUnitResult{}
	if ctx.Err() != nil {
		return empty, ErrCancelled
	}
	if input == nil {
		return empty, ErrInvalid
	}
	var raw bytes.Buffer
	limited := io.LimitReader(input, maxXMLBytes+1)
	buffer := make([]byte, 64<<10)
	for {
		if ctx.Err() != nil {
			return empty, ErrCancelled
		}
		n, err := limited.Read(buffer)
		if ctx.Err() != nil {
			return empty, ErrCancelled
		}
		if n > 0 {
			raw.Write(buffer[:n])
			if raw.Len() > maxXMLBytes {
				return empty, ErrLimit
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil || n == 0 {
			return empty, ErrInvalid
		}
	}
	data := raw.Bytes()
	if !utf8.Valid(data) {
		return empty, ErrInvalid
	}
	if hasSecret(ctx, string(data), secrets) {
		return empty, ErrSecret
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.Strict = true
	result := protocol.JUnitResult{Diagnostics: []protocol.JUnitDiagnostic{}}
	stack := []junitFrame{}
	rootSeen := false
	declared := false
	used := 0
	cases := int64(0)
	for {
		if ctx.Err() != nil {
			return empty, ErrCancelled
		}
		before := decoder.InputOffset()
		token, err := decoder.Token()
		if ctx.Err() != nil {
			return empty, ErrCancelled
		}
		if err == io.EOF {
			if !rootSeen || len(stack) != 0 {
				return empty, ErrInvalid
			}
			return result, nil
		}
		if err != nil {
			return empty, ErrInvalid
		}
		switch token := token.(type) {
		case xml.ProcInst:
			if before != 0 || declared || token.Target != "xml" || !xmlDeclaration(token.Inst) {
				return empty, ErrInvalid
			}
			declared = true
		case xml.Directive:
			return empty, ErrInvalid
		case xml.Comment:
			if hasSecret(ctx, string(token), secrets) {
				return empty, ErrSecret
			}
		case xml.StartElement:
			if token.Name.Space != "" || len(stack) >= 64 {
				return empty, func() error {
					if token.Name.Space != "" {
						return ErrInvalid
					}
					return ErrLimit
				}()
			}
			name := token.Name.Local
			if len(stack) == 0 {
				if rootSeen || name != "testsuite" && name != "testsuites" {
					return empty, ErrInvalid
				}
				rootSeen = true
			} else if !allowedChild(stack[len(stack)-1].name, name) {
				return empty, ErrInvalid
			}
			if len(token.Attr) > 64 {
				return empty, ErrLimit
			}
			attrs := map[string]string{}
			for _, a := range token.Attr {
				if a.Name.Space != "" || a.Name.Local == "xmlns" {
					return empty, ErrInvalid
				}
				if len(a.Name.Local) > 256 || len(a.Value) > 4096 {
					return empty, ErrLimit
				}
				if _, exists := attrs[a.Name.Local]; exists {
					return empty, ErrInvalid
				}
				if hasSecret(ctx, a.Value, secrets) || hasSecret(ctx, a.Name.Local, secrets) {
					return empty, ErrSecret
				}
				attrs[a.Name.Local] = a.Value
			}
			frame := junitFrame{name: name, attrs: attrs}
			if name == "testcase" {
				cases++
				if cases > maxCases || len(attrs["name"]) > 512 {
					return empty, ErrLimit
				}
				frame.counts.Tests = 1
				frame.caseName = attrs["name"]
				if value, ok := attrs["time"]; ok {
					n, e := durationNS(value, maxCaseDuration)
					if e != nil {
						return empty, e
					}
					frame.counts.DurationNS = n
				}
			} else if name == "testsuite" || name == "testsuites" {
				if value, ok := attrs["time"]; ok {
					if _, e := durationNS(value, maxDuration); e != nil {
						return empty, e
					}
				}
			} else if name == "failure" || name == "error" || name == "skipped" {
				parent := &stack[len(stack)-1]
				if parent.outcome != "" {
					return empty, ErrInvalid
				}
				parent.outcome = name
			}
			stack = append(stack, frame)
		case xml.CharData:
			if hasSecret(ctx, string(token), secrets) {
				return empty, ErrSecret
			}
			if len(stack) == 0 {
				if len(bytes.TrimSpace(token)) != 0 {
					return empty, ErrInvalid
				}
			} else if container(stack[len(stack)-1].name) {
				if len(bytes.TrimSpace(token)) != 0 {
					return empty, ErrInvalid
				}
			} else {
				stack[len(stack)-1].text.Write(token)
			}
		case xml.EndElement:
			if len(stack) == 0 {
				return empty, ErrInvalid
			}
			frame := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			text := frame.text.String()
			if hasSecret(ctx, text, secrets) {
				return empty, ErrSecret
			}
			if frame.name == "failure" || frame.name == "error" {
				stack[len(stack)-1].message = frame.attrs["message"] + " " + text
			}
			if frame.name == "testcase" {
				switch frame.outcome {
				case "failure":
					frame.counts.Failures = 1
				case "error":
					frame.counts.Errors = 1
				case "skipped":
					frame.counts.Skipped = 1
				}
				if frame.outcome == "failure" || frame.outcome == "error" {
					caseName := cleanDiagnostic(frame.caseName, len(frame.caseName))
					message := cleanDiagnostic(frame.message, len(frame.message))
					// 合成和规范化也可能形成秘密；先检查完整值，再执行显示截断。
					if hasSecret(ctx, caseName, secrets) || hasSecret(ctx, message, secrets) {
						return empty, ErrSecret
					}
					appendDiagnostic(&result.Diagnostics, protocol.JUnitDiagnostic{Case: cleanDiagnostic(caseName, 512), Outcome: frame.outcome, Message: cleanDiagnostic(message, 1024)}, &used)
				}
			}
			if frame.name == "testsuite" || frame.name == "testsuites" {
				values := map[string]int64{"tests": frame.counts.Tests, "failures": frame.counts.Failures, "errors": frame.counts.Errors, "skipped": frame.counts.Skipped}
				for name, want := range values {
					if value, ok := frame.attrs[name]; ok {
						n, e := decimalInteger(value, maxCases)
						if e != nil {
							return empty, e
						}
						if n != want {
							return empty, ErrInvalid
						}
					}
				}
			}
			if len(stack) > 0 {
				parent := &stack[len(stack)-1]
				counts, e := addCounts(parent.counts, frame.counts)
				if e != nil {
					return empty, e
				}
				parent.counts = counts
			} else {
				result.Counts = frame.counts
			}
		default:
			return empty, ErrInvalid
		}
	}
}
