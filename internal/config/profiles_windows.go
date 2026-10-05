//go:build windows

package config

import (
	"golang.org/x/sys/windows"
	"os"
)

func profileSingleLink(file *os.File, _ os.FileInfo) bool {
	var info windows.ByHandleFileInformation
	return windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &info) == nil && info.NumberOfLinks == 1
}
