//go:build !darwin && !linux

package scm

import "os"

func checkoutDirectory(os.FileInfo) bool { return false }
