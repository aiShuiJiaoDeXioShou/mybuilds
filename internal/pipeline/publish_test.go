package pipeline

import (
	"context"
	"mybuilds/internal/process"
	"mybuilds/internal/protocol"
	"os"
	"path/filepath"
	"testing"
)

func TestPublishBarrierUsesCollectedSnapshotAndOriginalJUnit(t *testing.T) {
	root := t.TempDir()
	doc := localDocument(t, `version: 1
params:
 version: {default: '1.2.3'}
steps:
 - {kind: run, name: compile, run: 'mkdir -p output; printf original > output/app.aab; printf "<testsuite tests=\"1\"><testcase name=\"pass\"/></testsuite>" > result.xml'}
 - {kind: artifact, name: package, paths: [output/app.aab]}
 - {kind: upload, name: publish, target: google_play, app_identifier: com.example.app, file: output/app.aab, credentials: '${PLAY_JSON}'}
reports:
 junit: {paths: [result.xml]}
post:
 always:
  - {kind: run, run: 'printf changed > output/app.aab; printf invalid > result.xml'}
`)
	remote := remoteOptions(t)
	sealed := false
	called := 0
	remote.Progress = func(_ context.Context, p protocol.ExecutionProgress) error {
		if p.Kind == "reports_sealed" {
			sealed = true
		}
		if p.Kind == "intent" && p.StepKind == "upload" && !sealed {
			t.Fatal("商店意图先于原报告封存")
		}
		return nil
	}
	remote.Publish = func(ctx context.Context, in PublishInput) (protocol.PublishReceipt, error) {
		called++
		if !sealed || in.ReportSealDigest == "" || len(in.ReportIDs) != 1 || in.Step.Name != "publish" || in.Artifact.SourcePath != "output/app.aab" {
			t.Fatal("发布没有原快照/seal")
		}
		data, err := os.ReadFile(filepath.Join(remote.ResultParent, filepath.Base(remoteResult(t, remote)), in.Artifact.SnapshotPath))
		if err != nil || string(data) != "original" {
			t.Fatal("发布读取了可变源而非原快照", err)
		}
		if err = in.OnStart(process.StartInfo{PID: 123, PGID: 123}); err != nil {
			return protocol.PublishReceipt{}, err
		}
		return protocol.PublishReceipt{Status: "uploaded", Started: true, StopConfirmed: true}, nil
	}
	// callback的假商店结果只测试Run的边界，不是任何外部商店验收证据。
	result, err := runWithCleanup(t, context.Background(), doc, RunOptions{Workspace: root, Remote: remote})
	if err != nil || result == nil || called != 1 || result.Builds[0].Reports == nil || result.Builds[0].Reports.Counts.Tests != 1 {
		t.Fatal("同Run发布屏障", err)
	}
	requireFile(t, root, "output/app.aab", "changed")
	if !sealed {
		t.Fatal("未封存")
	}
}
func remoteResult(t *testing.T, r *RemoteOptions) string {
	t.Helper()
	entries, err := os.ReadDir(r.ResultParent)
	if err != nil || len(entries) != 1 {
		t.Fatal("没有唯一结果目录", err)
	}
	return filepath.Join(r.ResultParent, entries[0].Name())
}
func TestPublishLocalAndMissingCallbackRejectBeforeScript(t *testing.T) {
	root := t.TempDir()
	doc := localDocument(t, `version: 1
steps:
 - {kind: run, run: touch forbidden}
 - {kind: upload, target: google_play, app_identifier: com.example.app, file: app.aab, credentials: '${PLAY_JSON}'}
`)
	for _, remote := range []*RemoteOptions{nil, remoteOptions(t)} {
		if result, err := runWithCleanup(t, context.Background(), doc, RunOptions{Workspace: root, Remote: remote}); err == nil || result != nil {
			t.Fatal("缺真正publisher却启动脚本")
		}
		requireAbsent(t, root, "forbidden")
	}
}
