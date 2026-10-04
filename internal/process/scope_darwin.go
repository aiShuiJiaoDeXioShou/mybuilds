//go:build darwin

package process

import (
	"bytes"
	"encoding/binary"
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

func darwinMember(info *unix.KinfoProc) scopeMember {
	return scopeMember{pid: int(info.Proc.P_pid), parent: int(info.Eproc.Ppid), group: int(info.Eproc.Pgid), birth: processBirth{uint64(info.Proc.P_starttime.Sec), uint64(info.Proc.P_starttime.Usec)}}
}
func readProcessBirth(pid int) (processBirth, bool, error) {
	infos, err := unix.SysctlKinfoProcSlice("kern.proc.pid", pid)
	if errors.Is(err, unix.ESRCH) {
		return processBirth{}, false, nil
	}
	if err != nil {
		return processBirth{}, false, err
	}
	if len(infos) == 0 {
		return processBirth{}, false, nil
	}
	if len(infos) != 1 {
		return processBirth{}, false, syscall.EINVAL
	}
	info := &infos[0]
	member := darwinMember(info)
	return member.birth, info.Proc.P_stat != 0, nil
}
func scopeProcesses(token string, since processBirth, root int) ([]scopeMember, error) {
	infos, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, err
	}
	if len(infos) > maxScopeProcesses {
		return nil, syscall.EOVERFLOW
	}
	var out []scopeMember
	for _, info := range infos {
		member := darwinMember(&info)
		if member.pid <= 1 || info.Eproc.Ucred.Uid != uint32(os.Geteuid()) || info.Proc.P_stat == 5 || info.Proc.P_stat == 0 || birthBefore(member.birth, since) {
			continue
		}
		// 本次root及仍在其原组的子进程已有真实组/父链证据；exec切换栈可EIO，
		// 此处无需读取环境。独立组仍必须有精确随机标记或可核实父链。
		if (member.pid == root && member.birth == since) || member.group == root {
			out = append(out, member)
			continue
		}
		data, err := unix.SysctlRaw("kern.procargs2", member.pid)
		if err != nil {
			if errors.Is(err, unix.EIO) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ESRCH) {
				member.unreadable = true
			} else {
				return out, err
			}
		} else {
			if len(data) > 1<<20 {
				return out, syscall.EOVERFLOW
			}
			// exec/exit切换时sysctl也可能成功返回不完整argv，不能当全表永久故障，
			// 更不能解释为无标记；同birth候选保留到实际完整可读或对象Gone。
			var readable bool
			member.marked, readable = darwinScopeMark(data, token)
			member.unreadable = !readable
		}
		actual, live, err := readProcessBirth(member.pid)
		if err != nil {
			return out, err
		}
		if !live || actual != member.birth {
			continue
		}
		if member.unreadable && (darwinExternalParent(member, since, root) || darwinBornToInit(member)) {
			// 未读到环境仍不能猜无标记；此处只用复核后的外部活父链排除归属。
			member.unreadable = false
		}
		out = append(out, member)
	}
	return out, nil
}

type darwinProcessIdentity struct {
	unique, parent          uint64
	version, originalParent int32
}

func darwinIdentity(pid int) (darwinProcessIdentity, error) {
	// XNU proc_uniqidentifierinfo ABI固定56B；只读原父身份，不取argv/环境。
	var data [56]byte
	size, _, errno := syscall.Syscall6(syscall.SYS_PROC_INFO, 2, uintptr(pid), 17, 0, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)))
	runtime.KeepAlive(&data)
	if errno != 0 {
		return darwinProcessIdentity{}, errno
	}
	if size != uintptr(len(data)) {
		return darwinProcessIdentity{}, syscall.EINVAL
	}
	return darwinProcessIdentity{binary.LittleEndian.Uint64(data[16:24]), binary.LittleEndian.Uint64(data[24:32]), int32(binary.LittleEndian.Uint32(data[32:36])), int32(binary.LittleEndian.Uint32(data[36:40]))}, nil
}

func darwinBornToInit(member scopeMember) bool {
	if member.parent != 1 {
		return false
	}
	init, err := darwinIdentity(1)
	if err != nil || init.unique == 0 || init.version == 0 {
		return false
	}
	identity, err := darwinIdentity(member.pid)
	if err != nil || identity.unique == 0 || identity.parent != init.unique || identity.originalParent != init.version {
		return false
	}
	infos, err := unix.SysctlKinfoProcSlice("kern.proc.pid", member.pid)
	if err != nil || len(infos) != 1 {
		return false
	}
	info := &infos[0]
	current := darwinMember(info)
	if current.birth != member.birth || current.parent != 1 || info.Proc.P_stat == 0 || info.Proc.P_stat == 5 || info.Proc.P_flag&0x800 != 0 || info.Proc.P_oppid != 0 {
		return false
	}
	initAgain, initErr := darwinIdentity(1)
	again, err := darwinIdentity(member.pid)
	return initErr == nil && err == nil && initAgain == init && again == identity
}

func darwinExternalParent(member scopeMember, since processBirth, root int) bool {
	// XNU普通孤儿重父到init；ptrace能重父到调试器，须保持未知。
	// P_TRACED来自Darwin sys/proc.h，不按名称或UID猜测父子关系。
	const traced = 0x00000800
	var path []scopeMember
	external := false
	for depth := 0; depth < 64; depth++ {
		if member.pid <= 1 || member.pid == root {
			return false
		}
		infos, err := unix.SysctlKinfoProcSlice("kern.proc.pid", member.pid)
		if err != nil || len(infos) != 1 {
			return false
		}
		info := &infos[0]
		current := darwinMember(info)
		if info.Proc.P_stat == 0 || info.Proc.P_stat == 5 || info.Proc.P_flag&traced != 0 || info.Proc.P_oppid != 0 || current.birth == (processBirth{}) || current.birth != member.birth || current.parent != member.parent {
			return false
		}
		path = append(path, current)
		if birthBefore(current.birth, since) {
			external = true
			break
		}
		if current.parent <= 1 {
			return false
		}
		parents, err := unix.SysctlKinfoProcSlice("kern.proc.pid", current.parent)
		if err != nil || len(parents) != 1 {
			return false
		}
		parent := darwinMember(&parents[0])
		if parent.birth == (processBirth{}) || !birthBefore(parent.birth, current.birth) {
			return false
		}
		member = parent
	}
	if !external {
		return false
	}
	// 各sysctl不是历史原子快照；排除前再次核对整条实际路径，变化仍未知。
	for _, observed := range path {
		infos, err := unix.SysctlKinfoProcSlice("kern.proc.pid", observed.pid)
		if err != nil || len(infos) != 1 {
			return false
		}
		info := &infos[0]
		current := darwinMember(info)
		if info.Proc.P_stat == 0 || info.Proc.P_stat == 5 || info.Proc.P_flag&traced != 0 || info.Proc.P_oppid != 0 || current.birth != observed.birth || current.parent != observed.parent {
			return false
		}
	}
	return true
}

func darwinScopeMark(data []byte, token string) (marked, readable bool) {
	if len(data) < 4 || len(data) > 1<<20 {
		return false, false
	}
	count := int(binary.LittleEndian.Uint32(data[:4]))
	data = data[4:]
	end := bytes.IndexByte(data, 0)
	if end < 0 {
		return false, false
	}
	data = data[end:]
	for len(data) > 0 && data[0] == 0 {
		data = data[1:]
	}
	for n := 0; n < count; n++ {
		end = bytes.IndexByte(data, 0)
		if end < 0 {
			return false, false
		}
		data = data[end+1:]
	}
	environmentPresent := false
	for _, value := range bytes.Split(data, []byte{0}) {
		if len(value) > 0 {
			environmentPresent = true
		}
		if string(value) == scopeEnvironment+"="+token {
			return true, true
		}
	}
	// SIP可省略整个环境；完整argv不代表空环境已核验，不得据此解除未知保护。
	return false, environmentPresent
}

func signalOwnedProcess(pid int, birth processBirth, signal syscall.Signal) (bool, error) {
	actual, live, err := readProcessBirth(pid)
	if err != nil {
		return false, err
	}
	if !live || actual != birth {
		return true, nil
	}
	// Darwin没有pidfd；每次发信号前核对真实birth，核对/kill仍存在平台TOCTOU限界。
	if err = unix.Kill(pid, signal); errors.Is(err, unix.ESRCH) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	actual, live, err = readProcessBirth(pid)
	return !live || actual != birth, err
}
