//go:build !darwin && !linux

package agent

import (
	"context"
	"mybuilds/internal/protocol"
)

// 无可靠出生身份的平台明确不具备节点删除能力。
func advanceNodeDeletion(context.Context, *agentHTTP, *dataLock, string, protocol.NodeDeletion) error {
	return failure("unsupported")
}
func recoverNodeDeletionConfirmations(context.Context, *agentHTTP, *dataLock, string) error {
	return failure("unsupported")
}
