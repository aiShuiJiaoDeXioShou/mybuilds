package agent

import (
	"context"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"sync"
	"time"
)

// 本地租约只接受请求开始时刻推导的保守期限，迟到回执不能复活失权执行。
type executionLease struct {
	mu         sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
	user       context.Context
	userCancel context.CancelFunc
	deadline   time.Time
	timer      *time.Timer
	lock       *dataLock
	ref        protocol.LeaseRef
	cfg        config.AgentConfig
}

func newExecutionLease(parent context.Context, lock *dataLock, cfg config.AgentConfig, grant protocol.LeaseGrant, requested time.Time) (*executionLease, error) {
	if !validRef(grant.Ref) || grant.TTLNS <= 0 || grant.TTLNS > int64(cfg.LeaseDuration) || grant.Task == nil || grant.RemainingPostBudgetNS < 0 || grant.RemainingBudgetNS != nil && *grant.RemainingBudgetNS < 0 {
		return nil, failure("invalid_response")
	}
	authority, cancel := context.WithCancel(parent)
	user, userCancel := context.WithCancel(parent)
	lease := &executionLease{ctx: authority, cancel: cancel, user: user, userCancel: userCancel, lock: lock, ref: grant.Ref, cfg: cfg}
	if err := lease.accept(grant, requested); err != nil {
		cancel()
		userCancel()
		return nil, err
	}
	return lease, nil
}
func (lease *executionLease) accept(grant protocol.LeaseGrant, requested time.Time) error {
	lease.mu.Lock()
	defer lease.mu.Unlock()
	now := time.Now()
	if lease.ctx.Err() != nil || !lease.deadline.IsZero() && !now.Before(lease.deadline) || grant.Ref != lease.ref || grant.TTLNS <= 0 || grant.TTLNS > int64(lease.cfg.LeaseDuration) {
		lease.cancel()
		return failure("authority_lost")
	}
	margin := 2 * time.Second
	if lease.cfg.HeartbeatInterval > margin {
		margin = lease.cfg.HeartbeatInterval
	}
	deadline := requested.Add(time.Duration(grant.TTLNS) - margin)
	if !now.Before(deadline) {
		lease.cancel()
		return failure("authority_lost")
	}
	lease.deadline = deadline
	if lease.timer != nil {
		lease.timer.Stop()
	}
	lease.timer = time.AfterFunc(time.Until(deadline), func() {
		lease.mu.Lock()
		defer lease.mu.Unlock()
		if !time.Now().Before(lease.deadline) {
			lease.cancel()
		}
	})
	if grant.CancelRequested {
		lease.userCancel()
	}
	return nil
}
func (lease *executionLease) check() error {
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if lease.ctx.Err() != nil || !time.Now().Before(lease.deadline) {
		lease.cancel()
		return failure("authority_lost")
	}
	if err := lease.lock.Check(); err != nil {
		lease.cancel()
		return err
	}
	return nil
}
func (lease *executionLease) close() {
	lease.cancel()
	lease.userCancel()
	lease.mu.Lock()
	if lease.timer != nil {
		lease.timer.Stop()
	}
	lease.mu.Unlock()
}
func (lease *executionLease) renew(client *agentHTTP) {
	ticker := time.NewTicker(lease.cfg.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-lease.ctx.Done():
			return
		case <-ticker.C:
			if lease.check() != nil {
				return
			}
			requested := time.Now()
			var grant protocol.LeaseGrant
			if err := client.retryPost(lease.ctx, "/api/agent/renew", lease.ref, &grant); err != nil {
				lease.cancel()
				return
			}
			if grant.Task != nil || lease.accept(grant, requested) != nil {
				lease.cancel()
				return
			}
		}
	}
}
