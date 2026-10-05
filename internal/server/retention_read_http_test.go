//go:build darwin || linux

package server

import (
	"context"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestRetentionRetiredEvidenceHTTPNeverEmptySuccess(t *testing.T) {
	for _, kind := range []string{"log", "artifact", "junit"} {
		t.Run(kind, func(t *testing.T) {
			s, _, project, object := retentionCleanupFixture(t, kind)
			if _, err := os.Lstat(retentionCleanupPath(s, object)); err != nil {
				t.Fatal("退役时原文件尚存", err)
			}
			h := httptest.NewServer(s.Handler())
			defer h.Close()
			paths := []string{"/api/builds/" + object.BuildID + "/log", "/api/builds/" + object.BuildID + "/artifacts"}
			if kind != "log" {
				paths = append(paths, "/api/artifacts/"+object.ObjectID)
			}
			for _, stage := range []string{"retired", "central_deleted"} {
				for _, path := range paths {
					code, body := request(t, h, "GET", path, adminToken, "")
					if code != 410 || !strings.Contains(body, `"code":"retention_retired"`) {
						t.Fatal("退役读取不伪空成功", stage, path, code, body)
					}
				}
				if stage == "retired" {
					if err := s.advanceCentralRetention(context.Background(), project, 100); err != nil {
						t.Fatal(err)
					}
					if _, err := os.Lstat(retentionCleanupPath(s, object)); !os.IsNotExist(err) {
						t.Fatal("实际中央删除", err)
					}
				}
			}
		})
	}
}
