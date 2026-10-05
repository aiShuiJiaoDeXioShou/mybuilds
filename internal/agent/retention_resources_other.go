//go:build !darwin && !linux

package agent

import (
	"context"
	"mybuilds/internal/protocol"
)

// 非Unix节点不授目录归属，保留三入口编译并明确拒绝登记能力。
func (*taskExecution) registerWorkspace(context.Context, string) error { return failure("unsupported") }
func (*taskExecution) registerResult(context.Context, string) error    { return failure("unsupported") }
func recordResourceTerminal(*executionJournal, protocol.ExecutionEvent, protocol.EventAck) error {
	return failure("unsupported")
}
func recordResourceStop(*executionJournal, protocol.StopConfirmation) error {
	return failure("unsupported")
}
func restoreResourceTerminal(context.Context, *agentHTTP, *executionJournal, protocol.TerminalReceipt, string) error {
	return failure("unsupported")
}

func completeStoppedResource(context.Context, *agentHTTP, *executionJournal, protocol.StopConfirmation) error {
	return failure("unsupported")
}
func readStoppedResourceJournal(*dataLock, string) (*executionJournal, error) {
	return nil, failure("unsupported")
}
func restoreStoppedResource(context.Context, *agentHTTP, *executionJournal) error {
	return failure("unsupported")
}
