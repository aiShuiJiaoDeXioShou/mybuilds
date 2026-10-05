package config

import (
	"fmt"
	"net/url"
	"path"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"
)

var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var secretReference = regexp.MustCompile(`^\$\{[A-Za-z_][A-Za-z0-9_]*\}$`)
var fieldTemplate = regexp.MustCompile(`\{\{[^{}]+\}\}`)

func invalid(field, reason string) error { return fmt.Errorf("%s：%s", field, reason) }

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Validate 检查完整文档，生成缺省步骤名称；不读取环境或运行事实。
func Validate(d *Document) error {
	if d == nil {
		return invalid("配置", "不能为空")
	}
	if d.Version != 1 {
		return invalid("version", "仅支持版本 1")
	}
	if len(d.Builds) == 0 {
		return invalid("builds", "至少需要一个构建")
	}
	if err := validateNotifications(d.Notifications, "notifications"); err != nil {
		return err
	}
	for i, name := range sortedKeys(d.Builds) {
		field := indexed("builds", i)
		if !safeName(name) {
			return invalid(field, "构建名称不是安全标识")
		}
		if err := validateBuild(d.Builds[name], field); err != nil {
			return err
		}
	}
	return nil
}

func validateBuild(b *Build, field string) error {
	if b == nil {
		return invalid(field, "构建不能为空")
	}
	if err := ValidateIOSSigning(b.IOSSigning, field+".ios_signing"); err != nil {
		return err
	}
	if b.Runner != nil {
		if b.Runner.Framework != "" && b.Runner.Framework != "native" && b.Runner.Framework != "flutter" {
			return invalid(field+".runner.framework", "只允许native/flutter")
		}
		if b.Runner.Platform != "android" && b.Runner.Platform != "ios" {
			return invalid(field+".runner.platform", "仅支持 android/ios")
		}
		if err := uniqueNonempty(b.Runner.Labels, field+".runner.labels"); err != nil {
			return err
		}
	}
	for i, name := range sortedKeys(b.Params) {
		key := indexed(field+".params", i)
		if !identifier.MatchString(name) || slices.Contains([]string{"project", "workspace"}, name) {
			return invalid(key, "参数名称不合法或占用上下文名称")
		}
		p := b.Params[name]
		if p.Choices != nil {
			if len(p.Choices) == 0 {
				return invalid(key+".choices", "列表不能为空")
			}
			seen := map[string]bool{}
			for _, value := range p.Choices {
				if seen[value] {
					return invalid(key+".choices", "不能重复")
				}
				seen[value] = true
			}
		}
		if p.Default != nil {
			if err := validateParam(p, *p.Default, key); err != nil {
				return err
			}
		}
	}
	if err := validateEnv(b.Env, field+".env"); err != nil {
		return err
	}
	if err := validateDuration(b.Timeout, field+".timeout"); err != nil {
		return err
	}
	if err := validateWhen(b.When, b.Params, field+".when"); err != nil {
		return err
	}
	if len(b.Steps) == 0 {
		return invalid(field+".steps", "至少需要一个步骤")
	}
	names := make(map[string]bool)
	hasUpload := false
	for _, step := range b.Steps {
		hasUpload = hasUpload || step.Kind == "upload"
	}
	publish := false
	for i := range b.Steps {
		step := &b.Steps[i]
		if hasUpload && (step.Kind == "approval" || step.Kind == "upload") {
			publish = true
		}
		if publish && (step.Kind == "run" || step.Kind == "artifact") {
			return invalid(indexed(field+".steps", i), "构建/产物步骤必须位于发布段之前")
		}
		if err := validateStep(step, b.Params, indexed(field+".steps", i), fmt.Sprintf("%s-%d", step.Kind, i+1), names, false); err != nil {
			return err
		}
	}
	if b.Post != nil {
		if err := validateDuration(b.Post.Timeout, field+".post.timeout"); err != nil {
			return err
		}
		if len(b.Post.Success)+len(b.Post.Failure)+len(b.Post.Always) == 0 {
			return invalid(field+".post", "至少需要一个收尾步骤")
		}
		for _, group := range []struct {
			name  string
			steps []Step
		}{{"success", b.Post.Success}, {"failure", b.Post.Failure}, {"always", b.Post.Always}} {
			for i := range group.steps {
				step := &group.steps[i]
				if err := validateStep(step, b.Params, indexed(field+".post."+group.name, i), fmt.Sprintf("post-%s-%s-%d", group.name, step.Kind, i+1), names, true); err != nil {
					return err
				}
			}
		}
	}
	if b.Reports != nil {
		if b.Reports.JUnit == nil {
			return invalid(field+".reports.junit", "缺少必要字段")
		}
		paths := b.Reports.JUnit.Paths
		if len(paths) > 32 {
			return invalid(field+".reports.junit.paths", "最多允许32个报告模式")
		}
		if err := validatePaths(paths, field+".reports.junit.paths"); err != nil {
			return err
		}
		for i, value := range paths {
			// 模板路径的实际长度在一次渲染后检查，未确定事实保持待定。
			if !fieldTemplate.MatchString(value) && (len(value) > 1024 || len(path.Base(value)) > 255) {
				return invalid(indexed(field+".reports.junit.paths", i), "报告路径或叶名称超过大小上限")
			}
		}
	}
	return validateNotifications(b.Notifications, field+".notifications")
}

func stepFields(kind, target string) map[string]bool {
	fields := map[string]bool{"kind": true, "name": true, "when": true}
	add := func(names ...string) {
		for _, name := range names {
			fields[name] = true
		}
	}
	switch kind {
	case "run":
		add("run", "shell", "working_dir", "env", "timeout")
	case "artifact":
		add("paths")
	case "approval":
		add("notify")
	case "upload":
		add("target", "channel", "app_identifier", "credentials")
		switch target {
		case "google_play":
			add("file", "track", "release_status")
		case "app_store":
			add("file", "submit_for_review", "automatic_release")
		case "custom":
			add("argv", "result_file", "query_argv", "working_dir", "timeout", "env", "file")
		}
	}
	return fields
}

func validateStep(s *Step, params map[string]Parameter, field, defaultName string, names map[string]bool, post bool) error {
	if !slices.Contains([]string{"run", "artifact", "approval", "upload"}, s.Kind) {
		return invalid(field+".kind", "只支持 run/artifact/approval/upload")
	}
	if post && s.Kind != "run" && s.Kind != "artifact" {
		return invalid(field+".kind", "收尾仅允许 run/artifact")
	}
	fields := stepFields(s.Kind, s.Target)
	value := reflect.ValueOf(*s)
	typ := value.Type()
	for i := 0; i < value.NumField(); i++ {
		key := strings.Split(typ.Field(i).Tag.Get("yaml"), ",")[0]
		if !fields[key] && !value.Field(i).IsZero() {
			return invalid(field, "含不适用于步骤种类的字段")
		}
	}
	if s.Name == "" {
		s.Name = defaultName
	}
	if !safeName(s.Name) {
		return invalid(field+".name", "步骤名称不是安全标识")
	}
	if names[s.Name] {
		return invalid(field+".name", "步骤名称不能重复")
	}
	names[s.Name] = true
	if err := validateWhen(s.When, params, field+".when"); err != nil {
		return err
	}
	if err := validateDuration(s.Timeout, field+".timeout"); err != nil {
		return err
	}
	if err := validateEnv(s.Env, field+".env"); err != nil {
		return err
	}
	if s.WorkingDir != "" {
		if err := validatePath(s.WorkingDir, false, field+".working_dir"); err != nil {
			return err
		}
	}
	switch s.Kind {
	case "run":
		if strings.TrimSpace(s.Run) == "" {
			return invalid(field+".run", "需要非空脚本")
		}
		if s.Shell != "" && s.Shell != "sh" && s.Shell != "bash" {
			return invalid(field+".shell", "只支持 sh/bash")
		}
	case "artifact":
		return validatePaths(s.Paths, field+".paths")
	case "upload":
		if err := ValidateStoreUpload(*s, field); err != nil {
			return err
		}
		if !slices.Contains([]string{"google_play", "app_store", "custom"}, s.Target) {
			return invalid(field+".target", "不支持上传目标")
		}
		if s.File != "" {
			if err := validatePath(s.File, true, field+".file"); err != nil {
				return err
			}
		}
		if s.Target == "custom" {
			if s.File == "" || s.AppIdentifier == "" {
				return invalid(field, "custom需要唯一产物模式和应用标识")
			}
			if s.Credentials != "" && !secretReference.MatchString(s.Credentials) {
				return invalid(field+".credentials", "需要完整节点环境引用")
			}
			for _, argv := range [][]string{s.Argv, s.QueryArgv} {
				if len(argv) > 128 {
					return invalid(field+".argv", "超过命令项上限")
				}
				total := 0
				for _, arg := range argv {
					total += len(arg)
					if len(arg) > 4096 || strings.ContainsRune(arg, 0) || total > 64<<10 {
						return invalid(field+".argv", "命令超过限额或含NUL")
					}
				}
			}

			if len(s.Argv) == 0 || strings.TrimSpace(s.Argv[0]) == "" {
				return invalid(field+".argv", "需要非空命令列表")
			}
			if s.QueryArgv != nil && (len(s.QueryArgv) == 0 || strings.TrimSpace(s.QueryArgv[0]) == "") {
				return invalid(field+".query_argv", "需要非空命令列表")
			}
			if err := validatePath(s.ResultFile, false, field+".result_file"); err != nil {
				return err
			}
		} else {
			if strings.TrimSpace(s.File) == "" {
				return invalid(field+".file", "缺少必要产物模式")
			}
			if strings.TrimSpace(s.Credentials) == "" {
				return invalid(field+".credentials", "缺少必要凭据引用")
			}
		}
		if s.ReleaseStatus != "" && !slices.Contains([]string{"draft", "inProgress", "halted", "completed"}, s.ReleaseStatus) {
			return invalid(field+".release_status", "不支持发布状态")
		}
	}
	return nil
}

func validateParam(p Parameter, value, field string) error {
	if p.Required && value == "" {
		return invalid(field, "必填参数不能为空")
	}
	if p.Choices != nil && !slices.Contains(p.Choices, value) {
		return invalid(field, "参数值不在允许列表内")
	}
	return nil
}

// ResolveParams 保留合法空值，覆盖只允许已声明参数；结果为独立映射。
func ResolveParams(b *Build, overrides map[string]string) (map[string]string, error) {
	if b == nil {
		return nil, invalid("params", "构建不能为空")
	}
	for _, name := range sortedKeys(overrides) {
		if _, ok := b.Params[name]; !ok {
			return nil, invalid("params", "覆盖了未声明参数")
		}
	}
	values := make(map[string]string, len(b.Params))
	for i, name := range sortedKeys(b.Params) {
		p := b.Params[name]
		value := ""
		if p.Default != nil {
			value = *p.Default
		}
		if override, ok := overrides[name]; ok {
			value = override
		}
		if err := validateParam(p, value, indexed("params", i)); err != nil {
			return nil, err
		}
		values[name] = value
	}
	return values, nil
}

// Select 保留显式输入顺序，全部选择使用字典序，不推测多个构建中的默认项。
func (d *Document) Select(names []string, all bool) ([]string, error) {
	if d == nil || len(d.Builds) == 0 {
		return nil, invalid("builds", "没有可选构建")
	}
	if all && len(names) > 0 {
		return nil, invalid("builds", "名称选择与全部选择互斥")
	}
	if all {
		return sortedKeys(d.Builds), nil
	}
	if len(names) == 0 {
		if len(d.Builds) != 1 {
			return nil, invalid("builds", "多个构建必须显式选择")
		}
		return sortedKeys(d.Builds), nil
	}
	seen := map[string]bool{}
	for _, name := range names {
		if name == "" {
			return nil, invalid("builds", "选择名称不能为空")
		}
		if _, ok := d.Builds[name]; !ok {
			return nil, invalid("builds", "选择了未知构建")
		}
		if seen[name] {
			return nil, invalid("builds", "选择名称不能重复")
		}
		seen[name] = true
	}
	return slices.Clone(names), nil
}

func safeName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	for _, r := range name {
		if !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '-' && r != '_' && r != '.' {
			return false
		}
	}
	return true
}

func validateDuration(value, field string) error {
	if value == "" {
		return nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return invalid(field, "需要正数 duration")
	}
	return nil
}

func validateEnv(env map[string]string, field string) error {
	for i, name := range sortedKeys(env) {
		if !identifier.MatchString(name) || strings.HasPrefix(name, "MYBUILDS_") {
			return invalid(indexed(field, i), "环境名称不合法或占用引擎变量")
		}
	}
	return nil
}

func validateWhen(w *When, params map[string]Parameter, field string) error {
	if w == nil {
		return nil
	}
	if len(w.Branches)+len(w.Params)+len(w.Changes) == 0 {
		return invalid(field, "条件不能为空")
	}
	if w.Branches != nil {
		if len(w.Branches) == 0 {
			return invalid(field+".branches", "列表不能为空")
		}
		for i, pattern := range w.Branches {
			if strings.TrimSpace(pattern) == "" {
				return invalid(indexed(field+".branches", i), "模式不能为空")
			}
			if _, err := path.Match(pattern, ""); err != nil {
				return invalid(indexed(field+".branches", i), "非法 glob")
			}
		}
	}
	if w.Params != nil && len(w.Params) == 0 {
		return invalid(field+".params", "条件映射不能为空")
	}
	for i, name := range sortedKeys(w.Params) {
		if _, ok := params[name]; !ok {
			return invalid(indexed(field+".params", i), "条件引用了未声明参数")
		}
	}
	if w.Changes != nil {
		return validatePaths(w.Changes, field+".changes")
	}
	return nil
}

func validatePaths(paths []string, field string) error {
	if len(paths) == 0 {
		return invalid(field, "需要非空路径列表")
	}
	for i, value := range paths {
		if err := validatePath(value, true, indexed(field, i)); err != nil {
			return err
		}
	}
	return nil
}

func validatePath(value string, glob bool, field string) error {
	if strings.TrimSpace(value) == "" || strings.HasPrefix(value, "/") || strings.Contains(value, "\\") || (len(value) > 1 && value[1] == ':') {
		return invalid(field, "需要仓库相对路径")
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return invalid(field, "路径不能含控制字符")
		}
	}
	for _, part := range strings.Split(value, "/") {
		if part == ".." {
			return invalid(field, "路径不能向上越界")
		}
	}
	if glob {
		if _, err := path.Match(value, ""); err != nil {
			return invalid(field, "非法 glob")
		}
	} else if strings.ContainsAny(value, "*?[") {
		return invalid(field, "路径不能包含 glob")
	}
	return nil
}

func uniqueNonempty(values []string, field string) error {
	seen := map[string]bool{}
	for _, value := range values {
		if strings.TrimSpace(value) == "" || seen[value] {
			return invalid(field, "列表项不能为空或重复")
		}
		seen[value] = true
	}
	return nil
}

func validateNotifications(n *Notifications, field string) error {
	if n == nil {
		return nil
	}
	if n.On != nil {
		if len(n.On) == 0 {
			return invalid(field+".on", "列表不能为空")
		}
		if err := uniqueNonempty(n.On, field+".on"); err != nil {
			return err
		}
		for _, event := range n.On {
			if !slices.Contains([]string{"success", "failure", "cancelled"}, event) {
				return invalid(field+".on", "不支持通知事件")
			}
		}
	}
	for i, hook := range n.Webhooks {
		item := indexed(field+".webhooks", i)
		if !slices.Contains([]string{"feishu", "wechat", "dingtalk", "generic"}, hook.Type) {
			return invalid(item+".type", "不支持通知类型")
		}
		if secretReference.MatchString(hook.URL) {
			continue
		}
		value := fieldTemplate.ReplaceAllString(hook.URL, "template")
		address, err := url.Parse(value)
		if err != nil || (address.Scheme != "http" && address.Scheme != "https") || address.Hostname() == "" || address.User != nil {
			return invalid(item+".url", "需要有效 HTTP(S) URL 或环境引用")
		}
	}
	return nil
}
