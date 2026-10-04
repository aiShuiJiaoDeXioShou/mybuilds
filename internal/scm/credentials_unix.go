//go:build darwin || linux

package scm

import (
	"io"
	"os"
	"syscall"
)

// NONBLOCK避免验证FIFO时阻塞；NOFOLLOW与identity检查拒绝验证后换文件。
func readPrivate(filename string, limit int64) ([]byte, error) {
	before, err := os.Lstat(filename)
	if err != nil || !ownedMaterial(before) || before.Size() <= 0 || before.Size() > limit {
		return nil, failure("credentials_invalid")
	}
	file, err := os.OpenFile(filename, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, failure("credentials_invalid")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !os.SameFile(info, before) || !ownedMaterial(info) {
		return nil, failure("credentials_invalid")
	}
	data, readErr := io.ReadAll(io.LimitReader(file, limit+1))
	after, statErr := file.Stat()
	if readErr != nil || statErr != nil || !ownedMaterial(after) || len(data) == 0 || int64(len(data)) > limit || int64(len(data)) != info.Size() || info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) {
		return nil, failure("credentials_invalid")
	}
	return data, nil
}

func ownedMaterial(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid()) && stat.Nlink == 1 && info.Mode().IsRegular() && info.Mode().Perm() == 0600
}
