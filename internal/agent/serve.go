package agent

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
)

// Serve每进程独立session且持有本地目录锁，注册/心跳只能使用节点独立凭据。
func Serve(ctx context.Context, cfg config.AgentConfig) (result error) {
	if ctx.Err() != nil {
		return nil
	}
	if cfg.Capacity < 1 || cfg.Capacity > 32 || cfg.LeaseDuration < 10*time.Second || cfg.LeaseDuration > 180*time.Second || cfg.LeaseDuration < 4*cfg.HeartbeatInterval+2*time.Second {
		return failure("configuration_error")
	}
	client, err := newAgentHTTP(cfg)
	if err != nil {
		return err
	}
	defer client.close()
	lock, err := lockDataDir(cfg.DataDir)
	if err != nil {
		return err
	}
	defer func() {
		if err := lock.Close(); err != nil && result == nil {
			result = err
		}
	}()
	if err = closeRecoveredIOSResources(ctx, lock); err != nil {
		return err
	}
	// 当前节点只读确认精确终态；任何旧活动/未知证据仍拒绝，不按旧PID发信号。
	if err = recoverTerminalJournals(ctx, client, lock, cfg.Node); err != nil {
		return err
	}
	if reason := inspectData(cfg.DataDir); reason != "" && reason != "uninitialized" {
		return failure(reason)
	}
	if err = lock.prepareJournal(); err != nil {
		return err
	}
	if reason := inspectData(cfg.DataDir); reason != "" {
		return failure(reason)
	}
	if err = lock.Check(); err != nil {
		return err
	}
	report, err := Doctor(ctx, cfg.DataDir)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	report.Capacity = cfg.Capacity
	sessionID, err := uuid.NewRandom()
	if err != nil {
		return failure("session_error")
	}
	request := protocol.SessionRequest{SessionID: sessionID.String(), Report: report, HeartbeatNS: int64(cfg.HeartbeatInterval), LeaseNS: int64(cfg.LeaseDuration)}
	var grant protocol.SessionGrant
	if err = client.post(ctx, "/api/agent/session", request, &grant); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	if err = checkSessionGrant(cfg, request.SessionID, "", grant); err != nil {
		return err
	}

	live, stop := context.WithCancel(ctx)
	var workers sync.WaitGroup
	defer func() { stop(); workers.Wait() }()
	heartbeatErrors := make(chan error, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		ticker := time.NewTicker(cfg.HeartbeatInterval)
		defer ticker.Stop()
		lastHeartbeat := time.Now()
		for {
			select {
			case <-live.Done():
				return
			case <-ticker.C:
				e := lock.Check()
				var heartbeat protocol.SessionGrant
				if e == nil {
					e = client.post(live, "/api/agent/heartbeat", protocol.HeartbeatRequest{SessionID: request.SessionID, Report: report}, &heartbeat)
				}
				if e == nil {
					e = checkSessionGrant(cfg, request.SessionID, grant.NodeID, heartbeat)
				}
				if e == nil {
					lastHeartbeat = time.Now()
					client.paused.Store(false)
				}
				if temporaryNetwork(e) && time.Now().Before(lastHeartbeat.Add(cfg.LeaseDuration)) {
					continue
				}
				if e != nil {
					client.paused.Store(true)
					select {
					case heartbeatErrors <- e:
					case <-live.Done():
					}
					return
				}
			}
		}
	}()
	// 商店只读管理工作独立于构建容量；一次claim，失败不自动重授动作。
	workers.Add(1)
	go func() {
		defer workers.Done()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-live.Done():
				return
			case <-ticker.C:
				if cfg.PublishTools == nil || client.paused.Load() {
					continue
				}
				err := runPublishQuery(live, client, lock, cfg, request.SessionID, grant.NodeID)
				if err != nil && !temporaryNetwork(err) {
					select {
					case heartbeatErrors <- err:
					case <-live.Done():
					}
					return
				}
			}
		}
	}()
	completion := make(chan error, cfg.Capacity)
	active := 0
	var pending *executionJournal
	var requested, claimDeadline time.Time
	claimExpired := false
	usage := &spoolUsage{}
	// 清理与构建互斥；心跳仍由原goroutine维持，未知确认阻断新claim。
	deletionReady := false
	var deletionNext time.Time
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case e := <-heartbeatErrors:
			if ctx.Err() != nil {
				return nil
			}
			return e
		case e := <-completion:
			active--
			if e != nil {
				if ctx.Err() != nil {
					return nil
				}
				return e
			}

		case <-ticker.C:
			if e := lock.Check(); e != nil {
				return e
			}
			if active == 0 && pending == nil && !client.paused.Load() {
				if !time.Now().Before(deletionNext) {
					deletionNext = time.Now().Add(5 * time.Second)
					cleanupCtx, cancelCleanup := context.WithTimeout(live, 30*time.Second)
					e := recoverNodeDeletionConfirmations(cleanupCtx, client, lock, grant.NodeID)
					deletionReady = e == nil
					if e == nil {
						var items []protocol.NodeDeletion
						e = client.get(cleanupCtx, "/api/agent/deletions?limit=1", &items)
						if e == nil && len(items) != 0 {
							// 每次只领取实际会处理的一项，由中央持久位置保证公平。
							e = advanceNodeDeletion(cleanupCtx, client, lock, grant.NodeID, items[0])
							// ACK不确定时先补原确认，不能直接进入构建。
							if temporaryNetwork(e) {
								deletionReady = false
							}
						}
					}
					cancelCleanup()
					if ctx.Err() != nil {
						return nil
					}
					if e != nil && (!deletionReady && !temporaryNetwork(e) || !nodeDeletionCanWait(e)) {
						return e
					}
				}
				if !deletionReady {
					continue
				}
			}
			if pending == nil {
				if active >= cfg.Capacity || client.paused.Load() {
					continue
				}
				var e error
				pending, e = newJournal(lock, uuid.NewString(), request.SessionID)
				if e != nil {
					return e
				}
				requested = time.Now()
				claimDeadline = requested.Add(cfg.LeaseDuration - leaseMargin(cfg))
				claimExpired = false
			}
			// 未确认claim只在原请求起点限定的窗口确认，心跳不能清掉未知证据。
			if claimExpired || !time.Now().Before(claimDeadline) {
				claimExpired = true
				continue
			}
			var task protocol.LeaseGrant
			claimCtx, stopClaim := context.WithDeadline(live, claimDeadline)
			e := client.post(claimCtx, "/api/agent/claim", protocol.ClaimRequest{SessionID: request.SessionID, ClaimKey: pending.state.ClaimKey}, &task)
			stopClaim()
			if e != nil {
				if ctx.Err() != nil {
					return nil
				}
				if temporaryNetwork(e) {
					continue
				}
				if !time.Now().Before(claimDeadline) && e.Error() == "agent_cancelled" {
					claimExpired = true
					continue
				}
				return e
			}
			if !time.Now().Before(claimDeadline) {
				claimExpired = true
				continue
			}
			journal := pending
			if task.Task == nil {
				if task.Ref != (protocol.LeaseRef{}) || task.TTLNS != 0 {
					return failure("invalid_response")
				}
				if e = journal.remove(); e != nil {
					return e
				}
				pending = nil
				continue
			}
			if task.Ref.NodeID != grant.NodeID || task.Ref.SessionID != request.SessionID {
				return failure("invalid_response")
			}
			if e = journal.setLease(task.Ref, task.RemainingBudgetNS, task.RemainingPostBudgetNS); e != nil {
				return e
			}
			if task.TTLNS <= 0 || task.TTLNS > int64(cfg.LeaseDuration) {
				return failure("invalid_response")
			}
			if !time.Now().Before(requested.Add(time.Duration(task.TTLNS) - leaseMargin(cfg))) {
				claimExpired = true
				continue
			}
			pending = nil
			// goroutine保存本次原请求时刻，后续claim不能覆盖其Authority起点。
			taskRequested := requested

			active++
			workers.Add(1)
			go func() {
				defer workers.Done()
				e := executeTask(live, client, lock, cfg, journal, task, taskRequested, usage)
				select {
				case completion <- e:
				case <-live.Done():
				}
			}()
		}
	}
}

// 单事项保护/物理失败保留证据等待下一轮；身份、协议及控制权失败退出。
func nodeDeletionCanWait(err error) bool {
	if err == nil || temporaryNetwork(err) {
		return true
	}
	safe, ok := err.(*Error)
	if !ok {
		return false
	}
	switch safe.Code {
	case "retention_protected", "retention_readers_active", "retention_ownership_unknown", "retention_timeout", "retention_io_error", "identity_mismatch", "invalid_object", "permission_denied", "authority_expired", "operation_timeout", "resource_unconfirmed", "persistence_error":
		return true
	}
	return false
}

func checkSessionGrant(cfg config.AgentConfig, sessionID, nodeID string, grant protocol.SessionGrant) error {
	id, err := uuid.Parse(grant.NodeID)
	if err != nil || id == uuid.Nil || id.String() != grant.NodeID || grant.SessionID != sessionID || nodeID != "" && grant.NodeID != nodeID {
		return failure("invalid_response")
	}
	if grant.NodeName != cfg.Node {
		return failure("node_identity_mismatch")
	}
	if grant.HeartbeatNS != int64(cfg.HeartbeatInterval) || grant.LeaseNS != int64(cfg.LeaseDuration) {
		return failure("policy_mismatch")
	}
	return nil
}
