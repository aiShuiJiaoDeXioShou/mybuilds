package pipeline

import (
	"context"
	"encoding/json"
	"mybuilds/internal/mobile"
	"mybuilds/internal/protocol"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "__ios-signing" {
		if mobile.HandleIOSHelper(os.Stdin, os.Stdout) != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

const explicitIOSSigning = `
    ios_signing:
      p12: "${PIPELINE_IOS_P12}"
      profile: "${PIPELINE_IOS_PROFILE}"
      password: "${PIPELINE_IOS_PASSWORD}"
      bundle_id: com.example.mybuilds
      export_method: debugging
`

func TestIOSInvalidMaterialPreventsEveryBatchScript(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PIPELINE_IOS_P12", root+"/absent-p12")
	t.Setenv("PIPELINE_IOS_PROFILE", root+"/absent-profile")
	t.Setenv("PIPELINE_IOS_PASSWORD", "PRIVATE_IOS_PASSWORD")
	d := localDocument(t, "version: 1\nbuilds:\n  first:\n    steps: [{kind: run, run: touch first-started}]\n  ios:\n"+explicitIOSSigning+"    steps: [{kind: run, run: touch ios-started}]\n")
	result, err := Run(context.Background(), d, RunOptions{Workspace: root, PreviewOptions: PreviewOptions{Names: []string{"first", "ios"}}})
	if err == nil || result != nil || strings.Contains(err.Error(), "PRIVATE") {
		t.Fatal("无效材料未阻止整批执行")
	}
	entries, e := os.ReadDir(root)
	if e != nil || len(entries) != 0 {
		t.Fatal("预检查创建了脚本或资源")
	}
}
func TestIOSArtifactOnlyCannotUseSystemOutput(t *testing.T) {
	root := t.TempDir()
	d := localDocument(t, "version: 1\nbuilds:\n  ios:\n"+explicitIOSSigning+`    steps:
      - {kind: run, name: archive, run: touch should-not-start}
      - {kind: artifact, name: collect, paths: ["{{ios.output_dir}}/*.ipa"]}
`)
	result, err := Run(context.Background(), d, RunOptions{Workspace: root, PreviewOptions: PreviewOptions{Step: "collect", Facts: map[string]string{"ios.output_dir": "forged"}}})
	if err == nil || result != nil {
		t.Fatal("单独artifact借用了签名输出")
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatal("artifact-only创建了资源")
	}
}
func TestIOSSkippedDoesNotReadMaterials(t *testing.T) {
	root := t.TempDir()
	d := localDocument(t, "version: 1\nbuilds:\n  ios:\n"+explicitIOSSigning+`    params: {enabled: no}
    when: {params: {enabled: yes}}
    steps: [{kind: run, run: touch should-not-start}]
`)
	result, err := Run(context.Background(), d, RunOptions{Workspace: root})
	if err != nil || result.Builds[0].Status != "skipped" || !result.Builds[0].IOSCleanupConfirmed {
		t.Fatalf("跳过构建读取材料: %v", err)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatal("跳过构建创建了资源")
	}
}
func TestIOSPrepareFailureClosesBeforeAnyScript(t *testing.T) {
	if !mobile.IOSSigningSupported() {
		t.Skip("当前宿主没有原生组件")
	}
	root, _ := filepath.EvalSymlinks(t.TempDir())
	temporary := t.TempDir()
	t.Setenv("TMPDIR", temporary)
	d := localDocument(t, "version: 1\nsteps: [{kind: run, name: archive, run: touch should-not-start}]\npost:\n  always: [{kind: run, run: touch post-should-not-start}]\n")
	p := runPreparation{ctx: context.Background(), root: root, facts: map[string]string{}, tried: map[string]bool{}}
	build, err := p.build("default", d.Builds["default"], nil, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	build.iosDeclared = true
	build.iosSigning = &mobile.IOSSigningOptions{Workspace: root, OutputDir: ".mybuilds-ios-test", P12File: "absent.p12", ProfileFile: "absent.profile", BundleID: "com.example.app", ExportMethod: "debugging"}
	logger := newRunLogger(nil, nil)
	result, unsafe := executeBuild(context.Background(), root, build, logger)
	logger.close()
	if result.Status != "failed" || result.Reason != "start_error" || unsafe || result.Steps[0].started || !result.IOSCleanupConfirmed {
		data, _ := json.Marshal(result)
		t.Fatalf("准备失败状态错误: %s", data)
	}
	entries, _ := os.ReadDir(root)
	temps, _ := os.ReadDir(temporary)
	if len(entries) != 0 || len(temps) != 0 {
		t.Fatal("准备失败遗留本次资源或启动脚本")
	}
}

func TestIOSPrepareCancellationPersistsAndClosesBeforeScript(t *testing.T) {
	if !mobile.IOSSigningSupported() {
		t.Skip("当前宿主没有原生组件")
	}
	root, _ := filepath.EvalSymlinks(t.TempDir())
	d := localDocument(t, "version: 1\nsteps: [{kind: run, name: archive, run: touch should-not-start}]\npost:\n  always: [{kind: run, run: touch post-should-not-start}]\n")
	p := runPreparation{ctx: context.Background(), root: root, facts: map[string]string{}, tried: map[string]bool{}}
	build, err := p.build("default", d.Builds["default"], nil, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	build.iosDeclared = true
	build.iosSigning = &mobile.IOSSigningOptions{Workspace: root, OutputDir: ".mybuilds-ios-cancel", P12File: "absent.p12", ProfileFile: "absent.profile", BundleID: "com.example.app", ExportMethod: "debugging"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var checkpoints []mobile.IOSResourceOwnership
	resultParent := t.TempDir()
	if err := os.Chmod(resultParent, 0700); err != nil {
		t.Fatal(err)
	}
	remote, err := newRemote(&RemoteOptions{AuthorityContext: context.Background(), ResultParent: resultParent, RemainingPostBudgetNS: 1 << 40, Log: func(context.Context, protocol.LogRecord) error { return nil }, Progress: func(context.Context, protocol.ExecutionProgress) error { return nil }, IOSCheckpoint: func(own mobile.IOSResourceOwnership) error {
		checkpoints = append(checkpoints, own)
		if own.Preparing {
			cancel()
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer remote.cancel()
	logger := newRunLogger(nil, nil)
	logger.remote = remote
	result, unsafe := executeBuild(ctx, root, build, logger)
	logger.close()
	if logger.root != nil {
		defer os.RemoveAll(logger.root.Name())
		logger.root.Close()
	}
	if result.Status != "cancelled" || result.Reason != "cancelled" || result.Steps[0].started || unsafe || !result.IOSCleanupConfirmed || len(checkpoints) < 3 || !checkpoints[1].Preparing || !checkpoints[len(checkpoints)-1].Closed {
		t.Fatalf("准备取消未保持真实持久顺序和独立关闭: %+v", result)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatal("取消准备启动了用户脚本或遗留输出")
	}
}
func TestIOSUnknownNativeLeafPreservesOriginalFailure(t *testing.T) {
	if !mobile.IOSSigningSupported() {
		t.Skip("当前宿主没有原生组件")
	}
	root, _ := filepath.EvalSymlinks(t.TempDir())
	d := localDocument(t, "version: 1\nsteps: [{kind: run,name: archive,run: touch should-not-start}]\n")
	p := runPreparation{ctx: context.Background(), root: root, facts: map[string]string{}, tried: map[string]bool{}}
	build, err := p.build("default", d.Builds["default"], nil, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	build.iosDeclared = true
	build.iosSigning = &mobile.IOSSigningOptions{Workspace: root, OutputDir: ".mybuilds-ios-unknown", P12File: "absent", ProfileFile: "absent", BundleID: "com.example.app", ExportMethod: "debugging"}
	var own mobile.IOSResourceOwnership
	resultParent := t.TempDir()
	if err := os.Chmod(resultParent, 0700); err != nil {
		t.Fatal(err)
	}
	remote, err := newRemote(&RemoteOptions{AuthorityContext: context.Background(), ResultParent: resultParent, RemainingPostBudgetNS: 1 << 40, Log: func(context.Context, protocol.LogRecord) error { return nil }, Progress: func(context.Context, protocol.ExecutionProgress) error { return nil }, IOSCheckpoint: func(value mobile.IOSResourceOwnership) error {
		own = value
		if value.Preparing {
			return os.WriteFile(value.Keychain, []byte("owned-fixture-unknown-leaf"), 0600)
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer remote.cancel()
	logger := newRunLogger(nil, nil)
	logger.remote = remote
	result, unsafe := executeBuild(context.Background(), root, build, logger)
	logger.close()
	if logger.root != nil {
		defer os.RemoveAll(logger.root.Name())
		logger.root.Close()
	}
	// 测试清理自己引入的未知叶，不替产品声称StopKnown。
	defer os.RemoveAll(own.Output)
	defer os.RemoveAll(own.Temporary)
	if result.Status != "failed" || result.Reason != "start_error" || !unsafe || !result.CleanupFailed || result.IOSCleanupConfirmed || result.Steps[0].started {
		t.Fatal("原生未知覆盖原失败或被称为已关闭", result)
	}
	if data, err := os.ReadFile(own.Keychain); err != nil || string(data) != "owned-fixture-unknown-leaf" {
		t.Fatal("未知叶被产品删除")
	}
}
