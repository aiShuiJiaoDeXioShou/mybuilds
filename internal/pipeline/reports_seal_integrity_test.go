package pipeline

import (
	"context"
	"mybuilds/internal/config"
	"os"
	"path/filepath"
	"testing"
)

func TestReportCollectionSealRejectsMutatedCandidate(t *testing.T) {
	for _, field := range []string{"size", "source", "diagnostic"} {
		t.Run(field, func(t *testing.T) {
			work, wr, dr := reportRoots(t)
			c, err := newReportCollection(context.Background(), wr, dr, []string{"result.xml"}, true, nil, config.DefaultJUnitMaxFiles)
			if err != nil {
				t.Fatal(err)
			}
			xml := []byte(`<testsuite><testcase><failure message="original"/></testcase></testsuite>`)
			if err = os.WriteFile(filepath.Join(work, "result.xml"), xml, 0600); err != nil {
				t.Fatal(err)
			}
			reportCheck(t, c, 1, false)
			final, _ := reportCheck(t, c, 0, true)
			// 真实consumer只应封存check返回的精确结果，不能借共享slice改掉独立证据。
			switch field {
			case "size":
				final.Files[0].Size++
			case "source":
				final.Files[0].SourceIndex++
			case "diagnostic":
				final.Diagnostics[0].Message = "replaced"
			}
			if _, _, err = c.seal(context.Background(), final); err == nil {
				t.Fatal("被改写的候选仍可封存")
			}
		})
	}
}

func TestReportCollectionNoRealRunDoesNotInventXMLSource(t *testing.T) {
	work, wr, dr := reportRoots(t)
	c, err := newReportCollection(context.Background(), wr, dr, []string{"result.xml"}, true, nil, config.DefaultJUnitMaxFiles)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(work, "result.xml"), []byte(`<testsuite><testcase/></testsuite>`), 0600); err != nil {
		t.Fatal(err)
	}
	final, files := reportCheck(t, c, 0, true)
	if final.Outcome != "failed" || final.Reason != "report_missing" || len(files) != 0 || final.Counts.Tests != 0 {
		t.Fatal("final不是run，不能伪造Source")
	}
}
