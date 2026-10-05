//go:build linux

package agent

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"syscall"
)

func resourceNativeIdentity(f *os.File, info os.FileInfo) (string, error) {
	previous, ok := info.Sys().(*syscall.Stat_t)
	var stat unix.Statx_t
	const required = unix.STATX_TYPE | unix.STATX_MODE | unix.STATX_INO | unix.STATX_BTIME | unix.STATX_UID | unix.STATX_NLINK
	if !ok || unix.Statx(int(f.Fd()), "", unix.AT_EMPTY_PATH, required, &stat) != nil || stat.Mask&required != required || unix.Mkdev(stat.Dev_major, stat.Dev_minor) != uint64(previous.Dev) || stat.Ino != previous.Ino || stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Mode&0777 != 0700 || stat.Uid != uint32(os.Geteuid()) || stat.Btime.Nsec > 999999999 {
		return "", failure("persistence_error")
	}
	return fmt.Sprintf("v1:linux:%016x:%016x:%d:%09d", unix.Mkdev(stat.Dev_major, stat.Dev_minor), stat.Ino, stat.Btime.Sec, stat.Btime.Nsec), nil
}
