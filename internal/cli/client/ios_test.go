package client

import (
	"mybuilds/internal/config"
	"os"
	"strings"
	"testing"
)

func TestNativeIOSInitPreviewAndFlags(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("IOS_P12_PASSWORD", "sensitive_ios_cli")
	if _, err := execute(t, "init", "--framework", "native", "--platform", "ios"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("mybuilds.yml")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := config.Parse(data)
	if err != nil || doc.Builds["ios"] == nil || doc.Builds["ios"].IOSSigning == nil || strings.Contains(string(data), "sensitive_ios_cli") {
		t.Fatal("模板定义或秘密边界错误", err)
	}
	if _, err := execute(t, "run", "--build", "ios", "--dry-run", "--param", "xcode_project=Example.xcodeproj", "--param", "scheme=Example", "--param", "bundle_id=com.example.app", "--param", "version=1.2.3", "--param", "build_number=4"); err != nil {
		t.Fatal("无秘密预览失败", err)
	}
	if _, err := execute(t, "init", "--platform", "ios"); err == nil {
		t.Fatal("覆盖已有配置")
	}
	for _, args := range [][]string{{"doctor", "--platform", "ios", "--p12", "private-path"}, {"doctor", "--platform", "android", "--password-env", "INVALID"}, {"doctor", "--server", "--p12", "private-path"}, {"doctor", "--platform", "ios", "--keystore", "private-path"}} {
		_, err := execute(t, args...)
		if err == nil || strings.Contains(err.Error(), "private-path") {
			t.Fatal("非法组合未安全拒绝", args, err)
		}
	}
	after, _ := os.ReadFile("mybuilds.yml")
	if string(after) != string(data) {
		t.Fatal("只读命令改变配置")
	}
}
