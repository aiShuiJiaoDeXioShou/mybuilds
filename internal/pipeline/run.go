package pipeline

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"mybuilds/internal/config"
	"mybuilds/internal/mobile"
	"mybuilds/internal/process"
	"mybuilds/internal/protocol"
)

var batchRunError = errors.New("本地流水线执行未成功")

type preparedStep struct {
	step        config.Step
	skipped     bool
	command     shellCommand
	relativeDir string
	patterns    []string
	phase       string
	index       int
	ios         *iosBuildResources
}

type preparedBuild struct {
	resume                   *ApprovalResume
	confirm                  func(context.Context, ApprovalPrompt) (bool, error)
	hasUpload                bool
	name                     string
	iosDeclared              bool
	skipped                  bool
	steps                    []preparedStep
	success, failure, always []preparedStep
	timeout, postTimeout     time.Duration
	reportPatterns           []string
	reportRequired           bool
	iosSigning               *mobile.IOSSigningOptions
}

type runPreparation struct {
	changes *protocol.ChangeFacts
	resume  *ApprovalResume
	ctx     context.Context
	root    string
	facts   map[string]string
	tried   map[string]bool
	secrets []string
	remote  *remoteRun
}

// Run 在整批预检查之后才启动流水线脚本，不修改配置或重置工作树。
func Run(ctx context.Context, document *config.Document, options RunOptions) (*RunResult, error) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return nil, errors.New("本平台不支持本地执行")
	}
	if ctx.Err() != nil && options.Remote == nil {
		return nil, batchRunError
	}
	var remote *remoteRun
	if options.Remote != nil {
		if options.All || options.Step != "" {
			return nil, errors.New("远程执行只允许单个完整构建")
		}
		var err error
		remote, err = newRemote(options.Remote)
		if err != nil {
			return nil, err
		}
		defer remote.cancel()
		if remote.blocked() != "" {
			return nil, errors.New("远程运行权已失效")
		}
	}
	targetSHA := options.Facts["git.sha"]
	if options.Remote != nil {
		targetSHA = options.Remote.Facts["git.sha"]
	}
	if !protocol.ValidateChanges(options.Changes, targetSHA) {
		return nil, errors.New("changes: 冻结事实无效")
	}
	document = validationCopy(document)
	if err := config.Validate(document); err != nil {
		return nil, err
	}
	names, err := document.Select(options.Names, options.All)
	if err != nil {
		return nil, err
	}
	if remote != nil && len(names) != 1 {
		return nil, errors.New("远程执行只允许单个完整构建")
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
	p := runPreparation{changes: options.Changes, ctx: ctx, root: root, facts: map[string]string{}, tried: map[string]bool{}, remote: remote, resume: options.Resume}
	if remote != nil {
		for _, key := range []string{"project", "build.id", "build.number", "node.name", "git.sha", "git.branch"} {
			if value, ok := remote.options.Facts[key]; ok {
				if strings.ContainsRune(value, 0) {
					return nil, errors.New("远程事实格式错误")
				}
				p.facts[key] = value
			}
		}
		p.facts["workspace"] = root
	}
	for _, key := range []string{"git.sha", "git.branch"} {
		if value := options.Facts[key]; remote == nil && value != "" {
			if strings.ContainsRune(value, 0) {
				return nil, errors.New("本地事实格式错误")
			}
			p.facts[key] = value
		}
	}
	parameters, err := resolveBuildParameters(document, names, options.PreviewOptions)
	if err != nil {
		return nil, err
	}
	if remote != nil {
		for i, name := range names {
			build := document.Builds[name]
			if build.Runner != nil && build.Runner.Framework == "flutter" {
				if err := mobile.ValidateFlutterParameters(build.Runner.Platform, parameters[i], p.facts["build.number"]); err != nil {
					return nil, err
				}
			}
		}
	}
	// 所有模板（包括未显示步骤、post 和公共通知）先按共享规则检查。
	for i, name := range names {
		if _, err = previewBuild(document, name, parameters[i], p.facts, p.changes); err != nil {
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
		if err := p.flutterTools(document.Builds[name], &build, parameters[i]); err != nil {
			return nil, err
		}
		build.resume = options.Resume
		build.confirm = options.ConfirmApproval
		for _, s := range build.steps {
			build.hasUpload = build.hasUpload || s.step.Kind == "upload"
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
	logger.remote = remote
	if err = resumeResultRoot(options, logger); err != nil {
		return nil, err
	}
	result := &RunResult{Builds: make([]BuildRun, 0, len(prepared))}
	for _, build := range prepared {
		if remote != nil {
			break
		}
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
		if ctx.Err() != nil && remote == nil {
			result.Builds = append(result.Builds, unstartedBuild(build, "cancelled", "cancelled"))
			failed = true
			continue
		}
		b, unsafe := executeBuild(ctx, p.root, build, logger)
		cleanupFailed = unsafe
		failed = failed || b.Status == "failed" || b.Status == "cancelled"
		result.Builds = append(result.Builds, b)
		if b.paused != nil {
			result.Paused = b.paused
			break
		}
	}
	if remote != nil {
		result.ResultDir = remote.resultDir
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
	if remote != nil {
		// 已ACK暂停已结束本次执行权；随后服务退出不覆盖这一真实checkpoint。
		if result.Paused == nil && remote.blocked() != "" {
			failed = true
			if len(result.Builds) > 0 && result.Builds[0].Status == "succeeded" {
				result.Builds[0].Status, result.Builds[0].Reason = "failed", remote.blocked()
			}
		}
		if result.Paused == nil && remote.blocked() == "" && len(result.Builds) > 0 {
			b := result.Builds[0]
			p := protocol.ExecutionProgress{IOSResourceDigest: b.iosResourceDigest, IOSCleanupConfirmed: b.IOSCleanupConfirmed, Kind: "build_finished", Status: b.Status, Reason: b.Reason, StopConfirmed: !cleanupFailed, CleanupFailed: cleanupFailed, ExitCode: 0}
			if b.Reports != nil && b.Reports.Sealed {
				ids := make([]string, 0, len(b.Reports.Files))
				for _, file := range b.Reports.Files {
					ids = append(ids, file.ArtifactID)
				}
				p.ReportManifest = &protocol.ReportManifest{SealDigest: b.ReportSealDigest, IDs: ids}
			}
			for _, step := range append(append([]StepRun{}, b.Steps...), b.Post...) {
				p.Started = p.Started || step.started
			}
			if remote.emit(p) != nil {
				failed = true
				if result.Builds[0].Status == "succeeded" {
					result.Builds[0].Status, result.Builds[0].Reason = "failed", "persistence_error"
				}
			}
		}
	}
	if cleanupFailed {
		failed = true
	}
	if result.Paused != nil && !failed {
		return result, ErrApprovalPaused
	}
	if failed {
		return result, batchRunError
	}
	return result, nil
}

// resultDirectory 先检查真实临时目录边界，避免 TMPDIR 把运行数据放入工作区。
func resultDirectory(workspace string) (string, error) {
	directory, err := process.TemporaryDirectory(workspace, "mybuilds-")
	if err != nil {
		return "", errors.New("结果目录不可用")
	}
	return directory, nil
}

func (p *runPreparation) build(name string, build *config.Build, params map[string]string, selected string, notifications *config.Notifications) (preparedBuild, error) {
	b := preparedBuild{iosDeclared: build.IOSSigning != nil, name: name, steps: []preparedStep{}, postTimeout: 2 * time.Minute}
	if p.remote == nil {
		delete(p.facts, "build.number")
		if build.Runner != nil && build.Runner.Framework == "flutter" && params["build_number"] != "" {
			p.facts["build.number"] = params["build_number"]
		}
	}
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
	}
	// 系统目录仅由本次有效普通run生成，不采用调用者提供的事实。
	if !b.skipped && build.IOSSigning != nil {
		activeRun := false
		for index, step := range build.Steps {
			if p.resume != nil && index+1 < p.resume.Evidence.NextOrdinaryIndex {
				continue
			}
			state, e := p.condition(state, step.When, params)
			if e != nil {
				return b, e
			}
			activeRun = activeRun || step.Kind == "run" && state.Condition != "skipped" && (selected == "" || step.Name == selected)
		}
		if activeRun {
			if p.remote != nil && p.remote.options.IOSCheckpoint == nil {
				return b, errors.New("ios_signing: 资源持久化不可用")
			}
			p.facts["ios.output_dir"] = ".mybuilds-ios-" + rand.Text()
			if p.remote != nil {
				// 原任务身份派生系统路径，中央可按同一冻结build核对发布文件。
				id := p.remote.options.Facts["build.id"]
				if matched, _ := regexp.MatchString(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, id); !matched {
					return b, errors.New("远程iOS构建身份无效")
				}
				p.facts["ios.output_dir"] = ".mybuilds-ios-" + id
			}
			defer delete(p.facts, "ios.output_dir")
			local := maps.Clone(p.facts)
			local["build.name"] = name
			signing, missing, e := renderIOSSigning(build.IOSSigning, "build.ios_signing", params, local)
			if e != nil || missing {
				return b, errors.New("ios_signing: 签名参数不可用")
			}
			options := mobile.IOSSigningOptions{Workspace: p.root, OutputDir: p.facts["ios.output_dir"], BundleID: signing.BundleID, ExportMethod: signing.ExportMethod}
			for _, field := range []struct {
				value string
				out   *string
			}{{signing.P12, &options.P12File}, {signing.Profile, &options.ProfileFile}, {signing.Password, &options.Password}} {
				*field.out, e = p.envValue(field.value, params, local)
				if e != nil {
					return b, e
				}
			}
			checkCtx := p.ctx
			stop := func() {}
			if p.remote != nil {
				checkCtx, stop = p.remote.merge(checkCtx)
				remaining, _ := p.remote.budgets()
				if remaining != nil {
					if *remaining <= 0 {
						stop()
						return b, errors.New("ios_signing: 普通预算已耗尽")
					}
					bounded, cancel := context.WithTimeout(checkCtx, time.Duration(*remaining))
					previousStop := stop
					stop = func() { cancel(); previousStop() }
					checkCtx = bounded
				}
			}
			defer stop()
			if e = mobile.ValidateIOSSigning(checkCtx, options); e != nil {
				return b, e
			}
			b.iosSigning = &options
		} else if p.resume != nil {
			p.facts["ios.output_dir"] = ".mybuilds-ios-" + p.remote.options.Facts["build.id"]
			defer delete(p.facts, "ios.output_dir")
		} else if iosOutputReferenced(build) {
			return b, errors.New("ios_signing: 系统产物目录需要本次有效普通run")
		}
	}
	anyActive, found := false, selected == ""
	if p.resume != nil {
		for _, prior := range p.resume.Evidence.Steps {
			anyActive = anyActive || prior.Phase == "ordinary" && prior.Index > 0 && prior.Index < p.resume.Evidence.NextOrdinaryIndex && prior.Started
		}
	}
	for index, step := range build.Steps {
		if p.resume != nil && index+1 < p.resume.Evidence.NextOrdinaryIndex {
			b.steps = append(b.steps, preparedStep{step: step, phase: "ordinary", index: index + 1})
			continue
		}
		prepared, err := p.step(name, step, build.Env, params, state)
		if err != nil {
			return b, err
		}
		prepared.phase, prepared.index = "ordinary", index+1
		anyActive = anyActive || !prepared.skipped
		if selected == "" || step.Name == selected {
			b.steps = append(b.steps, prepared)
			found = true
		}
	}
	if !found {
		return b, errors.New("step: 未找到指定步骤")
	}
	if !b.skipped && build.Reports != nil {
		local := maps.Clone(p.facts)
		delete(local, "workspace")
		delete(local, "ios.output_dir")
		local["build.name"] = name
		// 报告是build层相对路径，只使用能在控制端核对的构建事实。
		for _, pattern := range build.Reports.JUnit.Paths {
			rendered, err := p.render(pattern, "build.reports.junit.paths", params, local)
			if err != nil {
				return b, err
			}
			b.reportPatterns = append(b.reportPatterns, rendered)
		}
		b.reportRequired = build.Reports.JUnit.Required == nil || *build.Reports.JUnit.Required
	}
	if build.Post != nil && (anyActive || p.remote != nil) {
		for _, phase := range []struct {
			steps  []config.Step
			output *[]preparedStep
			name   string
		}{{build.Post.Success, &b.success, "success"}, {build.Post.Failure, &b.failure, "failure"}, {build.Post.Always, &b.always, "always"}} {
			for index, step := range phase.steps {
				prepared := preparedStep{step: step, skipped: true}
				var err error
				if anyActive {
					prepared, err = p.step(name, step, build.Env, params, state)
				}
				if err != nil {
					return b, err
				}
				prepared.phase, prepared.index = phase.name, index+1
				*phase.output = append(*phase.output, prepared)
			}
		}
	}
	return b, nil
}

func (p *runPreparation) condition(parent ConditionPreview, when *config.When, params map[string]string) (ConditionPreview, error) {
	state := combine(parent, WhenCondition(when, params, p.facts, p.changes))
	if state.Condition == "pending" {
		p.fact("git.branch")
		state = combine(parent, WhenCondition(when, params, p.facts, p.changes))
	}
	if state.Condition == "pending" {
		return state, errors.New("when: 无法可靠确定本地分支")
	}
	return state, nil
}

func (p *runPreparation) fact(key string) {
	if _, ok := p.facts[key]; ok || p.tried[key] || p.remote != nil {
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
	env := process.HostEnvironment()
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
	rendered, missing, err := config.RenderField(value, field, params, local, false, false)
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
		rendered, missing, err = config.RenderField(value, field, params, local, false, false)
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
		var secret string
		var ok bool
		if p.remote != nil {
			secret, ok = p.remote.options.Secrets[rest[:end]]
		} else {
			secret, ok = os.LookupEnv(rest[:end])
		}
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
	if step.Kind == "upload" && p.remote != nil && p.remote.options.Publish != nil && (step.Target == "google_play" || step.Target == "app_store" || step.Target == "custom") {
		local := maps.Clone(p.facts)
		local["build.name"], local["workspace"], local["step.name"] = buildName, p.root, step.Name
		for _, item := range []struct {
			source string
			out    *string
		}{{step.File, &prepared.step.File}, {step.AppIdentifier, &prepared.step.AppIdentifier}, {step.Track, &prepared.step.Track}, {step.WorkingDir, &prepared.step.WorkingDir}, {step.ResultFile, &prepared.step.ResultFile}} {
			value, e := p.render(item.source, "upload", params, local)
			if e != nil {
				return prepared, e
			}
			*item.out = value
		}
		if step.Target == "custom" {
			declared := maps.Clone(buildEnv)
			if declared == nil {
				declared = map[string]string{}
			}
			maps.Copy(declared, step.Env)
			env := process.HostEnvironment()
			for _, key := range sortedKeys(declared) {
				value, e := p.envValue(declared[key], params, local)
				if e != nil {
					return prepared, e
				}
				env[key] = value
			}
			prepared.command.Env = environmentList(env)
		}
		return prepared, nil
	}
	if step.Kind == "approval" {
		if step.Notify != nil && *step.Notify {
			return prepared, errors.New("unsupported")
		}
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
	env := process.HostEnvironment()
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
	b := BuildRun{IOSCleanupConfirmed: build.iosDeclared, Name: build.name, Status: status, Reason: reason, Steps: []StepRun{}}
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
	if logger.remote != nil {
		r := logger.remote
		if reason := r.blocked(); reason != "" {
			result.Status, result.Reason = "failed", reason
			return result
		}
		actionStart := time.Now()
		if err := r.ensureResult(root, logger); err != nil {
			result.Status, result.Reason = "failed", "directory_error"
			return result
		}
		intent := stepProgress("intent", step)
		if r.emit(intent) != nil {
			result.Status, result.Reason = "failed", "persistence_error"
			return result
		}
		defer func() { r.finishStep(step, result, actionStart) }()
		if reason := r.blocked(); reason != "" {
			result.Status, result.Reason = "failed", reason
			return result
		}
		limit = r.limit(step)
		if limit < 0 {
			result.Status, result.Reason = "failed", "timeout"
			return result
		}
		merged, cancel := r.merge(ctx)
		defer cancel()
		ctx = merged
	}

	runContext := ctx
	cancel := func() {}
	if limit > 0 {
		runContext, cancel = context.WithTimeout(ctx, limit)
	}
	defer cancel()
	if step.ios != nil {
		if err := step.ios.prepare(runContext); err != nil {
			result.Status, result.Reason = "failed", "start_error"
			result.CleanupFailed = errors.Is(err, mobile.ErrIOSCleanup)
			if runContext.Err() != nil {
				result.Reason = "timeout"
				if ctx.Err() != nil {
					result.Status, result.Reason = "cancelled", "cancelled"
				}
			}
			return result
		}
		for key, value := range step.ios.resources.Environment() {
			step.command.Env = append(step.command.Env, key+"="+value)
		}
	}
	if step.step.Kind == "artifact" {
		start := time.Now()
		result.Status, result.Reason = "failed", "artifact_error"
		workspace, err := os.OpenRoot(root)
		if err == nil && logger.root != nil {
			prefix := filepath.Join("artifacts", build, step.step.Name)
			err = logger.root.MkdirAll(filepath.Dir(prefix), 0700)
			if err == nil {
				result.started = true
				calledAt := time.Now().UTC()
				result.Artifacts, err = collectArtifacts(runContext, workspace, filepath.Join(logger.root.Name(), prefix), step.patterns)
				if err == nil {
					for i := range result.Artifacts {
						result.Artifacts[i].SnapshotPath = filepath.ToSlash(filepath.Join(prefix, result.Artifacts[i].SnapshotPath))
					}
				}
				if logger.remote != nil {
					progress := stepProgress("started", step)
					progress.Started = true
					progress.At = calledAt
					if logger.remote.emit(progress) != nil && err == nil {
						err = errors.New("进度保存失败")
						result.Reason = "progress_error"
					}
				}
			}
			if err == nil {
				result.Status, result.Reason, result.ExitCode = "succeeded", "", 0
			}
		}
		if workspace != nil {
			if err := workspace.Close(); err != nil && result.Status == "succeeded" {
				result.Status, result.Reason, result.ExitCode = "failed", "artifact_error", -1
			}
		}
		if runContext.Err() != nil && (logger.remote == nil || logger.remote.blocked() == "") {
			result.Status, result.Reason, result.ExitCode = "failed", "timeout", -1
			if ctx.Err() != nil {
				result.Status, result.Reason = "cancelled", "cancelled"
			}
		}
		stream := logger.stepStream(build, step, "system")
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
	if logger.remote != nil {
		command.OnStart = func(info process.StartInfo) error {
			p := stepProgress("started", step)
			p.Started = true
			p.PID, p.PGID, p.At = info.PID, info.PGID, info.At
			return logger.remote.emit(p)
		}
	}
	stdout, stderr := logger.stepStream(build, step, "stdout"), logger.stepStream(build, step, "stderr")
	shell := process.Run(runContext, command, stdout, stderr)
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
	if (ctx.Err() != nil || shell.Reason == "cancelled") && (logger.remote == nil || logger.remote.blocked() == "") {
		result.Status, result.Reason = "cancelled", "cancelled"
	}
	if result.started {
		stream := logger.stepStream(build, step, "system")
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

func executeBuild(ctx context.Context, root string, build preparedBuild, logger *runLogger) (result BuildRun, cleanupFailed bool) {
	if build.skipped {
		if logger.remote != nil {
			for _, step := range build.steps {
				logger.remote.skipped(step, "condition")
			}
		}
		b := unstartedBuild(build, "skipped", "condition")
		if logger.remote != nil {
			logger.remote.inactivePost(build, &b, "not_selected")
		}
		return b, false
	}
	b := BuildRun{IOSCleanupConfirmed: build.iosDeclared, Name: build.name, Status: "succeeded", Steps: []StepRun{}}
	var ios *iosBuildResources
	if build.iosSigning != nil {
		ios = &iosBuildResources{options: *build.iosSigning}
		if logger.remote != nil {
			ios.checkpoint = logger.remote.options.IOSCheckpoint
		}
		defer func() {
			if ios.resources != nil {
				closeCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
				err := ios.resources.Close(closeCtx)
				stop()
				if e := ios.save(); e != nil {
					err = errors.Join(err, e)
				}
				if err != nil {
					cleanupFailed = true
					result.CleanupFailed = true
					if result.Status == "succeeded" {
						result.Status, result.Reason = "failed", "cleanup_error"
					}
				}
			}
			result.IOSCleanupConfirmed = ios.resources == nil || ios.resources.Ownership().Closed
			if ios.resources != nil && result.IOSCleanupConfirmed {
				result.iosResourceDigest, _ = mobile.IOSResourceDigest(ios.resources.Ownership())
			}
		}()
	}
	start := time.Now()
	elapsed := time.Duration(0)
	started, unsafe := false, false
	var reportSet *reportCollection
	var reportWorkspace *os.Root
	reportReady := true
	if build.resume != nil {
		if err := verifyApprovalArtifacts(ctx, logger.root, build.resume.Local.Steps); err != nil {
			b.Status, b.Reason = "failed", "approval_checkpoint_invalid"
			return b, true
		}
		restored, err := restoreApprovalSteps(build.resume, build)
		if err != nil {
			b.Status, b.Reason = "failed", "approval_checkpoint_invalid"
			return b, true
		}
		b.Steps = restored
		b.iosTeamID = build.resume.Local.IOSteamID
		b.iosResourceDigest = build.resume.Evidence.IOSResourceDigest
		for _, s := range restored {
			started = started || s.started
		}
		b.Reports = build.resume.Evidence.Reports
		if build.resume.Evidence.ReportManifest != nil {
			b.ReportSealDigest = build.resume.Evidence.ReportManifest.SealDigest
		}
		if build.resume.Local.Collection != nil {
			reportWorkspace, err = os.OpenRoot(root)
			if err == nil {
				secrets := []string{}
				for _, value := range logger.secrets {
					secrets = append(secrets, string(value))
				}
				bounded, cancel, e := reportContext(ctx, reportRemaining(build, elapsed, logger.remote), logger.remote, 10*time.Second)
				if e == nil {
					reportSet, err = restoreReportCollection(bounded, reportWorkspace, logger.root, build.reportPatterns, secrets, build.reportRequired, build.resume.Local.Collection)
				} else {
					err = e
				}
				cancel()
			}
			if err != nil {
				b.Status, b.Reason = "failed", "approval_checkpoint_invalid"
				return b, true
			}
		}
	}
	defer func() {
		if reportWorkspace != nil {
			_ = reportWorkspace.Close()
		}
	}()
	for _, step := range build.steps {
		if build.resume != nil && step.index < build.resume.Evidence.NextOrdinaryIndex {
			continue
		}
		if logger.remote != nil && logger.remote.blocked() != "" {
			reason := logger.remote.blocked()
			b.Steps = append(b.Steps, skippedStep(step, reason))
			if b.Status == "succeeded" {
				b.Status, b.Reason = "failed", reason
			}
			continue
		}
		if step.skipped {
			b.Steps = append(b.Steps, skippedStep(step, "condition"))
			if logger.remote != nil {
				logger.remote.skipped(step, "condition")
			}
			continue
		}
		if b.Status != "succeeded" {
			b.Steps = append(b.Steps, skippedStep(step, "not_started"))
			if logger.remote != nil {
				logger.remote.skipped(step, "not_started")
			}
			continue
		}
		if ctx.Err() != nil {
			b.Status, b.Reason = "cancelled", "cancelled"
			b.Steps = append(b.Steps, skippedStep(step, "not_started"))
			if logger.remote != nil {
				logger.remote.skipped(step, "not_started")
			}
			continue
		}
		if (logger.remote == nil && build.timeout > 0 && elapsed >= build.timeout) || (logger.remote != nil && logger.remote.limit(step) < 0) {
			b.Status, b.Reason = "failed", "timeout"
			b.Steps = append(b.Steps, skippedStep(step, "budget_exhausted"))
			if logger.remote != nil {
				logger.remote.skipped(step, "budget_exhausted")
			}
			continue
		}
		stepStart := time.Now()
		if len(build.reportPatterns) != 0 && reportSet == nil {
			remaining := reportRemaining(build, elapsed, logger.remote)
			checkCtx, stop, err := reportContext(ctx, remaining, logger.remote, 10*time.Second)
			if err == nil && logger.remote != nil {
				err = logger.remote.ensureResult(root, logger)
			}
			if err == nil {
				reportWorkspace, err = os.OpenRoot(root)
			}
			if err == nil {
				secrets := make([]string, len(logger.secrets))
				for i, value := range logger.secrets {
					secrets[i] = string(value)
				}
				reportSet, err = newReportCollection(checkCtx, reportWorkspace, logger.root, build.reportPatterns, build.reportRequired, secrets)
			}
			stop()
			elapsed += time.Since(stepStart)
			if err != nil {
				b.Status, b.Reason = "failed", reportFailureReason(err)
				reportReady = false
				b.Steps = append(b.Steps, skippedStep(step, "not_started"))
				if logger.remote != nil {
					logger.remote.skipped(step, "not_started")
				}
				continue
			}
			stepStart = time.Now()
		}
		limit := durationLimit(step.step, build.timeout-elapsed, build.timeout > 0)
		if logger.remote != nil {
			limit = logger.remote.limit(step)
		}
		if ios != nil && step.step.Kind == "run" {
			step.ios = ios
		}
		// 首个发布屏障封存声明测试；之后仅使用原seal，不重采被post改写的工作区。
		if (step.step.Kind == "upload" || step.step.Kind == "approval" && build.hasUpload) && reportSet != nil && b.ReportSealDigest == "" {
			err := checkBuildReports(ctx, build, &b, logger, reportSet, 0, "", true, reportRemaining(build, elapsed, logger.remote))
			if err != nil || b.Reports == nil || b.Reports.Outcome != "passed" {
				b.Status, b.Reason = "failed", "report_failed"
				b.Steps = append(b.Steps, skippedStep(step, "not_started"))
				if logger.remote != nil {
					logger.remote.skipped(step, "not_started")
				}
				continue
			}
		}

		if step.step.Kind == "approval" {
			if logger.remote != nil {
				pause, err := pauseApproval(ctx, root, build, step, &b, logger, reportSet, ios)
				if err != nil {
					b.Status, b.Reason = "failed", "persistence_error"
					b.CleanupFailed = errors.Is(err, mobile.ErrIOSCleanup)
					return b, b.CleanupFailed
				}
				b.Status = "waiting_approval"
				b.paused = pause
				return b, false
			}
			waiting := time.Now()
			ok := false
			var err error
			if build.confirm == nil {
				err = errors.New("approval_requires_tty")
			} else {
				ok, err = build.confirm(ctx, ApprovalPrompt{Build: build.name, Step: step.step.Name, Index: step.index})
			}
			start = start.Add(time.Since(waiting))
			if err != nil || !ok {
				b.Status, b.Reason = "cancelled", "approval_rejected"
				if err != nil {
					b.Reason = "approval_requires_tty"
					if err.Error() == "approval_input_error" {
						b.Reason = "approval_input_error"
					}
				}
				if ctx.Err() != nil {
					b.Reason = "cancelled"
				}
				b.Steps = append(b.Steps, StepRun{Name: step.step.Name, Kind: "approval", Status: "cancelled", Reason: b.Reason, ExitCode: -1})
				return b, false
			}
			b.Steps = append(b.Steps, StepRun{Name: step.step.Name, Kind: "approval", Status: "succeeded", ExitCode: -1})
			continue
		}
		var s StepRun
		if step.step.Kind == "upload" {
			s = executePublishStep(ctx, root, build.name, step, b.Steps, b.Reports, b.ReportSealDigest, b.iosTeamID, limit, logger)
		} else {
			s = executeStep(ctx, root, build.name, step, limit, logger)
		}
		elapsed += time.Since(stepStart)
		b.Steps = append(b.Steps, s)
		if ios != nil && ios.resources != nil && ios.resources.Ownership().Prepared {
			b.iosTeamID = ios.resources.Environment()["MYBUILDS_IOS_TEAM_ID"]
		}
		started = started || s.started
		if s.Status != "succeeded" {
			b.Status, b.Reason = s.Status, s.Reason
		}
		unsafe = unsafe || s.CleanupFailed
		if reportSet != nil && s.Kind == "run" && s.started && !unsafe && logger.failure() == nil && (logger.remote == nil || logger.remote.blocked() == "") {
			checkStart := time.Now()
			err := checkBuildReports(ctx, build, &b, logger, reportSet, step.index, step.step.Name, false, reportRemaining(build, elapsed, logger.remote))
			elapsed += time.Since(checkStart)
			if err != nil {
				reportReady = false
				if b.Status == "succeeded" {
					b.Status, b.Reason = "failed", reportFailureReason(err)
				}
				if logger.remote != nil && logger.remote.blocked() == "" {
					logger.remote.fail("persistence_error")
				}
			}
		}
	}
	if !started && b.Status == "succeeded" {
		b.Status, b.Reason = "skipped", "condition"
	}
	if reportSet != nil && b.ReportSealDigest == "" && started && !unsafe && reportReady && logger.failure() == nil && (logger.remote == nil || logger.remote.blocked() == "") {
		checkStart := time.Now()
		err := checkBuildReports(ctx, build, &b, logger, reportSet, 0, "", true, reportRemaining(build, elapsed, logger.remote))
		elapsed += time.Since(checkStart)
		if err != nil {
			reportReady = false
			if b.Status == "succeeded" {
				b.Status, b.Reason = "failed", reportFailureReason(err)
			}
			if logger.remote != nil && logger.remote.blocked() == "" {
				logger.remote.fail("persistence_error")
			}
		}
	}
	if logger.remote != nil && started && b.Status == "succeeded" {
		remaining, _ := logger.remote.budgets()
		if remaining != nil && *remaining == 0 {
			b.Status, b.Reason = "failed", "timeout"
		}
	}
	if logger.remote == nil && started && b.Status == "succeeded" && build.timeout > 0 && elapsed >= build.timeout {
		b.Status, b.Reason = "failed", "timeout"
	}
	if logger.remote != nil && logger.remote.blocked() != "" && b.Status == "succeeded" {
		b.Status, b.Reason = "failed", logger.remote.blocked()
	}
	if started && !unsafe && reportReady && logger.failure() == nil && (logger.remote == nil || logger.remote.blocked() == "") {
		if ios != nil && ios.resources != nil {
			for _, group := range [][]preparedStep{build.success, build.failure, build.always} {
				for i := range group {
					for key, value := range ios.resources.Environment() {
						group[i].command.Env = append(group[i].command.Env, key+"="+value)
					}
				}
			}
		}
		unsafe = executePost(ctx, root, build, &b, logger)
	} else if logger.remote != nil {
		reason := "not_selected"
		if unsafe {
			reason = "cleanup_error"
		}
		if logger.remote.blocked() != "" {
			reason = logger.remote.blocked()
		}
		logger.remote.inactivePost(build, &b, reason)
	}
	b.DurationMS = time.Since(start).Milliseconds()
	b.CleanupFailed = unsafe
	return b, unsafe
}

func reportRemaining(build preparedBuild, elapsed time.Duration, remote *remoteRun) *int64 {
	if remote != nil {
		remaining, _ := remote.budgets()
		return remaining
	}
	if build.timeout == 0 {
		return nil
	}
	remaining := max(int64(0), int64(build.timeout-elapsed))
	return &remaining
}

// reportContext允许取消后核对已产生的报告，仍受原预算与独立运行权限制。
func reportContext(ctx context.Context, remaining *int64, remote *remoteRun, maximum time.Duration) (context.Context, context.CancelFunc, error) {
	noop := func() {}
	if remaining != nil {
		maximum = min(maximum, time.Duration(*remaining))
	}
	if maximum <= 0 {
		return ctx, noop, context.DeadlineExceeded
	}
	base := ctx
	if ctx.Err() != nil {
		base = context.WithoutCancel(ctx)
	}
	stopAuthority := noop
	if remote != nil {
		if remote.blocked() != "" {
			return base, noop, errReportSave
		}
		base, stopAuthority = remote.merge(base)
	}
	bounded, stop := context.WithTimeout(base, maximum)
	return bounded, func() { stop(); stopAuthority() }, nil
}

func checkBuildReports(ctx context.Context, build preparedBuild, result *BuildRun, logger *runLogger, set *reportCollection, index int, name string, final bool, remaining *int64) error {
	bounded, stop, err := reportContext(ctx, remaining, logger.remote, 10*time.Second)
	if err != nil {
		return err
	}
	defer stop()
	pureStart := time.Now()
	evidence, locals, err := set.check(bounded, index, name, final)
	if err != nil {
		return err
	}
	pureSpent := time.Since(pureStart)
	result.Reports = &evidence
	if evidence.Outcome == "failed" && result.Status == "succeeded" {
		result.Status, result.Reason = "failed", evidence.Reason
	}
	if logger.remote != nil {
		p := protocol.ExecutionProgress{Kind: "reports_checked", Index: index, ExitCode: -1, Reports: &evidence, LocalReports: locals}
		if !final {
			p.Phase, p.Name, p.StepKind = "ordinary", name, "run"
		}
		if err = logger.remote.emit(p); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			return errReportSave
		}
	}
	if !final {
		return nil
	}
	sealCtx := bounded
	if logger.remote != nil {
		current, _ := logger.remote.budgets()
		var sealStop context.CancelFunc
		sealCtx, sealStop, err = reportContext(ctx, current, logger.remote, 10*time.Second-pureSpent)
		if err != nil {
			return err
		}
		defer sealStop()
	}
	sealed, digest, err := set.seal(sealCtx, evidence)
	if err != nil {
		return err
	}
	if logger.remote != nil {
		p := protocol.ExecutionProgress{Kind: "reports_sealed", ExitCode: -1, Reports: &sealed}
		if err = logger.remote.emit(p); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			return errReportSave
		}
	}
	result.Reports, result.ReportSealDigest = &sealed, digest
	return nil
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
	if logger.remote != nil {
		selected := "success"
		if b.Status == "failed" {
			selected = "failure"
		}
		if b.Status == "cancelled" || ctx.Err() != nil {
			selected = "none"
		}
		reason := ""
		if selected == "failure" && b.Reason == "timeout" {
			ordinaryFailed := false
			for _, step := range b.Steps {
				ordinaryFailed = ordinaryFailed || step.Status == "failed" || step.Status == "cancelled"
			}
			if !ordinaryFailed {
				reason = "timeout"
			}
		}
		logger.remote.beginPost(selected, reason)
		for _, group := range []struct {
			name  string
			steps []preparedStep
		}{{"success", build.success}, {"failure", build.failure}} {
			if group.name != selected {
				for _, step := range group.steps {
					logger.remote.skipped(step, "not_selected")
				}
			}
		}
	}
	deadline := time.Now().Add(build.postTimeout)
	unsafe := false
	for _, group := range []struct {
		steps  []preparedStep
		always bool
	}{{phase, false}, {build.always, true}} {
		for _, step := range group.steps {
			reason := ""
			if logger.remote != nil && logger.remote.blocked() != "" {
				reason = logger.remote.blocked()
			} else if logger.remote != nil && logger.remote.limit(step) < 0 {
				reason = "budget_exhausted"
			} else if unsafe {
				reason = "cleanup_error"
			} else if logger.remote == nil && time.Until(deadline) <= 0 {
				reason = "budget_exhausted"
			} else if !group.always && ctx.Err() != nil {
				reason = "cancelled"
			} else if step.skipped {
				reason = "condition"
			}
			if reason != "" {
				b.Post = append(b.Post, skippedStep(step, reason))
				if logger.remote != nil && logger.remote.blocked() == "" {
					logger.remote.skipped(step, reason)
				}
				if reason == "budget_exhausted" && b.Status == "succeeded" {
					b.Status, b.Reason = "failed", "post_error"
				}
				continue
			}
			base := ctx
			if group.always && ctx.Err() != nil {
				base = context.WithoutCancel(ctx)
			}
			limit := durationLimit(step.step, time.Until(deadline), true)
			if logger.remote != nil {
				limit = logger.remote.limit(step)
			}
			s := executeStep(base, root, build.name, step, limit, logger)
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
	if logger.remote != nil && b.Status == "succeeded" {
		_, remaining := logger.remote.budgets()
		started := false
		for _, step := range b.Post {
			started = started || step.started
		}
		if started && remaining == 0 {
			b.Status, b.Reason = "failed", "post_error"
		}
	}
	if ctx.Err() != nil && originalSuccess {
		b.Status, b.Reason = "cancelled", "cancelled"
	}
	return unsafe
}
