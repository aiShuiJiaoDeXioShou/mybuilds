//go:build darwin || linux

package process

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestStaleGroupMemberHelper(t *testing.T) {
	if os.Getenv("MYBUILDS_STALE_GROUP_HELPER") != "member" {
		return
	}
	terminated := make(chan os.Signal, 1)
	signal.Notify(terminated, syscall.SIGTERM)
	if os.WriteFile(os.Getenv("OWN_STARTED"), []byte("ready"), 0600) != nil {
		os.Exit(3)
	}
	select {
	case <-terminated:
		// 只有原组清理之后的成员阶段才能向本独立SID送TERM。
		if os.WriteFile(os.Getenv("OWN_TERM"), []byte("term"), 0600) != nil {
			os.Exit(4)
		}
		os.Exit(0)
	case <-time.After(20 * time.Second):
		os.Exit(5)
	}
}

func TestScopeStopRechecksLateReapedGroup(t *testing.T) {
	for _, name := range []string{"late_reap", "group_still_present", "uncertain", "unknown_alive"} {
		t.Run(name, func(t *testing.T) {
			// 先启动无关对象，既不属于本次父链，也不继承本次标记。
			unrelated := exec.Command("/bin/sleep", "30")
			if err := unrelated.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = unrelated.Process.Kill(); _ = unrelated.Wait() })
			unrelatedBirth, live, err := readProcessBirth(unrelated.Process.Pid)
			if err != nil || !live {
				t.Fatal("无关自有对象没有真实birth")
			}
			scope, err := newProcessScope()
			if err != nil {
				t.Fatal(err)
			}
			var root *exec.Cmd
			prepareUntil := time.Now().Add(time.Second)
			for attempt := 0; attempt < 32 && time.Now().Before(prepareUntil); attempt++ {
				candidate := exec.Command("/bin/sleep", "30")
				candidate.Env = scope.environment(nil)
				candidate.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
				if err = candidate.Start(); err != nil {
					t.Fatal(err)
				}
				birth, exists, readErr := readProcessBirth(candidate.Process.Pid)
				if readErr == nil && exists && birthBefore(unrelatedBirth, birth) {
					root = candidate
					break
				}
				_ = candidate.Process.Kill()
				_ = candidate.Wait()
				if readErr != nil || !exists {
					t.Fatal("自有root准备期间实际birth不可读")
				}
				// Linux birth粒度为tick；短poll只准备实际前置，不能推断停止。
				time.Sleep(5 * time.Millisecond)
			}
			if root == nil {
				t.Fatal("fixture未能在1秒/32次内形成严格birth先后")
			}
			rootWaited := false
			t.Cleanup(func() {
				if !rootWaited {
					_ = root.Process.Kill()
					_ = root.Wait()
				}
			})
			scope.bind(root.Process.Pid)
			if scope.uncertain {
				t.Fatal("本次root没有实际kernel身份")
			}
			if !birthBefore(unrelatedBirth, scope.rootBirth) {
				t.Fatal("fixture实际严格birth前置变化")
			}
			t.Log("strict_birth", unrelatedBirth, scope.rootBirth)
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			directory := t.TempDir()
			started, term := filepath.Join(directory, "started"), filepath.Join(directory, "term")
			member := exec.Command(executable, "-test.run=^TestStaleGroupMemberHelper$")
			member.Env = scope.environment([]string{"MYBUILDS_STALE_GROUP_HELPER=member", "OWN_STARTED=" + started, "OWN_TERM=" + term})
			member.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
			if err = member.Start(); err != nil {
				t.Fatal(err)
			}
			memberDone := make(chan struct{})
			go func() { _ = member.Wait(); close(memberDone) }()
			t.Cleanup(func() { _ = member.Process.Kill(); <-memberDone })
			awaitStaleGroupReceipt(t, started, "ready")
			group, err := syscall.Getpgid(member.Process.Pid)
			if err != nil || group != member.Process.Pid || group == root.Process.Pid {
				t.Fatal("成员未产生真实独立组")
			}
			scope.discover()
			if _, known := scope.members[member.Process.Pid]; !known {
				t.Fatal("成员没有真实标记归属")
			}
			if name == "uncertain" {
				scope.uncertain = true
			}
			if name == "unknown_alive" {
				// 已有未知保护不能因原组随后Gone而被清除，也不能向它送信号。
				scope.unknown[unrelated.Process.Pid] = unrelatedBirth
			}
			stopped := make(chan bool, 1)
			go func() { stopped <- scope.stop() }()
			awaitStaleGroupReceipt(t, term, "term")
			// 此真实回执证明首次stopProcessGroup已经结束；主测试尚未Wait，
			// 原组仍有同birth对象。没有用固定sleep猜测清理阶段。
			actual, exists, err := readProcessBirth(root.Process.Pid)
			groupErr := signalProcessGroup(root.Process.Pid, 0)
			if err != nil || !exists || actual != scope.rootBirth || (groupErr != nil && !errors.Is(groupErr, syscall.EPERM)) {
				t.Fatal("首次窗口之后原组未保留真实存在证据")
			}
			if name != "group_still_present" {
				_ = root.Wait()
				rootWaited = true
				if !errors.Is(signalProcessGroup(root.Process.Pid, 0), syscall.ESRCH) {
					t.Fatal("唯一Wait后原组尚未真正Gone")
				}
			}
			select {
			case <-memberDone:
			case <-time.After(5 * time.Second):
				t.Fatal("真实TERM成员未完成唯一Wait")
			}
			var result bool
			select {
			case result = <-stopped:
			case <-time.After(5 * time.Second):
				t.Fatal("scope超过原清理窗口仍未返回")
			}
			if result != (name == "late_reap") {
				t.Fatal("最终实际原组证据/未知保护判定错误", result)
			}
			if name == "group_still_present" && errors.Is(signalProcessGroup(root.Process.Pid, 0), syscall.ESRCH) {
				t.Fatal("负门未保留真实原组")
			}
			if name == "unknown_alive" && scope.unknown[unrelated.Process.Pid] != unrelatedBirth {
				t.Fatal("仍活未知对象的保护被清除")
			}
			actual, exists, err = readProcessBirth(unrelated.Process.Pid)
			if err != nil || !exists || actual != unrelatedBirth || syscall.Kill(unrelated.Process.Pid, 0) != nil {
				t.Fatal("无关同用户对象被误信号或误停止")
			}
		})
	}
}

func awaitStaleGroupReceipt(t *testing.T, path, expected string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		file, err := os.Open(path)
		if err == nil {
			data, readErr := io.ReadAll(io.LimitReader(file, 1025))
			_ = file.Close()
			if readErr != nil || len(data) > 1024 {
				t.Fatal("自有阶段回执读取失败或超限")
			}
			if string(data) == expected {
				return
			}
		} else if !os.IsNotExist(err) {
			t.Fatal("自有阶段回执不可读")
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("未取得真实自有阶段回执")
}
