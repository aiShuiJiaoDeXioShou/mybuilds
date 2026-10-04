package client

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReportsCLICentralDownloadAfterNodeDisabledAndRoles(t *testing.T) {
	st, address, d, data := realReportRemoteCLI(t)
	adminToken := os.Getenv("MYBUILDS_CLIENT_TOKEN")
	admin, err := st.Authenticate(context.Background(), adminToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = executeRemote(t, "--server-url", address, "node", "disable", "report-worker"); err != nil {
		t.Fatal(err)
	}
	approver, err := st.CreateToken(context.Background(), admin, "approver")
	if err != nil {
		t.Fatal(err)
	}
	trigger, err := st.CreateToken(context.Background(), admin, "trigger")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, token string
		allowed     bool
	}{{"admin", adminToken, true}, {"approver", approver.Token, true}, {"trigger", trigger.Token, false}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MYBUILDS_CLIENT_TOKEN", tc.token)
			destination := filepath.Join(t.TempDir(), "downloads", "result.xml")
			out, err := executeRemote(t, "--server-url", address, "artifact", "download", d.ID, "--output", destination)
			if tc.allowed {
				if err != nil {
					t.Fatal("封存XML离线下载失败", err, out)
				}
				actual, err := os.ReadFile(destination)
				if err != nil || !bytes.Equal(actual, data) {
					t.Fatal("原XML字节错误", err)
				}
				if _, err = executeRemote(t, "--server-url", address, "artifact", "download", d.ID, "--output", destination); err == nil {
					t.Fatal("下载覆盖已有文件")
				}
				actual, err = os.ReadFile(destination)
				if err != nil || !bytes.Equal(actual, data) {
					t.Fatal("覆盖拒绝破坏原文件", err)
				}
			} else {
				if err == nil {
					t.Fatal("trigger下载用户证据")
				}
				if _, err = os.Stat(destination); !os.IsNotExist(err) {
					t.Fatal("越权写出文件")
				}
			}
		})
	}
}

// 代理只改变真实中央响应流，元数据与原XML仍来自实际sealed Store/Server。
func TestReportsCLIDownloadShortAndDigestMismatchNoOutput(t *testing.T) {
	_, address, d, _ := realReportRemoteCLI(t)
	upstream, err := url.Parse(address)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"short", "digest"} {
		t.Run(kind, func(t *testing.T) {
			proxy := httputil.NewSingleHostReverseProxy(upstream)
			proxy.ErrorLog = log.New(io.Discard, "", 0)
			proxy.ModifyResponse = func(response *http.Response) error {
				if response.StatusCode == 200 && strings.HasSuffix(response.Request.URL.Path, "/result.xml") {
					body, err := io.ReadAll(response.Body)
					response.Body.Close()
					if err != nil {
						return err
					}
					if kind == "short" {
						body = body[:5]
					} else {
						body[0] = 'X'
					}
					response.Body = io.NopCloser(bytes.NewReader(body))
				}
				return nil
			}
			api := httptest.NewServer(proxy)
			defer api.Close()
			destination := filepath.Join(t.TempDir(), "downloads", "result.xml")
			out, err := executeRemote(t, "--server-url", api.URL, "artifact", "download", d.ID, "--output", destination)
			if err == nil || strings.Contains(out, "<testsuite") {
				t.Fatal("坏流被确认或原XML被显示", err, out)
			}
			if _, err = os.Stat(destination); !os.IsNotExist(err) {
				t.Fatal("坏流发布到输出")
			}
			entries, err := os.ReadDir(filepath.Dir(destination))
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatal("坏流私有stage未清理", entries)
			}
		})
	}
}
