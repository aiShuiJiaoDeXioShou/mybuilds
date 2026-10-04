//go:build darwin || linux

package server

import (
	"golang.org/x/sys/unix"
	"os"
	"syscall"
)

func evidenceOpenFlags() int { return unix.O_NOFOLLOW | unix.O_NONBLOCK }
func evidenceInfo(info os.FileInfo, directory bool) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) {
		return false
	}
	if directory {
		return info.IsDir() && info.Mode().Perm() == 0700
	}
	return info.Mode().IsRegular() && info.Mode().Perm() == 0600 && stat.Nlink == 1
}
