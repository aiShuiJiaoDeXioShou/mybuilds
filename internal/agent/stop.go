package agent

import (
	"context"
	"mybuilds/internal/protocol"
	"time"
)

// 停止确认只传真实回收证据，不借过期fence提交终态或重新执行。
func (execution *taskExecution) confirmStopped(parent context.Context) error {
	journal := execution.journal
	journal.mu.Lock()
	if journal.state.Ref == nil || !journal.state.StopConfirmed || journal.state.CleanupFailed || journal.state.PendingEvent != nil && journal.state.PendingEvent.Progress.Kind == "build_finished" {
		journal.mu.Unlock()
		return failure("execution_unconfirmed")
	}
	confirmation := protocol.StopConfirmation{Ref: *journal.state.Ref, EvidenceCode: "process_group_reaped", Note: "本次执行进程组已完成真实回收"}
	journal.state.PendingStop = &confirmation
	err := journal.saveLocked()
	journal.mu.Unlock()
	if err != nil {
		return err
	}
	// 有界续租重试可能已在中央提交而丢回执，等待覆盖最后实际请求的最大TTL。
	execution.lease.mu.Lock()
	deadline := execution.lease.deadline
	possibleExpiry := execution.lease.possibleExpiry
	execution.lease.mu.Unlock()
	margin := 2 * time.Second
	if execution.lease.cfg.HeartbeatInterval > margin {
		margin = execution.lease.cfg.HeartbeatInterval
	}
	deadline = deadline.Add(margin + execution.lease.cfg.HeartbeatInterval + 2*time.Second)
	if later := possibleExpiry.Add(2 * time.Second); later.After(deadline) {
		deadline = later
	}
	for {
		// 服务退出不等整段TTL；只做一次有界的独立确认，失败留下pending材料。
		callParent := parent
		if parent.Err() != nil {
			callParent = context.WithoutCancel(parent)
		}
		ctx, cancel := context.WithTimeout(callParent, execution.client.timeout)
		err := execution.client.post(ctx, "/api/agent/stop-confirmation", confirmation, nil)
		if err == nil {
			err = completeStoppedResource(ctx, execution.client, journal, confirmation)
			cancel()
			if err != nil {
				return err
			}
			return journal.remove()
		}
		cancel()
		safe, ok := err.(*Error)
		if parent.Err() != nil || !ok || safe.Code != "stop_unconfirmed" && safe.Code != "network_error" || !time.Now().Before(deadline) {
			return failure("execution_unconfirmed")
		}
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-parent.Done():
			timer.Stop()
			return failure("execution_unconfirmed")
		case <-timer.C:
		}
	}
}
