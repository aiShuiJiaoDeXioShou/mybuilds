package mobile

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"mybuilds/internal/process"

	"mybuilds/internal/config"
)

func TestAndroidTemplateConfiguration(t *testing.T) {
	data := AndroidTemplate()
	document, err := config.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	build := document.Builds["android"]
	if len(document.Builds) != 1 || build == nil || build.Runner.Platform != "android" {
		t.Fatal("模板未声明单个 Android build")
	}
	params, err := config.ResolveParams(build, map[string]string{"version": "1.2.3", "build_number": "42"})
	if err != nil || params["version"] != "1.2.3" || params["build_number"] != "42" {
		t.Fatal(params, err)
	}
	if build.Env["APP_VERSION"] != "{{version}}" || build.Env["BUILD_NUMBER"] != "{{build_number}}" {
		t.Fatal("参数未显式映射环境")
	}
	if len(build.Steps) != 2 || build.Steps[0].Kind != "run" || build.Steps[1].Kind != "artifact" || len(build.Steps[1].Paths) != 3 {
		t.Fatal("模板未只构建/收集三类产物")
	}
	for _, forbidden := range []string{"-Pversion", "MYBUILDS_BUILD_NUMBER", "--stop", "sdkmanager", " clean"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("模板包含禁止的隐式操作: %s", forbidden)
		}
	}
	data[0] = '!'
	if AndroidTemplate()[0] == '!' {
		t.Fatal("模板返回共享可变字节")
	}
}

func writeAndroidFile(t *testing.T, filename, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func androidDoctorFixture(t *testing.T) AndroidDoctorOptions {
	t.Helper()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("实际本机工具执行仅支持 macOS/Linux")
	}
	workspace := t.TempDir()
	javaHome := filepath.Join(workspace, "jdk")
	writeAndroidFile(t, filepath.Join(javaHome, "bin", "java"), "#!/bin/sh\nprintf 'openjdk version \"21.0.2\"\\n' >&2\n", 0700)
	sdk := filepath.Join(workspace, "sdk")
	writeAndroidFile(t, filepath.Join(sdk, "platforms", "android-35", "android.jar"), "fixture", 0600)
	for _, tool := range []string{"aapt2", "apksigner"} {
		writeAndroidFile(t, filepath.Join(sdk, "build-tools", "35.0.0", tool), "#!/bin/sh\nexit 0\n", 0700)
	}
	writeAndroidFile(t, filepath.Join(workspace, "gradlew"), "#!/bin/sh\nprintf 'Gradle 8.13\\n'\n", 0700)
	writeAndroidFile(t, filepath.Join(workspace, "gradle", "wrapper", "gradle-wrapper.jar"), "fixture", 0600)
	writeAndroidFile(t, filepath.Join(workspace, "gradle", "wrapper", "gradle-wrapper.properties"), "distributionUrl=https\\://services.gradle.org/distributions/gradle-8.13-bin.zip\n", 0600)
	t.Setenv("JAVA_HOME", javaHome)
	t.Setenv("ANDROID_HOME", sdk)
	t.Setenv("ANDROID_SDK_ROOT", sdk)
	return AndroidDoctorOptions{Workspace: workspace}
}

func checkAndroid(t *testing.T, checks []DoctorCheck, name, status, reason string) {
	t.Helper()
	for _, check := range checks {
		if check.Name == name {
			if check.Status != status || (reason != "" && check.Reason != reason) {
				t.Fatalf("错误检查结果: %+v", check)
			}
			return
		}
	}
	t.Fatalf("缺少检查项 %s: %+v", name, checks)
}

func TestAndroidDoctorTools(t *testing.T) {
	options := androidDoctorFixture(t)
	checks := AndroidDoctor(context.Background(), options)
	for _, name := range []string{"android_java", "android_sdk", "android_gradle"} {
		checkAndroid(t, checks, name, "passed", "")
	}
	checkAndroid(t, checks, "android_signing", "skipped", "not_declared")
	for _, check := range checks {
		if check.Name == "android_java" && check.Version != "21.0.2" {
			t.Fatal(check)
		}
		if check.Name == "android_gradle" && check.Version != "8.13" {
			t.Fatal(check)
		}
	}
}

func TestAndroidDoctorSDKSameDirectoryAlias(t *testing.T) {
	options := androidDoctorFixture(t)
	alias := filepath.Join(options.Workspace, "sdk-link")
	if err := os.Symlink(os.Getenv("ANDROID_HOME"), alias); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANDROID_SDK_ROOT", alias)
	checkAndroid(t, AndroidDoctor(context.Background(), options), "android_sdk", "passed", "")
}

func TestAndroidToolCleanupDiagnostic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if reason := androidToolReason(ctx, errors.New("tool_cleanup_error"), "tool_error"); reason != "cleanup_error" {
		t.Fatal("清理未确认必须保留", reason)
	}
}

func TestAndroidDoctorFailures(t *testing.T) {
	for _, name := range []string{"java_missing", "java_old", "sdk_missing", "sdk_conflict", "sdk_packages", "wrapper_missing", "wrapper_escape", "wrapper_error", "signing_partial"} {
		t.Run(name, func(t *testing.T) {
			options := androidDoctorFixture(t)
			checkName := "android_java"
			switch name {
			case "java_missing":
				t.Setenv("JAVA_HOME", filepath.Join(options.Workspace, "absent"))
			case "java_old":
				writeAndroidFile(t, filepath.Join(os.Getenv("JAVA_HOME"), "bin", "java"), "#!/bin/sh\nprintf 'openjdk version \"11.0.2\"\\n'\n", 0700)
			case "sdk_missing":
				checkName = "android_sdk"
				t.Setenv("ANDROID_HOME", "")
				t.Setenv("ANDROID_SDK_ROOT", "")
			case "sdk_conflict":
				checkName = "android_sdk"
				t.Setenv("ANDROID_SDK_ROOT", filepath.Join(options.Workspace, "other"))
			case "sdk_packages":
				checkName = "android_sdk"
				if err := os.Remove(filepath.Join(os.Getenv("ANDROID_HOME"), "platforms", "android-35", "android.jar")); err != nil {
					t.Fatal(err)
				}
			case "wrapper_missing":
				checkName = "android_gradle"
				options.GradleWrapper = "absent"
			case "wrapper_escape":
				checkName = "android_gradle"
				options.GradleWrapper = "../SECRET_wrapper"
			case "wrapper_error":
				checkName = "android_gradle"
				writeAndroidFile(t, filepath.Join(options.Workspace, "gradlew"), "#!/bin/sh\necho SECRET_TOOL_OUTPUT >&2\nexit 9\n", 0700)
			case "signing_partial":
				checkName = "android_signing"
				options.Keystore = "SECRET_keystore"
			}
			checks := AndroidDoctor(context.Background(), options)
			checkAndroid(t, checks, checkName, "failed", "")
			data, err := json.Marshal(checks)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "SECRET") {
				t.Fatal("诊断回显原始输入/输出")
			}
		})
	}
}

func TestAndroidDoctorCancelledBeforeStart(t *testing.T) {
	options := androidDoctorFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	checkAndroid(t, AndroidDoctor(ctx, options), "android_java", "failed", "cancelled")
}

func TestAndroidTemplateRejectsInvalidBuildNumber(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("本机执行仅支持 macOS/Linux")
	}
	document, err := config.Parse(AndroidTemplate())
	if err != nil {
		t.Fatal(err)
	}
	script := document.Builds["android"].Steps[0].Run
	for _, number := range []string{"0", "2100000001", "999999999999999999999999", "1; touch injected", "$(touch injected)", "-1", ""} {
		t.Run(number, func(t *testing.T) {
			workspace := t.TempDir()
			writeAndroidFile(t, filepath.Join(workspace, "gradlew"), "#!/bin/sh\ntouch wrapper_started\n", 0700)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			result := process.Run(ctx, process.Command{Path: "/bin/sh", Args: []string{"-c", script}, Dir: workspace, Env: []string{"PATH=/usr/bin:/bin", "APP_VERSION=1.2.3", "BUILD_NUMBER=" + number}}, nil, nil)
			if result.ExitCode != 2 || result.Reason != "exit" {
				t.Fatal(result)
			}
			for _, marker := range []string{"wrapper_started", "injected"} {
				if _, err := os.Stat(filepath.Join(workspace, marker)); !os.IsNotExist(err) {
					t.Fatal("无效参数执行了命令")
				}
			}
		})
	}
}

func TestAndroidDoctorRealSigning(t *testing.T) {
	javaHome := os.Getenv("JAVA_HOME")
	options := androidDoctorFixture(t)
	t.Setenv("JAVA_HOME", javaHome)
	keytool := androidJDKTool("keytool")
	if !androidRegular(keytool, true) {
		t.Fatal("真实签名验证缺少 keytool")
	}
	t.Setenv("ANDROID_TEST_STORE_PASS", "store_test_7348")
	t.Setenv("ANDROID_TEST_KEY_PASS", "key_test_9732")
	options.Keystore = filepath.Join(options.Workspace, "test.jks")
	options.KeyAlias = "mybuilds_test"
	options.StorePasswordEnv = "ANDROID_TEST_STORE_PASS"
	options.KeyPasswordEnv = "ANDROID_TEST_KEY_PASS"
	command := toolCommand{Workspace: options.Workspace, Executable: keytool, ExtraEnvNames: []string{options.StorePasswordEnv, options.KeyPasswordEnv}, Args: []string{"-genkeypair", "-storetype", "JKS", "-keystore", options.Keystore, "-alias", options.KeyAlias, "-keyalg", "RSA", "-keysize", "2048", "-dname", "CN=mybuilds Android test", "-validity", "1", "-storepass:env", options.StorePasswordEnv, "-keypass:env", options.KeyPasswordEnv}}
	if _, err := toolOutput(context.Background(), command); err != nil {
		t.Fatal("真实测试密钥生成失败", err)
	}
	certificate := filepath.Join(options.Workspace, "test.cer")
	command.Args = []string{"-exportcert", "-rfc", "-keystore", options.Keystore, "-alias", options.KeyAlias, "-file", certificate, "-storepass:env", options.StorePasswordEnv}
	if _, err := toolOutput(context.Background(), command); err != nil {
		t.Fatal("测试证书导出失败", err)
	}
	command.Args = []string{"-importcert", "-noprompt", "-keystore", options.Keystore, "-alias", "certificate_only", "-file", certificate, "-storepass:env", options.StorePasswordEnv}
	if _, err := toolOutput(context.Background(), command); err != nil {
		t.Fatal("测试证书条目生成失败", err)
	}
	before, err := os.ReadFile(options.Keystore)
	if err != nil {
		t.Fatal(err)
	}
	checkAndroid(t, AndroidDoctor(context.Background(), options), "android_signing", "passed", "")
	after, err := os.ReadFile(options.Keystore)
	if err != nil || string(before) != string(after) {
		t.Fatal("doctor 修改了 keystore")
	}
	for _, name := range []string{"store", "key", "alias", "certificate", "missing_env", "invalid_env"} {
		t.Run(name, func(t *testing.T) {
			copy := options
			switch name {
			case "store":
				t.Setenv(options.StorePasswordEnv, "SECRET_bad_store")
			case "key":
				t.Setenv(options.KeyPasswordEnv, "SECRET_bad_key")
			case "alias":
				copy.KeyAlias = "SECRET_missing_alias"
			case "certificate":
				copy.KeyAlias = "certificate_only"
			case "missing_env":
				copy.StorePasswordEnv = "ANDROID_TEST_ABSENT_PASSWORD"
			case "invalid_env":
				copy.StorePasswordEnv = "SECRET;env"
			}
			checks := AndroidDoctor(context.Background(), copy)
			checkAndroid(t, checks, "android_signing", "failed", "")
			data, _ := json.Marshal(checks)
			if strings.Contains(string(data), "SECRET") || strings.Contains(string(data), "store_test_7348") || strings.Contains(string(data), "key_test_9732") {
				t.Fatal("签名诊断泄漏")
			}
		})
	}
}
