//go:build !darwin && !linux

package client

import "os"

func downloadOpenFlags() int                 { return 0 }
func downloadPrivate(os.FileInfo, bool) bool { return false }
