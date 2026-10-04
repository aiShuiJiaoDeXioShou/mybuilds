//go:build !darwin && !linux

package config

import "os"

// 其他客户端平台先拒非普通文件，并在打开后核对身份。
func openConfiguration(filename string) (*os.File, error) {
	before, err := os.Lstat(filename)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, os.ErrPermission
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	after, err := file.Stat()
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		file.Close()
		return nil, os.ErrPermission
	}
	return file, nil
}
