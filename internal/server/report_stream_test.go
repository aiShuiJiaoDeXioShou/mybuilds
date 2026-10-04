package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
)

func waitReportStage(t *testing.T, s *Server) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		entries, _ := os.ReadDir(filepath.Join(s.config.DataDir, "artifacts"))
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".stage-") {
				info, err := entry.Info()
				if err == nil && info.Size() > 0 {
					return
				}
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("未观察到真实body已经写入自有stage")
}
func partialReportConnection(t *testing.T, api *httptest.Server, token string, d protocol.ArtifactDeclaration) net.Conn {
	t.Helper()
	address := strings.TrimPrefix(api.URL, "http://")
	conn, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err = conn.SetDeadline(time.Now().Add(4 * time.Second)); err != nil {
		t.Fatal(err)
	}
	_, err = fmt.Fprintf(conn, "PUT /api/agent/artifacts/%s HTTP/1.1\r\nHost: %s\r\nAuthorization: Bearer %s\r\nX-Mybuilds-Artifact: %s\r\nContent-Length: %d\r\n\r\n<test", d.ID, address, token, base64.RawURLEncoding.EncodeToString([]byte(encodeMessage(t, d))), d.Size)
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func TestReportHTTPSlowBodyUsesIndependentDeadline(t *testing.T) {
	data := []byte(`<testsuite tests="1"><testcase/></testsuite>`)
	s, _, _, token, d, _ := reportHTTPFixture(t, data, nil)
	api := httptest.NewUnstartedServer(s.Handler())
	api.Config.ReadTimeout = 100 * time.Millisecond
	api.Config.WriteTimeout = 100 * time.Millisecond
	api.Start()
	defer api.Close()
	reader, writer := io.Pipe()
	done := make(chan error, 1)
	go func() {
		_, err := writer.Write(data[:5])
		if err == nil {
			time.Sleep(300 * time.Millisecond)
			_, err = writer.Write(data[5:])
		}
		writer.CloseWithError(err)
		done <- err
	}()
	defer reader.Close()
	r, err := http.NewRequest("PUT", api.URL+"/api/agent/artifacts/"+d.ID, reader)
	if err != nil {
		t.Fatal(err)
	}
	r.ContentLength = d.Size
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("X-Mybuilds-Artifact", base64.RawURLEncoding.EncodeToString([]byte(encodeMessage(t, d))))
	response, err := api.Client().Do(r)
	if err != nil {
		t.Fatal("普通100ms截断合法慢XML", err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 {
		t.Fatal(response.StatusCode, string(body), err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}

func TestReportHTTPPartialBodyAuthorityLossClosesConnection(t *testing.T) {
	data := []byte(`<testsuite tests="1"><testcase/></testsuite>`)
	s, st, api, token, d, _ := reportHTTPFixture(t, data, nil)
	conn := partialReportConnection(t, api, token, d)
	waitReportStage(t, s)
	start := time.Now()
	code, out := request(t, api, "POST", "/api/nodes/worker/disable", adminToken, "")
	if code != 204 {
		t.Fatal(code, out)
	}
	response, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal("失权响应无界或未返回", err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode < 400 {
		t.Fatal("失权仍确认或无界drain", response.StatusCode, string(body), err)
	}
	var one [1]byte
	_, err = conn.Read(one[:])
	if err != io.EOF || time.Since(start) > 2*time.Second {
		t.Fatal("失权未实际关闭连接", err, time.Since(start))
	}
	admin, err := st.Authenticate(context.Background(), adminToken)
	if err != nil {
		t.Fatal(err)
	}
	items, err := st.ListArtifacts(context.Background(), admin, d.Ref.BuildID, store.Page{Limit: 100})
	if err != nil || len(items) != 0 {
		t.Fatal("部分XML被确认", err, items)
	}
	entries, err := os.ReadDir(filepath.Join(s.config.DataDir, "artifacts"))
	if err != nil || len(entries) != 0 {
		t.Fatal("失权stage未清理", entries, err)
	}
}

func TestReportHTTPPartialBodyOrdinaryBudgetExpires(t *testing.T) {
	budget := int64(1200 * time.Millisecond)
	data := []byte(`<testsuite tests="1"><testcase/></testsuite>`)
	s, st, api, token, d, _ := reportHTTPFixture(t, data, &budget)
	conn := partialReportConnection(t, api, token, d)
	waitReportStage(t, s)
	start := time.Now()
	response, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err == nil {
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil || response.StatusCode < 400 {
			t.Fatal("预算耗尽仍确认", response.StatusCode, string(body), readErr)
		}
	} else if err != io.EOF && err != io.ErrUnexpectedEOF {
		t.Fatal("预算终止不是实际关闭", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("上传扩大ordinary预算", time.Since(start))
	}
	node, err := st.AuthenticateNode(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.ReportUploadBudget(context.Background(), node, d.Ref); err == nil {
		t.Fatal("已耗尽的共同预算恢复")
	}
	if _, err = st.FindNodeArtifact(context.Background(), node, d.Ref, d.ID); err == nil {
		t.Fatal("部分XML获得回执")
	}
	entries, err := os.ReadDir(filepath.Join(s.config.DataDir, "artifacts"))
	if err != nil || len(entries) != 0 {
		t.Fatal("超时stage未清理", entries, err)
	}
}

// 短body必须拒绝；不能以正确声明或错误连接结束冒充完整字节。
func TestReportHTTPShortBodyNeverCommits(t *testing.T) {
	data := []byte(`<testsuite tests="1"><testcase/></testsuite>`)
	_, st, api, token, d, _ := reportHTTPFixture(t, data, nil)
	r, err := http.NewRequest("PUT", api.URL+"/api/agent/artifacts/"+d.ID, bytes.NewReader(data[:5]))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("X-Mybuilds-Artifact", base64.RawURLEncoding.EncodeToString([]byte(encodeMessage(t, d))))
	response, err := api.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if response.StatusCode != 400 {
		t.Fatal("短XML被接受", response.StatusCode)
	}
	node, err := st.AuthenticateNode(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.FindNodeArtifact(context.Background(), node, d.Ref, d.ID); err == nil {
		t.Fatal("短XML获得可信回执")
	}
}
