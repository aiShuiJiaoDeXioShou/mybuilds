package mobile

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"

	"mybuilds/internal/process"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

//go:embed templates/flutter-android.yml
var flutterAndroidTemplate string

//go:embed templates/flutter-ios.yml
var flutterIOSTemplate string

// FlutterTemplate 只组合已选择的可编辑声明，不访问工程、工具或签名材料。
func FlutterTemplate(platforms []string) ([]byte, error) {
	selected, err := flutterPlatforms(platforms, false)
	if err != nil {
		return nil, err
	}
	var document strings.Builder
	document.WriteString("version: 1\nbuilds:\n")
	for _, platform := range []string{"android", "ios"} {
		if selected[platform] {
			if platform == "android" {
				document.WriteString(flutterAndroidTemplate)
			} else {
				document.WriteString(flutterIOSTemplate)
			}
		}
	}
	return []byte(document.String()), nil
}

func flutterPlatforms(platforms []string, allowEmpty bool) (map[string]bool, error) {
	if !allowEmpty && len(platforms) == 0 {
		return nil, errors.New("flutter_platform_invalid")
	}
	selected := make(map[string]bool, len(platforms))
	for _, platform := range platforms {
		if (platform != "android" && platform != "ios") || selected[platform] {
			return nil, errors.New("flutter_platform_invalid")
		}
		selected[platform] = true
	}
	return selected, nil
}

var flutterVersionValue = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
var flutterNumberValue = regexp.MustCompile(`^[0-9]+$`)

// ValidateFlutterParameters 不注入未声明字段，有效编号独立于本地参数来源。
func ValidateFlutterParameters(platform string, params map[string]string, effectiveNumber string) error {
	if platform != "android" && platform != "ios" {
		return errors.New("flutter_platform_invalid")
	}
	for _, name := range []string{"version", "channel", "flavor", "build_number"} {
		value, declared := params[name]
		if !declared {
			continue
		}
		if !utf8.ValidString(value) || strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return errors.New("flutter_" + name + "_invalid")
		}
		if name == "version" && !flutterVersionValue.MatchString(value) {
			return errors.New("flutter_version_invalid")
		}
		if name == "build_number" && !flutterValidNumber(platform, value) {
			return errors.New("flutter_build_number_invalid")
		}
	}
	if effectiveNumber != "" && !flutterValidNumber(platform, effectiveNumber) {
		return errors.New("flutter_build_number_invalid")
	}
	return nil
}

func flutterValidNumber(platform, number string) bool {
	if !flutterNumberValue.MatchString(number) {
		return false
	}
	// 字符串比较避免用户值或当前 Apple 编号超出本机整数宽度。
	number = strings.TrimLeft(number, "0")
	if number == "" {
		return false
	}
	return platform != "android" || len(number) < 10 || (len(number) == 10 && number <= "2100000000")
}

// FlutterDoctorOptions 的非 nil 环境是本次明确声明，绝不重新继承宿主变量。
type FlutterDoctorOptions struct {
	Workspace   string
	Environment map[string]string
	Platforms   []string
}

// FlutterDoctor 仅运行已初始化 SDK 的版本查询，平台工具不调用仓库脚本。
func FlutterDoctor(ctx context.Context, options FlutterDoctorOptions) (checks []DoctorCheck) {
	selected, selectionErr := flutterPlatforms(options.Platforms, true)
	names := []string{"flutter.sdk", "flutter.dart"}
	if selected["android"] {
		names = append(names, "flutter.android_java", "flutter.android_aapt2", "flutter.android_apksigner")
	}
	if selected["ios"] {
		names = append(names, "flutter.ios_xcode", "flutter.cocoapods")
	}
	checks = make([]DoctorCheck, 0, len(names))
	env, envErr := flutterEnvironment(options.Environment)
	var sdk *flutterSDKInfo
	var sdkReason string
	if selectionErr != nil || envErr != nil {
		sdkReason = "tool_incompatible"
	} else if ctx.Err() != nil {
		sdkReason = flutterReason(ctx, ctx.Err())
	} else if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		sdkReason = "unsupported"
	} else {
		sdk, sdkReason = flutterSDK(ctx, env)
	}
	if sdkReason != "" {
		for _, name := range names {
			checks = append(checks, DoctorCheck{Name: name, Status: "failed", Reason: sdkReason})
		}
		return
	}
	// 版本查询所用 HOME 自有且短命，不接触用户偏好或任何签名材料。
	home, err := os.MkdirTemp("", "mybuilds-flutter-doctor-")
	if err != nil {
		for _, name := range names {
			checks = append(checks, DoctorCheck{Name: name, Status: "failed", Reason: "tool_error"})
		}
		return
	}
	identity, err := os.Stat(home)
	if err != nil {
		os.Remove(home)
		for _, name := range names {
			checks = append(checks, DoctorCheck{Name: name, Status: "failed", Reason: "tool_error"})
		}
		return
	}
	defer func() {
		current, e := os.Lstat(home)
		if e != nil || !current.IsDir() || !os.SameFile(identity, current) || os.RemoveAll(home) != nil {
			for i := range checks {
				checks[i].Status = "failed"
				checks[i].Reason = "cleanup_error"
			}
		}
	}()
	env["HOME"] = home
	env["FLUTTER_ROOT"] = sdk.root
	// 官方工具的自动化入口同时关闭首次告知及 Azure metadata 探测，非永久偏好。
	env["CI"] = "true"
	env["FLUTTER_SUPPRESS_ANALYTICS"] = "true"
	unsafe := false
	for _, name := range names {
		check := DoctorCheck{Name: name, Status: "passed"}
		if unsafe {
			check.Status, check.Reason = "failed", "cleanup_error"
		} else if ctx.Err() != nil {
			check.Status, check.Reason = "failed", flutterReason(ctx, ctx.Err())
		} else {
			var output string
			var callErr error
			switch name {
			case "flutter.sdk":
				output, callErr = toolOutput(ctx, toolCommand{Workspace: options.Workspace, Executable: sdk.dart, Args: []string{sdk.snapshot, "--no-version-check", "--suppress-analytics", "--version", "--machine"}, Env: env})
				if callErr == nil {
					var actual flutterVersionInfo
					if json.Unmarshal([]byte(output), &actual) != nil || actual.Framework != sdk.version.Framework || actual.Dart != sdk.version.Dart || actual.Engine != sdk.version.Engine || actual.Revision != sdk.version.Revision || actual.Channel != "stable" {
						check.Reason = "tool_version_invalid"
					} else {
						check.Version = actual.Framework
					}
				}
			case "flutter.dart":
				output, callErr = toolOutput(ctx, toolCommand{Workspace: options.Workspace, Executable: sdk.dart, Args: []string{"--version"}, Env: env})
				if callErr == nil {
					match := flutterDartVersion.FindStringSubmatch(output)
					if len(match) != 2 || match[1] != sdk.version.Dart {
						check.Reason = "tool_version_invalid"
					} else {
						check.Version = match[1]
					}
				}
			default:
				check.Version, check.Reason = flutterPlatformTool(ctx, options.Workspace, env, name)
			}
			if callErr != nil {
				check.Reason = flutterReason(ctx, callErr)
			}
			if check.Reason != "" {
				check.Status = "failed"
			}
		}
		if check.Reason == "cleanup_error" {
			unsafe = true
		}
		checks = append(checks, check)
	}
	return
}

var flutterDartVersion = regexp.MustCompile(`(?m)^Dart SDK version: ([0-9]+\.[0-9]+\.[0-9]+)(?: |$)`)
var flutterHash = regexp.MustCompile(`^[a-f0-9]{40}$`)
var flutterNumericVersion = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)+$`)

type flutterVersionInfo struct {
	Framework   string `json:"frameworkVersion"`
	Dart        string `json:"dartSdkVersion"`
	Revision    string `json:"frameworkRevision"`
	Engine      string `json:"engineRevision"`
	ContentHash string `json:"engineContentHash"`
	Channel     string `json:"channel"`
}
type flutterSDKInfo struct {
	root, dart, snapshot string
	version              flutterVersionInfo
}

func flutterEnvironment(explicit map[string]string) (map[string]string, error) {
	if explicit == nil {
		explicit = process.HostEnvironment()
	}
	env := make(map[string]string)
	for name, value := range explicit {
		if !toolEnvName.MatchString(name) || strings.ContainsRune(value, 0) {
			return nil, errors.New("tool_environment_error")
		}
	}
	for _, name := range []string{"PATH", "HOME", "TMPDIR", "LANG", "LC_ALL", "JAVA_HOME", "ANDROID_HOME", "ANDROID_SDK_ROOT", "DEVELOPER_DIR", "FLUTTER_ROOT", "PUB_CACHE"} {
		if value, ok := explicit[name]; ok {
			env[name] = value
		}
	}
	return env, nil
}

// flutterRead 的非阻塞打开只用于实际 Unix 平台，随后同一 fd 核普通类型和限额。
func flutterRead(ctx context.Context, filename string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	flags := os.O_RDONLY
	switch runtime.GOOS {
	case "linux":
		flags |= 0x800
	case "darwin":
		flags |= 0x4
	default:
		return nil, errors.New("unsupported")
	}
	f, err := os.OpenFile(filename, flags, 0)
	if err != nil {
		return nil, errors.New("tool_cache_error")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 64*1024 {
		return nil, errors.New("tool_cache_error")
	}
	bytes, err := io.ReadAll(io.LimitReader(f, 64*1024+1))
	if err != nil || len(bytes) > 64*1024 {
		return nil, errors.New("tool_cache_error")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return bytes, nil
}

func flutterSDK(ctx context.Context, env map[string]string) (*flutterSDKInfo, string) {
	root := env["FLUTTER_ROOT"]
	if root == "" {
		executable := flutterFindTool(env, "flutter")
		if executable == "" {
			return nil, "tool_missing"
		}
		root = filepath.Dir(filepath.Dir(executable))
	}
	root, err := filepath.Abs(root)
	if err == nil {
		root, err = filepath.EvalSymlinks(root)
	}
	if err != nil {
		return nil, "tool_missing"
	}
	sdk := &flutterSDKInfo{root: root, dart: filepath.Join(root, "bin/cache/dart-sdk/bin/dart"), snapshot: filepath.Join(root, "bin/cache/flutter_tools.snapshot")}
	data, err := flutterRead(ctx, filepath.Join(root, "bin/cache/flutter.version.json"))
	if err != nil {
		return nil, flutterCacheReason(ctx)
	}
	if json.Unmarshal(data, &sdk.version) != nil || !flutterVersionValue.MatchString(sdk.version.Framework) || len(sdk.version.Framework) > 128 || !flutterVersionValue.MatchString(sdk.version.Dart) || len(sdk.version.Dart) > 128 || !flutterHash.MatchString(sdk.version.Revision) || !flutterHash.MatchString(sdk.version.Engine) || sdk.version.Channel != "stable" {
		return nil, "tool_incompatible"
	}
	for name, want := range map[string]string{"bin/cache/engine.stamp": sdk.version.Engine, "bin/cache/engine-dart-sdk.stamp": sdk.version.Engine, "bin/cache/engine_stamp.stamp": sdk.version.Engine, "bin/cache/dart-sdk/version": sdk.version.Dart} {
		actual, e := flutterRead(ctx, filepath.Join(root, name))
		if e != nil || strings.TrimSpace(string(actual)) != want {
			return nil, flutterCacheReason(ctx)
		}
	}
	stamp, err := flutterRead(ctx, filepath.Join(root, "bin/cache/flutter_tools.stamp"))
	if err != nil || !strings.HasPrefix(string(stamp), sdk.version.Revision+":") {
		return nil, flutterCacheReason(ctx)
	}
	engine, err := flutterRead(ctx, filepath.Join(root, "bin/cache/engine_stamp.json"))
	var engineInfo struct {
		Revision    string `json:"git_revision"`
		ContentHash string `json:"content_hash"`
	}
	if err != nil || json.Unmarshal(engine, &engineInfo) != nil || engineInfo.Revision != sdk.version.Engine || !flutterHash.MatchString(engineInfo.ContentHash) || engineInfo.ContentHash != sdk.version.ContentHash {
		return nil, flutterCacheReason(ctx)
	}
	packageConfig, err := flutterRead(ctx, filepath.Join(root, "packages/flutter_tools/.dart_tool/package_config.json"))
	var packages struct {
		Version  int                              `json:"configVersion"`
		Packages []struct{ Name, RootURI string } `json:"packages"`
	}
	if err != nil || json.Unmarshal(packageConfig, &packages) != nil || packages.Version != 2 || len(packages.Packages) == 0 {
		return nil, flutterCacheReason(ctx)
	}
	// 当前 stable 新布局默认不写 legacy version；旧布局必须已有该文件。
	legacy := filepath.Join(root, "version")
	if _, e := os.Lstat(legacy); e == nil {
		actual, e := flutterRead(ctx, legacy)
		if e != nil || strings.TrimSpace(string(actual)) != sdk.version.Framework {
			return nil, flutterCacheReason(ctx)
		}
	} else if !os.IsNotExist(e) || flutterOlderVersion(sdk.version.Framework, "3.38.0") {
		return nil, "uninitialized"
	}
	// 版本查询会清除此旧缓存，必须事先准备而不能在体检中改变它。
	if _, e := os.Lstat(filepath.Join(root, "bin/cache/canvaskit")); !os.IsNotExist(e) {
		return nil, "uninitialized"
	}
	for _, name := range []string{sdk.dart, sdk.snapshot} {
		info, e := os.Stat(name)
		if e != nil || !info.Mode().IsRegular() || info.Size() == 0 || (name == sdk.dart && info.Mode().Perm()&0111 == 0) {
			return nil, "uninitialized"
		}
	}
	return sdk, ""
}

func flutterCacheReason(ctx context.Context) string {
	if ctx.Err() != nil {
		return flutterReason(ctx, ctx.Err())
	}
	return "uninitialized"
}
func flutterOlderVersion(actual, want string) bool {
	a, b := strings.Split(actual, "."), strings.Split(want, ".")
	for i := 0; i < 3; i++ {
		x, e := strconv.Atoi(a[i])
		y, _ := strconv.Atoi(b[i])
		if e != nil {
			return true
		}
		if x != y {
			return x < y
		}
	}
	return false
}

func flutterFindTool(env map[string]string, name string) string {
	if filepath.IsAbs(name) {
		if androidRegular(name, true) {
			path, e := filepath.EvalSymlinks(name)
			if e == nil {
				return path
			}
		}
		return ""
	}
	for _, dir := range filepath.SplitList(env["PATH"]) {
		// 不以工作区或 ambient PATH 填补相对项。
		if !filepath.IsAbs(dir) {
			continue
		}
		candidate := filepath.Join(dir, name)
		if androidRegular(candidate, true) {
			path, e := filepath.EvalSymlinks(candidate)
			if e == nil {
				return path
			}
		}
	}
	return ""
}

func flutterPlatformTool(ctx context.Context, workspace string, env map[string]string, name string) (string, string) {
	executable := ""
	var args []string
	var pattern *regexp.Regexp
	switch name {
	case "flutter.android_java":
		if env["JAVA_HOME"] != "" {
			executable = filepath.Join(env["JAVA_HOME"], "bin/java")
		} else {
			executable = flutterFindTool(env, "java")
		}
		args = []string{"-version"}
		pattern = javaVersion
	case "flutter.android_aapt2", "flutter.android_apksigner":
		sdk := env["ANDROID_HOME"]
		if sdk == "" {
			sdk = env["ANDROID_SDK_ROOT"]
		}
		if sdk == "" {
			return "", "tool_missing"
		}
		if env["ANDROID_HOME"] != "" && env["ANDROID_SDK_ROOT"] != "" {
			a, e := os.Stat(env["ANDROID_HOME"])
			b, f := os.Stat(env["ANDROID_SDK_ROOT"])
			if e != nil || f != nil || !os.SameFile(a, b) {
				return "", "tool_incompatible"
			}
		}
		entries, err := os.ReadDir(filepath.Join(sdk, "build-tools"))
		if err != nil {
			return "", "tool_missing"
		}
		versions := []string{}
		for _, entry := range entries {
			if androidBuildTools.MatchString(entry.Name()) {
				versions = append(versions, entry.Name())
			}
		}
		sort.Slice(versions, func(i, j int) bool { return flutterOlderVersion(versions[i], versions[j]) })
		if len(versions) == 0 {
			return "", "tool_missing"
		}
		tool := "aapt2"
		args = []string{"version"}
		pattern = regexp.MustCompile(`(?m)^Android Asset Packaging Tool \(aapt\) ([0-9]+(?:\.[0-9]+)+)`)
		if name == "flutter.android_apksigner" {
			tool = "apksigner"
			args = []string{"version"}
			pattern = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)+)\s*$`)
		}
		executable = filepath.Join(sdk, "build-tools", versions[len(versions)-1], tool)
	case "flutter.ios_xcode":
		if runtime.GOOS != "darwin" {
			return "", "unsupported"
		}
		executable = flutterFindTool(env, "xcodebuild")
		args = []string{"-version"}
		pattern = regexp.MustCompile(`(?m)^Xcode ([0-9]+(?:\.[0-9]+)+)\r?$`)
	case "flutter.cocoapods":
		if runtime.GOOS != "darwin" {
			return "", "unsupported"
		}
		executable = flutterFindTool(env, "pod")
		args = []string{"--version"}
		pattern = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)+)\s*$`)
	}
	if !androidRegular(executable, true) {
		return "", "tool_missing"
	}
	output, err := toolOutput(ctx, toolCommand{Workspace: workspace, Executable: executable, Args: args, Env: env})
	if err != nil {
		return "", flutterReason(ctx, err)
	}
	match := pattern.FindStringSubmatch(output)
	if len(match) != 2 || len(match[1]) > 128 || !flutterNumericVersion.MatchString(match[1]) {
		return "", "tool_version_invalid"
	}
	if name == "flutter.android_java" {
		major, _ := strconv.Atoi(strings.Split(match[1], ".")[0])
		if major < 17 {
			return match[1], "tool_incompatible"
		}
	}
	return match[1], ""
}

func flutterReason(ctx context.Context, err error) string {
	if err != nil && err.Error() == "tool_cleanup_error" {
		return "cleanup_error"
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "tool_timeout"
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return "cancelled"
	}
	if errors.Is(err, errToolOutputLimit) {
		return "output_limit"
	}
	switch err.Error() {
	case "tool_timeout":
		return "tool_timeout"
	case "tool_cancelled":
		return "cancelled"
	case "tool_cleanup_error":
		return "cleanup_error"
	}
	return "tool_error"
}
