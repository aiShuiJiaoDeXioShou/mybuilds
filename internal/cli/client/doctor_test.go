package client

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mybuilds/internal/config"
)

func TestNativeAndroidInitAndOptionBoundaries(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("ANDROID_KEYSTORE_PASSWORD", "DO_NOT_READ_IN_INIT")
	if _, err := execute(t, "init", "--framework", "native", "--platform", "android"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("mybuilds.yml")
	if err != nil {
		t.Fatal(err)
	}
	document, err := config.Parse(data)
	if err != nil || len(document.Builds) != 1 || document.Builds["android"] == nil {
		t.Fatalf("平台配置无效: %v", err)
	}
	if strings.Contains(string(data), "DO_NOT_READ_IN_INIT") {
		t.Fatal("初始化读取了签名密码")
	}
	if _, err := execute(t, "init", "--platform", "android"); err == nil {
		t.Fatal("已有配置被覆盖")
	}
	after, _ := os.ReadFile("mybuilds.yml")
	if string(after) != string(data) {
		t.Fatal("已有配置发生变化")
	}
	if _, err := execute(t, "run", "--build", "android", "--dry-run"); err != nil {
		t.Fatalf("初始化配置不能预览: %v", err)
	}
	if err := os.Remove("mybuilds.yml"); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "--framework", "native"},
		{"init", "--framework", "flutter", "--platform", "android"},
		{"init", "--platform", "unknown"},
		{"init", "--template", "absent.yml", "--platform", "android"},
	} {
		if _, err := execute(t, args...); err == nil {
			t.Fatalf("非法选项未拒绝: %v", args)
		}
		if _, err := os.Stat("mybuilds.yml"); !os.IsNotExist(err) {
			t.Fatal("非法选项创建了配置")
		}
	}
}

func TestAndroidDoctorJSONFailureAndOptionValidation(t *testing.T) {
	workspace := t.TempDir()
	t.Chdir(workspace)
	t.Setenv("DOCTOR_TEST_PASSWORD", "DO_NOT_LEAK_DOCTOR_PASSWORD")
	out, diagnostic, err := localExecute(t, context.Background(), "doctor", "--platform", "android", "--json", "--gradle-wrapper", "absent-wrapper", "--keystore", "absent-secret-file", "--store-password-env", "DOCTOR_TEST_PASSWORD")
	if err == nil || !json.Valid([]byte(out)) {
		t.Fatalf("失败检查没有非零JSON结果: %q %v", out, err)
	}
	var checks []struct {
		Name, Status, Version, Reason string
	}
	if err := json.Unmarshal([]byte(out), &checks); err != nil {
		t.Fatal(err)
	}
	failed := false
	for _, check := range checks {
		if check.Status != "passed" && check.Status != "failed" && check.Status != "skipped" {
			t.Fatalf("检查状态无效: %+v", check)
		}
		failed = failed || check.Status == "failed"
	}
	if !failed || strings.Contains(out+diagnostic+err.Error(), "DO_NOT_LEAK_DOCTOR_PASSWORD") || strings.Contains(out, "absent-secret-file") {
		t.Fatal("诊断缺失或泄露输入")
	}
	for _, args := range [][]string{
		{"doctor", "--platform", "unknown", "--json"},
		{"doctor", "--server"},
		{"doctor", "--node", "unknown"},
		{"doctor", "unexpected"},
	} {
		if _, err := execute(t, args...); err == nil {
			t.Fatalf("未交付/非法选项未拒绝: %v", args)
		}
	}
	entries, err := os.ReadDir(workspace)
	if err != nil || len(entries) != 0 {
		t.Fatalf("检查修改了工作树: %v %v", entries, err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "absent-secret-file")); !os.IsNotExist(err) {
		t.Fatal("签名检查创建了声明资源")
	}
}
