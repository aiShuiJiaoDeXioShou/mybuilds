package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReportCollectionDiagnosticsReboundedAfterPathKey(t *testing.T) {
	work, wr, dr := reportRoots(t)
	c, err := newReportCollection(context.Background(), wr, dr, []string{"result.xml"}, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	var xml strings.Builder
	xml.WriteString(`<testsuite tests="13" failures="13">`)
	for i := 0; i < 13; i++ {
		fmt.Fprintf(&xml, `<testcase name="%s"><failure message="%s"/></testcase>`, strings.Repeat("c", 512), strings.Repeat("m", 1024))
	}
	xml.WriteString(`</testsuite>`)
	if err = os.WriteFile(filepath.Join(work, "result.xml"), []byte(xml.String()), 0600); err != nil {
		t.Fatal(err)
	}
	e, files := reportCheck(t, c, 1, false)
	if e.Reason != "report_failed" || e.Counts.Tests != 13 || e.Counts.Failures != 13 || len(files) != 1 || len(e.Diagnostics) != 12 {
		t.Fatalf("补Key只能截断显示，不能破坏合法报告: counts=%+v files=%d diagnostics=%d reason=%s", e.Counts, len(files), len(e.Diagnostics), e.Reason)
	}
	bytes := 0
	for _, d := range e.Diagnostics {
		bytes += len(d.PathKey) + len(d.Case) + len(d.Outcome) + len(d.Message)
	}
	if bytes > 20<<10 {
		t.Fatal("诊断总字节没有重新限额")
	}
}

func TestReportCollectionOptionalPartialPatternsKeepCounts(t *testing.T) {
	work, wr, dr := reportRoots(t)
	c, err := newReportCollection(context.Background(), wr, dr, []string{"present.xml", "absent.xml"}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(work, "present.xml"), []byte(`<testsuite tests="1"><testcase/></testsuite>`), 0600); err != nil {
		t.Fatal(err)
	}
	reportCheck(t, c, 1, false)
	final, files := reportCheck(t, c, 0, true)
	if final.Outcome != "passed" || final.Reason != "" || final.Counts.Tests != 1 || len(files) != 1 {
		t.Fatal("可选缺一模式不应抹掉真实文件")
	}
}

func TestReportCollectionTooManyNewFilesIsSafeFailure(t *testing.T) {
	work, wr, dr := reportRoots(t)
	c, err := newReportCollection(context.Background(), wr, dr, []string{"*.xml"}, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 65; i++ {
		if err = os.WriteFile(filepath.Join(work, fmt.Sprintf("result-%d.xml", i)), []byte(`<testsuite/>`), 0600); err != nil {
			t.Fatal(err)
		}
	}
	e, files := reportCheck(t, c, 1, false)
	if e.Outcome != "failed" || e.Reason != "report_invalid" || len(files) != 0 {
		t.Fatal("超文件限额仍发布部分集合")
	}
}

func TestReportCollectionCheckpointRemainsPendingUntilFinal(t *testing.T) {
	work, wr, dr := reportRoots(t)
	c, err := newReportCollection(context.Background(), wr, dr, []string{"result.xml"}, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(work, "result.xml"), []byte(`<testsuite tests="1"><testcase/></testsuite>`), 0600); err != nil {
		t.Fatal(err)
	}
	checked, files := reportCheck(t, c, 1, false)
	if checked.Outcome != "pending" || checked.Sealed || checked.Counts.Tests != 1 || len(files) != 1 {
		t.Fatal("中途checkpoint不能表示最终通过")
	}
	final, _ := reportCheck(t, c, 0, true)
	if final.Outcome != "passed" || final.Sealed || final.Counts.Tests != 1 {
		t.Fatal("final结论与checkpoint不一致")
	}
}
