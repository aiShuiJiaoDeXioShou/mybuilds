package mobile

import (
	"context"
	"errors"
	"regexp"
	"runtime"
	"strings"
	"time"
)

var iosXcodeVersion = regexp.MustCompile(`(?m)^Xcode ([0-9]+(?:\.[0-9]+){0,2})\r?$`)
var iosSDKVersion = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+){0,2}$`)
var iosIdentityCount = regexp.MustCompile(`(?m)^\s*([0-9]+) valid identities found\s*$`)

// IOSDoctor 只解析工具摘要；没有明确材料时不扫描或选择宿主身份。
func IOSDoctor(ctx context.Context, options IOSDoctorOptions) []DoctorCheck {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if runtime.GOOS != "darwin" {
		return []DoctorCheck{{Name: "ios.platform", Status: "failed", Reason: "ios_platform_unsupported"}}
	}
	checks := []DoctorCheck{{Name: "ios.platform", Status: "passed", Version: "macOS"}}
	unsafe := false
	for _, tool := range []struct {
		name, executable string
		args             []string
		pattern          *regexp.Regexp
	}{
		{"ios.xcode", "/usr/bin/xcodebuild", []string{"-version"}, iosXcodeVersion},
		{"ios.sdk", "/usr/bin/xcrun", []string{"--sdk", "iphoneos", "--show-sdk-version"}, iosSDKVersion},
		{"ios.identities", "/usr/bin/security", []string{"find-identity", "-v", "-p", "codesigning"}, iosIdentityCount},
	} {
		check := DoctorCheck{Name: tool.name, Status: "failed", Reason: "ios_tool_cleanup_failed"}
		if !unsafe {
			output, err := toolOutput(ctx, toolCommand{Workspace: options.Workspace, Executable: tool.executable, Args: tool.args})
			check = iosDoctorToolCheck(tool.name, output, tool.pattern, err)
			unsafe = errors.Is(err, errToolCleanup)
		}
		checks = append(checks, check)
	}
	signing := DoctorCheck{Name: "ios.signing", Status: "skipped", Reason: "ios_signing_material_not_provided"}
	if unsafe {
		signing.Status = "failed"
		signing.Reason = "ios_tool_cleanup_failed"
	} else if !iosNativeAvailable() {
		signing.Status = "failed"
		signing.Reason = "ios_signing_requires_macos15_cgo"
	} else if options.Signing != nil {
		signing.Status = "passed"
		signing.Reason = ""
		selected := *options.Signing
		if selected.Workspace == "" {
			selected.Workspace = options.Workspace
		}
		if err := ValidateIOSSigning(ctx, selected); err != nil {
			signing.Status = "failed"
			signing.Reason = err.Error()
		}
	}
	return append(checks, signing)
}

func iosDoctorToolCheck(name, output string, pattern *regexp.Regexp, err error) DoctorCheck {
	check := DoctorCheck{Name: name, Status: "failed", Reason: "ios_tool_unavailable"}
	if errors.Is(err, errToolCleanup) {
		check.Reason = "ios_tool_cleanup_failed"
		return check
	}
	if err != nil {
		return check
	}
	value := strings.TrimSpace(output)
	match := pattern.FindStringSubmatch(value)
	if name == "ios.sdk" && pattern.MatchString(value) {
		check.Status = "passed"
		check.Version = value
		check.Reason = ""
	} else if len(match) == 2 {
		check.Status = "passed"
		check.Version = match[1]
		check.Reason = ""
	}
	return check
}
