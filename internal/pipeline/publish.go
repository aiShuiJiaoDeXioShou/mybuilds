package pipeline

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"mybuilds/internal/config"
	"mybuilds/internal/process"
	"mybuilds/internal/protocol"
)

// PublishInput只在本次Run与Agent之间传递原快照，私有路径不进入公共JSON。
type PublishInput struct {
	Workspace          string
	Environment        map[string]string
	DistributionTeamID string
	Index              int
	Step               config.Step
	ArtifactID         string
	Artifact           ArtifactRecord
	ReportSealDigest   string
	ReportIDs          []string
	OnStart            func(process.StartInfo) error
}

func executePublishStep(ctx context.Context, root, build string, step preparedStep, previous []StepRun, reports *protocol.ReportEvidence, seal, teamID string, limit time.Duration, logger *runLogger) (result StepRun) {
	result = StepRun{Name: step.step.Name, Kind: "upload", Status: "failed", Reason: "publish_precheck", ExitCode: -1}
	if logger.remote == nil || logger.remote.options.Publish == nil {
		return result
	}
	r := logger.remote
	if r.blocked() != "" {
		result.Reason = r.blocked()
		return result
	}
	actionStart := time.Now()
	if r.ensureResult(root, logger) != nil {
		result.Reason = "persistence_error"
		return result
	}
	if r.emit(stepProgress("intent", step)) != nil {
		result.Reason = "persistence_error"
		return result
	}
	defer func() { r.finishStep(step, result, actionStart) }()
	var matches []ArtifactRecord
	for _, p := range previous {
		if p.Kind != "artifact" || p.Status != "succeeded" || p.CleanupFailed {
			continue
		}
		for _, a := range p.Artifacts {
			matched, err := doublestar.PathMatch(step.step.File, filepath.ToSlash(a.SourcePath))
			if err != nil {
				return result
			}
			if matched {
				matches = append(matches, a)
			}
		}
	}
	if len(matches) != 1 {
		return result
	}
	reportIDs := []string{}
	if reports != nil {
		if reports.Outcome != "passed" || !reports.Sealed || seal == "" {
			return result
		}
		for _, file := range reports.Files {
			reportIDs = append(reportIDs, file.ArtifactID)
		}
	}
	limit = r.limit(step)
	if limit < 0 {
		result.Reason = "timeout"
		return result
	}
	merged, cancel := r.merge(ctx)
	defer cancel()
	ctx = merged
	if limit > 0 {
		var stop context.CancelFunc
		ctx, stop = context.WithTimeout(ctx, limit)
		defer stop()
	}
	environment := map[string]string{}
	for _, value := range step.command.Env {
		key, val, ok := strings.Cut(value, "=")
		if ok && !strings.HasPrefix(key, "MYBUILDS_") {
			// 发布输入只带白名单和显式声明；Run自己的保留变量不转为用户声明。
			environment[key] = val
		}
	}
	input := PublishInput{Workspace: root, Environment: environment, DistributionTeamID: teamID, Index: step.index, Step: step.step, Artifact: matches[0], ReportSealDigest: seal, ReportIDs: reportIDs}
	input.OnStart = func(info process.StartInfo) error {
		if r.blocked() != "" || ctx.Err() != nil {
			return errors.New("发布执行权失效")
		}
		if result.started {
			return nil
		}
		result.started = true
		progress := stepProgress("started", step)
		progress.Started = true
		progress.PID, progress.PGID = info.PID, info.PGID
		return r.emit(progress)
	}
	receipt, err := r.options.Publish(ctx, input)
	result.CleanupFailed = receipt.CleanupFailed || receipt.Started && !receipt.StopConfirmed
	if err == nil && receipt.Status != "unknown" && receipt.Status != "failed" && receipt.StopConfirmed && !receipt.CleanupFailed && result.started {
		result.Status, result.Reason, result.ExitCode = "succeeded", "", 0
	} else if ctx.Err() != nil {
		result.Status, result.Reason = "cancelled", "cancelled"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			result.Status, result.Reason = "failed", "timeout"
		}
	} else if receipt.Status == "unknown" {
		result.Reason = "publish_unknown"
	}
	if result.CleanupFailed {
		result.Reason = "cleanup_error"
	}
	return result
}
