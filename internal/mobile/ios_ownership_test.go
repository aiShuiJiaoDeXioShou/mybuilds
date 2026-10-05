package mobile

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func iosPlannedFixture(t *testing.T) (*IOSResources, IOSSigningOptions) {
	t.Helper()
	if !iosNativeAvailable() {
		t.Skip("资源规划仅支持当前macOS原生组件")
	}
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	options := IOSSigningOptions{Workspace: workspace, OutputDir: "owned-output", P12File: "missing-private.p12", ProfileFile: "missing-private.profile", Password: "OWNERSHIP_PRIVATE_PASSWORD", BundleID: "com.example.app", ExportMethod: "debugging"}
	resources, err := PlanIOSResources(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := resources.Close(context.Background()); err != nil {
			t.Error("自产计划未清理", err)
		}
	})
	return resources, options
}

func TestIOSOwnershipPlanAndRestoreWithoutMaterial(t *testing.T) {
	resources, _ := iosPlannedFixture(t)
	ownership := resources.Ownership()
	data, err := json.Marshal(ownership)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "PRIVATE") || strings.Contains(string(data), "missing-private") || ownership.Version != 1 || ownership.Preparing || ownership.Prepared {
		t.Fatal("规划泄漏秘密或虚称已准备")
	}
	if _, err = os.Stat(ownership.Output); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(ownership.Temporary); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(ownership.Keychain); !os.IsNotExist(err) {
		t.Fatal("规划提前创建了keychain")
	}
	if _, err = os.Stat(ownership.Profile); !os.IsNotExist(err) {
		t.Fatal("规划提前写profile")
	}
	var decoded IOSResourceOwnership
	if json.Unmarshal(data, &decoded) != nil {
		t.Fatal("所有权不可持久化")
	}
	restored, err := RestoreIOSResources(context.Background(), decoded)
	if err != nil {
		t.Fatal(err)
	}
	if err = restored.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{ownership.Output, ownership.Temporary} {
		if _, err = os.Stat(p); !os.IsNotExist(err) {
			t.Fatal("计划恢复未清理自有空目录")
		}
	}
	if !restored.Ownership().Closed {
		t.Fatal("关闭确认未记录")
	}
}

func TestIOSOwnershipPreparingWithoutLeafEvidenceStaysUnknown(t *testing.T) {
	resources, _ := iosPlannedFixture(t)
	ownership := resources.Ownership()
	ownership.Preparing = true
	restored, err := RestoreIOSResources(context.Background(), ownership)
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(restored.Close(context.Background()), ErrIOSCleanup) {
		t.Fatal("准备中断被猜测为已关闭")
	}
	for _, p := range []string{ownership.Output, ownership.Temporary} {
		if _, err = os.Stat(p); err != nil {
			t.Fatal("未知准备证据被删除")
		}
	}
}

func TestIOSOwnershipRejectsReplacementAndUnknownKeychain(t *testing.T) {
	resources, _ := iosPlannedFixture(t)
	ownership := resources.Ownership()
	old := ownership.Output + "-original"
	if os.Rename(ownership.Output, old) != nil || os.Mkdir(ownership.Output, 0700) != nil {
		t.Fatal("替换目录失败")
	}
	if _, err := RestoreIOSResources(context.Background(), ownership); !errors.Is(err, ErrIOSCleanup) {
		t.Fatal("替换目录被重新授权")
	}
	if os.Remove(ownership.Output) != nil || os.Rename(old, ownership.Output) != nil {
		t.Fatal("恢复自产目录失败")
	}
	if os.WriteFile(ownership.Keychain, []byte("unowned"), 0600) != nil {
		t.Fatal("写入自产未知叶失败")
	}
	if _, err := RestoreIOSResources(context.Background(), ownership); !errors.Is(err, ErrIOSCleanup) {
		t.Fatal("只凭父目录接管了未知keychain")
	}
	if data, err := os.ReadFile(ownership.Keychain); err != nil || string(data) != "unowned" {
		t.Fatal("未知叶被删除")
	}
	if os.Remove(ownership.Keychain) != nil {
		t.Fatal("测试叶清理失败")
	}
}
