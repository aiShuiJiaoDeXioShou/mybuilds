package scm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"mybuilds/internal/config"
	"mybuilds/internal/process"
)

type Options struct{ DataDir, Repository, Branch, Ref, File, SecretsFile, FileMode string }
type Snapshot struct {
	Missing      bool
	SHA          string
	Content      []byte
	Digest, File string
}

// Error只携带固定原因，不能附带Git、URL或凭据正文。
type Error struct{ Code string }

func (e *Error) Error() string  { return e.Code }
func failure(code string) error { return &Error{Code: "scm_" + code} }

var fullOID = regexp.MustCompile(`^(?:[a-fA-F0-9]{40}|[a-fA-F0-9]{64})$`)
var scpURL = regexp.MustCompile(`^(?:[A-Za-z0-9][A-Za-z0-9._-]*@)?[A-Za-z0-9][A-Za-z0-9.-]*:(.+)$`)

func controls(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return r < 32 || r == 127 }) >= 0
}
func sourceProtocol(repository string) (string, error) {
	if repository == "" || len(repository) > 4096 || controls(repository) || strings.HasPrefix(repository, "-") {
		return "", failure("input_invalid")
	}
	if filepath.IsAbs(repository) {
		return "file", nil
	}
	if strings.Contains(repository, "://") {
		u, err := url.Parse(repository)
		if err != nil || u.RawQuery != "" || u.Fragment != "" || controls(u.Path) || controls(u.Host) || u.Opaque != "" {
			return "", failure("input_invalid")
		}
		if u.User != nil {
			if _, password := u.User.Password(); password || u.Scheme != "ssh" || controls(u.User.Username()) || strings.HasPrefix(u.User.Username(), "-") {
				return "", failure("input_invalid")
			}
		}
		switch u.Scheme {
		case "file":
			if (u.Host == "" || u.Host == "localhost") && filepath.IsAbs(u.Path) {
				return "file", nil
			}
		case "ssh":
			if u.Hostname() != "" && !strings.HasPrefix(u.Hostname(), "-") && u.Path != "" {
				return "ssh", nil
			}
		case "https", "http":
			if u.Hostname() == "" || strings.HasPrefix(u.Hostname(), "-") {
				break
			}
			if u.Scheme == "http" {
				ip := net.ParseIP(u.Hostname())
				if !strings.EqualFold(u.Hostname(), "localhost") && (ip == nil || !ip.IsLoopback()) {
					break
				}
			}
			return u.Scheme, nil
		}
		return "", failure("input_invalid")
	}
	if match := scpURL.FindStringSubmatch(repository); len(match) == 2 && !strings.HasPrefix(match[1], ":") && !strings.HasPrefix(match[1], "-") {
		return "ssh", nil
	}
	return "", failure("input_invalid")
}
func validFile(file string) bool {
	if file == "" || len(file) > 1024 || controls(file) || strings.ContainsAny(file, `\:*?[]`) || path.IsAbs(file) || path.Clean(file) != file {
		return false
	}
	for _, part := range strings.Split(file, "/") {
		if part == "." || part == ".." || part == "" {
			return false
		}
	}
	return true
}

// repositoryRead只属于一次真实固定SHA读取；关闭只清理自己创建的目录。
type repositoryRead struct {
	runner               gitRunner
	sha, workspace, base string
	owned, baseInfo      os.FileInfo
}

func (opened *repositoryRead) Close() error {
	if opened.runner.cleanupFailed {
		return failure("cleanup_failed")
	}
	now, e := os.Lstat(opened.workspace)
	parent, f := os.Lstat(opened.base)
	if e != nil || f != nil || !os.SameFile(now, opened.owned) || !os.SameFile(parent, opened.baseInfo) || os.RemoveAll(opened.workspace) != nil {
		return failure("cleanup_failed")
	}
	return nil
}
func openRepository(ctx context.Context, options Options) (opened *repositoryRead, err error) {
	if ctx.Err() != nil {
		return nil, contextFailure(ctx)
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return nil, failure("platform_unsupported")
	}
	protocol, e := sourceProtocol(options.Repository)
	if e != nil || options.Branch == "" || len(options.Branch) > 1024 || controls(options.Branch) || (options.Ref != "" && !fullOID.MatchString(options.Ref)) {
		return nil, failure("input_invalid")
	}
	if options.DataDir == "" {
		return nil, failure("cache_failed")
	}
	dataDir, e := filepath.Abs(options.DataDir)
	if e == nil {
		dataDir, e = filepath.EvalSymlinks(dataDir)
	}
	if e != nil {
		return nil, failure("cache_failed")
	}
	base := filepath.Join(dataDir, "scm")
	if os.Mkdir(base, 0700) != nil {
		info, e := os.Lstat(base)
		if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0700 {
			return nil, failure("cache_failed")
		}
	}
	baseInfo, e := os.Lstat(base)
	if e != nil {
		return nil, failure("cache_failed")
	}
	workspace, e := os.MkdirTemp(base, "read-")
	if e != nil {
		return nil, failure("cache_failed")
	}
	owned, e := os.Lstat(workspace)
	if e != nil {
		return nil, failure("cache_failed")
	}
	opened = &repositoryRead{runner: gitRunner{dir: workspace}, workspace: workspace, base: base, owned: owned, baseInfo: baseInfo}
	ownedRead := opened
	success := false
	defer func() {
		if !success {
			if cleanup := ownedRead.Close(); cleanup != nil {
				err = cleanup
			}
		}
	}()
	runner := &opened.runner
	var sshEnv map[string]string
	if protocol == "ssh" {
		sshEnv, e = sshEnvironment(workspace, options.SecretsFile)
		if e != nil {
			return nil, e
		}
	}
	*runner, e = prepareGitRunner(workspace, protocol, sshEnv)
	if e != nil {
		return nil, e
	}
	refName := "refs/heads/" + options.Branch
	if _, _, e = runner.run(ctx, 32<<10, "check-ref-format", refName); e != nil {
		return nil, e
	}
	remote, _, e := runner.run(ctx, 32<<10, "ls-remote", "--refs", "--exit-code", "--", options.Repository, refName)
	if e != nil {
		return nil, e
	}
	format := ""
	matched := 0
	for _, line := range strings.Split(strings.TrimSpace(string(remote)), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) == 2 && fields[1] == refName {
			matched++
			if !fullOID.MatchString(fields[0]) {
				return nil, failure("git_failed")
			}
			format = "sha1"
			if len(fields[0]) == 64 {
				format = "sha256"
			}
		}
	}
	if matched != 1 {
		return nil, failure("branch_missing")
	}
	if options.Ref != "" && ((format == "sha1" && len(options.Ref) != 40) || (format == "sha256" && len(options.Ref) != 64)) {
		return nil, failure("ref_invalid")
	}
	_, _, e = runner.run(ctx, 32<<10, "init", "--bare", "--object-format="+format, "--template="+filepath.Join(workspace, "empty"), filepath.Join(workspace, "repo.git"))
	if e != nil {
		return nil, e
	}
	runner.prefix = append(runner.prefix, "--git-dir="+filepath.Join(workspace, "repo.git"))
	_, _, e = runner.run(ctx, 32<<10, "fetch", "--quiet", "--no-tags", "--no-recurse-submodules", "--no-auto-maintenance", "--no-write-fetch-head", "--", options.Repository, "+"+refName+":refs/mybuilds/branch")
	if e != nil {
		return nil, e
	}
	headData, _, e := runner.run(ctx, 32<<10, "rev-parse", "--verify", "--end-of-options", "refs/mybuilds/branch")
	if e != nil {
		return nil, e
	}
	head := strings.TrimSpace(string(headData))
	if !fullOID.MatchString(head) {
		return nil, failure("git_failed")
	}
	// merge-base会自动剥tag；先确认固定分支HEAD本身就是commit。
	if options.Ref != "" {
		headKind, _, headErr := runner.run(ctx, 32<<10, "cat-file", "-t", head)
		if headErr != nil {
			return nil, headErr
		}
		if strings.TrimSpace(string(headKind)) != "commit" {
			return nil, failure("ref_invalid")
		}
	}
	selected := head
	if options.Ref != "" {
		selected = strings.ToLower(options.Ref)
	}
	kind, _, e := runner.run(ctx, 32<<10, "cat-file", "-t", selected)
	if e != nil || strings.TrimSpace(string(kind)) != "commit" {
		if ctx.Err() != nil {
			return nil, contextFailure(ctx)
		}
		return nil, failure("ref_invalid")
	}
	_, exit, e := runner.run(ctx, 32<<10, "merge-base", "--is-ancestor", selected, head)
	if e != nil {
		if exit == 1 {
			return nil, failure("ref_unreachable")
		}
		return nil, e
	}
	opened.sha = selected
	success = true
	return opened, nil
}

// ReadPipeline只读来源；每请求自有bare目录，避免共享引用与并发锁层。
func ReadPipeline(parent context.Context, options Options) (snapshot Snapshot, err error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	if (options.FileMode != "" && options.FileMode != "required" && options.FileMode != "optional" && options.FileMode != "none") || !validFile(options.File) {
		return Snapshot{}, failure("input_invalid")
	}
	opened, err := openRepository(ctx, options)
	if err != nil {
		return Snapshot{}, err
	}
	defer func() {
		if e := opened.Close(); e != nil {
			snapshot = Snapshot{}
			err = e
		}
	}()
	runner, selected := &opened.runner, opened.sha
	if options.FileMode == "none" {
		return Snapshot{SHA: selected, File: options.File}, nil
	}
	// 每个父级必须是真tree；symlink/gitlink下的空输出不是真缺失。
	components := strings.Split(options.File, "/")
	for i := 1; i < len(components); i++ {
		prefix := strings.Join(components[:i], "/")
		parent, _, err := runner.run(ctx, 32<<10, "ls-tree", "-l", "-z", "--full-tree", selected, "--", prefix)
		if err != nil {
			return Snapshot{}, err
		}
		if len(parent) == 0 {
			return missingPipeline(selected, options)
		}
		rows := strings.Split(string(parent), "\x00")
		if len(rows) != 2 || rows[1] != "" {
			return Snapshot{}, failure("config_not_regular")
		}
		metadata, name, ok := strings.Cut(rows[0], "\t")
		fields := strings.Fields(metadata)
		if !ok || name != prefix || len(fields) != 4 || fields[0] != "040000" || fields[1] != "tree" || !fullOID.MatchString(fields[2]) {
			return Snapshot{}, failure("config_not_regular")
		}
	}
	tree, _, e := runner.run(ctx, 32<<10, "ls-tree", "-l", "-z", "--full-tree", selected, "--", options.File)
	if e != nil {
		return Snapshot{}, e
	}
	rows := strings.Split(string(tree), "\x00")
	if len(rows) == 1 && rows[0] == "" {
		return missingPipeline(selected, options)
	}
	if len(rows) != 2 || rows[1] != "" {
		return Snapshot{}, failure("config_not_regular")
	}
	metadata, name, ok := strings.Cut(rows[0], "\t")
	fields := strings.Fields(metadata)
	if !ok || name != options.File || len(fields) != 4 || fields[1] != "blob" || (fields[0] != "100644" && fields[0] != "100755") || !fullOID.MatchString(fields[2]) {
		return Snapshot{}, failure("config_not_regular")
	}
	size, e := strconv.ParseInt(fields[3], 10, 64)
	if e != nil || size < 0 || size > config.MaxConfigBytes {
		return Snapshot{}, failure("config_limit")
	}
	content, _, e := runner.run(ctx, config.MaxConfigBytes, "cat-file", "blob", fields[2])
	if e != nil {
		return Snapshot{}, e
	}
	if int64(len(content)) != size {
		return Snapshot{}, failure("git_failed")
	}
	digest := sha256.Sum256(content)
	return Snapshot{SHA: selected, Content: content, Digest: hex.EncodeToString(digest[:]), File: options.File}, nil
}
func contextFailure(ctx context.Context) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return failure("timeout")
	}
	return failure("cancelled")
}

type gitRunner struct {
	dir, path     string
	env, prefix   []string
	cleanupFailed bool
	combinedLimit bool
}

func (runner *gitRunner) run(ctx context.Context, limit int, args ...string) ([]byte, int, error) {
	return runner.runInput(ctx, limit, nil, args...)
}
func (runner *gitRunner) runInput(ctx context.Context, limit int, input []byte, args ...string) ([]byte, int, error) {
	stdout := &boundedBuffer{limit: limit}
	stderr := &boundedBuffer{limit: 32 << 10}
	if runner.combinedLimit {
		stderr = stdout
	}
	result := process.Run(ctx, process.Command{Path: runner.path, Args: append(append([]string{}, runner.prefix...), args...), Dir: runner.dir, Env: runner.env, Stdin: input}, stdout, stderr)
	if result.CleanupFailed {
		runner.cleanupFailed = true
		return nil, result.ExitCode, failure("cleanup_failed")
	}
	if ctx.Err() != nil {
		return nil, result.ExitCode, contextFailure(ctx)
	}
	if stdout.overflow || stderr.overflow {
		return nil, result.ExitCode, failure("output_limit")
	}
	if !result.Started || result.Reason != "" || result.ExitCode != 0 {
		return nil, result.ExitCode, failure("git_failed")
	}
	return stdout.data, result.ExitCode, nil
}

type boundedBuffer struct {
	mu       sync.Mutex
	data     []byte
	limit    int
	overflow bool
}

func (buffer *boundedBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	n := min(len(data), buffer.limit-len(buffer.data))
	buffer.data = append(buffer.data, data[:n]...)
	if n != len(data) {
		buffer.overflow = true
		return n, failure("output_limit")
	}
	return n, nil
}

func prepareGitRunner(workspace, protocol string, sshEnv map[string]string) (gitRunner, error) {
	runner := gitRunner{dir: workspace}
	for _, name := range []string{"empty", "home"} {
		if err := os.Mkdir(filepath.Join(workspace, name), 0700); err != nil {
			return runner, failure("cache_failed")
		}
	}
	var e error
	executable, e := exec.LookPath("git")
	if e != nil {
		return gitRunner{}, failure("git_unavailable")
	}
	runner.path, e = filepath.Abs(executable)
	if e != nil {
		return gitRunner{}, failure("git_unavailable")
	}
	env := process.HostEnvironment()
	env["HOME"] = filepath.Join(workspace, "home")
	env["LANG"] = "C"
	env["LC_ALL"] = "C"
	env["GIT_CONFIG_GLOBAL"] = "/dev/null"
	env["GIT_CONFIG_SYSTEM"] = "/dev/null"
	env["GIT_CONFIG_NOSYSTEM"] = "1"
	env["GIT_TERMINAL_PROMPT"] = "0"
	env["GIT_NO_REPLACE_OBJECTS"] = "1"
	env["GIT_NO_LAZY_FETCH"] = "1"
	for key, value := range sshEnv {
		env[key] = value
	}
	for key, value := range env {
		if strings.ContainsRune(value, 0) {
			return gitRunner{}, failure("input_invalid")
		}
		runner.env = append(runner.env, key+"="+value)
	}
	runner.prefix = []string{"--no-replace-objects", "--literal-pathspecs", "-c", "core.hooksPath=" + filepath.Join(workspace, "empty"), "-c", "credential.helper=", "-c", "core.askPass=/usr/bin/false", "-c", "protocol.allow=never", "-c", "protocol." + protocol + ".allow=always", "-c", "fetch.recurseSubmodules=false", "-c", "fetch.fsckObjects=true", "-c", "gc.auto=0", "-c", "http.sslVerify=true", "-c", "http.followRedirects=false"}

	return runner, nil
}

func missingPipeline(sha string, options Options) (Snapshot, error) {
	if options.FileMode == "optional" {
		return Snapshot{SHA: sha, File: options.File, Missing: true}, nil
	}
	return Snapshot{}, failure("config_missing")
}
