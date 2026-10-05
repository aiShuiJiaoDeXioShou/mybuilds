package distribute

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"mybuilds/internal/process"
	"mybuilds/internal/protocol"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppleTransportEvidenceRequiresBoundedActualCorrelation(t *testing.T) {
	g := protocol.PublishGrant{Apple: &protocol.ApplePublishAuthorization{Action: "upload_binary", RequestSHA256: strings.Repeat("a", 64)}}
	bytes := []byte("UPLOAD SUCCEEDED with no errors\nRequestUUID: 12345678-1234-1234-1234-123456789abc\n")
	sum := sha256.Sum256(bytes)
	out := appleTransportResult(g, "owned-app", process.Result{Started: true, ExitCode: 0}, bytes)
	if out.Status != "confirmed" || out.Remote.Apple.ResponseSHA256 != hex.EncodeToString(sum[:]) {
		t.Fatal("实际确认字节未绑定")
	}
	for _, raw := range [][]byte{[]byte("exit0"), []byte("UPLOAD SUCCEEDED with no errors"), append(append([]byte{}, bytes...), bytes...)} {
		if appleTransportResult(g, "owned-app", process.Result{Started: true}, raw).Status != "unknown" {
			t.Fatal("缺少唯一相关ID仍确认")
		}
	}
	for _, result := range []process.Result{{Started: true, ExitCode: 1}, {Started: true, CleanupFailed: true}, {Started: true, Reason: "timeout"}, {}} {
		if appleTransportResult(g, "owned-app", result, bytes).Status != "unknown" {
			t.Fatal("本机执行事实未核对")
		}
	}
	out.Remote.Apple.ResponseSHA256 = ""
	if receipt(g, process.Result{Started: true}, out).Status != "unknown" {
		t.Fatal("缺响应摘要仍确认")
	}
}

func TestPreparedArtifactReplacementAndPlistScope(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "artifact.aab")
	os.WriteFile(file, []byte("own"), 0600)
	sum := sha256.Sum256([]byte("own"))
	p := &prepared{artifactCopy: file}
	if p.verifyArtifact(context.Background(), hex.EncodeToString(sum[:]), 3) != nil {
		t.Fatal("原包")
	}
	os.WriteFile(file, []byte("bad"), 0600)
	if p.verifyArtifact(context.Background(), hex.EncodeToString(sum[:]), 3) == nil {
		t.Fatal("替换包接受")
	}
	v, e := plistValues([]byte(`<plist><dict><key>CFBundleIdentifier</key><string>com.real.app</string><key>nested</key><dict><key>CFBundleIdentifier</key><string>com.false.app</string></dict></dict></plist>`))
	if e != nil || v["CFBundleIdentifier"] != "com.real.app" {
		t.Fatal("嵌套plist改主标识")
	}
	if _, e = plistValues([]byte(`<plist><dict><key>CFBundleIdentifier</key><string>a</string><key>CFBundleIdentifier</key><string>b</string></dict></plist>`)); e == nil {
		t.Fatal("重复plist")
	}
}
