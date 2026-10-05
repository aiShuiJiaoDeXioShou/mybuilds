//go:build !darwin && !linux && !windows

package config

import "os"

// 缺少实际文件身份能力的平台不猜普通文件归属。
func profileSingleLink(_ *os.File, _ os.FileInfo) bool { return false }
