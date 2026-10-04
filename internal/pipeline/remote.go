package pipeline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mybuilds/internal/protocol"
)

// remoteRun保存本次具体运行权及单调预算，不恢复或缓存执行结果。
type remoteRun struct {
	options    *RemoteOptions
	authority  context.Context
	cancel     context.CancelFunc
	mu         sync.Mutex
	broken     string
	remaining  *int64
	post       int64
	postActive bool
	tick       time.Time
	resultDir  string
}

func newRemote(options *RemoteOptions) (*remoteRun, error) {
	if options.AuthorityContext == nil || options.Progress == nil || options.Log == nil || options.ResultParent == "" || options.RemainingPostBudgetNS < 0 || options.RemainingBudgetNS != nil && *options.RemainingBudgetNS < 0 {
		return nil, errors.New("远程执行选项无效")
	}
	authority, cancel := context.WithCancel(options.AuthorityContext)
	r := &remoteRun{options: options, authority: authority, cancel: cancel, post: options.RemainingPostBudgetNS, tick: time.Now()}
	if options.RemainingBudgetNS != nil {
		v := *options.RemainingBudgetNS
		r.remaining = &v
	}
	return r, nil
}
func (r *remoteRun) fail(reason string) {
	r.mu.Lock()
	if r.broken == "" {
		r.broken = reason
	}
	r.mu.Unlock()
	r.cancel()
}
func (r *remoteRun) blocked() string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.broken == "" && r.authority.Err() != nil {
		r.broken = "authority_lost"
	}
	if r.broken == "" {
		return ""
	}
	if r.broken == "authority_lost" {
		return r.broken
	}
	return "persistence_error"
}
func (r *remoteRun) chargeLocked() {
	now := time.Now()
	spent := now.Sub(r.tick).Nanoseconds()
	r.tick = now
	if r.postActive {
		r.post = max(0, r.post-spent)
	} else if r.remaining != nil {
		*r.remaining = max(0, *r.remaining-spent)
	}
}
func (r *remoteRun) budgets() (*int64, int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.chargeLocked()
	var remaining *int64
	if r.remaining != nil {
		v := *r.remaining
		remaining = &v
	}
	return remaining, r.post
}
func (r *remoteRun) limit(step preparedStep) time.Duration {
	remaining, post := r.budgets()
	budget := remaining
	if step.phase != "ordinary" {
		budget = &post
	}
	own, _ := time.ParseDuration(step.step.Timeout)
	if budget == nil {
		return own
	}
	if *budget <= 0 {
		return -1
	}
	if own == 0 || int64(own) > *budget {
		return time.Duration(*budget)
	}
	return own
}
func (r *remoteRun) emit(p protocol.ExecutionProgress) error {
	remaining, post := r.budgets()
	p.RemainingBudgetNS, p.RemainingPostBudgetNS = remaining, post
	if p.At.IsZero() {
		p.At = time.Now().UTC()
	}
	if p.ArtifactSteps == nil {
		p.ArtifactSteps = []protocol.ArtifactExpectation{}
	}
	p.LocalResultDir = r.resultDir
	if err := r.options.Progress(r.authority, p); err != nil {
		r.fail("persistence_error")
		return errors.New("进度保存失败")
	}
	return nil
}
func stepProgress(kind string, step preparedStep) protocol.ExecutionProgress {
	return protocol.ExecutionProgress{Kind: kind, Phase: step.phase, Index: step.index, Name: step.step.Name, StepKind: step.step.Kind, ExitCode: -1}
}
func (r *remoteRun) skipped(step preparedStep, reason string) {
	p := stepProgress("skipped", step)
	p.Status, p.Reason, p.StopConfirmed = "skipped", reason, true
	_ = r.emit(p)
}
func (r *remoteRun) ensureResult(workspace string, logger *runLogger) error {
	if logger.root != nil {
		return nil
	}
	dir, err := resultDirectoryParent(workspace, r.options.ResultParent)
	if err != nil {
		return err
	}
	logger.root, err = os.OpenRoot(dir)
	if err != nil {
		_ = os.RemoveAll(dir)
		return errors.New("结果目录不可用")
	}
	r.resultDir = dir
	return nil
}
func resultDirectoryParent(workspace, parent string) (string, error) {
	info, err := os.Lstat(parent)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return "", errors.New("结果目录不可用")
	}
	base, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", errors.New("结果目录不可用")
	}
	base, err = filepath.Abs(base)
	relative, relErr := filepath.Rel(workspace, base)
	if err != nil || relErr != nil || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("结果目录不可用")
	}
	dir, err := os.MkdirTemp(base, "mybuilds-")
	if err != nil {
		return "", errors.New("结果目录不可用")
	}
	return dir, nil
}
func (r *remoteRun) merge(ctx context.Context) (context.Context, context.CancelFunc) {
	merged, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(r.authority, cancel)
	if r.authority.Err() != nil {
		cancel()
	}
	return merged, func() { stop(); cancel() }
}
func (r *remoteRun) beginPost(phase, reason string) {
	r.mu.Lock()
	r.chargeLocked()
	r.postActive = true
	r.tick = time.Now()
	r.mu.Unlock()
	_ = r.emit(protocol.ExecutionProgress{Kind: "post_selected", PostPhase: phase, Reason: reason, ExitCode: -1})
}

func (r *remoteRun) inactivePost(build preparedBuild, b *BuildRun, reason string) {
	selected := "none"
	started := false
	for _, step := range b.Steps {
		started = started || step.started
	}
	if started && b.Status != "cancelled" {
		selected = "success"
		if b.Status == "failed" {
			selected = "failure"
		}
	}
	if r.blocked() == "" {
		r.beginPost(selected, "")
	}
	for _, group := range []struct {
		name  string
		steps []preparedStep
	}{{"success", build.success}, {"failure", build.failure}, {"always", build.always}} {
		for _, step := range group.steps {
			skipReason := reason
			if r.blocked() == "" && (group.name != selected && group.name != "always" || !started) {
				skipReason = "not_selected"
			}
			b.Post = append(b.Post, skippedStep(step, skipReason))
			if r.blocked() == "" {
				r.skipped(step, skipReason)
			}
		}
	}
}
func (r *remoteRun) finishStep(step preparedStep, result StepRun, started time.Time) {
	p := stepProgress("finished", step)
	p.Status, p.Reason, p.Started, p.StopConfirmed, p.CleanupFailed, p.ExitCode = result.Status, result.Reason, result.started, !result.CleanupFailed, result.CleanupFailed, result.ExitCode
	p.ElapsedNS = time.Since(started).Nanoseconds()
	for _, artifact := range result.Artifacts {
		p.LocalArtifacts = append(p.LocalArtifacts, protocol.CollectedArtifact{SnapshotPath: artifact.SnapshotPath, Name: filepath.Base(artifact.SnapshotPath), SHA256: artifact.SHA256, Size: artifact.Size})
	}
	_ = r.emit(p)
}
