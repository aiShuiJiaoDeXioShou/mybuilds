package mobile

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestIOSHelperRejectsMalformedInputWithoutEcho(t *testing.T) {
	for _, input := range []string{`{"unknown":"sensitive-mark"}`, `{"action":"inspect","password":"sensitive-mark"} {}`, strings.Repeat("sensitive-mark", 6000)} {
		var out strings.Builder
		err := HandleIOSHelper(strings.NewReader(input), &out)
		if err == nil {
			t.Fatal("非法helper输入未拒绝")
		}
		if strings.Contains(out.String(), "sensitive-mark") || strings.Contains(err.Error(), "sensitive-mark") {
			t.Fatal("helper错误泄漏输入")
		}
		var response map[string]any
		if json.Unmarshal([]byte(out.String()), &response) != nil {
			t.Fatal("安全响应不是JSON")
		}
	}
}

func TestIOSValidationNoResourceSideEffects(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := ValidateIOSSigning(ctx, IOSSigningOptions{Workspace: t.TempDir(), Password: "sensitive-mark"})
	if err == nil || strings.Contains(err.Error(), "sensitive-mark") {
		t.Fatal("取消/坏输入诊断未安全拒绝")
	}
}

func TestIOSHelperRejectsDuplicateAndNullJSON(t *testing.T) {
	for _, data := range []string{`{"action":"inspect","action":"close","options":{}}`, `{"action":"inspect","ACTION":"close","options":{}}`, `{"action":"inspect","options":null}`} {
		var output bytes.Buffer
		if err := HandleIOSHelper(strings.NewReader(data), &output); err == nil || !strings.Contains(output.String(), "ios_helper_input_invalid") {
			t.Fatal("有歧义的helper输入未拒绝")
		}
	}
}
