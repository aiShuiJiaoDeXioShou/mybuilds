package mobile

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"mybuilds/internal/process"
)

type DoctorCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Version string `json:"version,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

type toolCommand struct {
	Workspace, Executable string
	Args, ExtraEnvNames   []string
}

var toolEnvName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var errToolOutputLimit = errors.New("tool_output_limit")

// toolOutput 只内部返回工具原文，公开诊断由平台检查严格解析后生成。
func toolOutput(ctx context.Context, command toolCommand) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return "", toolContextError(ctx)
	}
	workspace := command.Workspace
	if workspace == "" {
		var err error
		workspace, err = os.Getwd()
		if err != nil {
			return "", errors.New("tool_directory_error")
		}
	}
	workspace, err := filepath.Abs(workspace)
	if err == nil {
		workspace, err = filepath.EvalSymlinks(workspace)
	}
	if err != nil {
		return "", errors.New("tool_directory_error")
	}
	info, err := os.Stat(workspace)
	if err != nil || !info.IsDir() {
		return "", errors.New("tool_directory_error")
	}
	executable := command.Executable
	if !filepath.IsAbs(executable) && strings.ContainsAny(executable, `/\`) {
		executable = filepath.Join(workspace, executable)
	}
	executable, err = exec.LookPath(executable)
	if err != nil {
		return "", errors.New("tool_executable_error")
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return "", errors.New("tool_executable_error")
	}
	env := process.HostEnvironment()
	for _, name := range command.ExtraEnvNames {
		if !toolEnvName.MatchString(name) {
			return "", errors.New("tool_environment_error")
		}
		value, ok := os.LookupEnv(name)
		if !ok {
			return "", errors.New("tool_environment_error")
		}
		env[name] = value
	}
	values := make([]string, 0, len(env))
	for name, value := range env {
		if strings.ContainsRune(value, 0) {
			return "", errors.New("tool_environment_error")
		}
		values = append(values, name+"="+value)
	}
	slices.Sort(values)
	output := &toolBuffer{}
	result := process.Run(ctx, process.Command{Path: executable, Args: command.Args, Dir: workspace, Env: values}, output, output)
	output.mu.Lock()
	defer output.mu.Unlock()
	if result.CleanupFailed {
		return "", errors.New("tool_cleanup_error")
	}
	if ctx.Err() != nil {
		return "", toolContextError(ctx)
	}
	if output.overflow {
		return "", errToolOutputLimit
	}
	if !result.Started || result.Reason != "" || result.ExitCode != 0 {
		return "", errors.New("tool_execution_error")
	}
	return string(output.data), nil
}

func toolContextError(ctx context.Context) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return errors.New("tool_timeout")
	}
	return errors.New("tool_cancelled")
}

// 两流由进程复制 goroutine 同时写入，共用锁和总限额；超限沿既有执行器取消本组。
type toolBuffer struct {
	mu       sync.Mutex
	data     []byte
	overflow bool
}

func (output *toolBuffer) Write(data []byte) (int, error) {
	output.mu.Lock()
	defer output.mu.Unlock()
	n := min(len(data), 32*1024-len(output.data))
	output.data = append(output.data, data[:n]...)
	if n != len(data) {
		output.overflow = true
		return n, errToolOutputLimit
	}
	return n, nil
}
