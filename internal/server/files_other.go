//go:build !darwin && !linux

package server

import "os"

// 控制端文件证据目前只支持已验收的macOS/Linux。
func evidenceOpenFlags() int              { return 0 }
func evidenceInfo(os.FileInfo, bool) bool { return false }
