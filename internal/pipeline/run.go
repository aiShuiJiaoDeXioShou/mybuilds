package pipeline

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"mybuilds/internal/config"
)

var batchRunError = errors.New("本地流水线执行未成功")

var inheritedEnvironment = []string{"PATH", "HOME", "TMPDIR", "LANG", "LC_ALL", "JAVA_HOME", "ANDROID_HOME", "ANDROID_SDK_ROOT", "DEVELOPER_DIR"}

type preparedStep struct {
	step        config.Step
	skipped     bool
	command     shellCommand
	relativeDir string
	patterns    []string
}

type preparedBuild struct {
	name                     string
	skipped                  bool
	steps                    []preparedStep
	success, failure, always []preparedStep
	timeout, postTimeout     time.Duration
}

type runPreparation struct {
	ctx     context.Context
	root    string
	facts   map[string]string
	tried   map[string]bool
	secrets []string
}

// Run 在整批预检查之后才启动流水线脚本，不修改配置或重置工作树。
func Run(ctx context.Context, document *config.Document, options RunOptions) (*RunResult, error) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return nil, errors.New("本平台不支持本地执行")
	}
	if ctx.Err() != nil {
		return nil, batchRunError
	}
	document = validationCopy(document)
	if err := config.Validate(document); err != nil {
		return nil, err
	}
	names, err := document.Select(options.Names, options.All)
	if err != nil {
		return nil, err
	}
	if options.Step != "" && len(names) != 1 {
		return nil, errors.New("step: 只允许选择一个 build")
	}
	root := options.Workspace
	if root == "" {
		root, err = os.Getwd()
		if err != nil {
			return nil, errors.New("工作区不可用")
		}
	}
	root, err = filepath.Abs(root)
	if err == nil {
		root, err = filepath.EvalSymlinks(root)
	}
	if err != nil {
		return nil, errors.New("工作区不可用")
	}
	if _, err = runDirectory(root, "."); err != nil {
		return nil, err
	}
	p := runPreparation{ctx: ctx, root: root, facts: map[string]string{}, tried: map[string]bool{}}
	for _, key := range []string{"git.sha", "git.branch"} {
		if value := options.Facts[key]; value != "" {
			if strings.ContainsRune(value, 0) {
				return nil, errors.New("本地事实格式错误")
			}
			p.facts[key] = value
		}
	}
	parameters := make([]map[string]string, len(names))
	for i, name := range names {
		parameters[i], err = config.ResolveParams(document.Builds[name], options.Params)
		if err != nil {
			return nil, err
		}
	}
	// 所有模板（包括未显示步骤、post 和公共通知）先按共享规则检查。
	for i, name := range names {
		if _, err = previewBuild(document, name, parameters[i], p.facts); err != nil {
			return nil, err
		}
	}
	prepared := make([]preparedBuild, 0, len(names))
	for i, name := range names {
		effectiveNotifications := document.Notifications
		if document.Builds[name].Notifications != nil {
			effectiveNotifications = document.Builds[name].Notifications
		}
		build, err := p.build(name, document.Builds[name], parameters[i], options.Step, effectiveNotifications)
		if err != nil {
			return nil, err
		}
		prepared = append(prepared, build)
	}
	// 后面的步骤可能需要 Git；已取得的真实事实统一注入本批所有命令。
	for i := range prepared {
		for _, steps := range [][]preparedStep{prepared[i].steps, prepared[i].success, prepared[i].failure, prepared[i].always} {
			for j := range steps {
				for _, item := range []struct{ key, env string }{{"git.sha", "MYBUILDS_GIT_SHA"}, {"git.branch", "MYBUILDS_GIT_BRANCH"}} {
					if value, ok := p.facts[item.key]; ok {
						steps[j].command.Env = append(steps[j].command.Env, item.env+"="+value)
					}
				}
			}
		}
	}
	logger := newRunLogger(options.Output, p.secrets)
	result := &RunResult{Builds: make([]BuildRun, 0, len(prepared))}
	for _, build := range prepared {
		for _, step := range build.steps {
			if step.skipped {
				continue
			}
			result.ResultDir, err = resultDirectory(p.root)
			if err != nil {
				return nil, err
			}
			logger.root, err = os.OpenRoot(result.ResultDir)
			if err != nil {
				_ = os.RemoveAll(result.ResultDir)
				return nil, errors.New("结果目录不可用")
			}
			break
		}
		if logger.root != nil {
			break
		}
	}
	failed, cleanupFailed := false, false
	for _, build := range prepared {
		if cleanupFailed {
			result.Builds = append(result.Builds, unstartedBuild(build, "skipped", "cleanup_error"))
			continue
		}
		if ctx.Err() != nil {
			result.Builds = append(result.Builds, unstartedBuild(build, "cancelled", "cancelled"))
			failed = true
			continue
		}
		b, unsafe := executeBuild(ctx, p.root, build, logger)
		cleanupFailed = unsafe
		failed = failed || b.Status == "failed" || b.Status == "cancelled"
		result.Builds = append(result.Builds, b)
	}
	logError := logger.close()
	if logger.root != nil {
		if err := logger.root.Close(); err != nil {
			logError = err
		}
	}
	if logError != nil {
		for i := range result.Builds {
			if result.Builds[i].Status == "succeeded" {
				result.Builds[i].Status, result.Builds[i].Reason = "failed", "log_error"
			}
		}
		failed = true
	}
	if failed {
		return result, batchRunError
	}
	return result, nil
}

// resultDirectory 先检查真实临时目录边界，避免 TMPDIR 把运行数据放入工作区。
func resultDirectory(workspace string) (string, error) {
	for _, temporary := range []string{os.TempDir(), "/tmp"} {
		base, err := filepath.EvalSymlinks(temporary)
		if err != nil {
			continue
		}
		base, err = filepath.Abs(base)
		if err != nil {
			continue
		}
		relative, err := filepath.Rel(workspace, base)
		if err != nil || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
			continue
		}
		if directory, err := os.MkdirTemp(base, "mybuilds-"); err == nil {
			return directory, nil
		}
	}
	return "", errors.New("结果目录不可用")
}

func (p *runPreparation) build(name string, build *config.Build, params map[string]string, selected string, notifications *config.Notifications) (preparedBuild, error) {
	b := preparedBuild{name: name, steps: []preparedStep{}, postTimeout: 2 * time.Minute}
	b.timeout, _ = time.ParseDuration(build.Timeout)
	if build.Post != nil && build.Post.Timeout != "" {
		b.postTimeout, _ = time.ParseDuration(build.Post.Timeout)
	}
	state, err := p.condition(ready(), build.When, params)
	if err != nil {
		return b, err
	}
	b.skipped = state.Condition == "skipped"
	if !b.skipped {
		if notifications != nil && (notifications.Enabled == nil || *notifications.Enabled) {
			return b, errors.New("notifications: 本地通知尚未支持")
		}
		if build.Runner != nil && build.Runner.Platform == "ios" && runtime.GOOS != "darwin" {
			return b, errors.New("runner: iOS 需要 macOS")
		}
		if build.Reports != nil {
			return b, errors.New("reports: 本地报告尚未支持")
		}
	}
	anyActive, found := false, selected == ""
	for _, step := range build.Steps {
		prepared, err := p.step(name, step, build.Env, params, state)
		if err != nil {
			return b, err
		}
		anyActive = anyActive || !prepared.skipped
		if selected == "" || step.Name == selected {
			b.steps = append(b.steps, prepared)
			found = true
		}
	}
	if !found {
		return b, errors.New("step: 未找到指定步骤")
	}
	if build.Post != nil && anyActive {
		for _, phase := range []struct {
			steps  []config.Step
			output *[]preparedStep
		}{{build.Post.Success, &b.success}, {build.Post.Failure, &b.failure}, {build.Post.Always, &b.always}} {
			for _, step := range phase.steps {
				prepared, err := p.step(name, step, build.Env, params, state)
				if err != nil {
					return b, err
				}
				*phase.output = append(*phase.output, prepared)
			}
		}
	}
	return b, nil
}

func (p *runPreparation) condition(parent ConditionPreview, when *config.When, params map[string]string) (ConditionPreview, error) {
	state := combine(parent, evaluateWhen(when, params, p.facts))
	if state.Condition == "pending" {
		p.fact("git.branch")
		state = combine(parent, evaluateWhen(when, params, p.facts))
	}
	if state.Condition == "pending" {
		return state, errors.New("when: 无法可靠确定本地分支")
	}
	return state, nil
}

func (p *runPreparation) fact(key string) {
	if _, ok := p.facts[key]; ok || p.tried[key] {
		return
	}
	p.tried[key] = true
	executable, err := exec.LookPath("git")
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(p.ctx, 2*time.Second)
	defer cancel()
	args := []string{"-C", p.root, "rev-parse", "--verify", "HEAD"}
	if key == "git.branch" {
		args = []string{"-C", p.root, "symbolic-ref", "--quiet", "--short", "HEAD"}
	}
	command := exec.CommandContext(ctx, executable, args...)
	env := hostEnvironment()
	env["GIT_CONFIG_NOSYSTEM"] = "1"
	env["GIT_CONFIG_GLOBAL"] = os.DevNull
	env["GIT_OPTIONAL_LOCKS"] = "0"
	command.Env = environmentList(env)
	value, err := command.Output()
	if err == nil && strings.TrimSpace(string(value)) != "" {
		p.facts[key] = strings.TrimSpace(string(value))
	}
}

func (p *runPreparation) render(value, field string, params, local map[string]string) (string, error) {
	rendered, missing, err := renderField(value, field, params, local, false, false)
	if err != nil {
		return "", err
	}
	if missing {
		for rest := value; ; {
			start := strings.Index(rest, "{{")
			if start < 0 {
				break
			}
			rest = rest[start+2:]
			end := strings.Index(rest, "}}")
			if end < 0 {
				break
			}
			key := strings.TrimSpace(rest[:end])
			if key == "git.sha" || key == "git.branch" {
				p.fact(key)
				if value, ok := p.facts[key]; ok {
					local[key] = value
				}
			}
			rest = rest[end+2:]
		}
		rendered, missing, err = renderField(value, field, params, local, false, false)
	}
	if err != nil {
		return "", err
	}
	if missing {
		return "", fmt.Errorf("%s: 本地上下文不可用", field)
	}
	return rendered, nil
}

// envValue 只分解原字段里的显式引用；参数和密钥插入的文本均作为值保留。
func (p *runPreparation) envValue(value string, params, local map[string]string) (string, error) {
	var out strings.Builder
	for rest := value; ; {
		start := strings.Index(rest, "${")
		literal := rest
		if start >= 0 {
			literal = rest[:start]
		}
		rendered, err := p.render(literal, "env", params, local)
		if err != nil {
			return "", err
		}
		out.WriteString(rendered)
		if start < 0 {
			break
		}
		rest = rest[start+2:]
		end := strings.IndexByte(rest, '}')
		if end < 0 || !referenceName.MatchString(rest[:end]) {
			return "", errors.New("env: 环境引用格式错误")
		}
		secret, ok := os.LookupEnv(rest[:end])
		if !ok {
			return "", errors.New("env: 声明的环境引用不可用")
		}
		out.WriteString(secret)
		if secret != "" {
			p.secrets = append(p.secrets, secret)
		}
		rest = rest[end+1:]
	}
	if strings.ContainsRune(out.String(), 0) {
		return "", errors.New("env: 值不允许 NUL")
	}
	return out.String(), nil
}

func (p *runPreparation) step(buildName string, step config.Step, buildEnv map[string]string, params map[string]string, parent ConditionPreview) (preparedStep, error) {
	prepared := preparedStep{step: step}
	state, err := p.condition(parent, step.When, params)
	if err != nil {
		return prepared, err
	}
	prepared.skipped = state.Condition == "skipped"
	if prepared.skipped {
		return prepared, nil
	}
	if step.Kind != "run" && step.Kind != "artifact" {
		return prepared, fmt.Errorf("%s: 本地能力尚未支持", step.Kind)
	}
	local := maps.Clone(p.facts)
	local["build.name"], local["workspace"], local["step.name"] = buildName, p.root, step.Name
	if step.Kind == "artifact" {
		for _, pattern := range step.Paths {
			rendered, err := p.render(pattern, "artifact.paths", params, local)
			if err != nil {
				return prepared, err
			}
			prepared.patterns = append(prepared.patterns, rendered)
		}
		return prepared, validateArtifactPatterns(prepared.patterns)
	}
	if strings.ContainsRune(step.Run, 0) {
		return prepared, errors.New("run: 正文不允许 NUL")
	}
	shell := step.Shell
	if shell == "" {
		shell = "sh"
	}
	executable, err := exec.LookPath(shell)
	if err != nil {
		return prepared, errors.New("shell: 解释器不可用")
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return prepared, errors.New("shell: 解释器不可用")
	}
	relative, err := p.render(step.WorkingDir, "step.working_dir", params, local)
	if err != nil {
		return prepared, err
	}
	if relative == "" {
		relative = "."
	}
	dir, err := runDirectory(p.root, relative)
	if err != nil {
		return prepared, err
	}
	declared := maps.Clone(buildEnv)
	if declared == nil {
		declared = map[string]string{}
	}
	maps.Copy(declared, step.Env)
	env := hostEnvironment()
	for _, key := range sortedKeys(declared) {
		value, err := p.envValue(declared[key], params, local)
		if err != nil {
			return prepared, err
		}
		env[key] = value
	}
	env["MYBUILDS_BUILD_NAME"], env["MYBUILDS_WORKSPACE"], env["MYBUILDS_STEP_NAME"] = buildName, p.root, step.Name
	args := []string{"-e", "-c", step.Run}
	if shell == "bash" {
		args = []string{"-e", "-o", "pipefail", "-c", step.Run}
	}
	prepared.command = shellCommand{Path: executable, Args: args, Dir: dir, Env: environmentList(env)}
	prepared.relativeDir = relative
	return prepared, nil
}

func hostEnvironment() map[string]string {
	env := map[string]string{}
	for _, key := range inheritedEnvironment {
		if value, ok := os.LookupEnv(key); ok {
			env[key] = value
		}
	}
	return env
}

func environmentList(env map[string]string) []string {
	values := make([]string, 0, len(env))
	for _, key := range sortedKeys(env) {
		values = append(values, key+"="+env[key])
	}
	return values
}

func runDirectory(root, relative string) (string, error) {
	real, err := filepath.EvalSymlinks(filepath.Join(root, relative))
	if err != nil {
		return "", errors.New("working_dir: 目录不可用")
	}
	rel, err := filepath.Rel(root, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("working_dir: 真实目录越界")
	}
	info, err := os.Stat(real)
	if err != nil || !info.IsDir() {
		return "", errors.New("working_dir: 目录不可用")
	}
	return real, nil
}

func unstartedBuild(build preparedBuild, status, reason string) BuildRun {
	b := BuildRun{Name: build.name, Status: status, Reason: reason, Steps: []StepRun{}}
	for _, step := range build.steps {
		stepReason := "not_started"
		if reason == "condition" {
			stepReason = "condition"
		}
		b.Steps = append(b.Steps, skippedStep(step, stepReason))
	}
	return b
}

func skippedStep(step preparedStep, reason string) StepRun {
	return StepRun{Name: step.step.Name, Kind: step.step.Kind, Status: "skipped", Reason: reason, ExitCode: -1}
}

func executeStep(ctx context.Context, root, build string, step preparedStep, limit time.Duration, logger *runLogger) (result StepRun) {
	result = StepRun{Name: step.step.Name, Kind: step.step.Kind, ExitCode: -1}
	defer func() { result.LogPath = logger.logPath(build, step.step.Name) }()
	if limit < 0 {
		result.Status, result.Reason = "failed", "timeout"
		return result
	}
	if ctx.Err() != nil {
		result.Status, result.Reason = "cancelled", "cancelled"
		return result
	}
	runContext := ctx
	cancel := func() {}
	if limit > 0 {
		runContext, cancel = context.WithTimeout(ctx, limit)
	}
	defer cancel()
	if step.step.Kind == "artifact" {
		start := time.Now()
		result.Status, result.Reason = "failed", "artifact_error"
		workspace, err := os.OpenRoot(root)
		if err == nil && logger.root != nil {
			prefix := filepath.Join("artifacts", build, step.step.Name)
			err = logger.root.MkdirAll(filepath.Dir(prefix), 0700)
			if err == nil {
				result.started = true
				result.Artifacts, err = collectArtifacts(runContext, workspace, filepath.Join(logger.root.Name(), prefix), step.patterns)
			}
			if err == nil {
				for i := range result.Artifacts {
					result.Artifacts[i].SnapshotPath = filepath.ToSlash(filepath.Join(prefix, result.Artifacts[i].SnapshotPath))
				}
				result.Status, result.Reason, result.ExitCode = "succeeded", "", 0
			}
		}
		if workspace != nil {
			if err := workspace.Close(); err != nil && result.Status == "succeeded" {
				result.Status, result.Reason, result.ExitCode = "failed", "artifact_error", -1
			}
		}
		if runContext.Err() != nil {
			result.Status, result.Reason, result.ExitCode = "failed", "timeout", -1
			if ctx.Err() != nil {
				result.Status, result.Reason = "cancelled", "cancelled"
			}
		}
		stream := logger.stream(build, step.step.Name, "system")
		if result.Status == "succeeded" {
			_, err = fmt.Fprintf(stream, "产物收集成功: %d 个文件\n", len(result.Artifacts))
		} else {
			_, err = fmt.Fprintf(stream, "产物收集失败: %s\n", result.Reason)
		}
		closeError := stream.Close()
		if (err != nil || closeError != nil) && result.Status == "succeeded" {
			result.Status, result.Reason, result.ExitCode = "failed", "log_error", -1
		}
		result.DurationMS = time.Since(start).Milliseconds()
		return result
	}
	dir, err := runDirectory(root, step.relativeDir)
	if err != nil {
		result.Status, result.Reason = "failed", "directory_error"
		return result
	}
	command := step.command
	command.Dir = dir
	stdout, stderr := logger.stream(build, step.step.Name, "stdout"), logger.stream(build, step.step.Name, "stderr")
	shell := runShell(runContext, command, stdout, stderr)
	closeOut, closeErr := stdout.Close(), stderr.Close()
	if (closeOut != nil || closeErr != nil) && shell.Reason == "" && shell.ExitCode == 0 && !shell.CleanupFailed {
		shell.Reason = "log_error"
		shell.ExitCode = -1
	}
	result.ExitCode, result.DurationMS, result.CleanupFailed = shell.ExitCode, shell.Duration.Milliseconds(), shell.CleanupFailed
	result.started = shell.Started
	result.Reason = shell.Reason
	result.Status = "succeeded"
	if shell.Reason != "" || shell.ExitCode != 0 || shell.CleanupFailed {
		result.Status = "failed"
	}
	if result.Reason == "" && shell.ExitCode != 0 {
		result.Reason = "exit"
	}
	if result.Reason == "" && shell.CleanupFailed {
		result.Reason = "cleanup_error"
	}
	if ctx.Err() != nil || shell.Reason == "cancelled" {
		result.Status, result.Reason = "cancelled", "cancelled"
	}
	if result.started {
		stream := logger.stream(build, step.step.Name, "system")
		_, writeError := fmt.Fprintf(stream, "步骤执行结果: %s %s\n", result.Status, result.Reason)
		closeError := stream.Close()
		if (writeError != nil || closeError != nil) && result.Status == "succeeded" {
			result.Status, result.Reason, result.ExitCode = "failed", "log_error", -1
		}
	}
	return result
}

func durationLimit(step config.Step, remaining time.Duration, budget bool) time.Duration {
	own, _ := time.ParseDuration(step.Timeout)
	if budget && remaining <= 0 {
		return -1
	}
	if budget && (own == 0 || remaining < own) {
		return remaining
	}
	return own
}

func executeBuild(ctx context.Context, root string, build preparedBuild, logger *runLogger) (BuildRun, bool) {
	if build.skipped {
		return unstartedBuild(build, "skipped", "condition"), false
	}
	b := BuildRun{Name: build.name, Status: "succeeded", Steps: []StepRun{}}
	start := time.Now()
	elapsed := time.Duration(0)
	started, unsafe := false, false
	for _, step := range build.steps {
		if step.skipped {
			b.Steps = append(b.Steps, skippedStep(step, "condition"))
			continue
		}
		if b.Status != "succeeded" {
			b.Steps = append(b.Steps, skippedStep(step, "not_started"))
			continue
		}
		if ctx.Err() != nil {
			b.Status, b.Reason = "cancelled", "cancelled"
			b.Steps = append(b.Steps, skippedStep(step, "not_started"))
			continue
		}
		if build.timeout > 0 && elapsed >= build.timeout {
			b.Status, b.Reason = "failed", "timeout"
			b.Steps = append(b.Steps, skippedStep(step, "budget_exhausted"))
			continue
		}
		stepStart := time.Now()
		s := executeStep(ctx, root, build.name, step, durationLimit(step.step, build.timeout-elapsed, build.timeout > 0), logger)
		elapsed += time.Since(stepStart)
		b.Steps = append(b.Steps, s)
		started = started || s.started
		if s.Status != "succeeded" {
			b.Status, b.Reason = s.Status, s.Reason
		}
		unsafe = unsafe || s.CleanupFailed
	}
	if !started && b.Status == "succeeded" {
		b.Status, b.Reason = "skipped", "condition"
	}
	if started && !unsafe {
		unsafe = executePost(ctx, root, build, &b, logger)
	}
	b.DurationMS = time.Since(start).Milliseconds()
	return b, unsafe
}

func executePost(ctx context.Context, root string, build preparedBuild, b *BuildRun, logger *runLogger) bool {
	originalSuccess := b.Status == "succeeded"
	if ctx.Err() != nil && originalSuccess {
		b.Status, b.Reason = "cancelled", "cancelled"
	}
	phase := build.success
	if b.Status == "failed" {
		phase = build.failure
	}
	if b.Status == "cancelled" || ctx.Err() != nil {
		phase = nil
	}
	deadline := time.Now().Add(build.postTimeout)
	unsafe := false
	for _, group := range []struct {
		steps  []preparedStep
		always bool
	}{{phase, false}, {build.always, true}} {
		for _, step := range group.steps {
			reason := ""
			if unsafe {
				reason = "cleanup_error"
			} else if time.Until(deadline) <= 0 {
				reason = "budget_exhausted"
			} else if !group.always && ctx.Err() != nil {
				reason = "cancelled"
			} else if step.skipped {
				reason = "condition"
			}
			if reason != "" {
				b.Post = append(b.Post, skippedStep(step, reason))
				if reason == "budget_exhausted" && b.Status == "succeeded" {
					b.Status, b.Reason = "failed", "post_error"
				}
				continue
			}
			base := ctx
			if group.always && ctx.Err() != nil {
				base = context.WithoutCancel(ctx)
			}
			s := executeStep(base, root, build.name, step, durationLimit(step.step, time.Until(deadline), true), logger)
			b.Post = append(b.Post, s)
			unsafe = unsafe || s.CleanupFailed
			if ctx.Err() != nil && originalSuccess {
				b.Status, b.Reason = "cancelled", "cancelled"
			}
			if s.Status != "succeeded" && b.Status == "succeeded" {
				b.Status, b.Reason = "failed", "post_error"
			}
		}
	}
	if ctx.Err() != nil && originalSuccess {
		b.Status, b.Reason = "cancelled", "cancelled"
	}
	return unsafe
}
