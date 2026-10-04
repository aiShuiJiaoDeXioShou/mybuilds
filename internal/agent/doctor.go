// Package agent负责真实节点诊断与受限执行，不授予控制端用户权限。
package agent

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"mybuilds/internal/process"
	"mybuilds/internal/protocol"
)

type Error struct{ Code string }

func (err *Error) Error() string { return "agent_" + err.Code }
func failure(code string) error  { return &Error{Code: code} }

var gitVersion = regexp.MustCompile(`^git version ([0-9]+(?:\.[0-9]+)+)(?:[ .-].*)?$`)
var javaVersion = regexp.MustCompile(`(?m)^(?:openjdk|java) version "([0-9]+(?:\.[0-9]+)*(?:_[0-9]+)?)"`)
var aaptVersion = regexp.MustCompile(`^Android Asset Packaging Tool \(aapt\) ([0-9]+(?:\.[0-9]+)*)(?:[- ].*)?$`)
var numberVersion = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)*$`)
var xcodeVersion = regexp.MustCompile(`(?m)^Xcode ([0-9]+(?:\.[0-9]+)*)$`)

// Doctor无需业务配置/身份，不创建目录或连接控制端。
func Doctor(parent context.Context, dataDir string) (protocol.NodeReport, error) {
	report := protocol.NodeReport{OS: runtime.GOOS, Arch: runtime.GOARCH, Capacity: 1, Tools: []protocol.ToolCheck{}}
	ctx, cancel := context.WithTimeout(parent, time.Minute)
	defer cancel()
	journal := inspectData(dataDir)
	status := "passed"
	if journal == "uninitialized" {
		status = "skipped"
	} else if journal != "" {
		status = "failed"
	}
	report.Tools = append(report.Tools, protocol.ToolCheck{Name: "node_journal", Status: status, Reason: journal})
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return report, failure("unsupported")
	}
	if ctx.Err() != nil {
		return report, failure("cancelled")
	}
	aapt, signer := androidTools()
	java := jdkTool("java")
	commands := []struct {
		name, path string
		args       []string
		version    *regexp.Regexp
	}{
		{"shell", "sh", []string{"-c", "exit 0"}, nil},
		{"git", "git", []string{"--version"}, gitVersion},
		{"java", java, []string{"-version"}, javaVersion},
		{"android_aapt2", aapt, []string{"version"}, aaptVersion},
		{"android_apksigner", signer, []string{"version"}, numberVersion},
	}
	if runtime.GOOS == "darwin" {
		commands = append(commands, struct {
			name, path string
			args       []string
			version    *regexp.Regexp
		}{"xcode", "xcodebuild", []string{"-version"}, xcodeVersion})
	}
	unsafe := ""
	for _, command := range commands {
		check := protocol.ToolCheck{Name: command.name, Status: "failed"}
		if unsafe != "" {
			check.Reason = unsafe
			report.Tools = append(report.Tools, check)
			continue
		}
		if command.path == "" {
			check.Reason = "tool_missing"
		} else {
			output, reason := toolOutput(ctx, command.path, command.args, os.TempDir())
			check.Reason = reason
			if reason == "" && command.version != nil {
				match := command.version.FindStringSubmatch(strings.TrimSpace(output))
				if command.version == numberVersion && command.version.MatchString(strings.TrimSpace(output)) {
					match = []string{output, strings.TrimSpace(output)}
				}
				if len(match) < 2 || len(match[1]) > 128 {
					check.Reason = "tool_version_invalid"
				} else {
					// Java旧式构建号也归一为数字版本，避免向节点协议传递原始文本。
					check.Version = strings.ReplaceAll(match[1], "_", ".")
				}
				if check.Reason == "" && command.name == "java" {
					major, _ := strconv.Atoi(strings.Split(check.Version, ".")[0])
					if major < 17 {
						check.Reason = "tool_incompatible"
					}
				}
			}
			if reason == "tool_error" || reason == "output_limit" || reason == "cleanup_error" || reason == "tool_timeout" || reason == "cancelled" {
				unsafe = reason
			}
		}
		if check.Reason == "tool_version_invalid" || check.Reason == "tool_incompatible" {
			unsafe = check.Reason
		}
		if check.Reason == "" {
			check.Status = "passed"
		}
		report.Tools = append(report.Tools, check)
	}
	if runtime.GOOS != "darwin" {
		report.Tools = append(report.Tools, protocol.ToolCheck{Name: "xcode", Status: "skipped", Reason: "unsupported"})
	}
	report.Tools = append(report.Tools, protocol.ToolCheck{Name: "ios_signing", Status: "skipped", Reason: "unsupported"})
	if unsafe == "cleanup_error" {
		return report, failure("cleanup_error")
	}
	if ctx.Err() != nil {
		return report, failure("cancelled")
	}
	return report, nil
}

func jdkTool(name string) string {
	if home := os.Getenv("JAVA_HOME"); home != "" {
		return filepath.Join(home, "bin", name)
	}
	executable, _ := exec.LookPath(name)
	return executable
}

func androidTools() (string, string) {
	home, root := os.Getenv("ANDROID_HOME"), os.Getenv("ANDROID_SDK_ROOT")
	if home == "" {
		home = root
	}
	if home == "" {
		return "", ""
	}
	if root != "" {
		a, ea := os.Stat(home)
		b, eb := os.Stat(root)
		if ea != nil || eb != nil || !os.SameFile(a, b) {
			return "", ""
		}
	}
	directory, err := openToolDirectory(filepath.Join(home, "build-tools"))
	if err != nil {
		return "", ""
	}
	defer directory.Close()
	entries, err := directory.ReadDir(1025)
	if err != nil && err != io.EOF {
		return "", ""
	}
	if len(entries) > 1024 {
		return "", ""
	}
	versions := []string{}
	for _, entry := range entries {
		if entry.IsDir() && numberVersion.MatchString(entry.Name()) {
			versions = append(versions, entry.Name())
		}
	}
	sort.Slice(versions, func(i, j int) bool { return versionLess(versions[i], versions[j]) })
	for i := len(versions) - 1; i >= 0; i-- {
		base := filepath.Join(home, "build-tools", versions[i])
		a, b := filepath.Join(base, "aapt2"), filepath.Join(base, "apksigner")
		if executableFile(a) && executableFile(b) {
			return a, b
		}
	}
	return "", ""
}
func versionLess(a, b string) bool {
	left, right := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < max(len(left), len(right)); i++ {
		x, y := 0, 0
		if i < len(left) {
			x, _ = strconv.Atoi(left[i])
		}
		if i < len(right) {
			y, _ = strconv.Atoi(right[i])
		}
		if x != y {
			return x < y
		}
	}
	return a < b
}
func executableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0
}

type toolBuffer struct {
	mu   sync.Mutex
	data []byte
	full bool
}

func (buffer *toolBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	n := min(len(data), (32<<10)-len(buffer.data))
	buffer.data = append(buffer.data, data[:n]...)
	if n < len(data) {
		buffer.full = true
		return n, io.ErrShortWrite
	}
	return n, nil
}

// toolOutput仅运行固定诊断命令，输出限额与九项白名单共享真实process.Run。
func toolOutput(parent context.Context, executable string, args []string, workspace string) (string, string) {
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	if ctx.Err() != nil {
		return "", "cancelled"
	}
	if executable == "" {
		return "", "tool_missing"
	}
	resolved, err := exec.LookPath(executable)
	if err != nil {
		return "", "tool_missing"
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil || !executableFile(resolved) {
		return "", "tool_missing"
	}
	env := []string{}
	for key, value := range process.HostEnvironment() {
		if strings.ContainsRune(value, 0) {
			return "", "tool_error"
		}
		env = append(env, key+"="+value)
	}
	buffer := &toolBuffer{}
	result := process.Run(ctx, process.Command{Path: resolved, Args: args, Dir: workspace, Env: env}, buffer, buffer)
	if result.CleanupFailed {
		return "", "cleanup_error"
	}
	if buffer.full {
		return "", "output_limit"
	}
	if ctx.Err() != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", "tool_timeout"
		}
		return "", "cancelled"
	}
	if !result.Started || result.Reason != "" || result.ExitCode != 0 {
		return "", "tool_error"
	}
	return string(buffer.data), ""
}
