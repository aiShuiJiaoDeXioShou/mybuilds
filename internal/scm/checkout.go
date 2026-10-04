package scm

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type CheckoutOptions struct{ DataDir, Repository, Branch, SHA, SSHKey, KnownHosts string }
type CheckoutResult struct {
	Workspace     string
	StopConfirmed bool
}

// Checkout只创建本attempt的私有工作区，固定commit且不触及来源工作树。
func Checkout(parent context.Context, options CheckoutOptions) (result CheckoutResult, err error) {
	result.StopConfirmed = true
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	if ctx.Err() != nil {
		return result, contextFailure(ctx)
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return result, failure("platform_unsupported")
	}
	protocol, e := sourceProtocol(options.Repository)
	if e != nil || options.Branch == "" || len(options.Branch) > 1024 || controls(options.Branch) || !fullOID.MatchString(options.SHA) {
		return result, failure("input_invalid")
	}
	info, e := os.Lstat(options.DataDir)
	if e != nil || !checkoutDirectory(info) {
		return result, failure("cache_failed")
	}
	dataRoot, e := os.OpenRoot(options.DataDir)
	if e != nil {
		return result, failure("cache_failed")
	}
	defer dataRoot.Close()
	if e = dataRoot.Mkdir("scm", 0700); e != nil && !os.IsExist(e) {
		return result, failure("cache_failed")
	}
	baseInfo, e := dataRoot.Lstat("scm")
	if e != nil || !checkoutDirectory(baseInfo) {
		return result, failure("cache_failed")
	}
	base := filepath.Join(dataRoot.Name(), "scm")
	attempt, e := os.MkdirTemp(base, "checkout-")
	if e != nil {
		return result, failure("cache_failed")
	}
	attemptInfo, e := os.Lstat(attempt)
	if e != nil {
		return result, failure("cache_failed")
	}
	runner := gitRunner{dir: attempt}
	defer func() {
		result.StopConfirmed = !runner.cleanupFailed
		now, first := os.Lstat(attempt)
		currentBase, second := dataRoot.Lstat("scm")
		currentData, third := os.Lstat(dataRoot.Name())
		if first != nil || second != nil || third != nil || !os.SameFile(now, attemptInfo) || !os.SameFile(currentBase, baseInfo) || !os.SameFile(currentData, info) {
			result.Workspace = ""
			err = failure("cleanup_failed")
			return
		}
		if runner.cleanupFailed {
			result.Workspace = ""
			err = failure("cleanup_failed")
			return
		}
		if err != nil {
			result.Workspace = ""
			if os.RemoveAll(attempt) != nil {
				err = failure("cleanup_failed")
			}
		}
	}()
	var sshEnv map[string]string
	if protocol == "ssh" {
		sshEnv, e = sshPairEnvironment(attempt, options.SSHKey, options.KnownHosts)
		if e != nil {
			return result, e
		}
	} else if options.SSHKey != "" || options.KnownHosts != "" {
		return result, failure("credentials_invalid")
	}
	runner, e = prepareGitRunner(attempt, protocol, sshEnv)
	if e != nil {
		return result, e
	}
	runner.combinedLimit = true
	ref := "refs/heads/" + options.Branch
	if _, _, e = runner.run(ctx, 32<<10, "check-ref-format", ref); e != nil {
		return result, e
	}
	format := "sha1"
	if len(options.SHA) == 64 {
		format = "sha256"
	}
	gitdir := filepath.Join(attempt, "repo.git")
	if _, _, e = runner.run(ctx, 32<<10, "init", "--bare", "--object-format="+format, "--template="+filepath.Join(attempt, "empty"), gitdir); e != nil {
		return result, e
	}
	runner.prefix = append(runner.prefix, "--git-dir="+gitdir)
	if _, _, e = runner.run(ctx, 32<<10, "fetch", "--quiet", "--no-tags", "--no-recurse-submodules", "--no-auto-maintenance", "--no-write-fetch-head", "--", options.Repository, "+"+ref+":refs/mybuilds/branch"); e != nil {
		return result, e
	}
	headData, _, e := runner.run(ctx, 32<<10, "rev-parse", "--verify", "--end-of-options", "refs/mybuilds/branch")
	if e != nil {
		return result, e
	}
	head := strings.TrimSpace(string(headData))
	selected := strings.ToLower(options.SHA)
	if !fullOID.MatchString(head) {
		return result, failure("ref_invalid")
	}
	for _, oid := range []string{head, selected} {
		kind, _, e := runner.run(ctx, 32<<10, "cat-file", "-t", oid)
		if e != nil || strings.TrimSpace(string(kind)) != "commit" {
			if ctx.Err() != nil {
				return result, contextFailure(ctx)
			}
			return result, failure("ref_invalid")
		}
	}
	if _, exit, e := runner.run(ctx, 32<<10, "merge-base", "--is-ancestor", selected, head); e != nil {
		if exit == 1 {
			return result, failure("ref_unreachable")
		}
		return result, e
	}
	workspace := filepath.Join(attempt, "workspace")
	if e = os.Mkdir(workspace, 0700); e != nil {
		return result, failure("checkout_failed")
	}
	if _, _, e = runner.run(ctx, 32<<10, "config", "core.bare", "false"); e != nil {
		return result, e
	}
	if _, _, e = runner.run(ctx, 32<<10, "config", "core.worktree", "../workspace"); e != nil {
		return result, e
	}
	runner.prefix = append(runner.prefix, "--work-tree="+workspace, "-c", "core.attributesFile=/dev/null")
	if _, _, e = runner.run(ctx, 32<<10, "checkout", "--quiet", "--detach", "--force", selected, "--"); e != nil {
		return result, e
	}
	actual, _, e := runner.run(ctx, 32<<10, "rev-parse", "--verify", "--end-of-options", "HEAD")
	if e != nil || strings.TrimSpace(string(actual)) != selected {
		return result, failure("checkout_failed")
	}
	dotgit, e := os.OpenFile(filepath.Join(workspace, ".git"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return result, failure("checkout_failed")
	}
	_, writeErr := dotgit.WriteString("gitdir: ../repo.git\n")
	syncErr := dotgit.Sync()
	closeErr := dotgit.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return result, failure("checkout_failed")
	}
	if ctx.Err() != nil {
		return result, contextFailure(ctx)
	}
	result.Workspace = workspace
	return result, nil
}
