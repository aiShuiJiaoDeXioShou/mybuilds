//go:build darwin

package process

import (
	"context"
	"encoding/binary"
	"encoding/json"
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

type darwinOrphanExecEvidence struct {
	PID                     int
	BirthFirst, BirthSecond uint64
	Unique, Parent          uint64
	Version, OriginalParent int32
}

func TestScopeDarwinOrphanExecHelper(t *testing.T) {
	mode := os.Getenv("MYBUILDS_DARWIN_ORPHAN_HELPER")
	if mode == "" {
		return
	}
	if mode == "parent" {
		executable, err := os.Executable()
		if err != nil {
			os.Exit(3)
		}
		child := exec.Command(executable, "-test.run=^TestScopeDarwinOrphanExecHelper$")
		child.Env = []string{"MYBUILDS_DARWIN_ORPHAN_HELPER=child", "OWN_STARTED=" + os.Getenv("OWN_STARTED"), "OWN_ORPHAN=" + os.Getenv("OWN_ORPHAN"), scopeEnvironment + "=" + os.Getenv(scopeEnvironment)}
		child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if child.Start() != nil {
			os.Exit(4)
		}
		// 主测试只Wait这一个父helper；子helper特意由init回收，不能等待其退出。
		_ = child.Process.Release()
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(os.Getenv("OWN_STARTED")); err == nil {
				os.Exit(0)
			}
			time.Sleep(10 * time.Millisecond)
		}
		os.Exit(5)
	}
	if mode != "child" {
		os.Exit(6)
	}
	if os.WriteFile(os.Getenv("OWN_STARTED"), []byte("ready"), 0600) != nil {
		os.Exit(7)
	}
	deadline := time.Now().Add(10 * time.Second)
	for os.Getppid() != 1 {
		if !time.Now().Before(deadline) {
			os.Exit(8)
		}
		time.Sleep(10 * time.Millisecond)
	}
	// 必须先真实观察PPID1，再保存exec前身份；不靠父进程已经Wait来猜重父。
	birth, live, err := readProcessBirth(os.Getpid())
	identity, identityErr := darwinIdentity(os.Getpid())
	if err != nil || identityErr != nil || !live {
		os.Exit(9)
	}
	evidence := darwinOrphanExecEvidence{os.Getpid(), birth.first, birth.second, identity.unique, identity.parent, identity.version, identity.originalParent}
	data, err := json.Marshal(evidence)
	if err != nil || len(data) > 1024 || os.WriteFile(os.Getenv("OWN_ORPHAN"), data, 0600) != nil {
		os.Exit(10)
	}
	if syscall.Exec("/bin/sleep", []string{"sleep", "20"}, os.Environ()) != nil {
		os.Exit(11)
	}
}

func TestScopeDarwinOrphanExecAfterParentGoneKeepsUnknown(t *testing.T) {
	scope, err := newProcessScope()
	if err != nil {
		t.Fatal(err)
	}
	root := exec.Command("/bin/sleep", "20")
	root.Env = scope.environment(nil)
	root.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err = root.Start(); err != nil {
		t.Fatal(err)
	}
	rootDone := make(chan struct{})
	go func() { _ = root.Wait(); close(rootDone) }()
	t.Cleanup(func() { _ = root.Process.Kill(); <-rootDone })
	scope.bind(root.Process.Pid)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	started, orphan := filepath.Join(directory, "started"), filepath.Join(directory, "orphan")
	parent := exec.Command(executable, "-test.run=^TestScopeDarwinOrphanExecHelper$")
	parent.Env = scope.environment([]string{"MYBUILDS_DARWIN_ORPHAN_HELPER=parent", "OWN_STARTED=" + started, "OWN_ORPHAN=" + orphan})
	parent.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err = parent.Run(); err != nil {
		t.Fatal("自有父helper未正常退出", err)
	}
	var evidence darwinOrphanExecEvidence
	deadline := time.Now().Add(10 * time.Second)
	for {
		file, openErr := os.Open(orphan)
		if openErr == nil {
			data, readErr := io.ReadAll(io.LimitReader(file, 1025))
			_ = file.Close()
			if readErr != nil || len(data) > 1024 {
				t.Fatal("自有helper证据超限或读取失败")
			}
			if json.Unmarshal(data, &evidence) == nil {
				break
			}
		} else if !os.IsNotExist(openErr) {
			t.Fatal("自有helper证据不可读")
		}
		if !time.Now().Before(deadline) {
			t.Fatal("未取得真实重父后exec前证据")
		}
		time.Sleep(10 * time.Millisecond)
	}
	birth := processBirth{evidence.BirthFirst, evidence.BirthSecond}
	if evidence.PID <= 1 || birth == (processBirth{}) || evidence.Unique == 0 {
		t.Fatal("自有helper身份无效")
	}
	t.Cleanup(func() {
		// 只清理由本测试helper交付、且仍为同birth/unique对象的PID，不按旧PID盲杀。
		identity, identityErr := darwinIdentity(evidence.PID)
		actual, live, birthErr := readProcessBirth(evidence.PID)
		if identityErr == nil && birthErr == nil && live && actual == birth && identity.unique == evidence.Unique {
			_ = syscall.Kill(evidence.PID, syscall.SIGKILL)
			until := time.Now().Add(5 * time.Second)
			for time.Now().Before(until) {
				current, exists, readErr := readProcessBirth(evidence.PID)
				if readErr == nil && (!exists || current != birth) {
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
			t.Error("自有孤儿helper清理未确认Gone")
		}
	})
	var identity darwinProcessIdentity
	var member scopeMember
	for {
		identity, err = darwinIdentity(evidence.PID)
		init, identityErr := darwinIdentity(1)
		infos, infoErr := unix.SysctlKinfoProcSlice("kern.proc.pid", evidence.PID)
		if err != nil || identityErr != nil || infoErr != nil || len(infos) != 1 {
			t.Fatal("实际exec对象身份不可读")
		}
		member = darwinMember(&infos[0])
		if identity.version != evidence.Version {
			if member.parent != 1 || member.birth != birth || identity.unique != evidence.Unique || identity.parent != init.unique || evidence.Parent == init.unique || identity.originalParent != evidence.OriginalParent || identity.originalParent == init.version {
				t.Fatal("exec后实际身份未形成所需孤儿反例")
			}
			break
		}
		if !time.Now().Before(deadline) {
			t.Fatal("未观察到父gone后真实exec")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if darwinBornToInit(member) {
		t.Fatal("孤儿再次exec被错认为init原生对象")
	}
	data, err := unix.SysctlRaw("kern.procargs2", evidence.PID)
	if err != nil || len(data) > 1<<20 {
		t.Fatal("实际SIP参数栈不可核对")
	}
	if marked, readable := darwinScopeMark(data, scope.token); marked || readable {
		t.Fatal("未形成SIP省略环境的真实未知负例")
	}
	scope.discover()
	if scope.unknown[evidence.PID] != birth {
		t.Fatal("父gone后exec的同birth对象未保留unknown")
	}
	if _, owned := scope.members[evidence.PID]; owned {
		t.Fatal("未观察父链或标记却猜测本次归属")
	}
	if scope.stop() {
		t.Fatal("未知孤儿仍活却误确认停止")
	}
	again, identityErr := darwinIdentity(evidence.PID)
	actual, live, birthErr := readProcessBirth(evidence.PID)
	if identityErr != nil || birthErr != nil || !live || actual != birth || again != identity {
		t.Fatal("未证明归属的同对象被信号或身份发生变化")
	}
}

func TestScopeDarwinIncompleteArgsRetainUnknown(t *testing.T) {
	// 实际并发Git复现：argc=2却在第一个argv后到达零字节长度，不是可靠空环境。
	short := make([]byte, 4)
	binary.LittleEndian.PutUint32(short, 2)
	short = append(short, []byte("/usr/bin/git\x00\x00git\x00")...)
	if marked, readable := darwinScopeMark(short, "own-scope"); marked || readable {
		t.Fatal("不完整参数栈被解释为已核对归属")
	}
	full := make([]byte, 4)
	binary.LittleEndian.PutUint32(full, 1)
	full = append(full, []byte("/tmp/helper\x00\x00helper\x00"+scopeEnvironment+"=own-scope\x00")...)
	if marked, readable := darwinScopeMark(full, "own-scope"); !marked || !readable {
		t.Fatal("完整参数栈未核对实际标记")
	}
	if marked, readable := darwinScopeMark(full, "another-scope"); marked || !readable {
		t.Fatal("另一个完整标记被收归本次")
	}
}

func TestScopeDarwinLiveUnknownCannotConfirmStop(t *testing.T) {
	scope, err := newProcessScope()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("/bin/sleep", "20")
	command.Env = scope.environment(nil)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	scope.bind(command.Process.Pid)
	// 调用者不是本次子进程，未知候选只有可读/实际Gone才可解除；不能信号或猜停止。
	birth, live, err := readProcessBirth(os.Getpid())
	if err != nil || !live {
		t.Fatal("未知候选缺实际kernel身份", err)
	}
	scope.unknown[os.Getpid()] = birth
	if scope.stop() {
		t.Fatal("仍存活未知对象被误确认停止")
	}
	<-done
	assertProcessStopped(t, command.Process.Pid)
	if err = syscall.Kill(os.Getpid(), 0); err != nil {
		t.Fatal("未知对象被误停止", err)
	}
}

func TestScopeDarwinRestrictedProgramHasActualBirthAndParentScope(t *testing.T) {
	scope, err := newProcessScope()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("/bin/sh", "-c", "/bin/sleep 20 & wait")
	command.Env = scope.environment(nil)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	// 唯一Wait真实回收直接子进程，不靠kill0把僵尸解释为成功。
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	scope.bind(command.Process.Pid)
	if scope.rootBirth.first == 0 {
		t.Fatal("kernel没有真实出生时间")
	}
	scope.discover()
	if !scope.stop() {
		t.Fatal("真实父组/子孙停止未确认")
	}
	<-done
	assertProcessStopped(t, command.Process.Pid)
	if err = syscall.Kill(os.Getpid(), 0); err != nil {
		t.Fatal("调用进程被误停止", err)
	}
}

func TestScopeDarwinRestrictedDetachedKeepsUnknown(t *testing.T) {
	// 系统程序的环境可能被SIP省略；没有真实父链不能把未读标记当成不属于本次。
	scope, err := newProcessScope()
	if err != nil {
		t.Fatal(err)
	}
	root := exec.Command("/bin/sleep", "20")
	root.Env = scope.environment(nil)
	root.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err = root.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Process.Kill(); _ = root.Wait() })
	scope.bind(root.Process.Pid)
	ready := filepath.Join(t.TempDir(), "ready")
	child := exec.Command("/bin/sh", "-c", `/bin/sleep 20 </dev/null >/dev/null 2>&1 & printf '%s' "$!" > "$OWN_READY"`)
	child.Env = scope.environment(nil)
	child.Env = append(child.Env, "OWN_READY="+ready)
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err = child.Run(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(ready)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	infos, err := unix.SysctlKinfoProcSlice("kern.proc.pid", pid)
	if err != nil || len(infos) != 1 || infos[0].Eproc.Ppid != 1 {
		t.Fatal("未产生真实重父独立对象", err)
	}
	identity, err := darwinIdentity(pid)
	init, initErr := darwinIdentity(1)
	if err != nil || initErr != nil || identity.unique == 0 || identity.originalParent == init.version {
		t.Fatal("实际原父身份未区分重新父化对象", err, initErr)
	}
	if darwinBornToInit(darwinMember(&infos[0])) {
		t.Fatal("重新父化被错认为init原生对象")
	}
	birth, live, err := readProcessBirth(pid)
	if err != nil || !live {
		t.Fatal("自产独立程序没有实际birth", err)
	}
	scope.discover()
	if scope.unknown[pid] != birth {
		t.Fatal("SIP省略环境的独立活对象未保留未知保护")
	}
	if _, owned := scope.members[pid]; owned {
		t.Fatal("没有标记或父链的对象被猜测归属")
	}
	if scope.stop() {
		t.Fatal("独立系统程序仍活却确认停止")
	}
	actual, live, err := readProcessBirth(pid)
	if err != nil || !live || actual != birth {
		t.Fatal("未证明归属的系统对象被发信号", err)
	}
}

func TestScopeDarwinParallelRestrictedRunIsIndependent(t *testing.T) {
	var done [2]chan Result
	var cancel [2]context.CancelFunc
	var pids [2]int
	for index := range done {
		ctx, stop := context.WithCancel(context.Background())
		cancel[index] = stop
		defer stop()
		output := newReadyOutput()
		done[index] = make(chan Result, 1)
		result := done[index]
		command := shellTestCommand(t, `printf 'READY %s\n' "$$"; /bin/sleep 20 & wait`)
		go func() { result <- Run(ctx, command, output, io.Discard) }()
		pids[index] = awaitReady(t, output)
	}
	cancel[0]()
	first := awaitShellResult(t, done[0])
	if first.CleanupFailed || first.Reason != "cancelled" {
		t.Fatal("另一正常Run造成未知停止", first)
	}
	if err := syscall.Kill(pids[1], 0); err != nil {
		t.Fatal("另一Run被误杀", err)
	}
	cancel[1]()
	second := awaitShellResult(t, done[1])
	if second.CleanupFailed || second.Reason != "cancelled" {
		t.Fatal(second)
	}
	assertProcessStopped(t, pids[0])
	assertProcessStopped(t, pids[1])
}
