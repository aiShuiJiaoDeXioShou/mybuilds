// Package pipeline 提供流水线预览与本地执行，共享配置校验规则。
package pipeline

import (
	"fmt"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"

	"mybuilds/internal/config"
	"mybuilds/internal/mobile"
)

type PreviewOptions struct {
	Names       []string
	All         bool
	Params      map[string]string
	BuildParams map[string]map[string]string
	Step        string
	Facts       map[string]string
}

// PreviewPlan 仅含可公开的摘要，不保存配置正文或任何参数值。
type PreviewPlan struct {
	Builds                []BuildPreview `json:"builds"`
	SensitiveValuesHidden bool           `json:"sensitive_values_hidden"`
}

type BuildPreview struct {
	Name          string             `json:"name"`
	Platform      string             `json:"platform,omitempty"`
	Framework     string             `json:"framework,omitempty"`
	Parameters    []ParameterPreview `json:"parameters"`
	Condition     string             `json:"condition"`
	Reasons       []string           `json:"reasons"`
	Timeout       string             `json:"timeout,omitempty"`
	Steps         []StepPreview      `json:"steps"`
	Post          *PostPreview       `json:"post,omitempty"`
	Reports       bool               `json:"reports,omitempty"`
	Notifications *ConditionPreview  `json:"notifications,omitempty"`
}

type ParameterPreview struct {
	Name        string `json:"name"`
	ValueHidden bool   `json:"value_hidden"`
}

type ConditionPreview struct {
	Condition string   `json:"condition"`
	Reasons   []string `json:"reasons"`
}

type StepPreview struct {
	Index     int      `json:"index"`
	Kind      string   `json:"kind"`
	Name      string   `json:"name"`
	Condition string   `json:"condition"`
	Reasons   []string `json:"reasons"`
	Timeout   string   `json:"timeout,omitempty"`
	PathCount int      `json:"path_count,omitempty"`
}

type PostPreview struct {
	Timeout string        `json:"timeout,omitempty"`
	Success []StepPreview `json:"success,omitempty"`
	Failure []StepPreview `json:"failure,omitempty"`
	Always  []StepPreview `json:"always,omitempty"`
}

var referenceName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Preview 完整校验所选构建后生成纯数据；Facts 是唯一的运行上下文来源。
func Preview(document *config.Document, options PreviewOptions) (*PreviewPlan, error) {
	document = validationCopy(document)
	if err := config.Validate(document); err != nil {
		return nil, err
	}
	names, err := document.Select(options.Names, options.All)
	if err != nil {
		return nil, err
	}
	if options.Step != "" && len(names) != 1 {
		return nil, fmt.Errorf("step: 只允许选择一个 build")
	}
	params, err := resolveBuildParameters(document, names, options)
	if err != nil {
		return nil, err
	}
	plan := &PreviewPlan{Builds: make([]BuildPreview, 0, len(names)), SensitiveValuesHidden: true}
	for i, name := range names {
		b, err := previewBuild(document, name, params[i], options.Facts)
		if err != nil {
			return nil, err
		}
		plan.Builds = append(plan.Builds, b)
	}
	// 裁剪发生在全部参数和模板检查之后，不能借此绕过未显示步骤的错误。
	if options.Step != "" {
		var selected []StepPreview
		for _, step := range plan.Builds[0].Steps {
			if step.Name == options.Step {
				selected = append(selected, step)
			}
		}
		if len(selected) == 0 {
			return nil, fmt.Errorf("step: 未找到指定步骤")
		}
		plan.Builds[0].Steps = selected
	}
	return plan, nil
}

// Validate 会补全步骤名，仅复制会被它改写的结构，其他字段保持只读。
func validationCopy(document *config.Document) *config.Document {
	if document == nil {
		return nil
	}
	d := *document
	d.Builds = make(map[string]*config.Build, len(document.Builds))
	for name, build := range document.Builds {
		if build == nil {
			d.Builds[name] = nil
			continue
		}
		b := *build
		b.Steps = slices.Clone(build.Steps)
		if build.Post != nil {
			p := *build.Post
			p.Success = slices.Clone(p.Success)
			p.Failure = slices.Clone(p.Failure)
			p.Always = slices.Clone(p.Always)
			b.Post = &p
		}
		d.Builds[name] = &b
	}
	return &d
}

func previewBuild(document *config.Document, name string, params, facts map[string]string) (BuildPreview, error) {
	build := document.Builds[name]
	field := "builds." + name
	context := make(map[string]string, len(facts)+1)
	for key, value := range facts {
		if key != "ios.output_dir" {
			context[key] = value
		}
	}
	context["build.name"] = name
	if iosOutputReferenced(build) {
		hasRun := false
		for _, step := range build.Steps {
			hasRun = hasRun || step.Kind == "run"
		}
		if build.IOSSigning == nil || !hasRun {
			return BuildPreview{}, fmt.Errorf("%s.ios_signing: 系统产物目录需要签名与普通run", field)
		}
	}
	state := evaluateWhen(build.When, params, facts)
	// build.env 在每个步骤使用，step.name 引用有实际名称，不是缺失运行事实。
	if len(build.Steps) != 0 {
		context["step.name"] = stepName(build.Steps[0], 1)
	}
	missing, err := checkEnv(build.Env, field+".env", params, context)
	delete(context, "step.name")
	if err != nil {
		return BuildPreview{}, err
	}
	state = combine(state, templateCondition(missing))
	if build.IOSSigning != nil {
		_, missing, err := renderIOSSigning(build.IOSSigning, field+".ios_signing", params, context)
		if err != nil {
			return BuildPreview{}, err
		}
		state = combine(state, templateCondition(missing))
	}
	if build.Reports != nil && build.Reports.JUnit != nil {
		for _, value := range build.Reports.JUnit.Paths {
			missing, err := checkField(value, field+".reports.junit.paths", params, context, false, false)
			if err != nil {
				return BuildPreview{}, err
			}
			state = combine(state, templateCondition(missing))
		}
	}
	b := BuildPreview{Name: name, Parameters: []ParameterPreview{}, Condition: state.Condition, Reasons: state.Reasons, Timeout: build.Timeout, Steps: []StepPreview{}, Reports: build.Reports != nil}
	if build.Runner != nil {
		b.Platform = build.Runner.Platform
		b.Framework = build.Runner.Framework
	}
	for _, key := range sortedKeys(params) {
		b.Parameters = append(b.Parameters, ParameterPreview{Name: key, ValueHidden: true})
	}
	for i, step := range build.Steps {
		s, err := previewStep(step, i+1, field+".steps", params, context, state)
		if err != nil {
			return BuildPreview{}, err
		}
		b.Steps = append(b.Steps, s)
	}
	if build.Post != nil {
		b.Post = &PostPreview{Timeout: build.Post.Timeout}
		for _, phase := range []struct {
			name   string
			steps  []config.Step
			output *[]StepPreview
		}{{"success", build.Post.Success, &b.Post.Success}, {"failure", build.Post.Failure, &b.Post.Failure}, {"always", build.Post.Always, &b.Post.Always}} {
			phaseState := state
			if phase.name != "always" {
				phaseState = combine(phaseState, ConditionPreview{Condition: "pending", Reasons: []string{"收尾阶段等待构建结果"}})
			}
			for i, step := range phase.steps {
				s, err := previewStep(step, i+1, field+".post."+phase.name, params, context, phaseState)
				if err != nil {
					return BuildPreview{}, err
				}
				*phase.output = append(*phase.output, s)
			}
		}
	}
	for _, notification := range []struct {
		value *config.Notifications
		field string
	}{{document.Notifications, "notifications"}, {build.Notifications, field + ".notifications"}} {
		if notification.value == nil {
			continue
		}
		n, err := previewNotifications(notification.value, notification.field, params, context)
		if err != nil {
			return BuildPreview{}, err
		}
		// build 自身的配置覆盖公共通知，但公共通知仍完整校验。
		b.Notifications = &n
	}
	return b, nil
}

func stepName(step config.Step, index int) string {
	if step.Name != "" {
		return step.Name
	}
	return fmt.Sprintf("%s-%d", step.Kind, index)
}

func previewStep(step config.Step, index int, field string, params, context map[string]string, parent ConditionPreview) (StepPreview, error) {
	name := stepName(step, index)
	local := make(map[string]string, len(context)+1)
	for key, value := range context {
		local[key] = value
	}
	local["step.name"] = name
	state := combine(parent, evaluateWhen(step.When, params, context))
	field = fmt.Sprintf("%s[%d]", field, index)
	missing, err := checkEnv(step.Env, field+".env", params, local)
	if err != nil {
		return StepPreview{}, err
	}
	state = combine(state, templateCondition(missing))
	for _, value := range []struct {
		name, value string
		secret      bool
	}{{"working_dir", step.WorkingDir, false}, {"file", step.File, false}, {"channel", step.Channel, false}, {"track", step.Track, false}, {"app_identifier", step.AppIdentifier, false}, {"release_status", step.ReleaseStatus, false}, {"result_file", step.ResultFile, false}, {"credentials", step.Credentials, true}} {
		missing, err := checkField(value.value, field+"."+value.name, params, local, value.secret, false)
		if err != nil {
			return StepPreview{}, err
		}
		state = combine(state, templateCondition(missing))
	}
	for _, value := range step.Paths {
		missing, err := checkField(value, field+".paths", params, local, false, false)
		if err != nil {
			return StepPreview{}, err
		}
		state = combine(state, templateCondition(missing))
	}
	return StepPreview{Index: index, Kind: step.Kind, Name: name, Condition: state.Condition, Reasons: state.Reasons, Timeout: step.Timeout, PathCount: len(step.Paths)}, nil
}

func previewNotifications(notifications *config.Notifications, field string, params, context map[string]string) (ConditionPreview, error) {
	state := ready()
	if notifications.Enabled != nil && !*notifications.Enabled {
		state = ConditionPreview{Condition: "skipped", Reasons: []string{"通知已关闭"}}
	}
	values := []string{notifications.Template}
	for _, hook := range notifications.Webhooks {
		values = append(values, hook.URL)
	}
	for _, value := range values {
		missing, err := checkField(value, field, params, context, true, true)
		if err != nil {
			return ConditionPreview{}, err
		}
		state = combine(state, templateCondition(missing))
	}
	return state, nil
}

func evaluateWhen(when *config.When, params, facts map[string]string) ConditionPreview {
	state := ready()
	if when == nil {
		return state
	}
	if len(when.Changes) != 0 {
		state.Reasons = append(state.Reasons, "changes: 手动预览忽略变更筛选")
	}
	for _, key := range sortedKeys(when.Params) {
		if params[key] != when.Params[key] {
			state = combine(state, ConditionPreview{Condition: "skipped", Reasons: []string{"参数条件不满足"}})
		}
	}
	if len(when.Branches) != 0 {
		branch, known := facts["git.branch"]
		if !known {
			state = combine(state, ConditionPreview{Condition: "pending", Reasons: []string{"分支事实待确定"}})
		} else {
			matches := false
			for _, pattern := range when.Branches {
				match, _ := path.Match(pattern, branch)
				matches = matches || match
			}
			if !matches {
				state = combine(state, ConditionPreview{Condition: "skipped", Reasons: []string{"分支条件不满足"}})
			}
		}
	}
	return state
}

func ready() ConditionPreview {
	return ConditionPreview{Condition: "ready", Reasons: []string{"条件已满足"}}
}
func templateCondition(missing bool) ConditionPreview {
	if missing {
		return ConditionPreview{Condition: "pending", Reasons: []string{"模板上下文待确定"}}
	}
	return ConditionPreview{Condition: "ready"}
}

func combine(a, b ConditionPreview) ConditionPreview {
	state := "ready"
	if a.Condition == "pending" || b.Condition == "pending" {
		state = "pending"
	}
	if a.Condition == "skipped" || b.Condition == "skipped" {
		state = "skipped"
	}
	reasons := []string{}
	for _, reason := range append(append([]string{}, a.Reasons...), b.Reasons...) {
		if reason == "条件已满足" {
			continue
		}
		if !slices.Contains(reasons, reason) {
			reasons = append(reasons, reason)
		}
	}
	if len(reasons) == 0 {
		reasons = []string{"条件已满足"}
	}
	return ConditionPreview{Condition: state, Reasons: reasons}
}

func checkEnv(env map[string]string, field string, params, context map[string]string) (bool, error) {
	missing := false
	for _, key := range sortedKeys(env) {
		pending, err := checkField(env[key], field, params, context, true, false)
		if err != nil {
			return false, err
		}
		missing = missing || pending
	}
	return missing, nil
}

// checkField 只扫描原字段，参数值不会作为模板再次解释；渲染结果不进入预览。
func checkField(value, field string, params, context map[string]string, secrets, notification bool) (bool, error) {
	_, missing, err := config.RenderField(value, field, params, context, secrets, notification)
	return missing, err
}

func sortedKeys[V any](values map[string]V) []string {
	return slices.Sorted(maps.Keys(values))
}

// 签名只用同一个RenderField解释一次原字段；返回数据不读取材料。
func renderIOSSigning(in *config.IOSSigning, field string, params, facts map[string]string) (config.IOSSigning, bool, error) {
	out := *in
	missing := false
	for _, item := range []struct {
		name   string
		target *string
	}{{"bundle_id", &out.BundleID}, {"export_method", &out.ExportMethod}} {
		value, pending, err := config.RenderField(*item.target, field+"."+item.name, params, facts, false, false)
		if err != nil {
			return config.IOSSigning{}, false, err
		}
		missing = missing || pending
		if !pending {
			if strings.ContainsAny(value, "{}") {
				return config.IOSSigning{}, false, fmt.Errorf("%s: 渲染后需要合法字面量", field)
			}
			*item.target = value
		}
	}
	if err := config.ValidateIOSSigning(&out, field); err != nil {
		return config.IOSSigning{}, false, err
	}
	return out, missing, nil
}

var iosOutputPattern = regexp.MustCompile(`\{\{\s*ios\.output_dir\s*\}\}`)

func iosOutputReferenced(b *config.Build) bool {
	values := []string{}
	for _, v := range b.Env {
		values = append(values, v)
	}
	if b.Reports != nil && b.Reports.JUnit != nil {
		values = append(values, b.Reports.JUnit.Paths...)
	}
	steps := slices.Clone(b.Steps)
	if b.Post != nil {
		steps = append(steps, b.Post.Success...)
		steps = append(steps, b.Post.Failure...)
		steps = append(steps, b.Post.Always...)
	}
	for _, step := range steps {
		values = append(values, step.Run, step.WorkingDir, step.File)
		values = append(values, step.Paths...)
		for _, v := range step.Env {
			values = append(values, v)
		}
	}
	for _, v := range values {
		if iosOutputPattern.MatchString(v) {
			return true
		}
	}
	return false
}

// resolveBuildParameters 由纯预览与唯一 Run 共用，不混入未选择构建的作用域。
func resolveBuildParameters(document *config.Document, names []string, options PreviewOptions) ([]map[string]string, error) {
	for name := range options.BuildParams {
		if !slices.Contains(names, name) {
			return nil, fmt.Errorf("params: 未选择或未知构建作用域")
		}
	}
	resolved := make([]map[string]string, len(names))
	for i, name := range names {
		overrides := maps.Clone(options.Params)
		if overrides == nil {
			overrides = map[string]string{}
		}
		for key, value := range options.BuildParams[name] {
			overrides[key] = value
		}
		params, err := config.ResolveParams(document.Builds[name], overrides)
		if err != nil {
			return nil, err
		}
		build := document.Builds[name]
		if build.Runner != nil && build.Runner.Framework == "flutter" {
			if err := mobile.ValidateFlutterParameters(build.Runner.Platform, params, params["build_number"]); err != nil {
				return nil, err
			}
		}
		resolved[i] = params
	}
	return resolved, nil
}
