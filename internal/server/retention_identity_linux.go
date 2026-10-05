//go:build linux

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
	var stat unix.Statx_t
	// 仅使用同fd实际返回的字段；请求BTIME不代表该文件系统确实支持。
	const required = unix.STATX_TYPE | unix.STATX_MODE | unix.STATX_INO | unix.STATX_BTIME | unix.STATX_UID | unix.STATX_NLINK
	if !ok || unix.Statx(int(f.Fd()), "", unix.AT_EMPTY_PATH, required, &stat) != nil || stat.Mask&required != required ||
		unix.Mkdev(stat.Dev_major, stat.Dev_minor) != uint64(previous.Dev) || stat.Ino != previous.Ino ||
		stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Mode&0777 != 0600 || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 ||
		stat.Btime.Nsec > 999999999 {
		return "", store.ErrRetentionOwnershipUnknown
	}
	return fmt.Sprintf("v1:linux:%016x:%016x:%d:%09d", unix.Mkdev(stat.Dev_major, stat.Dev_minor), stat.Ino, stat.Btime.Sec, stat.Btime.Nsec), nil
}
