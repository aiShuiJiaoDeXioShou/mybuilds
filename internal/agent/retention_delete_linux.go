//go:build linux

package agent

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"syscall"
)

func deletionObjectIdentity(f *os.File, info os.FileInfo) (string, error) {
	previous, ok := info.Sys().(*syscall.Stat_t)
	var st unix.Statx_t
	const required = unix.STATX_TYPE | unix.STATX_MODE | unix.STATX_INO | unix.STATX_BTIME | unix.STATX_UID | unix.STATX_NLINK
	if !ok || unix.Statx(int(f.Fd()), "", unix.AT_EMPTY_PATH, required, &st) != nil || st.Mask&required != required || unix.Mkdev(st.Dev_major, st.Dev_minor) != uint64(previous.Dev) || st.Ino != previous.Ino || st.Uid != uint32(os.Geteuid()) || st.Btime.Nsec > 999999999 || (st.Mode&unix.S_IFMT != unix.S_IFDIR && (st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1)) {
		return "", failure("invalid_object")
	}
	return fmt.Sprintf("v1:linux:%016x:%016x:%d:%09d", unix.Mkdev(st.Dev_major, st.Dev_minor), st.Ino, st.Btime.Sec, st.Btime.Nsec), nil
}
func deletionRenameNoReplace(from *os.File, old string, to *os.File, name string) error {
	return unix.Renameat2(int(from.Fd()), old, int(to.Fd()), name, unix.RENAME_NOREPLACE)
}
