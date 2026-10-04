package pipeline

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mybuilds/internal/process"
	"mybuilds/internal/protocol"
)

func reportRoots(t *testing.T) (string, *os.Root, *os.Root) {
	t.Helper()
	work := t.TempDir()
	dst := t.TempDir()
	wr, e := os.OpenRoot(work)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { wr.Close() })
	dr, e := os.OpenRoot(dst)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { dr.Close() })
	return work, wr, dr
}
func reportScript(t *testing.T, work, script string) {
	t.Helper()
	r := process.Run(context.Background(), process.Command{Path: "/bin/sh", Args: []string{"-e", "-c", script}, Dir: work, Env: environmentList(process.HostEnvironment())}, nil, nil)
	if !r.Started || r.ExitCode != 0 || r.Reason != "" || r.CleanupFailed {
		t.Fatalf("真实脚本未正常完成: %+v", r)
	}
}
func reportCheck(t *testing.T, c *reportCollection, index int, final bool) (protocol.ReportEvidence, []protocol.CollectedReport) {
	t.Helper()
	name := "actual"
	if final {
		name = ""
	}
	e, files, err := c.check(context.Background(), index, name, final)
	if err != nil {
		t.Fatal(err)
	}
	return e, files
}
func TestReportCollectionRealReplacementAndSnapshot(t *testing.T) {
	work, wr, dr := reportRoots(t)
	c, err := newReportCollection(context.Background(), wr, dr, []string{"results/*.xml", "results/result.xml"}, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	reportScript(t, work, `mkdir results; printf '<testsuite tests="1"><testcase name="first" time="0.125"/></testsuite>' > results/result.xml`)
	first, f1 := reportCheck(t, c, 1, false)
	if first.Counts.Tests != 1 || first.Counts.DurationNS != 125000000 || len(first.Files) != 1 || len(f1) != 1 || first.Sealed {
		t.Fatalf("首次检查: %+v", first)
	}
	same, f2 := reportCheck(t, c, 2, false)
	if same.Files[0].ArtifactID != first.Files[0].ArtifactID || f1[0].SnapshotPath != f2[0].SnapshotPath || same.Counts.Tests != 1 {
		t.Fatal("未变化文件不应重复计算或换ID")
	}
	original, err := dr.ReadFile(f1[0].SnapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	reportScript(t, work, `printf '<testsuite tests="3" failures="1" errors="1" skipped="1"><testcase name="a"><failure message="mismatch"/></testcase><testcase name="b"><error>oops</error></testcase><testcase name="c"><skipped/></testcase></testsuite>' > results/result.xml`)
	replaced, f3 := reportCheck(t, c, 3, false)
	if replaced.Outcome != "failed" || replaced.Reason != "report_failed" || replaced.Counts.Tests != 3 || replaced.Counts.Errors != 1 || replaced.Counts.Skipped != 1 || len(replaced.Diagnostics) != 2 || f3[0].File.ArtifactID == f1[0].File.ArtifactID || f3[0].File.SourceIndex != 3 {
		t.Fatalf("替换结果: %+v", replaced)
	}
	old, _ := dr.ReadFile(f1[0].SnapshotPath)
	if string(old) != string(original) {
		t.Fatal("旧快照被覆盖")
	}
	final, files := reportCheck(t, c, 0, true)
	if final.Counts.Tests != 3 || final.Files[0].SourceIndex != 3 || len(files) != 1 {
		t.Fatal("final不是当前真实集合")
	}
	sealed, digest, err := c.seal(context.Background(), final)
	if err != nil || !sealed.Sealed || len(digest) != 64 {
		t.Fatalf("seal: %+v %q %v", sealed, digest, err)
	}
	reportScript(t, work, `printf '<testsuite tests="0"/>' > results/result.xml`)
	snapshot, err := dr.ReadFile(files[0].SnapshotPath)
	if err != nil || string(snapshot) == `<testsuite tests="0"/>` {
		t.Fatal("源改变污染封存快照")
	}
	if _, _, err = c.check(context.Background(), 4, "post", false); err == nil {
		t.Fatal("final后允许新检查")
	}
	wire, _ := json.Marshal(sealed)
	if len(wire) == 0 {
		t.Fatal("缺少有限JSON")
	}
}

func TestReportCollectionFreshnessAndMissing(t *testing.T) {
	for _, required := range []bool{true, false} {
		t.Run(map[bool]string{true: "required", false: "optional"}[required], func(t *testing.T) {
			work, wr, dr := reportRoots(t)
			file := filepath.Join(work, "result.xml")
			old := `<testsuite tests="1"><testcase/></testsuite>`
			if e := os.WriteFile(file, []byte(old), 0600); e != nil {
				t.Fatal(e)
			}
			info, _ := os.Stat(file)
			c, err := newReportCollection(context.Background(), wr, dr, []string{"result.xml"}, required, nil)
			if err != nil {
				t.Fatal(err)
			}
			prepared, _ := reportCheck(t, c, 1, false)
			if prepared.Outcome != "pending" || len(prepared.Files) != 0 {
				t.Fatal("旧报告计入本次或prepare误失败")
			}
			if err = os.WriteFile(file, []byte(`<testsuite tests="2"><testcase/><testcase/></testsuite>`), 0600); err != nil {
				t.Fatal(err)
			}
			if err = os.Chtimes(file, info.ModTime(), info.ModTime()); err != nil {
				t.Fatal(err)
			}
			changed, _ := reportCheck(t, c, 2, false)
			if changed.Counts.Tests != 2 {
				t.Fatal("保mtime改写未识别")
			}
			if err = os.Remove(file); err != nil {
				t.Fatal(err)
			}
			missing, _ := reportCheck(t, c, 0, true)
			expected := "missing"
			if required {
				expected = "failed"
			}
			if missing.Outcome != expected || len(missing.Files) != 0 || missing.Counts.Tests != 0 {
				t.Fatalf("缺失: %+v", missing)
			}
		})
	}
}

func TestReportCollectionNewIdentitySameBytes(t *testing.T) {
	work, wr, dr := reportRoots(t)
	file := filepath.Join(work, "result.xml")
	bytes := []byte(`<testsuite tests="1"><testcase/></testsuite>`)
	if err := os.WriteFile(file, bytes, 0600); err != nil {
		t.Fatal(err)
	}
	old, _ := os.Stat(file)
	c, err := newReportCollection(context.Background(), wr, dr, []string{"result.xml"}, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(work, "new.xml")
	if err = os.WriteFile(replacement, bytes, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Chtimes(replacement, old.ModTime(), old.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(replacement, file); err != nil {
		t.Fatal(err)
	}
	current, _ := os.Stat(file)
	if os.SameFile(old, current) {
		t.Fatal("夹具没有新inode")
	}
	evidence, _ := reportCheck(t, c, 1, false)
	if evidence.Counts.Tests != 1 {
		t.Fatal("同内容新身份没有识别")
	}
}

func TestReportCollectionCancelledBeforeBaseline(t *testing.T) {
	_, wr, dr := reportRoots(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if _, err := newReportCollection(ctx, wr, dr, []string{"**/*.xml"}, true, nil); err == nil || time.Since(start) > time.Second {
		t.Fatal("取消未受限")
	}
}
