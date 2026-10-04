//go:build darwin || linux

package store

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"syscall"
)

func supportedPlatform() bool { return true }
func singleLink(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Nlink == 1
}

// os.FileInfo 使用 syscall.Stat_t；这里只读取 nlink，不推导路径身份。
func (s *Store) openSQLiteLock(path string) error {
	normalized, err := normalizeSQLite(path)
	if err != nil {
		return err
	}
	s.dbPath = normalized
	s.lockPath = normalized + ".lock"
	if info, e := os.Stat(normalized); e == nil {
		if !info.Mode().IsRegular() || !singleLink(info) {
			return ErrInvalid
		}
	} else if !os.IsNotExist(e) {
		return ErrInvalid
	}
	fd, err := unix.Open(s.lockPath, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0600)
	if err != nil {
		return ErrInvalid
	}
	f := os.NewFile(uintptr(fd), s.lockPath)
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || !singleLink(info) {
		f.Close()
		return ErrInvalid
	}
	if err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return ErrLocked
		}
		return errDatabase
	}
	s.lockFile = f
	s.lockInfo = info
	dbfd, err := unix.Open(normalized, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0600)
	if err != nil {
		releaseFileLock(f)
		s.lockFile = nil
		return ErrInvalid
	}
	db := os.NewFile(uintptr(dbfd), normalized)
	defer db.Close()
	info, err = db.Stat()
	if err != nil || !info.Mode().IsRegular() || !singleLink(info) {
		releaseFileLock(f)
		s.lockFile = nil
		return ErrInvalid
	}
	if err = db.Chmod(0600); err != nil {
		releaseFileLock(f)
		s.lockFile = nil
		return errDatabase
	}
	s.dbInfo = info
	return nil
}
func releaseFileLock(f *os.File) error {
	err := unix.Flock(int(f.Fd()), unix.LOCK_UN)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func readPostgresFile(ctx context.Context, path string, secret bool) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, errDatabase
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, ErrInvalid
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 || (secret && info.Mode().Perm()&077 != 0) {
		return nil, ErrInvalid
	}
	data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return nil, ErrInvalid
	}
	if ctx.Err() != nil {
		return nil, errDatabase
	}
	return data, nil
}
