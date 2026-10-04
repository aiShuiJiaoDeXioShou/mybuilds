//go:build darwin || linux

package mobile

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// 当前测试二进制执行真实工具行为，不另建模拟执行器。
func TestDoctorToolHelper(t *testing.T) {
	if len(os.Args) < 2 || !strings.HasPrefix(os.Args[len(os.Args)-1], "doctor-helper=") {
		return
	}
	mode := strings.TrimPrefix(os.Args[len(os.Args)-1], "doctor-helper=")
	switch mode {
	case "environment":
		cwd, _ := os.Getwd()
		_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"cwd": cwd, "extra": os.Getenv("DOCTOR_EXTRA"), "unknown": os.Getenv("DOCTOR_UNKNOWN")})
		fmt.Fprint(os.Stderr, "stderr")
	case "exact", "overflow":
		fmt.Fprint(os.Stdout, strings.Repeat("x", 16*1024))
		fmt.Fprint(os.Stderr, strings.Repeat("x", 16*1024))
		if mode == "overflow" {
			fmt.Fprint(os.Stdout, "x")
		}
	case "blocked":
		signal.Ignore(syscall.SIGTERM)
		_ = os.WriteFile("pid", []byte(strconv.Itoa(os.Getpid())), 0600)
		for {
			time.Sleep(time.Second)
		}
	case "failure":
		fmt.Fprint(os.Stderr, "PRIVATE_TOOL_OUTPUT")
		os.Exit(7)
	default:
		os.Exit(8)
	}
	os.Exit(0)
}

func doctorHelperCommand(t *testing.T, mode string) toolCommand {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return toolCommand{Workspace: t.TempDir(), Executable: executable, Args: []string{"-test.run=^TestDoctorToolHelper$", "--", "doctor-helper=" + mode}}
}

func TestToolOutputEnvironmentAndCombinedStreams(t *testing.T) {
	t.Setenv("DOCTOR_UNKNOWN", "PRIVATE_UNKNOWN")
	t.Setenv("DOCTOR_EXTRA", "value $() {{literal}}\nline")
	command := doctorHelperCommand(t, "environment")
	command.ExtraEnvNames = []string{"DOCTOR_EXTRA", "DOCTOR_EXTRA"}
	output, err := toolOutput(context.Background(), command)
	if err != nil || !strings.Contains(output, "stderr") {
		t.Fatalf("工具未返回合并输出: %v", err)
	}
	var environment map[string]string
	if err := json.NewDecoder(strings.NewReader(output[strings.IndexByte(output, '{') : strings.LastIndexByte(output, '}')+1])).Decode(&environment); err != nil {
		t.Fatal("工具环境记录不能解析")
	}
	workspace, _ := filepath.EvalSymlinks(command.Workspace)
	if environment["cwd"] != workspace || environment["extra"] != os.Getenv("DOCTOR_EXTRA") || environment["unknown"] != "" {
		t.Fatal("工具目录或限定环境错误")
	}
}

func TestToolOutputRejectsUnsafeInputsAndFailureOutput(t *testing.T) {
	for _, modify := range []func(*toolCommand){
		func(c *toolCommand) { c.ExtraEnvNames = []string{"INVALID=PRIVATE_NAME"} },
		func(c *toolCommand) { c.ExtraEnvNames = []string{"MISSING_DOCTOR_PRIVATE_ENV"} },
		func(c *toolCommand) { c.ExtraEnvNames = []string{"INVALID\x00PRIVATE_NAME"} },
		func(c *toolCommand) { c.Workspace = filepath.Join(c.Workspace, "PRIVATE_MISSING_DIRECTORY") },
		func(c *toolCommand) { c.Executable = "PRIVATE_MISSING_TOOL" },
		func(c *toolCommand) { c.Args = []string{"PRIVATE_ARG\x00"} },
		func(c *toolCommand) {},
	} {
		command := doctorHelperCommand(t, "failure")
		modify(&command)
		output, err := toolOutput(context.Background(), command)
		if err == nil || output != "" || strings.Contains(err.Error(), "PRIVATE") {
			t.Fatal("工具错误泄露输入或原输出")
		}
	}
}

func TestToolOutputCombinedLimit(t *testing.T) {
	for _, mode := range []string{"exact", "overflow"} {
		output, err := toolOutput(context.Background(), doctorHelperCommand(t, mode))
		if mode == "exact" && (err != nil || len(output) != 32*1024) {
			t.Fatalf("限额内的两流输出错误: %d %v", len(output), err)
		}
		if mode == "overflow" && (err == nil || output != "") {
			t.Fatal("超过合并限额仍返回工具输出")
		}
	}
}

func requireDoctorProcessStopped(t *testing.T, workspace string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(workspace, "pid"))
	if err != nil {
		t.Fatal("真实阻塞工具尚未启动")
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil || !isDoctorProcessGone(pid) {
		t.Fatal("本次工具进程未停止")
	}
}

func isDoctorProcessGone(pid int) bool { return syscall.Kill(pid, 0) == syscall.ESRCH }

func TestToolOutputCancellationAndEarlierDeadline(t *testing.T) {
	command := doctorHelperCommand(t, "blocked")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if output, err := toolOutput(ctx, command); err == nil || output != "" {
		t.Fatal("开始前取消仍成功")
	}
	if _, err := os.Stat(filepath.Join(command.Workspace, "pid")); !os.IsNotExist(err) {
		t.Fatal("开始前取消启动了工具")
	}
	ctx, cancel = context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	if output, err := toolOutput(ctx, command); err == nil || output != "" {
		t.Fatal("提前截止仍成功")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("工具未继承更早预算")
	}
	requireDoctorProcessStopped(t, command.Workspace)
}

func TestToolOutputOwnTimeout(t *testing.T) {
	command := doctorHelperCommand(t, "blocked")
	start := time.Now()
	output, err := toolOutput(context.Background(), command)
	elapsed := time.Since(start)
	if err == nil || output != "" || elapsed < 15*time.Second || elapsed > 18*time.Second {
		t.Fatalf("单工具预算错误: %v %v", elapsed, err)
	}
	requireDoctorProcessStopped(t, command.Workspace)
}

func TestDoctorCheckJSONUsesSafeFields(t *testing.T) {
	data, err := json.Marshal(DoctorCheck{Name: "android_java", Status: "skipped"})
	if err != nil || string(data) != `{"name":"android_java","status":"skipped"}` {
		t.Fatal("公开记录字段或省略规则错误")
	}
}
