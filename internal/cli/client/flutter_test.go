package client

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"mybuilds/internal/config"
)

func TestFlutterInitAndScopedDryRun(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("ANDROID_KEYSTORE_PASSWORD", "UNREAD_FLUTTER_SECRET")
	if _, err := execute(t, "init", "--framework", "flutter", "--platform", "android"); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile("mybuilds.yml")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := config.Parse(original)
	if err != nil || parsed.Builds["android"].Runner.Framework != "flutter" {
		t.Fatal(err)
	}
	out, err := execute(t, "run", "--dry-run", "--param", "android:application_id=com.example.application", "--param", "android:version=2.3.4")
	if err != nil || strings.Contains(out, "UNREAD_FLUTTER_SECRET") {
		t.Fatalf("预览: %v", err)
	}
	if _, err = execute(t, "init", "--framework", "flutter", "--platform", "android"); err == nil {
		t.Fatal("覆盖已有配置")
	}
	after, _ := os.ReadFile("mybuilds.yml")
	if string(after) != string(original) {
		t.Fatal("文件被修改")
	}
	for _, args := range [][]string{{"run", "--dry-run", "--param", "other:version=1.2.3"}, {"run", "--dry-run", "--param", "version=1.2.3", "--param", "version=2.3.4"}} {
		if _, err := execute(t, args...); err == nil {
			t.Fatal("非法scope/重复参数被接受")
		}
	}
	if err := os.Remove("mybuilds.yml"); err != nil {
		t.Fatal(err)
	}
	for _, platform := range []string{"", "android,android", "android,unknown", ",android"} {
		if _, err := execute(t, "init", "--framework", "flutter", "--platform", platform); err == nil {
			t.Fatal("非法平台组合被接受")
		}
		if _, err := os.Stat("mybuilds.yml"); !os.IsNotExist(err) {
			t.Fatal("非法输入创建了文件")
		}
	}
}

func TestFlutterDoctorCLIInputAndSafeFailure(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("PATH", "")
	out, diagnostic, err := localExecute(t, context.Background(), "doctor", "--framework", "flutter", "--platform", "android", "--json")
	if err == nil || !json.Valid([]byte(out)) {
		t.Fatalf("未输出真实失败诊断: %v", err)
	}
	if !strings.Contains(out, "flutter.sdk") || strings.Contains(out+diagnostic, "exec:") {
		t.Fatal("缺少固定安全诊断")
	}
	for _, args := range [][]string{{"doctor", "--server", "--framework", "flutter"}, {"doctor", "--framework", "flutter", "--platform", "android,android"}, {"doctor", "--framework", "flutter", "--platform", "ios", "--keystore", "absent"}, {"doctor", "--framework", "unknown"}} {
		if _, err := execute(t, args...); err == nil {
			t.Fatal("跨平台/框架输入未拒绝")
		}
	}
}

func TestFlutterIOSDoctorNoMaterialDoesNotReadSecrets(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("PATH", "")
	t.Setenv("IOS_P12_PASSWORD", "UNREAD_IOS_SECRET")
	out, stderr, err := localExecute(t, context.Background(), "doctor", "--framework", "flutter", "--platform", "ios", "--json")
	if err == nil {
		t.Fatal("缺SDK不应通过")
	}
	var checks []struct{ Name, Status, Reason string }
	if json.Unmarshal([]byte(out), &checks) != nil {
		t.Fatal("缺安全JSON")
	}
	found := false
	for _, check := range checks {
		if check.Name == "ios.signing" {
			found = check.Status == "skipped"
		}
	}
	if !found || strings.Contains(out+stderr, "UNREAD_IOS_SECRET") {
		t.Fatal("未声明签名没有skipped或读取材料")
	}
}
