package mobile

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestIOSDoctorCancelledSafe(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	checks := IOSDoctor(ctx, IOSDoctorOptions{Workspace: t.TempDir(), Signing: &IOSSigningOptions{Password: "ios-sensitive-marker"}})
	output, err := json.Marshal(checks)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(output), "ios-sensitive-marker") {
		t.Fatal("doctor泄漏材料")
	}
	for _, check := range checks {
		if check.Status != "passed" && check.Status != "failed" && check.Status != "skipped" {
			t.Fatal("doctor状态不符合公共契约")
		}
	}
}

func TestIOSDoctorCleanupReasonCannotBeSuccess(t *testing.T) {
	check := iosDoctorToolCheck("ios.xcode", "Xcode 27.0", iosXcodeVersion, errors.Join(errToolCleanup, errors.New("tool_timeout")))
	if check.Status != "failed" || check.Reason != "ios_tool_cleanup_failed" || check.Version != "" {
		t.Fatal("停止未确认被工具输出覆盖")
	}
}
