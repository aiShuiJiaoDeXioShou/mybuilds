package mobile

import (
	"context"
	"crypto/x509"
	_ "embed"
	"encoding/pem"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// AndroidDoctorOptions 只接受明确的工程和签名声明，不搜寻未知用户密钥。
type AndroidDoctorOptions struct {
	Workspace, GradleWrapper                             string
	Keystore, KeyAlias, StorePasswordEnv, KeyPasswordEnv string
}

//go:embed templates/android.yml
var androidTemplate string

// AndroidTemplate 返回可编辑副本，不运行工具或读取凭据。
func AndroidTemplate() []byte {
	return []byte(androidTemplate)
}

var javaVersion = regexp.MustCompile(`(?m)^(?:openjdk|java) version "([0-9]+(?:\.[0-9]+)*(?:_[0-9]+)?)"`)
var gradleVersion = regexp.MustCompile(`(?m)^Gradle ([0-9]+(?:\.[0-9]+)+)\r?$`)
var androidPlatform = regexp.MustCompile(`^android-([1-9][0-9]*)$`)
var androidBuildTools = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
var androidEnvName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var privateKeyEntry = regexp.MustCompile(`(?m)^Entry type: PrivateKeyEntry\r?$`)

// AndroidDoctor 只返回固定诊断及已验证版本，工具原文始终留在内部。
func AndroidDoctor(parent context.Context, options AndroidDoctorOptions) []DoctorCheck {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	workspace := options.Workspace
	if workspace == "" {
		workspace, _ = os.Getwd()
	}
	checks := make([]DoctorCheck, 0, 4)
	unsafe := false
	for _, name := range []string{"android_java", "android_sdk", "android_gradle", "android_signing"} {
		check := DoctorCheck{Name: name, Status: "passed"}
		if unsafe {
			check.Status, check.Reason = "failed", "cleanup_error"
		} else if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
			check.Status, check.Reason = "failed", "unsupported_platform"
		} else if ctx.Err() != nil {
			check.Status, check.Reason = "failed", androidToolReason(ctx, ctx.Err(), "tool_error")
		} else {
			switch name {
			case "android_java":
				check.Version, check.Reason = androidJava(ctx, workspace)
			case "android_sdk":
				check.Version, check.Reason = androidSDK()
			case "android_gradle":
				check.Version, check.Reason = androidGradle(ctx, workspace, options.GradleWrapper)
			case "android_signing":
				check.Reason = androidSigning(ctx, workspace, options)
			}
			if check.Reason != "" {
				check.Status = "failed"
			}
		}
		if check.Reason == "cleanup_error" {
			unsafe = true
		}
		if !unsafe && name == "android_signing" && options.Keystore == "" && options.KeyAlias == "" && options.StorePasswordEnv == "" && options.KeyPasswordEnv == "" {
			check.Status, check.Reason = "skipped", "not_declared"
		}
		checks = append(checks, check)
	}
	return checks
}

func androidJDKTool(name string) string {
	if home := os.Getenv("JAVA_HOME"); home != "" {
		return filepath.Join(home, "bin", name)
	}
	tool, _ := exec.LookPath(name)
	return tool
}

func androidJava(ctx context.Context, workspace string) (string, string) {
	executable := androidJDKTool("java")
	if !androidRegular(executable, true) {
		return "", "java_missing"
	}
	output, err := toolOutput(ctx, toolCommand{Workspace: workspace, Executable: executable, Args: []string{"-version"}})
	if err != nil {
		return "", androidToolReason(ctx, err, "java_error")
	}
	match := javaVersion.FindStringSubmatch(output)
	if len(match) != 2 {
		return "", "java_version_invalid"
	}
	major, _ := strconv.Atoi(strings.Split(match[1], ".")[0])
	if major < 17 {
		return match[1], "java_incompatible"
	}
	return match[1], ""
}

func androidSDK() (string, string) {
	home, root := os.Getenv("ANDROID_HOME"), os.Getenv("ANDROID_SDK_ROOT")
	if home == "" {
		home = root
	}
	if home == "" {
		return "", "sdk_missing"
	}
	if root != "" {
		homeInfo, homeErr := os.Stat(home)
		rootInfo, rootErr := os.Stat(root)
		if homeErr != nil || rootErr != nil {
			return "", "sdk_missing"
		}
		if !os.SameFile(homeInfo, rootInfo) {
			return "", "sdk_conflict"
		}
	}
	platforms, err := os.ReadDir(filepath.Join(home, "platforms"))
	if err != nil {
		return "", "sdk_packages_missing"
	}
	var apis, versions []string
	for _, entry := range platforms {
		match := androidPlatform.FindStringSubmatch(entry.Name())
		if len(match) == 2 && androidRegular(filepath.Join(home, "platforms", entry.Name(), "android.jar"), false) {
			apis = append(apis, match[1])
		}
	}
	tools, err := os.ReadDir(filepath.Join(home, "build-tools"))
	if err != nil {
		return "", "sdk_packages_missing"
	}
	for _, entry := range tools {
		if androidBuildTools.MatchString(entry.Name()) && androidRegular(filepath.Join(home, "build-tools", entry.Name(), "aapt2"), true) && androidRegular(filepath.Join(home, "build-tools", entry.Name(), "apksigner"), true) {
			versions = append(versions, entry.Name())
		}
	}
	if len(apis) == 0 || len(versions) == 0 {
		return "", "sdk_packages_missing"
	}
	sort.Strings(apis)
	sort.Strings(versions)
	return "platforms=" + strings.Join(apis, ",") + ";build_tools=" + strings.Join(versions, ","), ""
}

func androidRegular(name string, executable bool) bool {
	info, err := os.Stat(name)
	return err == nil && info.Mode().IsRegular() && (!executable || info.Mode().Perm()&0111 != 0)
}

func androidGradle(ctx context.Context, workspace, wrapper string) (string, string) {
	if wrapper == "" {
		wrapper = "gradlew"
	}
	if filepath.IsAbs(wrapper) || strings.ContainsAny(wrapper, "\\:\x00\r\n") {
		return "", "wrapper_invalid"
	}
	for _, part := range strings.Split(filepath.ToSlash(wrapper), "/") {
		if part == ".." {
			return "", "wrapper_invalid"
		}
	}
	root, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return "", "workspace_invalid"
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", "workspace_invalid"
	}
	executable, err := filepath.EvalSymlinks(filepath.Join(root, wrapper))
	if err != nil {
		return "", "wrapper_missing"
	}
	relative, err := filepath.Rel(root, executable)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "wrapper_invalid"
	}
	if !androidRegular(executable, true) {
		return "", "wrapper_invalid"
	}
	dir := filepath.Dir(executable)
	for _, name := range []string{"gradle-wrapper.jar", "gradle-wrapper.properties"} {
		if !androidRegular(filepath.Join(dir, "gradle", "wrapper", name), false) {
			return "", "wrapper_incomplete"
		}
	}
	output, err := toolOutput(ctx, toolCommand{Workspace: dir, Executable: executable, Args: []string{"--version", "--no-daemon"}})
	if err != nil {
		return "", androidToolReason(ctx, err, "wrapper_error")
	}
	match := gradleVersion.FindStringSubmatch(output)
	if len(match) != 2 {
		return "", "wrapper_version_invalid"
	}
	return match[1], ""
}

func androidSigning(ctx context.Context, workspace string, options AndroidDoctorOptions) string {
	if options.Keystore == "" || options.KeyAlias == "" || options.StorePasswordEnv == "" || options.KeyPasswordEnv == "" {
		return "signing_incomplete"
	}
	if !androidEnvName.MatchString(options.StorePasswordEnv) || !androidEnvName.MatchString(options.KeyPasswordEnv) {
		return "signing_env_invalid"
	}
	if os.Getenv(options.StorePasswordEnv) == "" || os.Getenv(options.KeyPasswordEnv) == "" {
		return "signing_env_missing"
	}
	keystore := options.Keystore
	if !filepath.IsAbs(keystore) {
		keystore = filepath.Join(workspace, keystore)
	}
	if !androidRegular(keystore, false) {
		return "keystore_missing"
	}
	executable := androidJDKTool("keytool")
	if !androidRegular(executable, true) {
		return "keytool_missing"
	}
	args := []string{"-J-Duser.language=en", "-J-Duser.country=US", "-list", "-v", "-keystore", keystore, "-alias", options.KeyAlias, "-storepass:env", options.StorePasswordEnv}
	command := toolCommand{Workspace: workspace, Executable: executable, Args: args, ExtraEnvNames: []string{options.StorePasswordEnv, options.KeyPasswordEnv}}
	output, err := toolOutput(ctx, command)
	if err != nil {
		return androidToolReason(ctx, err, "signing_store_or_alias_invalid")
	}
	if !privateKeyEntry.MatchString(output) {
		return "signing_not_private_key"
	}
	command.Args = []string{"-J-Duser.language=en", "-J-Duser.country=US", "-certreq", "-keystore", keystore, "-alias", options.KeyAlias, "-storepass:env", options.StorePasswordEnv, "-keypass:env", options.KeyPasswordEnv}
	output, err = toolOutput(ctx, command)
	if err != nil {
		return androidToolReason(ctx, err, "signing_key_invalid")
	}
	block, _ := pem.Decode([]byte(output))
	if block == nil {
		return "signing_key_invalid"
	}
	request, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil || request.CheckSignature() != nil {
		return "signing_key_invalid"
	}
	return ""
}

func androidToolReason(ctx context.Context, err error, fallback string) string {
	// 公共 helper 将工具自身预算转换成固定错误，调用方 ctx 可能尚未截止。
	if err != nil && err.Error() == "tool_timeout" {
		return "timeout"
	}
	if err != nil && err.Error() == "tool_cancelled" {
		return "cancelled"
	}
	if errors.Is(err, errToolOutputLimit) {
		return "output_limit"
	}
	if err != nil && err.Error() == "tool_cleanup_error" {
		return "cleanup_error"
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return "cancelled"
	}
	return fallback
}
