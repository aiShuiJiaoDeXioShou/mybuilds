//go:build linux

package process

import (
	"bytes"
	"errors"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func linuxProcess(pid int) (scopeMember, bool, error) {
	var member scopeMember
	member.pid = pid
	data, err := readProcFile("/proc/"+strconv.Itoa(pid)+"/stat", 64*1024)
	if vanished(err) {
		return member, false, nil
	}
	if err != nil {
		return member, false, err
	}
	end := bytes.LastIndexByte(data, ')')
	if end < 0 {
		return member, false, syscall.EINVAL
	}
	fields := strings.Fields(string(data[end+1:]))
	if len(fields) < 20 {
		return member, false, syscall.EINVAL
	}
	member.parent, err = strconv.Atoi(fields[1])
	if err != nil {
		return member, false, err
	}
	member.group, err = strconv.Atoi(fields[2])
	if err != nil {
		return member, false, err
	}
	member.birth.first, err = strconv.ParseUint(fields[19], 10, 64)
	member.exited = fields[0] == "Z" || fields[0] == "X"
	// 连Z也保留存在证据；Run停止确认仍等本次对象真正消失，不仅信号已送达。
	return member, fields[0] != "X", err
}
func readProcessBirth(pid int) (processBirth, bool, error) {
	member, live, err := linuxProcess(pid)
	return member.birth, live, err
}
func scopeProcesses(token string, since processBirth, root int) ([]scopeMember, error) {
	directory, err := os.Open("/proc")
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	entries, err := directory.ReadDir(maxScopeProcesses + 1)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if len(entries) > maxScopeProcesses {
		return nil, syscall.EOVERFLOW
	}
	var out []scopeMember
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 1 {
			continue
		}
		// /proc目录owner会因PR_SET_DUMPABLE改变；用实际status有效UID筛选读取权限。
		status, err := readProcFile(filepath.Join("/proc", entry.Name(), "status"), 64*1024)
		if vanished(err) {
			continue
		}
		if err != nil {
			return out, err
		}
		if len(status) > 64*1024 {
			return out, syscall.EOVERFLOW
		}
		ownedUID := false
		for _, line := range strings.Split(string(status), "\n") {
			if strings.HasPrefix(line, "Uid:") {
				values := strings.Fields(line)
				if len(values) != 5 {
					return out, syscall.EINVAL
				}
				uid, err := strconv.ParseUint(values[2], 10, 32)
				if err != nil {
					return out, err
				}
				ownedUID = uid == uint64(os.Geteuid())
				break
			}
		}
		if !ownedUID {
			continue
		}
		member, live, err := linuxProcess(pid)
		if err != nil {
			return out, err
		}
		if !live || birthBefore(member.birth, since) {
			continue
		}
		// 已退出对象没有可执行用户态，仍保留birth等待真正gone；不读取失效的环境。
		if member.exited {
			out = append(out, member)
			continue
		}
		file, err := os.Open(filepath.Join("/proc", entry.Name(), "environ"))
		if vanished(err) {
			continue
		}
		if err != nil {
			after, exists, checkErr := linuxUnreadableProcess(member, err)
			if checkErr != nil {
				return out, checkErr
			}
			if exists {
				out = append(out, after)
			}
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(file, (1<<20)+1))
		file.Close()
		if vanished(readErr) {
			continue
		}
		if readErr != nil {
			if !errors.Is(readErr, syscall.EACCES) && !errors.Is(readErr, syscall.EPERM) {
				return out, readErr
			}
			after, exists, checkErr := linuxUnreadableProcess(member, readErr)
			if checkErr != nil {
				return out, checkErr
			}
			if exists {
				out = append(out, after)
			}
			continue
		}
		if len(data) > 1<<20 {
			return out, syscall.EOVERFLOW
		}
		after, exists, err := linuxProcess(pid)
		if err != nil {
			return out, err
		}
		if !exists || after.birth != member.birth {
			continue
		}
		for _, value := range bytes.Split(data, []byte{0}) {
			if string(value) == scopeEnvironment+"="+token {
				member.marked = true
				break
			}
		}
		out = append(out, member)
	}
	return out, nil
}

// 仅已读同UID/birth的权限暂态进入候选保护；其它元数据或IO错误仍闭锁。
func linuxUnreadableProcess(member scopeMember, cause error) (scopeMember, bool, error) {
	after, exists, err := linuxProcess(member.pid)
	if err != nil {
		return after, false, err
	}
	if !exists || after.birth != member.birth {
		return after, false, nil
	}
	if after.exited {
		return after, true, nil
	}
	if !errors.Is(cause, syscall.EACCES) && !errors.Is(cause, syscall.EPERM) {
		return after, false, cause
	}
	// 不能把权限拒读当作nonce不匹配，也不向尚未证明归属的候选发信号。
	after.unreadable = true
	return after, true, nil
}
func signalOwnedProcess(pid int, birth processBirth, signal syscall.Signal) (bool, error) {
	fd, err := unix.PidfdOpen(pid, 0)
	if errors.Is(err, unix.ESRCH) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	defer unix.Close(fd)
	actual, live, err := readProcessBirth(pid)
	if err != nil {
		return false, err
	}
	if !live || actual != birth {
		return true, nil
	}
	if err = unix.PidfdSendSignal(fd, signal, nil, 0); errors.Is(err, unix.ESRCH) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	actual, live, err = readProcessBirth(pid)
	return !live || actual != birth, err
}

// proc元数据在实际读取时限制，不依赖读完后的长度检查。
func readProcFile(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err == nil && int64(len(data)) > limit {
		return nil, syscall.EOVERFLOW
	}
	return data, err
}
