//go:build !darwin

package mobile

import (
	"errors"
	"os"
)

func openIOSMaterial(filename string) (*os.File, error) {
	info, err := os.Lstat(filename)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("ios_material_invalid")
	}
	return os.Open(filename)
}

func iosIdentity(info os.FileInfo) (IOSResourceIdentity, error) {
	return IOSResourceIdentity{}, ErrIOSCleanup
}
