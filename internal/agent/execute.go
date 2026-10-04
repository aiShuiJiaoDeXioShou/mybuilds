package agent

import (
	"context"
	"io"
	"mybuilds/internal/config"
	"mybuilds/internal/pipeline"
	"mybuilds/internal/protocol"
	"mybuilds/internal/scm"
	"strconv"
	"strings"
	"time"
)

// 每个真实租约只调用一次现有Run，私有journal先于任何网络进度写入。
type taskExecution struct {
	lease    *executionLease
	client   *agentHTTP
	journal  *executionJournal
	spool    *logSpool
	terminal bool
}

func (execution *taskExecution) progress(ctx context.Context, p protocol.ExecutionProgress) error {
	if err := execution.lease.check(); err != nil {
		// 失权仍保存本次真实清理证据，但不补传旧fence事件。
		execution.journal.mu.Lock()
		switch p.Kind {
		case "started":
			execution.journal.state.Started = true
			execution.journal.state.StopConfirmed = false
			if p.PID > 0 {
				execution.journal.state.PID = p.PID
				execution.journal.state.PGID = p.PGID
			}
		case "finished", "build_finished":
			execution.journal.state.StopConfirmed = p.StopConfirmed
			execution.journal.state.CleanupFailed = execution.journal.state.CleanupFailed || p.CleanupFailed
		}
		errSave := execution.journal.saveLocked()
		execution.journal.mu.Unlock()
		if errSave != nil {
			return errSave
		}
		return err
	}
	if err := execution.declare(&p); err != nil {
		execution.lease.cancel()
		return err
	}
	if p.Kind == "build_finished" {
		execution.journal.mu.Lock()
		p.LastLogSeq = execution.journal.state.LastLogSeq
		p.LastLogOffset = execution.journal.state.LastLogOffset
		p.LastArtifactSeq = int64(len(execution.journal.state.Artifacts))
		p.ArtifactSteps = append([]protocol.ArtifactExpectation{}, execution.journal.state.ArtifactSteps...)
		execution.journal.mu.Unlock()
	}
	event, err := execution.journal.prepareEvent(p)
	if err != nil {
		execution.lease.cancel()
		return err
	}
	var ack protocol.EventAck
	if err = execution.client.retryPost(ctx, "/api/agent/events", event, &ack); err != nil {
		execution.lease.cancel()
		return err
	}
	if err = execution.journal.ackEvent(ack); err != nil {
		execution.lease.cancel()
		return err
	}
	if p.Kind == "finished" && p.StepKind == "artifact" {
		if err = execution.uploadArtifacts(ctx); err != nil {
			execution.lease.cancel()
			return err
		}
	}
	if p.Kind == "build_finished" {
		execution.terminal = true
	}
	return nil
}
func (execution *taskExecution) log(ctx context.Context, record protocol.LogRecord) error {
	if err := execution.lease.check(); err != nil {
		return err
	}
	chunk, err := execution.spool.append(record)
	if err != nil {
		execution.lease.cancel()
		return err
	}
	var ack protocol.LogAck
	if err = execution.client.retryPost(ctx, "/api/agent/logs", chunk, &ack); err != nil {
		execution.lease.cancel()
		return err
	}
	if err = execution.spool.ack(ack); err != nil {
		execution.lease.cancel()
		return err
	}
	return nil
}
func executeTask(parent context.Context, client *agentHTTP, lock *dataLock, cfg config.AgentConfig, journal *executionJournal, grant protocol.LeaseGrant, requested time.Time, usage *spoolUsage) error {
	lease, err := newExecutionLease(parent, lock, cfg, grant, requested)
	if err != nil {
		return err
	}
	defer lease.close()
	renewDone := make(chan struct{})
	go func() { defer close(renewDone); lease.renew(client) }()
	defer func() { lease.cancel(); <-renewDone }()
	execution := &taskExecution{lease: lease, client: client, journal: journal, spool: newSpool(journal, usage)}
	task := grant.Task
	values, err := loadSecrets(cfg)
	if err != nil {
		return execution.zeroAction("precheck_error", grant.RemainingBudgetNS, grant.RemainingPostBudgetNS)
	}
	secrets, err := taskSecrets(cfg, task.Definition, values)
	if err != nil {
		return execution.zeroAction("precheck_error", grant.RemainingBudgetNS, grant.RemainingPostBudgetNS)
	}
	journal.mu.Lock()
	journal.state.StopConfirmed = false
	err = journal.saveLocked()
	journal.mu.Unlock()
	if err != nil {
		return err
	}
	checkoutStart := requested
	options := scm.CheckoutOptions{DataDir: cfg.DataDir, Repository: task.Repository, Branch: task.Branch, SHA: task.SHA}
	if strings.HasPrefix(task.Repository, "ssh://") || strings.Contains(task.Repository, "@") && !strings.Contains(task.Repository, "://") {
		options.SSHKey = values["MYBUILDS_GIT_SSH_KEY"]
		options.KnownHosts = values["MYBUILDS_GIT_KNOWN_HOSTS"]
	}
	checkout, checkoutErr := scm.Checkout(lease.ctx, options)
	journal.mu.Lock()
	journal.state.StopConfirmed = checkout.StopConfirmed
	journal.state.CleanupFailed = !checkout.StopConfirmed
	saveErr := journal.saveLocked()
	journal.mu.Unlock()
	if saveErr != nil {
		return saveErr
	}
	remaining := cloneBudget(grant.RemainingBudgetNS)
	if remaining != nil {
		*remaining -= int64(time.Since(checkoutStart))
		if *remaining < 0 {
			*remaining = 0
		}
	}
	if checkoutErr != nil {
		if checkout.StopConfirmed && lease.check() == nil {
			return execution.zeroAction("checkout_error", remaining, grant.RemainingPostBudgetNS)
		}
		return execution.confirmStopped(parent)
	}
	resultParent, err := lock.resultParent()
	if err != nil {
		return err
	}
	facts := map[string]string{"project": task.Project, "build.id": grant.Ref.BuildID, "build.number": strconv.FormatInt(task.Number, 10), "node.name": cfg.Node, "git.sha": task.SHA, "git.branch": task.Branch}
	document := &config.Document{Version: 1, Builds: map[string]*config.Build{task.BuildName: &task.Definition}}
	result, runErr := pipeline.Run(lease.user, document, pipeline.RunOptions{PreviewOptions: pipeline.PreviewOptions{Names: []string{task.BuildName}, Params: task.Parameters}, Workspace: checkout.Workspace, Output: io.Discard, Remote: &pipeline.RemoteOptions{AuthorityContext: lease.ctx, Facts: facts, Secrets: secrets, ResultParent: resultParent, RemainingBudgetNS: remaining, RemainingPostBudgetNS: grant.RemainingPostBudgetNS, Progress: execution.progress, Log: execution.log}})
	if execution.terminal {
		journal.mu.Lock()
		stopped := journal.state.StopConfirmed && !journal.state.CleanupFailed
		journal.mu.Unlock()
		if !stopped {
			return failure("execution_unconfirmed")
		}
		return journal.remove()
	}
	if result == nil && runErr != nil && lease.check() == nil {
		return execution.zeroAction("precheck_error", remaining, grant.RemainingPostBudgetNS)
	}
	return execution.confirmStopped(parent)
}
func (execution *taskExecution) zeroAction(reason string, remaining *int64, post int64) error {
	p := protocol.ExecutionProgress{Kind: "build_finished", Status: "failed", Reason: reason, PostPhase: "none", StopConfirmed: true, ExitCode: -1, At: time.Now().UTC(), RemainingBudgetNS: remaining, RemainingPostBudgetNS: post, ArtifactSteps: []protocol.ArtifactExpectation{}}
	if err := execution.progress(execution.lease.ctx, p); err != nil {
		return err
	}
	return execution.journal.remove()
}
