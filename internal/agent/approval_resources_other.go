//go:build !darwin && !linux

package agent

import (
	"context"
	"mybuilds/internal/protocol"
)

func validateApprovalResource(*executionJournal) error { return failure("unsupported") }
func approvalResourceEvidence(*executionJournal) (string, string, error) {
	return "", "", failure("unsupported")
}
func rebindApprovalResource(*executionJournal, protocol.LeaseGrant) error {
	return failure("unsupported")
}

func fillApprovalWorkspaceIdentity(*executionJournal, *protocol.ApprovalLocalCheckpoint) error {
	return failure("unsupported")
}

func closeApprovalResource(context.Context, *executionJournal) error { return failure("unsupported") }

// 不支持平台不取得资源归属，也不执行恢复。
func (*taskExecution) registerApprovalResource(context.Context) error { return failure("unsupported") }
