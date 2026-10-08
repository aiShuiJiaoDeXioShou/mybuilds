//go:build darwin || linux

package agent

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

type dataLock struct {
	root                *os.Root
	file                *os.File
	directory, fileInfo os.FileInfo
}

func privateInfo(info os.FileInfo, directory bool) bool {
	if !directory {
		return privateReadableInfo(info) && info.Mode().Perm() == 0600
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid()) && info.IsDir() && info.Mode().Perm() == 0700
}

func privateReadableInfo(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid()) && stat.Nlink == 1 && info.Mode().IsRegular() && (info.Mode().Perm() == 0400 || info.Mode().Perm() == 0600)
}

func lockDataDir(path string) (*dataLock, error) {
	if path == "" {
		return nil, failure("data_invalid")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, failure("data_invalid")
	}
	if err := os.MkdirAll(absolute, 0700); err != nil {
		return nil, failure("data_invalid")
	}
	directory, err := os.Lstat(absolute)
	if err != nil || !privateInfo(directory, true) {
		return nil, failure("data_invalid")
	}
	root, err := os.OpenRoot(absolute)
	if err != nil {
		return nil, failure("data_invalid")
	}
	file, err := root.OpenFile("agent.lock", os.O_CREATE|os.O_RDWR|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0600)
	if err != nil {
		root.Close()
		return nil, failure("data_invalid")
	}
	info, err := file.Stat()
	if err != nil || !privateInfo(info, false) {
		file.Close()
		root.Close()
		return nil, failure("data_invalid")
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		file.Close()
		root.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, failure("data_locked")
		}
		return nil, failure("data_invalid")
	}
	lock := &dataLock{root: root, file: file, directory: directory, fileInfo: info}
	if err := lock.Check(); err != nil {
		lock.Close()
		return nil, err
	}
	return lock, nil
}

// Check拒绝目录或锁文件被替换，不能在旧inode上继续领取。
func (lock *dataLock) Check() error {
	if lock == nil || lock.root == nil || lock.file == nil {
		return failure("data_invalid")
	}
	directory, err := os.Lstat(lock.root.Name())
	if err != nil || !privateInfo(directory, true) || !os.SameFile(directory, lock.directory) {
		return failure("data_invalid")
	}
	info, err := lock.root.Lstat("agent.lock")
	if err != nil || !privateInfo(info, false) || !os.SameFile(info, lock.fileInfo) {
		return failure("data_invalid")
	}
	return nil
}

func (lock *dataLock) prepareJournal() error {
	if err := lock.Check(); err != nil {
		return err
	}
	if err := lock.root.Mkdir("journal", 0700); err != nil && !os.IsExist(err) {
		return failure("data_invalid")
	}
	info, err := lock.root.Lstat("journal")
	if err != nil || !privateInfo(info, true) {
		return failure("data_invalid")
	}
	return lock.Check()
}
func (lock *dataLock) Close() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	unlock := unix.Flock(int(lock.file.Fd()), unix.LOCK_UN)
	closed := lock.file.Close()
	rootClosed := lock.root.Close()
	lock.file, lock.root = nil, nil
	if unlock != nil || closed != nil || rootClosed != nil {
		return failure("data_invalid")
	}
	return nil
}

// inspectData只读检查未确认journal；不解释旧PID，更不向它发信号。
func inspectData(path string) string {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return "uninitialized"
	}
	if err != nil || !privateInfo(info, true) {
		return "data_invalid"
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return "data_invalid"
	}
	defer root.Close()
	info, err = root.Lstat("journal")
	if os.IsNotExist(err) {
		return "uninitialized"
	}
	if err != nil || !privateInfo(info, true) {
		return "data_invalid"
	}
	dir, err := root.OpenFile("journal", os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return "data_invalid"
	}
	defer dir.Close()
	entries, err := dir.ReadDir(129)
	if err != nil && err != io.EOF {
		return "data_invalid"
	}
	if len(entries) > 128 {
		return "data_invalid"
	}
	for _, entry := range entries {
		info, err := root.Lstat(filepath.Join("journal", entry.Name()))
		if err != nil || !privateInfo(info, false) || info.Size() > maxJournalBytes {
			return "data_invalid"
		}
	}
	for _, entry := range entries {
		if !pausedJournalForInspection(root, entry.Name()) {
			return "journal_unconfirmed"
		}
	}
	return ""
}

// 工具目录也可能被错误配置为FIFO，不能用阻塞Open做类型检查。
func openToolDirectory(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	info, err := file.Stat()
	if err != nil || !info.IsDir() {
		file.Close()
		return nil, failure("data_invalid")
	}
	return file, nil
}
