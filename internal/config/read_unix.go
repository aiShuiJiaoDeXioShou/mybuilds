//go:build darwin || linux

package config

import (
	"golang.org/x/sys/unix"
	"os"
	"syscall"
)

// openConfiguration不跟随叶子链接，非阻塞打开后仍须检查普通文件。
func openConfiguration(filename string) (*os.File, error) {
	fd, err := unix.Open(filename, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), filename), nil
}

// configurationOwned用于明确私有Agent材料的宿主身份核对。
func configurationOwned(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid())
}

func configurationSingleLink(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Nlink == 1
}
