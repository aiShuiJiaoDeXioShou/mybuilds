package pipeline

import (
	"context"
	"encoding/json"
	"mybuilds/internal/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReportCollectionSecretsNeverPublishXML(t *testing.T) {
	for _, tc := range []struct{ name, xml string }{
		{"raw", `<testsuite><testcase name="case"><failure>private-marker</failure></testcase></testsuite>`},
		{"decoded-attribute", `<testsuite><testcase name="private&#45;marker"/></testsuite>`},
		{"decoded-text", `<testsuite><testcase><error>private&#45;marker</error></testcase></testsuite>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			work, wr, dr := reportRoots(t)
			c, err := newReportCollection(context.Background(), wr, dr, []string{"result.xml"}, true, []string{"private-marker"}, config.DefaultJUnitMaxFiles)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(work, "result.xml"), []byte(tc.xml), 0600); err != nil {
				t.Fatal(err)
			}
			e, files, err := c.check(context.Background(), 1, "actual", false)
			if err != nil {
				t.Fatal(err)
			}
			if e.Outcome != "failed" || e.Reason != "report_secret" || len(e.Files) != 0 || len(files) != 0 {
				t.Fatalf("含秘密原XML不应发布: %+v", e)
			}
			data, _ := json.Marshal(e)
			if strings.Contains(string(data), "private-marker") {
				t.Fatal("公共结果泄漏")
			}
		})
	}
}

func TestReportCollectionSecretRelativePath(t *testing.T) {
	work, wr, dr := reportRoots(t)
	if err := os.WriteFile(filepath.Join(work, "private-marker.xml"), []byte(`<testsuite/>`), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := newReportCollection(context.Background(), wr, dr, []string{"*.xml"}, true, []string{"private-marker"}, config.DefaultJUnitMaxFiles)
	if err == nil || reportFailureReason(err) != "report_secret" || strings.Contains(err.Error(), "private-marker") {
		t.Fatal("路径秘密未安全拒绝")
	}
}
