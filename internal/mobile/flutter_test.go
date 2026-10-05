package mobile

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"mybuilds/internal/process"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
	"mybuilds/internal/config"
)

// 纯数据门不启动 Flutter、shell 或签名工具。
func TestFlutterParameters(t *testing.T) {
	for _, platform := range []string{"android", "ios"} {
		for _, params := range []map[string]string{
			{}, {"version": "0.0.0"}, {"version": "1.2.3", "channel": "internal ; $(touch forbidden)", "flavor": "staging flavor"},
		} {
			if err := ValidateFlutterParameters(platform, params, "42"); err != nil {
				t.Fatal(platform, err)
			}
		}
	}
	if err := ValidateFlutterParameters("ios", map[string]string{"build_number": "1"}, "123456789012345678901234567890"); err != nil {
		t.Fatal("iOS 不应添加未经约定的编号上限", err)
	}
	if err := ValidateFlutterParameters("android", map[string]string{"build_number": "1"}, "2100000000"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ platform, field, value, number string }{
		{"unknown", "version", "1.2.3", "1"},
		{"android", "version", "1.2", "1"},
		{"android", "version", "1.2.3+private-marker", "1"},
		{"android", "version", "-1.2.3", "1"},
		{"android", "version", "1.2.3\n", "1"},
		{"ios", "version", "1.2.3\x00", "1"},
		{"android", "build_number", "0", "1"},
		{"ios", "build_number", "1.2", "1"},
		{"android", "channel", "secret\rmarker", "1"},
		{"ios", "flavor", "secret\x00marker", "1"},
		{"android", "version", "1.2.3", "2100000001"},
		{"android", "version", "1.2.3", "0"},
		{"ios", "version", "1.2.3", "-1"},
	} {
		err := ValidateFlutterParameters(tc.platform, map[string]string{tc.field: tc.value}, tc.number)
		if err == nil {
			t.Fatalf("未拒绝 %s/%s", tc.platform, tc.field)
		}
		if strings.Contains(err.Error(), "private-marker") || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "2100000001") {
			t.Fatal("错误回显输入值")
		}
	}
	// 自定义脚本未声明编号时无需注入；声明本地值不得替换可信远程编号。
	if err := ValidateFlutterParameters("android", nil, ""); err != nil {
		t.Fatal(err)
	}
	if err := ValidateFlutterParameters("android", map[string]string{"build_number": "2100000001"}, "42"); err == nil {
		t.Fatal("未校验已声明字段")
	}
}

func flutterYAML(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var document map[string]any
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	return document
}

func TestFlutterTemplateSelection(t *testing.T) {
	for _, selected := range [][]string{{"android"}, {"ios"}, {"ios", "android"}} {
		data, err := FlutterTemplate(selected)
		if err != nil {
			t.Fatal(err)
		}
		document := flutterYAML(t, data)
		builds := document["builds"].(map[string]any)
		if document["version"] != 1 || len(builds) != len(selected) {
			t.Fatal("模板不是单份所选构建配置")
		}
		for _, platform := range selected {
			build := builds[platform].(map[string]any)
			runner := build["runner"].(map[string]any)
			if runner["platform"] != platform || runner["framework"] != "flutter" {
				t.Fatal("框架声明不匹配")
			}
			params := build["params"].(map[string]any)
			for name, value := range map[string]string{"version": "1.0.0", "channel": "internal", "flavor": "", "build_number": "1"} {
				if params[name] != value {
					t.Fatalf("参数 %s 默认未保持字符串", name)
				}
			}
			env := build["env"].(map[string]any)
			if env["BUILD_NUMBER"] != "{{build.number}}" || env["APP_CHANNEL"] != "{{channel}}" || env["APP_FLAVOR"] != "{{flavor}}" {
				t.Fatal("模板未引用可信编号/参数env")
			}
			for _, raw := range build["steps"].([]any) {
				step := raw.(map[string]any)
				if step["kind"] != "run" && step["kind"] != "artifact" {
					t.Fatal("模板包含额外执行机制")
				}
			}
		}
		if len(selected) == 2 && strings.Index(string(data), "  android:") > strings.Index(string(data), "  ios:") {
			t.Fatal("组合顺序不稳定")
		}
		data[0] = '!'
		next, err := FlutterTemplate(selected)
		if err != nil || next[0] == '!' {
			t.Fatal("返回可变共享模板")
		}
	}
	for _, selected := range [][]string{nil, {}, {"android", "android"}, {"ios", "ios"}, {"secret-marker"}, {"android", "ios", "android"}} {
		data, err := FlutterTemplate(selected)
		if err == nil || len(data) != 0 || strings.Contains(err.Error(), "secret-marker") {
			t.Fatal("非法选择未安全拒绝")
		}
	}
}

func TestFlutterAndroidTemplateParse(t *testing.T) {
	data, err := FlutterTemplate([]string{"android"})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := config.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	build := doc.Builds["android"]
	if len(doc.Builds) != 1 || build.Runner.Framework != "flutter" {
		t.Fatal("框架没有实际解析")
	}
	params, err := config.ResolveParams(build, map[string]string{"application_id": "dev.mybuilds.flutter", "channel": "test ; $(touch forbidden)", "flavor": "staging"})
	if err != nil || params["build_number"] != "1" || params["flavor"] != "staging" {
		t.Fatal("实际参数解析失败", err)
	}
	if _, err := config.ResolveParams(build, nil); err == nil {
		t.Fatal("缺少应用标识未拒绝")
	}
	for _, step := range build.Steps {
		if step.Kind == "artifact" {
			for _, path := range step.Paths {
				if strings.ContainsAny(path, "*?") {
					t.Fatal("默认模板收集混入旧变体的宽glob")
				}
			}
		}
	}
	text := string(data)
	for _, required := range []string{"--no-pub", "--build-name", "--build-number", "apksigner", "aapt2", "jarsigner", "keytool", "DumpCommand", "-storepass:env"} {
		if !strings.Contains(text, required) {
			t.Fatalf("缺少独立包检查 %s", required)
		}
	}
	for _, forbidden := range []string{"eval ", "--split-per-abi", "sdkmanager", "signingConfigs.getByName(\"debug\")"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("不应包含 %s", forbidden)
		}
	}
}

func TestFlutterDoctorDataBoundaries(t *testing.T) {
	for _, tc := range []struct {
		platforms []string
		ctx       context.Context
		want      string
	}{
		{[]string{"private-marker"}, context.Background(), "tool_incompatible"},
		{[]string{"android", "android"}, context.Background(), "tool_incompatible"},
		{nil, context.Background(), "tool_missing"},
	} {
		checks := FlutterDoctor(tc.ctx, FlutterDoctorOptions{Workspace: t.TempDir(), Environment: map[string]string{"PATH": ""}, Platforms: tc.platforms})
		if len(checks) != 2 || checks[0].Reason != tc.want || checks[0].Status != "failed" {
			t.Fatal(checks)
		}
		for _, c := range checks {
			if strings.Contains(c.Reason, "private-marker") {
				t.Fatal("回显未知输入")
			}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	checks := FlutterDoctor(ctx, FlutterDoctorOptions{Workspace: t.TempDir(), Environment: map[string]string{}, Platforms: []string{"android"}})
	if len(checks) != 5 {
		t.Fatal(checks)
	}
	for _, c := range checks {
		if c.Reason != "cancelled" || c.Status != "failed" {
			t.Fatal(checks)
		}
	}
}

func TestFlutterSDKCacheReadBounded(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, "version.json")
	if err := os.WriteFile(filename, []byte(strings.Repeat("x", 64*1024+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := flutterRead(context.Background(), filename); err == nil {
		t.Fatal("缓存读取未受限")
	}
	if _, err := flutterRead(context.Background(), root); err == nil {
		t.Fatal("目录被视为缓存文件")
	}
	if err := os.Remove(filename); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte("3.38.6"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := flutterRead(context.Background(), filename)
	if err != nil || string(got) != "3.38.6" {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = flutterRead(ctx, filename); err == nil {
		t.Fatal("取消后还读取文件")
	}
}

// 正例只用调用者明确指定的真实 SDK；未准备环境不伪造版本输出。
func TestFlutterDoctorActualInitializedSDK(t *testing.T) {
	sdk := os.Getenv("MYBUILDS_FLUTTER_REAL_SDK")
	if sdk == "" {
		t.Skip("需明确真实初始化SDK；编译不替代工具验收")
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("实际工具只支持macOS/Linux")
	}
	sdk, err := filepath.EvalSymlinks(sdk)
	if err != nil {
		t.Fatal(err)
	}
	capture := func() map[string][32]byte {
		t.Helper()
		result := map[string][32]byte{}
		for _, name := range []string{"version", "bin/cache/flutter.version.json", "bin/cache/engine.stamp", "bin/cache/engine_stamp.stamp", "bin/cache/engine_stamp.json", "bin/cache/flutter_tools.stamp", "bin/cache/flutter_tools.snapshot", "bin/cache/dart-sdk/version", "packages/flutter_tools/.dart_tool/package_config.json"} {
			data, e := os.ReadFile(filepath.Join(sdk, name))
			if os.IsNotExist(e) {
				continue
			}
			if e != nil {
				t.Fatal(e)
			}
			result[name] = sha256.Sum256(data)
		}
		return result
	}
	before := capture()
	home := t.TempDir()
	marker := filepath.Join(home, "preference-marker")
	if err := os.WriteFile(marker, []byte("user preferences must stay intact"), 0600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"PATH": filepath.Join(sdk, "bin") + string(os.PathListSeparator) + "/usr/bin:/bin", "HOME": home, "FLUTTER_ROOT": sdk, "PRIVATE_SECRET_MARKER": "must-not-be-inherited"}
	checks := FlutterDoctor(context.Background(), FlutterDoctorOptions{Workspace: t.TempDir(), Environment: env})
	if len(checks) != 2 {
		t.Fatal(checks)
	}
	for _, c := range checks {
		if c.Status != "passed" || c.Version == "" || c.Reason != "" {
			t.Fatal(checks)
		}
	}
	if !reflect.DeepEqual(before, capture()) {
		t.Fatal("Doctor修改SDK缓存")
	}
	entries, err := os.ReadDir(home)
	if err != nil || len(entries) != 1 || entries[0].Name() != "preference-marker" {
		t.Fatal("Doctor修改声明HOME偏好目录")
	}
	if env["HOME"] != home || env["PRIVATE_SECRET_MARKER"] != "must-not-be-inherited" {
		t.Fatal("修改输入环境")
	}
	public, err := json.Marshal(checks)
	if err != nil || strings.Contains(string(public), sdk) || strings.Contains(string(public), "must-not-be-inherited") {
		t.Fatal("公共诊断泄露内部事实")
	}
	// 明确环境不使用宿主 SDK/PATH 填补缺失。
	bad := FlutterDoctor(context.Background(), FlutterDoctorOptions{Workspace: t.TempDir(), Environment: map[string]string{"PATH": ""}})
	if bad[0].Status != "failed" || bad[0].Reason != "tool_missing" {
		t.Fatal(bad)
	}
}

func TestFlutterIOSTemplateExplicitLifecycleText(t *testing.T) {
	data, err := FlutterTemplate([]string{"ios"})
	if err != nil {
		t.Fatal(err)
	}
	doc := flutterYAML(t, data)
	build := doc["builds"].(map[string]any)["ios"].(map[string]any)
	params := build["params"].(map[string]any)
	for _, name := range []string{"xcode_project", "scheme", "bundle_id"} {
		if params[name].(map[string]any)["required"] != true {
			t.Fatal("缺必填身份", name)
		}
	}
	signing := build["ios_signing"].(map[string]any)
	for _, name := range []string{"p12", "profile", "password"} {
		value := signing[name].(string)
		if !strings.HasPrefix(value, "${") || !strings.HasSuffix(value, "}") {
			t.Fatal("签名未使用完整env引用")
		}
	}
	text := string(data)
	for _, required := range []string{"--config-only", "--no-codesign", "--no-pub", "CODE_SIGN_STYLE=Manual", "--keychain", "-archivePath", "-exportArchive", "MYBUILDS_IOS_EXPORT_OPTIONS", "codesign --verify", "dwarfdump --uuid", "embedded.mobileprovision", "App.xcarchive.zip", "App.dSYM.zip"} {
		if !strings.Contains(text, required) {
			t.Fatal("缺少显式生命周期动作", required)
		}
	}
	for _, forbidden := range []string{"-allowProvisioningUpdates", "security import", "create-keychain", "list-keychains", "eval "} {
		if strings.Contains(text, forbidden) {
			t.Fatal("模板越过005系统资源边界", forbidden)
		}
	}
}

func TestFlutterTemplateRealShellSyntax(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("模板为明确Unix shell")
	}
	data, err := FlutterTemplate([]string{"android", "ios"})
	if err != nil {
		t.Fatal(err)
	}
	document := flutterYAML(t, data)
	for _, raw := range document["builds"].(map[string]any) {
		build := raw.(map[string]any)
		for _, rawStep := range build["steps"].([]any) {
			step := rawStep.(map[string]any)
			if step["kind"] != "run" {
				continue
			}
			script := step["run"].(string)
			result := process.Run(context.Background(), process.Command{Path: "/bin/bash", Args: []string{"-n", "-c", script}, Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"}}, io.Discard, io.Discard)
			if !result.Started || result.ExitCode != 0 || result.Reason != "" || result.CleanupFailed {
				t.Fatal("可编辑模板实际bash语法错误", step["name"], result)
			}
		}
	}
}

func TestFlutterCleanupEvidencePrecedesCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := flutterReason(ctx, errors.New("tool_cleanup_error")); got != "cleanup_error" {
		t.Fatal("取消掩盖未知停止证据", got)
	}
}
