package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReportCollectionRejectsUnsafeSources(t *testing.T) {
	for _, tc := range []struct {
		name, pattern string
		setup         func(t *testing.T, work string)
	}{
		{"leaf-link", "result.xml", func(t *testing.T, work string) {
			target := filepath.Join(t.TempDir(), "other.xml")
			if err := os.WriteFile(target, []byte(`<testsuite/>`), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(work, "result.xml")); err != nil {
				t.Fatal(err)
			}
		}},
		{"directory-link", "alias/*.xml", func(t *testing.T, work string) {
			target := t.TempDir()
			if err := os.WriteFile(filepath.Join(target, "other.xml"), []byte(`<testsuite/>`), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(work, "alias")); err != nil {
				t.Fatal(err)
			}
		}},
		{"oversized-old", "result.xml", func(t *testing.T, work string) {
			f, err := os.Create(filepath.Join(work, "result.xml"))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if err = f.Truncate((8 << 20) + 1); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			work, wr, dr := reportRoots(t)
			tc.setup(t, work)
			if _, err := newReportCollection(context.Background(), wr, dr, []string{tc.pattern}, false, nil); err == nil {
				t.Fatal("不安全旧源未拒绝")
			}
		})
	}
}

func TestReportCollectionUnsafePatterns(t *testing.T) {
	_, wr, dr := reportRoots(t)
	for _, pattern := range []string{"../x.xml", "/x.xml", "C:/x.xml", `a\x.xml`, "a/../x.xml", "a\nx.xml", strings.Repeat("a", 256) + ".xml"} {
		if _, err := newReportCollection(context.Background(), wr, dr, []string{pattern}, true, nil); err == nil {
			t.Fatalf("接受非法路径 %q", pattern)
		}
	}
}

func TestReportCollectionSnapshotPersistenceFailure(t *testing.T) {
	work, wr, dr := reportRoots(t)
	c, err := newReportCollection(context.Background(), wr, dr, []string{"result.xml"}, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = dr.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(work, "result.xml"), []byte(`<testsuite><testcase/></testsuite>`), 0600); err != nil {
		t.Fatal(err)
	}
	_, _, err = c.check(context.Background(), 1, "actual", false)
	if err == nil || reportFailureReason(err) != "report_error" {
		t.Fatalf("写失败必须闭锁: %v", err)
	}
}

func TestReportCollectionCorruptSnapshotCannotSeal(t *testing.T) {
	work, wr, dr := reportRoots(t)
	c, err := newReportCollection(context.Background(), wr, dr, []string{"result.xml"}, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(work, "result.xml"), []byte(`<testsuite><testcase/></testsuite>`), 0600); err != nil {
		t.Fatal(err)
	}
	_, locals := reportCheck(t, c, 1, false)
	final, _ := reportCheck(t, c, 0, true)
	if err = dr.WriteFile(locals[0].SnapshotPath, []byte(`<testsuite/>`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err = c.seal(context.Background(), final); err == nil || reportFailureReason(err) != "report_error" {
		t.Fatal("快照篡改仍成为可信seal")
	}
}

func TestReportCollectionDirectoryReadFailureClosesExecution(t *testing.T) {
	_, wr, dr := reportRoots(t)
	c, err := newReportCollection(context.Background(), wr, dr, []string{"results/*.xml"}, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	// 真实Root关闭导致后续目录访问失败，不用虚构FS或执行钩子。
	if err = wr.Close(); err != nil {
		t.Fatal(err)
	}
	_, _, err = c.check(context.Background(), 1, "actual", false)
	if err == nil || reportFailureReason(err) != "report_error" {
		t.Fatalf("目录IO错误不能变成可继续post的业务结论: %v", err)
	}
}

func TestReportCollectionSnapshotReadErrorIsNotXMLBusinessFailure(t *testing.T) {
	_, _, dr := reportRoots(t)
	if err := dr.WriteFile("stage.xml", []byte(`<testsuite/>`), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := dr.Open("stage.xml")
	if err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = parseReportSnapshot(context.Background(), file, nil); err == nil || reportFailureReason(err) != "report_error" {
		t.Fatal("实际快照IO错误被误当作可post的非法XML")
	}
}
