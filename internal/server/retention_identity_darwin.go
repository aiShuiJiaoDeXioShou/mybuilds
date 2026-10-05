//go:build darwin

package server

import (
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
	"mybuilds/internal/store"
)

func retentionNativeIdentity(f *os.File, info os.FileInfo) (string, error) {
	previous, ok := info.Sys().(*syscall.Stat_t)
	var stat unix.Stat_t
	if !ok || unix.Fstat(int(f.Fd()), &stat) != nil || stat.Dev != previous.Dev || stat.Ino != previous.Ino ||
		stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Mode&0777 != 0600 || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 ||
		stat.Btim.Nsec < 0 || stat.Btim.Nsec > 999999999 {
		return "", store.ErrRetentionOwnershipUnknown
	}
	return fmt.Sprintf("v1:darwin:%016x:%016x:%d:%09d", uint32(stat.Dev), stat.Ino, stat.Btim.Sec, stat.Btim.Nsec), nil
}
