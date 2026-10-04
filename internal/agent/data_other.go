//go:build !darwin && !linux

package agent

import "os"

// 不支持平台只保留编译，拒绝节点运行和目录授权。
type dataLock struct{}

func lockDataDir(string) (*dataLock, error) { return nil, failure("unsupported") }
func (*dataLock) Check() error              { return failure("unsupported") }
func (*dataLock) prepareJournal() error     { return failure("unsupported") }
func (*dataLock) Close() error              { return failure("unsupported") }
func inspectData(string) string             { return "unsupported" }

func openToolDirectory(string) (*os.File, error) { return nil, failure("unsupported") }
