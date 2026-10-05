//go:build darwin

package mobile

import (
	"os"
	"syscall"
)

// 明确材料必须为普通文件；非阻塞打开避免被替换的FIFO挂住父层复制。
func openIOSMaterial(filename string) (*os.File, error) {
	descriptor, err := syscall.Open(filename, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(descriptor), filename), nil
}

func iosIdentity(info os.FileInfo) (IOSResourceIdentity, error) {
	if info == nil {
		return IOSResourceIdentity{}, ErrIOSCleanup
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return IOSResourceIdentity{}, ErrIOSCleanup
	}
	return IOSResourceIdentity{Device: uint64(stat.Dev), Inode: stat.Ino, Mode: uint32(info.Mode())}, nil
}
