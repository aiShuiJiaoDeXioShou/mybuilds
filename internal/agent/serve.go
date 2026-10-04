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
	// 旧执行证据阻止新身份领取，既不重放也不按旧PID发信号。
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
				if e != nil {
					select {
					case heartbeatErrors <- e:
					case <-live.Done():
					}
					return
				}
			}
		}
	}()
	completion := make(chan error, cfg.Capacity)
	active := 0
	usage := &spoolUsage{}
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
			if active >= cfg.Capacity {
				continue
			}
			if e := lock.Check(); e != nil {
				return e
			}
			journal, e := newJournal(lock, uuid.NewString(), request.SessionID)
			if e != nil {
				return e
			}
			requested := time.Now()
			var task protocol.LeaseGrant
			e = client.retryPost(live, "/api/agent/claim", protocol.ClaimRequest{SessionID: request.SessionID, ClaimKey: journal.state.ClaimKey}, &task)
			if e != nil {
				if ctx.Err() != nil {
					return nil
				}
				return e
			}
			if task.Task == nil {
				if task.Ref != (protocol.LeaseRef{}) || task.TTLNS != 0 {
					return failure("invalid_response")
				}
				if e = journal.remove(); e != nil {
					return e
				}
				continue
			}
			if task.Ref.NodeID != grant.NodeID || task.Ref.SessionID != request.SessionID {
				return failure("invalid_response")
			}
			if e = journal.setLease(task.Ref, task.RemainingBudgetNS, task.RemainingPostBudgetNS); e != nil {
				return e
			}
			active++
			workers.Add(1)
			go func() {
				defer workers.Done()
				e := executeTask(live, client, lock, cfg, journal, task, requested, usage)
				select {
				case completion <- e:
				case <-live.Done():
				}
			}()
		}
	}
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
