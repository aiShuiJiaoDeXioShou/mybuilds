//go:build darwin || linux

package client

import (
	"bytes"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"testing"
)

func TestInitWriteFailureCleanup(t *testing.T) {
	if os.Getenv("MYBUILDS_TEST_WRITE_LIMIT") == "1" {
		// 子进程限制文件大小，真实触发部分写入而不影响其他测试。
		signal.Ignore(syscall.SIGXFSZ)
		var limit syscall.Rlimit
		if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &limit); err != nil {
			t.Fatal(err)
		}
		limit.Cur = 1024
		if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &limit); err != nil {
			t.Fatal(err)
		}
		if err := createConfig("mybuilds.yml", bytes.Repeat([]byte("x"), 2048)); err == nil {
			t.Fatal("部分写入应返回错误")
		}
		if _, err := os.Lstat("mybuilds.yml"); !os.IsNotExist(err) {
			t.Fatalf("部分写入遗留配置：%v", err)
		}
		return
	}
	process := exec.Command(os.Args[0], "-test.run=^TestInitWriteFailureCleanup$")
	process.Dir = t.TempDir()
	process.Env = append(os.Environ(), "MYBUILDS_TEST_WRITE_LIMIT=1")
	if output, err := process.CombinedOutput(); err != nil {
		t.Fatalf("写入失败清理检查：%s，%v", output, err)
	}
}
