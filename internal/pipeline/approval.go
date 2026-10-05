package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"mybuilds/internal/mobile"
	"mybuilds/internal/protocol"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

var ErrApprovalPaused = errors.New("approval_paused")

func approvalLocalSteps(steps []StepRun) []protocol.ApprovalLocalStep {
	out := make([]protocol.ApprovalLocalStep, 0, len(steps))
	for _, step := range steps {
		item := protocol.ApprovalLocalStep{Name: step.Name, Kind: step.Kind, Status: step.Status, Reason: step.Reason, LogPath: step.LogPath, ExitCode: step.ExitCode, DurationMS: step.DurationMS, Started: step.started, CleanupFailed: step.CleanupFailed, Artifacts: []protocol.ApprovalLocalArtifact{}}
		for _, a := range step.Artifacts {
			item.Artifacts = append(item.Artifacts, protocol.ApprovalLocalArtifact{SourcePath: a.SourcePath, SnapshotPath: a.SnapshotPath, SHA256: a.SHA256, Size: a.Size})
		}
		out = append(out, item)
	}
	return out
}
func restoreApprovalSteps(resume *ApprovalResume, build preparedBuild) ([]StepRun, error) {
	e := resume.Evidence
	if e.NextOrdinaryIndex < 2 || e.NextOrdinaryIndex > len(build.steps)+1 || len(resume.Local.Steps) != e.NextOrdinaryIndex-2 {
		return nil, errors.New("approval_checkpoint_invalid")
	}
	out := make([]StepRun, 0, e.NextOrdinaryIndex-1)
	for i, item := range resume.Local.Steps {
		step := build.steps[i]
		if item.Name != step.step.Name || item.Kind != step.step.Kind || item.CleanupFailed || item.Status != "succeeded" && item.Status != "skipped" {
			return nil, errors.New("approval_checkpoint_invalid")
		}
		ledgerFound := false
		for _, l := range e.Steps {
			if l.Phase == "ordinary" && l.Index == i+1 {
				ledgerFound = l.Name == item.Name && l.Kind == item.Kind && l.Status == item.Status && l.Started == item.Started && !l.CleanupFailed && l.StopConfirmed
			}
		}
		if !ledgerFound {
			return nil, errors.New("approval_checkpoint_invalid")
		}
		s := StepRun{Name: item.Name, Kind: item.Kind, Status: item.Status, Reason: item.Reason, LogPath: item.LogPath, ExitCode: item.ExitCode, DurationMS: item.DurationMS, started: item.Started}
		for _, a := range item.Artifacts {
			s.Artifacts = append(s.Artifacts, ArtifactRecord{SourcePath: a.SourcePath, SnapshotPath: a.SnapshotPath, SHA256: a.SHA256, Size: a.Size})
		}
		out = append(out, s)
	}
	approval := build.steps[e.NextOrdinaryIndex-2]
	if approval.step.Kind != "approval" {
		return nil, errors.New("approval_checkpoint_invalid")
	}
	out = append(out, StepRun{Name: approval.step.Name, Kind: "approval", Status: "succeeded", ExitCode: -1})
	return out, nil
}
func pauseApproval(ctx context.Context, root string, build preparedBuild, step preparedStep, b *BuildRun, logger *runLogger, set *reportCollection, ios *iosBuildResources) (*ApprovalPause, error) {
	if logger.remote == nil {
		return nil, errors.New("approval_checkpoint_invalid")
	}
	r := logger.remote
	if err := r.ensureResult(root, logger); err != nil {
		return nil, err
	}
	if ios != nil && ios.resources != nil {
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		err := ios.resources.Close(closeCtx)
		cancel()
		if e := ios.save(); e != nil {
			err = errors.Join(err, e)
		}
		if err != nil {
			return nil, mobile.ErrIOSCleanup
		}
		b.IOSCleanupConfirmed = true
		b.iosResourceDigest, _ = mobile.IOSResourceDigest(ios.resources.Ownership())
	}
	if err := logger.close(); err != nil {
		return nil, err
	}
	local := protocol.ApprovalLocalCheckpoint{Workspace: root, ResultDir: r.resultDir, Steps: approvalLocalSteps(b.Steps), IOSteamID: b.iosTeamID}
	var err error
	if set != nil {
		local.Collection, err = set.checkpoint()
		if err != nil {
			return nil, err
		}
	}
	pause := &ApprovalPause{Index: step.index, Step: step.step.Name, Local: local}
	p := stepProgress("approval_checkpoint", step)
	p.PostPhase = "none"
	p.StopConfirmed = true
	p.LocalApproval = &local
	p.Approval = &protocol.ApprovalCheckpointEvidence{NextOrdinaryIndex: step.index + 1, Artifacts: []protocol.ArtifactExpectation{}, Reports: b.Reports, PublishIntents: []protocol.PublishExpectation{}, SystemResourcesClosed: true, IOSResourceDigest: b.iosResourceDigest}
	if b.Reports != nil && b.Reports.Sealed {
		ids := []string{}
		for _, f := range b.Reports.Files {
			ids = append(ids, f.ArtifactID)
		}
		p.Approval.ReportManifest = &protocol.ReportManifest{SealDigest: b.ReportSealDigest, IDs: ids}
	}
	if err = r.emit(p); err != nil {
		return nil, err
	}
	return pause, nil
}
func resumeResultRoot(options RunOptions, logger *runLogger) error {
	if options.Resume == nil {
		return nil
	}
	workspace, err := filepath.EvalSymlinks(options.Workspace)
	if err != nil || options.Remote == nil || options.Resume.Local.Workspace != workspace || options.Resume.Local.ResultDir == "" {
		return errors.New("approval_checkpoint_invalid")
	}
	dir := options.Resume.Local.ResultDir
	parent, err := filepath.EvalSymlinks(options.Remote.ResultParent)
	if err != nil {
		return errors.New("approval_checkpoint_invalid")
	}
	relative, err := filepath.Rel(parent, dir)
	info, statErr := os.Lstat(dir)
	if err != nil || statErr != nil || !info.IsDir() || info.Mode().Perm() != 0700 || relative == "." || filepath.Dir(relative) != "." {
		return errors.New("approval_checkpoint_invalid")
	}
	logger.root, err = os.OpenRoot(dir)
	if err != nil {
		return errors.New("approval_checkpoint_invalid")
	}
	logger.remote.resultDir = dir
	return nil
}

// 恢复只复核原封存文件，不重新生成或把变化的叶当作原证据。
func verifyApprovalArtifacts(ctx context.Context, result *os.Root, saved []protocol.ApprovalLocalStep) error {
	var total int64
	count := 0
	for _, step := range saved {
		for _, item := range step.Artifacts {
			count++
			total += item.Size
			if count > 128 || item.Size < 0 || total > 4<<30 || len(item.SHA256) != 64 {
				return errors.New("approval_checkpoint_invalid")
			}
			bounded := reportFS{artifactFS{result.FS(), result, ctx}}
			if err := bounded.safe(item.SnapshotPath); err != nil {
				return errors.New("approval_checkpoint_invalid")
			}
			f, err := result.OpenFile(item.SnapshotPath, os.O_RDONLY|syscall.O_NONBLOCK, 0)
			if err != nil {
				return errors.New("approval_checkpoint_invalid")
			}
			err = func() error {
				defer f.Close()
				before, err := f.Stat()
				if err != nil || !before.Mode().IsRegular() || before.Size() != item.Size {
					return errors.New("approval_checkpoint_invalid")
				}
				hash := sha256.New()
				buf := make([]byte, 64*1024)
				var read int64
				for {
					if err := ctx.Err(); err != nil {
						return err
					}
					n, err := f.Read(buf)
					read += int64(n)
					if read > item.Size {
						return errors.New("approval_checkpoint_invalid")
					}
					hash.Write(buf[:n])
					if err == io.EOF {
						break
					}
					if err != nil {
						return errors.New("approval_checkpoint_invalid")
					}
				}
				after, err := f.Stat()
				current, e := result.Lstat(item.SnapshotPath)
				if err != nil || e != nil || !sameArtifactFile(before, after) || !sameArtifactFile(before, current) || read != item.Size || hex.EncodeToString(hash.Sum(nil)) != item.SHA256 {
					return errors.New("approval_checkpoint_invalid")
				}
				return nil
			}()
			if err != nil {
				return err
			}
		}
	}
	return nil
}
