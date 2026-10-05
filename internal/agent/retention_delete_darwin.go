//go:build darwin

package agent

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"syscall"
)

func deletionObjectIdentity(f *os.File, info os.FileInfo) (string, error) {
	previous, ok := info.Sys().(*syscall.Stat_t)
	var st unix.Stat_t
	if !ok || unix.Fstat(int(f.Fd()), &st) != nil || st.Dev != previous.Dev || st.Ino != previous.Ino || st.Uid != uint32(os.Geteuid()) || st.Btim.Nsec < 0 || st.Btim.Nsec > 999999999 || (st.Mode&unix.S_IFMT != unix.S_IFDIR && (st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1)) {
		return "", failure("invalid_object")
	}
	return fmt.Sprintf("v1:darwin:%016x:%016x:%d:%09d", uint32(st.Dev), st.Ino, st.Btim.Sec, st.Btim.Nsec), nil
}
func deletionRenameNoReplace(from *os.File, old string, to *os.File, name string) error {
	return unix.RenameatxNp(int(from.Fd()), old, int(to.Fd()), name, unix.RENAME_EXCL)
}
