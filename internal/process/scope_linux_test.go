//go:build linux

package process

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestScopeUnreadableProcessHelper(t *testing.T) {
	if os.Getenv("MYBUILDS_SCOPE_DENY") == "" {
		return
	}
	if err := unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0); err != nil {
		os.Exit(2)
	}
	fmt.Printf("READY %d\n", os.Getpid())
	for {
		time.Sleep(time.Second)
	}
}
func TestScopeUnreadableKnownGroupStopsAfterGone(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	output := newReadyOutput()
	done := make(chan Result, 1)
	go func() {
		done <- Run(ctx, Command{Path: executable, Args: []string{"-test.run=^TestScopeUnreadableProcessHelper$"}, Env: []string{"MYBUILDS_SCOPE_DENY=1"}}, output, io.Discard)
	}()
	pid := awaitReady(t, output)
	cancel()
	result := awaitShellResult(t, done)
	if result.CleanupFailed || result.Reason != "cancelled" {
		t.Fatal("已知本组真实gone后仍有永久权限不确定", result)
	}
	assertProcessStopped(t, pid)
}
func TestScopePidfdBindsActualIdentity(t *testing.T) {
	// 自己进程的错误birth不发信号；另一个实际sleep对象的正确birth由共享门验证。
	birth, live, err := readProcessBirth(os.Getpid())
	if err != nil || !live {
		t.Fatal(err)
	}
	birth.first++
	stopped, err := signalOwnedProcess(os.Getpid(), birth, syscall.SIGKILL)
	if err != nil || !stopped {
		t.Fatal("错误birth未拒绝", err)
	}
}

func TestScopeDetachedPermissionHelper(t *testing.T) {
	dir := os.Getenv("MYBUILDS_SCOPE_DIAG_DIR")
	if dir == "" {
		return
	}
	if _, err := unix.Setsid(); err != nil {
		os.Exit(2)
	}
	if err := unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0); err != nil {
		os.Exit(3)
	}
	if os.WriteFile(filepath.Join(dir, "ready"), []byte(strconv.Itoa(os.Getpid())), 0600) != nil {
		os.Exit(4)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "stop")); err == nil {
			os.Exit(0)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestScopePermissionCandidateGoneClearsUnknown(t *testing.T) {
	scope, pid, birth, dir := permissionScopeCandidate(t)
	scope.discover()
	if scope.uncertain || scope.unknown[pid] != birth {
		t.Fatal("单对象权限暂态被当成永久全表错误")
	}
	if err := os.WriteFile(filepath.Join(dir, "stop"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	awaitPermissionCandidateGone(t, pid, birth)
	scope.discover()
	if scope.uncertain {
		t.Fatal("同对象真实gone后仍保留永久不确定")
	}
	if !scope.stop() {
		t.Fatal("暂态消失后未确认本次真实组停止")
	}
	if _, exists := scope.unknown[pid]; exists {
		t.Fatal("停止末尾没有复核并清除已gone候选")
	}
}

func TestScopePermissionCandidateAliveKeepsGuardWithoutSignal(t *testing.T) {
	scope, pid, birth, _ := permissionScopeCandidate(t)
	scope.discover()
	if scope.uncertain || scope.unknown[pid] != birth {
		t.Fatal("未保存精确不可读候选")
	}
	if _, owned := scope.members[pid]; owned {
		t.Fatal("候选曾被父链或nonce证明归属")
	}
	if scope.stop() {
		t.Fatal("活着的不可读候选被错误确认")
	}
	actual, live, err := readProcessBirth(pid)
	if err != nil || !live || actual != birth {
		t.Fatal("未证明归属的候选被发信号或混淆birth")
	}
}

// 真setsid/PR_SET_DUMPABLE子进程；只由自己的文件握手退出，不靠模糊poll推gone。
func permissionScopeCandidate(t *testing.T) (*processScope, int, processBirth, string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("实际权限拒读门要求普通用户")
	}
	dir := t.TempDir()
	scope, err := newProcessScope()
	if err != nil {
		t.Fatal(err)
	}
	root := exec.Command("/bin/sleep", "30")
	root.Env = scope.environment(nil)
	root.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := root.Start(); err != nil {
		t.Fatal(err)
	}
	scope.bind(root.Process.Pid)
	waited := make(chan struct{})
	go func() { _ = root.Wait(); close(waited) }()
	t.Cleanup(func() {
		_ = root.Process.Kill()
		select {
		case <-waited:
		case <-time.After(5 * time.Second):
			t.Error("自产root未回收")
		}
	})
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	launcher := exec.Command("/bin/sh", "-c", `"$1" -test.run=^TestScopeDetachedPermissionHelper$ >/dev/null 2>&1 &`, "sh", executable)
	launcher.Env = append(scope.environment(nil), "MYBUILDS_SCOPE_DIAG_DIR="+dir)
	if launcher.Run() != nil {
		t.Fatal("自产helper启动失败")
	}
	until := time.Now().Add(5 * time.Second)
	pid := 0
	for pid == 0 {
		data, err := os.ReadFile(filepath.Join(dir, "ready"))
		if err == nil {
			pid, _ = strconv.Atoi(string(data))
		}
		if !time.Now().Before(until) {
			t.Fatal("helper没有实际ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	member, live, err := linuxProcess(pid)
	session, sessionErr := unix.Getsid(pid)
	if err != nil || !live || member.parent != 1 || member.group != pid || sessionErr != nil || session != pid || birthBefore(member.birth, scope.rootBirth) {
		t.Fatal("helper没有实际脱离父链")
	}
	file, err := os.Open("/proc/" + strconv.Itoa(pid) + "/environ")
	if file != nil {
		file.Close()
	}
	if !errors.Is(err, syscall.EACCES) && !errors.Is(err, syscall.EPERM) {
		t.Fatal("没有实际权限拒读")
	}
	t.Cleanup(func() {
		_ = os.WriteFile(filepath.Join(dir, "stop"), nil, 0600)
		awaitPermissionCandidateGone(t, pid, member.birth)
	})
	return scope, pid, member.birth, dir
}

func awaitPermissionCandidateGone(t *testing.T, pid int, birth processBirth) {
	t.Helper()
	until := time.Now().Add(5 * time.Second)
	for {
		actual, exists, err := readProcessBirth(pid)
		if err != nil {
			t.Fatal(err)
		}
		if !exists || actual != birth {
			return
		}
		if !time.Now().Before(until) {
			t.Fatal("自产helper尚未实际gone")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
