//go:build darwin || linux

package scm

import (
	"os"
	"syscall"
)

func checkoutDirectory(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid()) && info.IsDir() && info.Mode().Perm() == 0700
}
