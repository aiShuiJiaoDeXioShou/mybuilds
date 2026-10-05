package distribute

import (
	"context"
	"encoding/json"
	"errors"
	"mybuilds/internal/process"
	"mybuilds/internal/protocol"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestActualFastlaneExitZeroIsUnknownAndPrivate(t *testing.T) {
	dir := rubyTools(t)
	p, e := newPrepared(context.Background(), dir, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	root := p.workDir
	t.Setenv("PATH", "/opt/homebrew/opt/ruby/bin:"+os.Getenv("PATH"))
	secret := "OWNED_SECRET_DO_NOT_RETURN_123"
	p.credentialCopy, e = p.write("credential.json", []byte(`{"private_key":"`+secret+`"}`))
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	starts := 0
	out, result, e := p.run(ctx, "play_preflight", "com.example.app", nil, nil, func(process.StartInfo) error { starts++; return nil })
	if e != nil || !result.Started || result.CleanupFailed || result.ExitCode != 0 || out.Status != "unknown" || starts != 1 {
		t.Fatalf("真实入口结果 %v/%s/%d/%s started=%d", e, result.Reason, result.ExitCode, out.Status, starts)
	}
	data, _ := json.Marshal(out)
	if strings.Contains(string(data), secret) || strings.Contains(string(data), p.credentialCopy) {
		t.Fatal("秘密泄漏")
	}
	if e = p.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Lstat(root); !os.IsNotExist(e) {
		t.Fatal("自产目录未清")
	}
}
func TestPreparedCloseRefusesReplacedRoot(t *testing.T) {
	p, e := newPrepared(context.Background(), rubyTools(t), t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	old := p.workDir + "-original"
	if os.Rename(p.workDir, old) != nil || os.Mkdir(p.workDir, 0700) != nil {
		t.Fatal("夹具")
	}
	marker := filepath.Join(p.workDir, "unknown")
	os.WriteFile(marker, []byte("keep"), 0600)
	if !errors.Is(p.Close(), ErrCleanup) {
		t.Fatal("替换目录被清")
	}
	if _, e = os.Stat(marker); e != nil {
		t.Fatal("未知资源删除")
	}
}
func TestReceiptRequiresCompleteRemoteEvidence(t *testing.T) {
	g := protocol.PublishGrant{Action: "upload", ArtifactSHA256: strings.Repeat("a", 64), VersionCode: 101, Track: "internal", ReleaseName: "owned"}
	r := receipt(g, process.Result{Started: true, ExitCode: 0}, toolResult{Status: "confirmed"})
	if r.Status != "unknown" {
		t.Fatal("工具状态冒充远端回执")
	}
	out := toolResult{Status: "confirmed", Remote: protocol.PublishRemoteEvidence{BundleSHA256: g.ArtifactSHA256, VersionCode: 101, Track: "internal", ReleaseName: "owned", BundleAccepted: true, TrackAccepted: true, CommitAccepted: true}}
	r = receipt(g, process.Result{Started: true, ExitCode: 0}, out)
	if r.Status != "confirmed" {
		t.Fatal("完整回执未消费")
	}
	google := googleReceipt(g, process.Result{Started: true, ExitCode: 0}, out)
	digest, _ := protocol.PublishReceiptDigest(google)
	if google.Status != "uploaded" || google.Digest != digest {
		t.Fatal("Google具体上传状态/摘要未绑定")
	}
	r = receipt(g, process.Result{Started: true, ExitCode: 0, CleanupFailed: true}, out)
	if r.Status != "unknown" {
		t.Fatal("状态")
	}
	if r.StopConfirmed {
		t.Fatal("清理未知冒充停止")
	}
}
func TestStrictJSONCasingAndAppleDuration(t *testing.T) {
	var out toolResult
	for _, raw := range []string{`{"Status":"confirmed"}`, `{"status":"unknown","remote":{},"matches":null}`, `{"status":"unknown","remote":{"Apple":{}}}`} {
		if strictJSON([]byte(raw), &out) == nil {
			t.Fatal("非规范字段接受")
		}
	}
}
func TestRestrictedEnvironmentAndOutput(t *testing.T) {
	p := &prepared{workDir: "/owned", toolDir: "/tool", gemDir: "/gems"}
	t.Setenv("FASTLANE_SESSION", "must_not_inherit")
	t.Setenv("HTTPS_PROXY", "must_not_inherit")
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "must_not_inherit")
	for _, value := range p.environment("job") {
		if strings.Contains(value, "must_not_inherit") {
			t.Fatal("继承未知材料")
		}
	}
	var out limitedOutput
	if _, e := out.Write(make([]byte, outputLimit+1)); e == nil {
		t.Fatal("输出未限额")
	}
}
