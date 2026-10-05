//go:build darwin || linux

package agent

import "testing"

// 真查询子进程留下自身未知文件，独立Close必须保留原Ref为空的管理journal。
func TestPublishQueryCleanupKeepsManagementJournal(t *testing.T) { customAgentActual(t, true) }
