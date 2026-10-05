//go:build darwin || linux

package config

import (
	"os"
	"syscall"
)

func profileSingleLink(_ *os.File, info os.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && st.Nlink == 1
}
