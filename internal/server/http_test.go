package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mybuilds/internal/config"
	"mybuilds/internal/store"
)

const adminToken = "PRIVATE_SERVER_BOOTSTRAP_MARKER_0123456789"

func serverFixture(t *testing.T) (*Server, *store.Store, *httptest.Server) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(context.Background(), store.Options{Driver: "sqlite", DSN: filepath.Join(dir, "control.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Error(err)
		}
	})
	if err = st.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = st.Bootstrap(context.Background(), adminToken); err != nil {
		t.Fatal(err)
	}
	s := New(st, config.ServerConfig{Retention: config.Retention{Builds: 100, Days: 30}, DataDir: dir, Concurrency: 3, HeartbeatInterval: 5 * time.Second, LeaseDuration: 30 * time.Second, Listen: "127.0.0.1:8787"})
	if err = st.SyncGlobalRetention(context.Background(), s.config.Retention); err != nil {
		t.Fatal(err)
	}
	h := httptest.NewServer(s.Handler())
	t.Cleanup(h.Close)
	return s, st, h
}
func request(t *testing.T, h *httptest.Server, method, path, token, body string, key ...string) (int, string) {
	t.Helper()
	r, err := http.NewRequest(method, h.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", "application/json")
	if len(key) > 0 {
		r.Header.Set("Idempotency-Key", key[0])
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := h.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("非安全JSON响应: %s", b)
	}
	return res.StatusCode, string(b)
}
func TestActualHTTPAuthenticationStatusAndRevocation(t *testing.T) {
	_, st, h := serverFixture(t)
	for _, token := range []string{"", "WRONG_TOKEN_PRIVATE_MARKER_0123456789"} {
		code, body := request(t, h, "GET", "/api/status", token, "")
		if code != 401 || strings.Contains(body, "PRIVATE") {
			t.Fatalf("鉴权: %d %s", code, body)
		}
	}
	code, body := request(t, h, "GET", "/api/status", adminToken, "")
	var status StatusDTO
	if err := json.Unmarshal([]byte(body), &status); err != nil {
		t.Fatal(err)
	}
	if code != 200 || status.Concurrency != 3 || status.Queued != 0 || status.Running != 0 || status.Nodes != 0 {
		t.Fatalf("状态非真实: %d %s", code, body)
	}
	actor, err := st.Authenticate(context.Background(), adminToken)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"trigger", "approver"} {
		token, err := st.CreateToken(context.Background(), actor, role)
		if err != nil {
			t.Fatal(err)
		}
		code, _ = request(t, h, "GET", "/api/status", token.Token, "")
		if code != 200 {
			t.Fatal(code)
		}
		code, body = request(t, h, "POST", "/api/groups", token.Token, `{"name":"forbidden"}`)
		if code != 403 || strings.Contains(body, token.Token) {
			t.Fatalf("管理越权: %d %s", code, body)
		}
		if err := st.RevokeToken(context.Background(), actor, token.ID); err != nil {
			t.Fatal(err)
		}
		code, _ = request(t, h, "GET", "/api/status", token.Token, "")
		if code != 401 {
			t.Fatal(code)
		}
	}
	code, body = request(t, h, "GET", "/api/not-implemented", adminToken, "")
	if code != 404 || !strings.Contains(body, "not_found") {
		t.Fatalf("未来路径: %d %s", code, body)
	}
}
func TestActualHTTPStrictJSONQueryAndGroupCRUD(t *testing.T) {
	_, st, h := serverFixture(t)
	for _, body := range []string{`{"name":"x","name":"y"}`, `{"name":null}`, `{"name":1}`, `{"name":"x","unknown":"PRIVATE_MARKER"}`, `{"name":"x"} {}`, `[]`, `{"Name":"x"}`} {
		code, out := request(t, h, "POST", "/api/groups", adminToken, body)
		if code != 400 || strings.Contains(out, "PRIVATE_MARKER") {
			t.Fatalf("错误请求被接受: %d %s", code, out)
		}
	}
	code, _ := request(t, h, "POST", "/api/groups", adminToken, `{"name":"team"}`)
	if code != 201 {
		t.Fatal(code)
	}
	code, _ = request(t, h, "PATCH", "/api/groups/team", adminToken, `{"name":"renamed"}`)
	if code != 200 {
		t.Fatal(code)
	}
	code, body := request(t, h, "GET", "/api/groups?limit=1&offset=0", adminToken, "")
	if code != 200 || !strings.Contains(body, "renamed") {
		t.Fatalf("列表: %d %s", code, body)
	}
	for _, query := range []string{"limit=0", "limit=201", "offset=-1", "offset=1000001", "limit=1&limit=2", "unknown=PRIVATE_MARKER"} {
		code, body = request(t, h, "GET", "/api/groups?"+query, adminToken, "")
		if code != 400 || strings.Contains(body, "PRIVATE_MARKER") {
			t.Fatal(code, body)
		}
	}
	code, _ = request(t, h, "DELETE", "/api/groups/renamed", adminToken, "")
	if code != 204 {
		t.Fatal(code)
	}
	groups, err := st.ListGroups(context.Background(), store.Page{})
	if err != nil || len(groups) != 1 || groups[0].Name != "default" {
		t.Fatalf("真实CRUD: %v %v", groups, err)
	}
	code, _ = request(t, h, "POST", "/api/groups", adminToken, `{"name":"`+strings.Repeat("x", 1<<20)+`"}`)
	if code != 413 {
		t.Fatal(code)
	}
}
func TestActualHTTPProjectSafeViewAndPatch(t *testing.T) {
	_, _, h := serverFixture(t)
	code, body := request(t, h, "POST", "/api/projects", adminToken, `{"name":"app","repo":"/tmp/PRIVATE_REPOSITORY_MARKER","nodes":["linux"],"settings":{"pipeline":{"params":{"version":"PRIVATE_PARAMETER_MARKER"}}}}`)
	if code != 201 || strings.Contains(body, "PRIVATE_") {
		t.Fatalf("项目公开视图: %d %s", code, body)
	}
	var view ProjectView
	if err := json.Unmarshal([]byte(body), &view); err != nil {
		t.Fatal(err)
	}
	if view.Name != "app" || view.Group != "default" || view.PipelineFile != "mybuilds.yml" || view.NextNumber != 1 {
		t.Fatal(body)
	}
	for _, bad := range []string{`{"group":"default","settings":{}}`, `{}`, `{"settings":{"pipeline":{"unknown":"PRIVATE_MARKER"}}}`, `{"settings":{"pipeline":{"params":{"version":null}}}}`} {
		code, body = request(t, h, "PATCH", "/api/projects/app", adminToken, bad)
		if code != 400 || strings.Contains(body, "PRIVATE_MARKER") {
			t.Fatal(code, body)
		}
	}
	code, _ = request(t, h, "PATCH", "/api/projects/app", adminToken, `{"settings":{"pipeline":{"source":"repo","file":"build.yml"}}}`)
	if code != 200 {
		t.Fatal(code)
	}
	code, body = request(t, h, "GET", "/api/projects", adminToken, "")
	if code != 200 || !strings.Contains(body, "build.yml") || strings.Contains(body, "PRIVATE_") {
		t.Fatal(code, body)
	}
}
func TestActualHTTPRejectAfterLockReplacement(t *testing.T) {
	s, _, h := serverFixture(t)
	lock := filepath.Join(s.config.DataDir, "control.db.lock")
	if err := os.Rename(lock, lock+".owned-old"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lock, []byte("owned replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	code, body := request(t, h, "POST", "/api/groups", adminToken, `{"name":"never-written"}`)
	if code != 503 || !strings.Contains(body, "control_lock_lost") {
		t.Fatal(code, body)
	}
}

func TestActualHTTPTokenOneTimeOutputAndStrictHeaders(t *testing.T) {
	_, _, h := serverFixture(t)
	code, body := request(t, h, "POST", "/api/tokens", adminToken, `{"role":"approver"}`)
	if code != 201 {
		t.Fatal(code, body)
	}
	var token store.TokenCreated
	if err := json.Unmarshal([]byte(body), &token); err != nil || token.Token == "" || token.ID == "" {
		t.Fatal(body, err)
	}
	code, body = request(t, h, "GET", "/api/tokens", adminToken, "")
	if code != 200 || strings.Contains(body, token.Token) || strings.Contains(body, adminToken) || strings.Contains(body, "Digest") || strings.Contains(body, "digest") {
		t.Fatal(code, body)
	}
	code, body = request(t, h, "DELETE", "/api/tokens/"+token.ID, adminToken, "")
	if code != 204 {
		t.Fatal(code, body)
	}
	code, _ = request(t, h, "GET", "/api/status", token.Token, "")
	if code != 401 {
		t.Fatal(code)
	}
	for _, input := range []struct {
		auth              []string
		media, body, path string
		want              int
	}{
		{[]string{"Bearer " + adminToken, "Bearer " + adminToken}, "application/json", "", "/api/status", 401},
		{[]string{"bearer " + adminToken}, "application/json", "", "/api/status", 401},
		{[]string{"Bearer " + adminToken}, "text/plain", `{"name":"team"}`, "/api/groups", 400},
		{[]string{"Bearer " + adminToken}, "application/json", `{"role":null}`, "/api/tokens", 400},
		{[]string{"Bearer " + adminToken}, "application/json", `{"role":"unknown_PRIVATE"}`, "/api/tokens", 400},
	} {
		method := "POST"
		if input.path == "/api/status" {
			method = "GET"
		}
		r, err := http.NewRequest(method, h.URL+input.path, strings.NewReader(input.body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header["Authorization"] = input.auth
		r.Header.Set("Content-Type", input.media)
		response, err := h.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		output, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != input.want || strings.Contains(string(output), "PRIVATE") {
			t.Fatal(response.StatusCode, string(output))
		}
	}
	for _, path := range []string{"/api/status?limit=1", "/api/builds?status=unknown_PRIVATE", "/api/groups?offset=1;limit=2", "/api/projects?group=x&group=y"} {
		code, body = request(t, h, "GET", path, adminToken, "")
		if code != 400 || strings.Contains(body, "PRIVATE") {
			t.Fatal(code, body)
		}
	}
}

func TestActualHTTPProjectInitialNumberMissingAndExplicitNonpositive(t *testing.T) {
	_, st, h := serverFixture(t)
	code, body := request(t, h, "POST", "/api/projects", adminToken, `{"name":"default-number","repo":"/tmp/PRIVATE_REPOSITORY_MARKER","nodes":["linux"]}`)
	if code != 201 {
		t.Fatal(code, body)
	}
	var view ProjectView
	if err := json.Unmarshal([]byte(body), &view); err != nil {
		t.Fatal(err)
	}
	if view.NextNumber != 1 {
		t.Fatal("缺省编号应为1", body)
	}
	for _, input := range []struct{ name, value string }{{"explicit-zero", "0"}, {"explicit-negative", "-1"}} {
		code, body = request(t, h, "POST", "/api/projects", adminToken, `{"name":"`+input.name+`","repo":"/tmp/PRIVATE_REPOSITORY_MARKER","nodes":["linux"],"build_number_start":`+input.value+`}`)
		if code != 400 || !strings.Contains(body, "invalid_request") || strings.Contains(body, "PRIVATE_REPOSITORY_MARKER") {
			t.Errorf("显式非正初始号必须拒绝: %d %s", code, body)
		}
		if _, err := st.GetProject(context.Background(), input.name); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("非正初始号创建了项目/计数器: %v", err)
		}
	}
	projects, err := st.ListProjects(context.Background(), store.ProjectFilter{})
	if err != nil || len(projects) != 1 || projects[0].NextNumber != 1 {
		t.Fatalf("拒绝后项目/计数器变化: %+v %v", projects, err)
	}
	status, err := st.Status(context.Background())
	if err != nil || status.Projects != 1 || status.Queued != 0 || status.Skipped != 0 {
		t.Fatal(status, err)
	}
}
