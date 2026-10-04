//go:build darwin || linux

package client

import (
	"golang.org/x/sys/unix"
	"os"
	"syscall"
)

func downloadOpenFlags() int { return unix.O_NOFOLLOW | unix.O_NONBLOCK }
func downloadPrivate(info os.FileInfo, directory bool) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) {
		return false
	}
	if directory {
		return info.IsDir() && info.Mode().Perm()&022 == 0
	}
	return info.Mode().IsRegular() && info.Mode().Perm() == 0600 && stat.Nlink == 1
}
