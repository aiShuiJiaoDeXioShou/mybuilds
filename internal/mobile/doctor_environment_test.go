package mobile

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDoctorExplicitEnvironmentAndLookup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("本地执行明确不支持Windows")
	}
	t.Setenv("MYBUILDS_PRIVATE_AMBIENT", "ambient-secret")
	env := map[string]string{"PATH": "/bin:/usr/bin", "SAFE_VALUE": "declared"}
	out, err := toolOutput(context.Background(), toolCommand{Workspace: t.TempDir(), Executable: "sh", Args: []string{"-c", `printf '%s:%s' "$SAFE_VALUE" "${MYBUILDS_PRIVATE_AMBIENT-unset}"`}, Env: env})
	if err != nil || out != "declared:unset" {
		t.Fatalf("显式环境隔离失败: %q %v", out, err)
	}
	if env["MYBUILDS_PRIVATE_AMBIENT"] != "" || len(env) != 2 {
		t.Fatal("调用修改了传入环境")
	}
	for _, bad := range []map[string]string{{"PATH": t.TempDir()}, {"PATH": "."}, {"PATH": "/bin", "BAD=NAME": "x"}, {"PATH": "/bin", "SAFE_VALUE": "x\x00y"}} {
		out, err = toolOutput(context.Background(), toolCommand{Executable: "sh", Workspace: t.TempDir(), Env: bad})
		if err == nil || out != "" {
			t.Fatal("非法或缺失显式工具环境被接受")
		}
	}
	out, err = toolOutput(context.Background(), toolCommand{Executable: "/bin/sh", Workspace: t.TempDir(), Args: []string{"-c", "printf absolute"}, Env: map[string]string{}})
	if err != nil || out != "absolute" {
		t.Fatal("明确绝对工具路径被拒")
	}
	dir := t.TempDir()
	if err = os.WriteFile(filepath.Join(dir, "sh"), []byte("not executable"), 0600); err != nil {
		t.Fatal(err)
	}
	out, err = toolOutput(context.Background(), toolCommand{Executable: "sh", Workspace: dir, Env: map[string]string{"PATH": dir}})
	if err == nil || !strings.Contains(err.Error(), "tool_executable_error") || out != "" {
		t.Fatal("不可执行同名文件被接受")
	}
}
