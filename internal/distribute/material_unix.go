//go:build darwin || linux

package distribute

import (
	"golang.org/x/sys/unix"
	"os"
)

func openMaterial(path string, secret bool) (*os.File, error) {
	fd, e := unix.Open(path, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, errMaterial
	}
	f := os.NewFile(uintptr(fd), path)
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 || st.Mode&022 != 0 || (st.Uid != uint32(os.Getuid()) && st.Uid != 0) || (secret && (st.Uid != uint32(os.Getuid()) || st.Mode&077 != 0)) {
		f.Close()
		return nil, errMaterial
	}
	return f, nil
}
