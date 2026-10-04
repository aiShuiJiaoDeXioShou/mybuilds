//go:build darwin || linux

package process

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"syscall"
	"time"
)

const scopeEnvironment = "MYBUILDS_PROCESS_SCOPE"
const maxScopeProcesses = 16384

type processBirth struct{ first, second uint64 }
type scopeMember struct {
	pid, parent, group int
	birth              processBirth
	marked             bool
	unreadable         bool
	exited             bool
}
type processScope struct {
	token     string
	root      int
	rootBirth processBirth
	members   map[int]processBirth
	unknown   map[int]processBirth
	uncertain bool
}

// 本次随机证据仅在真实子进程继承，不作为凭据、持久身份或通用进程管理入口。
func newProcessScope() (*processScope, error) {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return nil, err
	}
	return &processScope{token: hex.EncodeToString(value[:]), members: map[int]processBirth{}, unknown: map[int]processBirth{}}, nil
}
func (scope *processScope) environment(env []string) []string {
	out := make([]string, 0, len(env)+1)
	for _, value := range env {
		if !strings.HasPrefix(value, scopeEnvironment+"=") {
			out = append(out, value)
		}
	}
	return append(out, scopeEnvironment+"="+scope.token)
}
func (scope *processScope) bind(pid int) {
	scope.root = pid
	birth, exists, err := readProcessBirth(pid)
	if err != nil || !exists {
		scope.uncertain = true
		return
	}
	scope.rootBirth = birth
	scope.members[pid] = birth
}
func (scope *processScope) discover() {
	candidates, err := scopeProcesses(scope.token, scope.rootBirth, scope.root)
	if err != nil {
		scope.uncertain = true
	}
	current := map[int]scopeMember{}
	for _, member := range candidates {
		current[member.pid] = member
		if member.unreadable {
			scope.unknown[member.pid] = member.birth
		} else {
			delete(scope.unknown, member.pid)
		}
		if member.marked {
			scope.members[member.pid] = member.birth
		}
	}
	// 实际父链补已观测的系统子孙；父PID的birth必须仍与本次记录一致。
	for changed := true; changed; {
		changed = false
		for _, member := range candidates {
			if _, known := scope.members[member.pid]; known {
				continue
			}
			parent, exists := current[member.parent]
			birth, owned := scope.members[member.parent]
			if exists && owned && birth == parent.birth {
				scope.members[member.pid] = member.birth
				changed = true
			}
		}
	}
}
func (scope *processScope) stop() bool {
	// 先记录尚未重设父链的成员；原父组保持既有清理时序。
	scope.discover()
	stopProcessGroup(scope.root)
	for _, signal := range []syscall.Signal{syscall.SIGTERM, syscall.SIGKILL} {
		until := time.Now().Add(processTermGrace)
		for {
			scope.discover()
			live := false
			for pid, birth := range scope.members {
				_, exists, err := readProcessBirth(pid)
				if err != nil {
					scope.uncertain = true
					continue
				}
				if !exists {
					continue
				}
				stopped, err := signalOwnedProcess(pid, birth, signal)
				if err != nil {
					scope.uncertain = true
				}
				if !stopped {
					live = true
				}
			}
			if !live {
				// 在已记录成员确实停止后再读取一次，捕获终止前刚派生并重设父链的成员。
				scope.discover()
				allStopped := true
				for pid, birth := range scope.members {
					actual, exists, err := readProcessBirth(pid)
					if err != nil {
						scope.uncertain = true
						allStopped = false
					} else if exists && actual == birth {
						allStopped = false
					}
				}
				// 暂态栈读取不明的候选不发信号；必须后续真实可读或同birth已消失。
				for pid, birth := range scope.unknown {
					actual, exists, err := readProcessBirth(pid)
					if err != nil {
						scope.uncertain = true
						allStopped = false
					} else if exists && actual == birth {
						allStopped = false
					} else {
						delete(scope.unknown, pid)
					}
				}
				if allStopped {
					// 首轮窗口结论不是最终证据；只读复核原组当前确实Gone。
					if scope.root <= 1 {
						return false
					}
					err := signalProcessGroup(scope.root, 0)
					if errors.Is(err, syscall.ESRCH) {
						return !scope.uncertain
					}
					if err != nil && !errors.Is(err, syscall.EPERM) {
						scope.uncertain = true
						return false
					}
					// nil/EPERM不证明停止，只在当前剩余清理窗口内继续复查。
				}
			}
			if !time.Now().Before(until) {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	return false
}

// 跨管道块隐藏内部随机值，writer慢速不会截断其它字节。
type scopeOutput struct {
	writer  io.Writer
	token   []byte
	pending []byte
}

func (output *scopeOutput) Write(data []byte) (int, error) {
	output.pending = append(output.pending, data...)
	for {
		index := bytes.Index(output.pending, output.token)
		if index >= 0 {
			if err := output.write(output.pending[:index]); err != nil {
				return 0, err
			}
			if err := output.write([]byte("[REDACTED]")); err != nil {
				return 0, err
			}
			output.pending = output.pending[index+len(output.token):]
			continue
		}
		// 仅保留与随机值前缀匹配的尾部；普通短日志必须即时到达真实消费者。
		keep := len(output.token) - 1
		if keep > len(output.pending) {
			keep = len(output.pending)
		}
		for keep > 0 && !bytes.Equal(output.pending[len(output.pending)-keep:], output.token[:keep]) {
			keep--
		}
		size := len(output.pending) - keep
		if size > 0 {
			if err := output.write(output.pending[:size]); err != nil {
				return 0, err
			}
			output.pending = append([]byte{}, output.pending[size:]...)
		}
		return len(data), nil
	}
}
func (output *scopeOutput) write(data []byte) error {
	if len(data) == 0 || output.writer == nil {
		return nil
	}
	n, err := output.writer.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	return err
}
func (output *scopeOutput) flush() error {
	err := output.write(output.pending)
	output.pending = nil
	return err
}
func birthBefore(a, b processBirth) bool {
	return a.first < b.first || a.first == b.first && a.second < b.second
}
func vanished(err error) bool { return errors.Is(err, syscall.ESRCH) || errors.Is(err, syscall.ENOENT) }
