package agent

import (
	"context"
	"errors"
	"io"
	"mybuilds/internal/config"
	"mybuilds/internal/mobile"
	"mybuilds/internal/pipeline"
	"mybuilds/internal/protocol"
	"mybuilds/internal/scm"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 每个真实租约只调用一次现有Run，私有journal先于任何网络进度写入。
type taskExecution struct {
	publishEvidence map[string]os.FileInfo
	cfg             config.AgentConfig
	task            *protocol.TaskSnapshot
	publishSecrets  map[string]string
	lease           *executionLease
	client          *agentHTTP
	journal         *executionJournal
	spool           *logSpool
	terminal        bool
	paused          bool
	stopRenew       func()
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
	if p.Kind == "approval_checkpoint" {
		if err := execution.prepareApproval(ctx, &p); err != nil {
			return err
		}
		execution.stopRenew()
		if err := execution.lease.check(); err != nil {
			return err
		}
	}
	if p.Kind == "build_finished" {
		if !execution.iosClosed() || execution.journal.state.IOSSigningRequired != p.IOSCleanupConfirmed || p.IOSResourceDigest != iosDigestState(execution.journal.state) {
			return failure("execution_unconfirmed")
		}
		// 真实终态会结束中央租约；先等待续租worker退出，不能由其409撤销回执读取。
		execution.stopRenew()
		if err := execution.lease.check(); err != nil {
			return err
		}
	}
	if err := execution.publishManifest(ctx, &p); err != nil {
		execution.lease.cancel()
		return err
	}
	if err := execution.prepareReports(&p); err != nil {
		execution.lease.cancel()
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
	if p.Kind == "build_finished" || p.Kind == "approval_checkpoint" {
		err = execution.client.post(ctx, "/api/agent/events", event, &ack)
	} else {
		err = execution.client.retryExecutionPost(ctx, "/api/agent/events", event, &ack)
	}
	if err != nil {
		execution.lease.cancel()
		return err
	}
	if p.Kind == "build_finished" {
		if err = recordResourceTerminal(execution.journal, event, ack); err != nil {
			execution.lease.cancel()
			return err
		}
	}
	if err = execution.journal.ackEvent(ack); err != nil {
		execution.lease.cancel()
		return err
	}
	if p.Kind == "finished" && p.StepKind == "artifact" || p.Kind == "reports_checked" && p.Index == 0 {
		if err = execution.uploadArtifacts(ctx); err != nil {
			execution.lease.cancel()
			return err
		}
	}
	if p.Kind == "approval_checkpoint" {
		execution.paused = true
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
	if err = execution.client.retryExecutionPost(ctx, "/api/agent/logs", chunk, &ack); err != nil {
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
	renewStop := make(chan struct{})
	var renewOnce sync.Once
	stopRenew := func() { renewOnce.Do(func() { close(renewStop) }); <-renewDone }
	go func() { defer close(renewDone); lease.renew(client, renewStop) }()
	defer func() { lease.cancel(); stopRenew() }()
	execution := &taskExecution{cfg: cfg, task: grant.Task, lease: lease, client: client, journal: journal, spool: newSpool(journal, usage), stopRenew: stopRenew}
	task := grant.Task
	journal.mu.Lock()
	journal.state.IOSSigningRequired = task.Definition.IOSSigning != nil
	err = journal.saveLocked()
	journal.mu.Unlock()
	if err != nil {
		return err
	}
	values, err := loadSecrets(cfg)
	if err != nil {
		return execution.zeroAction("precheck_error", grant.RemainingBudgetNS, grant.RemainingPostBudgetNS)
	}
	secrets, err := taskSecrets(cfg, task.Definition, values)
	if err != nil {
		return execution.zeroAction("precheck_error", grant.RemainingBudgetNS, grant.RemainingPostBudgetNS)
	}
	execution.publishSecrets = secrets
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
	var checkout scm.CheckoutResult
	var checkoutErr error
	if task.Resume != nil {
		checkout.Workspace = journal.state.Approval.Local.Workspace
		checkout.StopConfirmed = true
		checkoutErr = validateApprovalResource(journal)
	} else {
		checkout, checkoutErr = scm.Checkout(lease.ctx, options)
	}
	journal.mu.Lock()
	journal.state.StopConfirmed = checkout.StopConfirmed
	journal.state.CleanupFailed = !checkout.StopConfirmed
	saveErr := journal.saveLocked()
	journal.mu.Unlock()
	if saveErr != nil {
		return saveErr
	}
	if checkoutErr == nil && task.Resume != nil {
		if err = execution.registerApprovalResource(lease.ctx); err != nil {
			return err
		}
	}
	if checkoutErr == nil && task.Resume == nil {
		// 从本次领取起计时，登记网络与fsync不能取得新的构建预算。
		registrationCtx := lease.ctx
		registrationCancel := func() {}
		if grant.RemainingBudgetNS != nil {
			registrationCtx, registrationCancel = context.WithDeadline(lease.ctx, requested.Add(time.Duration(*grant.RemainingBudgetNS)))
		}
		err = execution.registerWorkspace(registrationCtx, checkout.Workspace)
		registrationCancel()
		if err != nil {
			return err
		}
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
	runStart := time.Now()
	result, runErr := pipeline.Run(lease.user, document, pipeline.RunOptions{PreviewOptions: pipeline.PreviewOptions{Names: []string{task.BuildName}, Params: task.Parameters, Changes: task.Changes}, Workspace: checkout.Workspace, Resume: approvalRunResume(journal, task), Output: io.Discard, Remote: &pipeline.RemoteOptions{Publish: execution.publish, AuthorityContext: lease.ctx, Facts: facts, Secrets: secrets, ResultParent: resultParent, ResultCreated: execution.registerResult, RemainingBudgetNS: remaining, RemainingPostBudgetNS: grant.RemainingPostBudgetNS, IOSCheckpoint: execution.saveIOSOwnership, Progress: execution.progress, Log: execution.log}})
	if execution.paused && errors.Is(runErr, pipeline.ErrApprovalPaused) {
		return nil
	}
	if execution.terminal {
		journal.mu.Lock()
		stopped := journal.state.StopConfirmed && !journal.state.CleanupFailed && iosClosedState(journal.state)
		journal.mu.Unlock()
		if !stopped {
			return failure("execution_unconfirmed")
		}
		return journal.remove()
	}
	if errors.Is(runErr, pipeline.ErrFlutterCleanup) {
		return execution.unconfirmedFlutterPrecheck()
	}
	if errors.Is(runErr, mobile.ErrIOSCleanup) {
		journal.mu.Lock()
		journal.state.CleanupFailed = true
		journal.state.StopConfirmed = false
		err = journal.saveLocked()
		journal.mu.Unlock()
		if err != nil {
			return err
		}
	}
	if result == nil && runErr != nil && lease.check() == nil && !errors.Is(runErr, mobile.ErrIOSCleanup) {
		if remaining != nil {
			*remaining = max(0, *remaining-int64(time.Since(runStart)))
		}
		return execution.zeroAction("precheck_error", remaining, grant.RemainingPostBudgetNS)
	}
	return execution.confirmStopped(parent)
}
func (execution *taskExecution) zeroAction(reason string, remaining *int64, post int64) error {
	p := protocol.ExecutionProgress{IOSCleanupConfirmed: execution.journal.state.IOSSigningRequired && execution.iosClosed(), Kind: "build_finished", Status: "failed", Reason: reason, PostPhase: "none", StopConfirmed: true, ExitCode: -1, At: time.Now().UTC(), RemainingBudgetNS: remaining, RemainingPostBudgetNS: post, ArtifactSteps: []protocol.ArtifactExpectation{}}
	if err := execution.progress(execution.lease.ctx, p); err != nil {
		return err
	}
	return execution.journal.remove()
}

// 真实工具预检清理不确定时保留私有证据，不能沿零用户动作分支确认整个构建停止。
func (execution *taskExecution) unconfirmedFlutterPrecheck() error {
	execution.lease.cancel()
	execution.journal.mu.Lock()
	defer execution.journal.mu.Unlock()
	execution.journal.state.StopConfirmed = false
	execution.journal.state.CleanupFailed = true
	if err := execution.journal.saveLocked(); err != nil {
		return err
	}
	return failure("execution_unconfirmed")
}
