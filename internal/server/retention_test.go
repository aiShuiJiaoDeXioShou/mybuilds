package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
)

func retentionHTTPFixture(t *testing.T) (*Server, *store.Store, *httptest.Server) {
	t.Helper()
	s, st, h := serverFixture(t)
	// Handler夹具明确调用真实初始化；查询本身不能懒同步配置。
	if err := st.SyncGlobalRetention(context.Background(), config.Retention{Builds: 100, Days: 30}); err != nil {
		t.Fatal(err)
	}
	code, body := request(t, h, "POST", "/api/projects", adminToken, `{"name":"retained","repo":"/tmp/PRIVATE_RETENTION_REPOSITORY","nodes":["linux"],"settings":{"pipeline":{"params":{"channel":"PRIVATE_RETENTION_PARAMETER"}}}}`)
	if code != 201 {
		t.Fatal(code, body)
	}
	return s, st, h
}

// 读取自有夹具的真实记录，只观察事务结果，不写业务行绕过登记入口。
func retentionResourceRows(t *testing.T, s *Server) string {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(s.config.DataDir, "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query("SELECT id, build_id, attempt_id, node_id, session_id, lease_id, epoch, ownership_digest, has_workspace, has_results, terminal_digest, terminal_seq, CAST(registered_at AS TEXT), COALESCE(CAST(completed_at AS TEXT),''), completion_json FROM node_resources ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	result := [][]any{}
	for rows.Next() {
		var id, build, attempt, node, session, lease, digest, terminal, at, completedAt, completion string
		var epoch, seq int64
		var workspace, results bool
		if err = rows.Scan(&id, &build, &attempt, &node, &session, &lease, &epoch, &digest, &workspace, &results, &terminal, &seq, &at, &completedAt, &completion); err != nil {
			t.Fatal(err)
		}
		result = append(result, []any{id, build, attempt, node, session, lease, epoch, digest, workspace, results, terminal, seq, at, completedAt, completion})
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return encodeMessage(t, result)
}

func TestRetentionResourceHTTPActualRegistrationAndTerminalReadOnly(t *testing.T) {
	s, st, h, token, claim := executionHTTPFixture(t)
	grant := claimHTTP(t, h, token, claim)
	registration := protocol.NodeResourceRegistration{Ref: grant.Ref, ID: uuid.NewString(), OwnershipDigest: strings.Repeat("a", 64), HasWorkspace: true}
	code, body := request(t, h, "POST", "/api/agent/resources", token, encodeMessage(t, registration))
	if code != 204 || body != "" {
		t.Fatal("实际资源登记失败", code, body)
	}
	before := retentionResourceRows(t, s)
	code, body = request(t, h, "POST", "/api/agent/resources", token, encodeMessage(t, registration))
	if code != 204 || retentionResourceRows(t, s) != before {
		t.Fatal("原登记重放修改归属/时间", code, body)
	}
	registration.HasResults = true
	registration.OwnershipDigest = strings.Repeat("b", 64)
	code, body = request(t, h, "POST", "/api/agent/resources", token, encodeMessage(t, registration))
	if code != 204 {
		t.Fatal("真实结果槽扩展失败", code, body)
	}
	step := protocol.ExecutionProgress{Kind: "intent", Phase: "ordinary", Index: 1, Name: "shell", StepKind: "run", At: time.Now().UTC(), RemainingPostBudgetNS: grant.RemainingPostBudgetNS, ArtifactSteps: []protocol.ArtifactExpectation{}}
	sequence := []protocol.ExecutionProgress{step}
	step.Kind = "started"
	step.Started = true
	sequence = append(sequence, step)
	step.Kind = "finished"
	step.Status = "succeeded"
	step.StopConfirmed = true
	sequence = append(sequence, step)
	sequence = append(sequence, protocol.ExecutionProgress{Kind: "post_selected", PostPhase: "success", At: time.Now().UTC(), RemainingPostBudgetNS: grant.RemainingPostBudgetNS, ArtifactSteps: []protocol.ArtifactExpectation{}}, protocol.ExecutionProgress{Kind: "build_finished", Status: "succeeded", Started: true, StopConfirmed: true, At: time.Now().UTC(), RemainingPostBudgetNS: grant.RemainingPostBudgetNS, ArtifactSteps: []protocol.ArtifactExpectation{}})
	for i, progress := range sequence {
		event := protocol.ExecutionEvent{Ref: grant.Ref, Seq: int64(i + 1), Digest: messageDigest(t, progress), Progress: progress}
		code, body = request(t, h, "POST", "/api/agent/events", token, encodeMessage(t, event))
		if code != 200 {
			t.Fatal("真实终态失败", i, code, body)
		}
	}
	admin, err := st.Authenticate(context.Background(), adminToken)
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := st.RotateNodeToken(context.Background(), admin, "worker")
	if err != nil {
		t.Fatal(err)
	}
	old, err := st.GetBuild(context.Background(), grant.Ref.BuildID)
	if err != nil {
		t.Fatal(err)
	}
	before = retentionResourceRows(t, s)
	for i := 0; i < 2; i++ {
		code, body = request(t, h, "POST", "/api/agent/resources", rotated.Token, encodeMessage(t, registration))
		if code != 204 || body != "" || retentionResourceRows(t, s) != before {
			t.Fatal("当前凭据未只读确认原终态登记", code, body)
		}
	}
	current, err := st.GetBuild(context.Background(), grant.Ref.BuildID)
	if err != nil || !reflect.DeepEqual(current, old) {
		t.Fatal("登记只读确认复活执行权/修改终态", err)
	}
	wrong := registration
	wrong.ID = uuid.NewString()
	code, _ = request(t, h, "POST", "/api/agent/resources", rotated.Token, encodeMessage(t, wrong))
	if code != 409 {
		t.Fatal("终态新建资源", code)
	}
	wrong = registration
	wrong.OwnershipDigest = strings.Repeat("c", 64)
	code, _ = request(t, h, "POST", "/api/agent/resources", rotated.Token, encodeMessage(t, wrong))
	if code != 409 {
		t.Fatal("终态修改归属", code)
	}
	code, _ = request(t, h, "POST", "/api/agent/resources", token, encodeMessage(t, registration))
	if code != 401 {
		t.Fatal("已撤销旧凭据仍能登记", code)
	}
	if retentionResourceRows(t, s) != before {
		t.Fatal("拒绝请求修改原资源记录")
	}
}

func TestRetentionResourceHTTPStrictBodyRolesAndFence(t *testing.T) {
	s, st, h, token, claim := executionHTTPFixture(t)
	grant := claimHTTP(t, h, token, claim)
	registration := protocol.NodeResourceRegistration{Ref: grant.Ref, ID: uuid.NewString(), OwnershipDigest: strings.Repeat("a", 64), HasWorkspace: true}
	before := retentionResourceRows(t, s)
	encoded := encodeMessage(t, registration)
	for _, body := range []string{
		`{"ref":null}`, strings.Replace(encoded, `"has_workspace":true`, `"has_workspace":null`, 1),
		strings.Replace(encoded, `"has_workspace":true`, `"has_workspace":true,"has_workspace":false`, 1),
		strings.TrimSuffix(encoded, "}") + `,"path":"PRIVATE_RESOURCE_PATH"}`,
		strings.Replace(encoded, `"has_workspace":true`, `"HasWorkspace":true`, 1),
		strings.TrimSuffix(encoded, "}") + `,"unknown":"` + strings.Repeat("x", 32*1024) + `"}`,
	} {
		code, out := request(t, h, "POST", "/api/agent/resources", token, body)
		if code != 400 && code != 413 || strings.Contains(out, "PRIVATE_RESOURCE_PATH") {
			t.Fatal("弱或超限JSON未拒绝", code, out)
		}
		if retentionResourceRows(t, s) != before {
			t.Fatal("非法body写入资源")
		}
	}
	code, body := request(t, h, "POST", "/api/agent/resources?unknown=PRIVATE_VALUE", token, encoded)
	if code != 400 || strings.Contains(body, "PRIVATE_VALUE") {
		t.Fatal("资源登记接受query", code, body)
	}
	admin, err := st.Authenticate(context.Background(), adminToken)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"admin", "trigger", "approver"} {
		userToken := adminToken
		if role != "admin" {
			created, err := st.CreateToken(context.Background(), admin, role)
			if err != nil {
				t.Fatal(err)
			}
			userToken = created.Token
		}
		code, _ := request(t, h, "POST", "/api/agent/resources", userToken, encoded)
		if code != 401 {
			t.Fatal("用户token获得节点登记权", role, code)
		}
	}
	other, err := st.CreateNode(context.Background(), admin, store.NodeInput{Name: "resource-outsider", Capacity: 1})
	if err != nil {
		t.Fatal(err)
	}
	code, _ = request(t, h, "POST", "/api/agent/resources", other.Token, encoded)
	if code != 401 {
		t.Fatal("别node登记原资源", code)
	}
	wrong := registration
	wrong.Ref.Epoch++
	code, _ = request(t, h, "POST", "/api/agent/resources", token, encodeMessage(t, wrong))
	if code != 409 {
		t.Fatal("旧fence登记资源", code)
	}
	if retentionResourceRows(t, s) != before {
		t.Fatal("拒绝请求修改资源")
	}
}

func retentionHTTPView(t *testing.T, h *httptest.Server) store.EffectiveRetention {
	t.Helper()
	code, body := request(t, h, "GET", "/api/projects/retained/retention", adminToken, "")
	if code != 200 {
		t.Fatal(code, body)
	}
	var view store.EffectiveRetention
	if err := json.Unmarshal([]byte(body), &view); err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &fields); err != nil || len(fields) != 6 || strings.Contains(body, "PRIVATE_") {
		t.Fatal("策略公开字段不安全", body, err)
	}
	return view
}

func TestRetentionHTTPPolicyAndIndependentSettings(t *testing.T) {
	s, st, h := retentionHTTPFixture(t)
	original, err := st.GetProject(context.Background(), "retained")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		body                     string
		builds, days             int64
		buildsSource, daysSource string
	}{
		{`{"settings":{"retention":{"builds":200}}}`, 200, 30, "project", "global"},
		{`{"settings":{"retention":{"days":45}}}`, 100, 45, "global", "project"},
		{`{"settings":{"retention":{"builds":200,"days":45}}}`, 200, 45, "project", "project"},
		{`{"settings":{"retention":{}}}`, 100, 30, "global", "global"},
		{`{"settings":{}}`, 100, 30, "global", "global"},
	} {
		code, body := request(t, h, "PATCH", "/api/projects/retained", adminToken, item.body)
		if code != 200 {
			t.Fatal(code, body)
		}
		view := retentionHTTPView(t, h)
		if view.Builds != item.builds || view.Days != item.days || view.BuildsSource != item.buildsSource || view.DaysSource != item.daysSource || view.GlobalVersion != 1 || view.ProjectVersion < 2 {
			t.Fatal("有效策略错误", view)
		}
		current, err := st.GetProject(context.Background(), "retained")
		if err != nil || !reflect.DeepEqual(current.Settings.Pipeline, original.Settings.Pipeline) {
			t.Fatal("retention导入重置原pipeline", err)
		}
	}
	before := retentionHTTPView(t, h)
	s.config.Retention = config.Retention{Builds: 9, Days: 2}
	if after := retentionHTTPView(t, h); after != before {
		t.Fatal("GET触发了热同步", before, after)
	}
}

func TestRetentionHTTPWholeSettingsRejection(t *testing.T) {
	s, st, h := retentionHTTPFixture(t)
	code, body := request(t, h, "PATCH", "/api/projects/retained", adminToken, `{"settings":{"retention":{"builds":200,"days":45}}}`)
	if code != 200 {
		t.Fatal(code, body)
	}
	before, err := st.GetProject(context.Background(), "retained")
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(s.config.DataDir, "unregistered-private-evidence")
	if err := os.WriteFile(marker, []byte("UNCHANGED"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{
		`{"settings":{"retention":null}}`, `{"settings":{"retention":{"builds":null}}}`, `{"settings":{"retention":{"builds":0}}}`, `{"settings":{"retention":{"days":106752}}}`, `{"settings":{"retention":{"builds":1.5}}}`, `{"settings":{"retention":{"builds":1.0}}}`, `{"settings":{"retention":{"builds":"200"}}}`, `{"settings":{"retention":{"builds":9223372036854775808}}}`,
		`{"settings":{"retention":{"builds":200,"builds":300}}}`, `{"settings":{"retention":{"Builds":200}}}`, `{"settings":{"retention":{"unknown":"PRIVATE_RETENTION_MARKER"}}}`, `{"settings":{"retention":{"builds":300,"days":0},"pipeline":{"params":{"channel":"PRIVATE_REPLACEMENT"}}}}`,
	} {
		code, body := request(t, h, "PATCH", "/api/projects/retained", adminToken, input)
		if code != 400 || strings.Contains(body, "PRIVATE_") {
			t.Fatal("非法整块未安全拒绝", code, body)
		}
		after, err := st.GetProject(context.Background(), "retained")
		if err != nil || !reflect.DeepEqual(before, after) {
			t.Fatal("非法导入发生部分写入", err)
		}
		if data, err := os.ReadFile(marker); err != nil || string(data) != "UNCHANGED" {
			t.Fatal("只读管理动了证据", err)
		}
	}
}

func TestRetentionHTTPRoleBeforeProjectLookup(t *testing.T) {
	_, st, h := retentionHTTPFixture(t)
	admin, err := st.Authenticate(context.Background(), adminToken)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"trigger", "approver"} {
		created, err := st.CreateToken(context.Background(), admin, role)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"retained", "nonexistent"} {
			code, body := request(t, h, "GET", "/api/projects/"+name+"/retention", created.Token, "")
			if code != 403 || strings.Contains(body, name) {
				t.Fatal("鉴权前泄露项目存在性", role, code, body)
			}
			code, body = request(t, h, "PATCH", "/api/projects/"+name, created.Token, `{"settings":{"retention":{"builds":1}}}`)
			if code != 403 {
				t.Fatal("非管理员管理策略", role, code, body)
			}
		}
	}
	node, err := st.CreateNode(context.Background(), admin, store.NodeInput{Name: "retention-node", Capacity: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"retained", "nonexistent"} {
		code, body := request(t, h, "GET", "/api/projects/"+name+"/retention", node.Token, "")
		if code != 401 || strings.Contains(body, node.Token) {
			t.Fatal("节点访问管理策略", code, body)
		}
	}
	for _, query := range []string{"limit=1", "unknown=PRIVATE_VALUE", "x=1&x=2"} {
		code, body := request(t, h, "GET", "/api/projects/retained/retention?"+query, adminToken, "")
		if code != 400 || strings.Contains(body, "PRIVATE_") {
			t.Fatal("未知/重复query未拒", code, body)
		}
	}
}

func TestRetentionActualServeSynchronizesBeforeListen(t *testing.T) {
	s, st, _ := retentionHTTPFixture(t)
	s.config.Retention = config.Retention{Builds: 200, Days: 45}
	reservation, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s.config.Listen = reservation.Addr().String()
	reservation.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.ListenAndServe(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("自有服务未停止")
		}
	})
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(3 * time.Second)
	for {
		r, _ := http.NewRequest("GET", "http://"+s.config.Listen+"/api/status", nil)
		r.Header.Set("Authorization", "Bearer "+adminToken)
		response, err := client.Do(r)
		if err == nil {
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if response.StatusCode != 200 {
				t.Fatal(response.StatusCode)
			}
			break
		}
		select {
		case err := <-done:
			t.Fatal("启动失败", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("真实监听未启动")
		}
		time.Sleep(10 * time.Millisecond)
	}
	actor, err := st.Authenticate(context.Background(), adminToken)
	if err != nil {
		t.Fatal(err)
	}
	project, err := st.GetProject(context.Background(), "retained")
	if err != nil {
		t.Fatal(err)
	}
	view, err := st.EffectiveRetention(context.Background(), actor, project.ID)
	if err != nil || view.Builds != 200 || view.Days != 45 || view.GlobalVersion != 2 {
		t.Fatal("监听时策略未完成受控同步", view, err)
	}
}

func TestRetentionInvalidStartupDoesNotListen(t *testing.T) {
	s, _, _ := retentionHTTPFixture(t)
	s.config.Retention = config.Retention{Builds: 0, Days: 30}
	reservation, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s.config.Listen = reservation.Addr().String()
	reservation.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	if err = s.ListenAndServe(ctx); !errors.Is(err, store.ErrRetentionInvalid) {
		t.Fatal("非法启动配置没有明确失败", err)
	}
	connection, err := net.DialTimeout("tcp", s.config.Listen, 100*time.Millisecond)
	if err == nil {
		connection.Close()
		t.Fatal("非法配置仍在监听")
	}
}

// 通过真实入队与取消产生可信终态；不通过数据库伪造时间或清理事项。
func retentionCandidateBuilds(t *testing.T, st *store.Store, actor store.Actor, project store.Project) []store.BuildView {
	t.Helper()
	result := []store.BuildView{}
	for _, name := range []string{"first", "second", "active"} {
		out, err := st.Enqueue(context.Background(), store.EnqueueInput{Actor: actor, ProjectID: project.ID, ProjectVersion: project.PolicyVersion, Key: name, RequestDigest: strings.Repeat("a", 64), SHA: strings.Repeat("b", 40), Branch: "main", Source: "repo", File: "mybuilds.yml", SourceDigest: strings.Repeat("c", 64), Builds: []store.PreparedBuild{{Name: name, Status: "queued", Snapshot: store.BuildSnapshot{Definition: config.Build{Params: map[string]config.Parameter{"channel": {}}, Steps: []config.Step{{Name: "shell", Kind: "run", Run: "echo PRIVATE_CANDIDATE_SCRIPT"}}}, Params: map[string]string{"channel": "PRIVATE_CANDIDATE_PARAM"}, Facts: map[string]string{}, Condition: "ready", AllowedNodes: project.AllowedNodes, DefaultNode: project.DefaultNode}, PostBudgetNS: int64(2 * time.Minute), Steps: []store.StepProgress{{Phase: "ordinary", Index: 1, Name: "shell", Kind: "run", Condition: "ready", Status: "pending"}}}}})
		if err != nil {
			t.Fatal(err)
		}
		build := out.Builds[0]
		if name != "active" {
			build, err = st.Cancel(context.Background(), actor, build.ID)
			if err != nil {
				t.Fatal(err)
			}
		}
		result = append(result, build)
	}
	return result
}

func TestRetentionCandidatesHTTPWholePageAndReadOnly(t *testing.T) {
	_, st, h := retentionHTTPFixture(t)
	actor, err := st.Authenticate(context.Background(), adminToken)
	if err != nil {
		t.Fatal(err)
	}
	project, err := st.GetProject(context.Background(), "retained")
	if err != nil {
		t.Fatal(err)
	}
	if err = st.SyncGlobalRetention(context.Background(), config.Retention{Builds: 1, Days: 30}); err != nil {
		t.Fatal(err)
	}
	builds := retentionCandidateBuilds(t, st, actor, project)
	for _, page := range []store.Page{{Limit: 20}, {Limit: 1}, {Limit: 1, Offset: 1}, {Limit: 200}, {Limit: 20, Offset: 1000000}} {
		expected, err := st.EvaluateRetention(context.Background(), actor, project.ID, page)
		if err != nil {
			t.Fatal(err)
		}
		path := "/api/projects/retained/retention/candidates?limit=" + strconv.Itoa(page.Limit) + "&offset=" + strconv.Itoa(page.Offset)
		code, body := request(t, h, "GET", path, adminToken, "")
		if code != 200 || strings.Contains(body, "PRIVATE_") || strings.Contains(body, "ownership_digest") || strings.Contains(body, "storage_id") {
			t.Fatal("候选公开页失败或泄露", code, body)
		}
		var actual store.RetentionPage
		if err = json.Unmarshal([]byte(body), &actual); err != nil || actual.Items == nil {
			t.Fatal("无效候选页", body, err)
		}
		for i := range expected.Items {
			expected.Items[i].EvaluatedAt = nil
		}
		for i := range actual.Items {
			if actual.Items[i].EvaluatedAt == nil || actual.Items[i].ProtectReasons == nil {
				t.Fatal("缺少评估时间或保护数组")
			}
			actual.Items[i].EvaluatedAt = nil
		}
		if !reflect.DeepEqual(actual, expected) {
			t.Fatal("HTTP过滤/改变真实Store页", actual, expected)
		}
	}
	code, body := request(t, h, "GET", "/api/projects/retained/retention/candidates", adminToken, "")
	var all store.RetentionPage
	if code != 200 || json.Unmarshal([]byte(body), &all) != nil || len(all.Items) != 3 || all.Limit != 20 {
		t.Fatal("默认整页错误", code, body)
	}
	candidates, protected := 0, 0
	for _, entry := range all.Items {
		if entry.Candidate {
			candidates++
		}
		if len(entry.ProtectReasons) > 0 {
			protected++
		}
	}
	if candidates != 1 || protected == 0 {
		t.Fatal("候选或活动保护被丢弃", all)
	}
	for _, before := range builds {
		after, err := st.GetBuild(context.Background(), before.ID)
		if err != nil || !reflect.DeepEqual(before, after) {
			t.Fatal("评估修改原构建", err)
		}
	}
	code, body = request(t, h, "POST", "/api/projects", adminToken, `{"name":"other-retention","repo":"/tmp/PRIVATE_OTHER_REPO","nodes":["linux"]}`)
	if code != 201 {
		t.Fatal(code, body)
	}
	code, body = request(t, h, "GET", "/api/projects/other-retention/retention/candidates", adminToken, "")
	var empty store.RetentionPage
	if code != 200 || json.Unmarshal([]byte(body), &empty) != nil || empty.Items == nil || len(empty.Items) != 0 {
		t.Fatal("跨项目候选泄露", code, body)
	}
}

func TestRetentionCandidatesHTTPQueryAndRoleBoundaries(t *testing.T) {
	_, st, h := retentionHTTPFixture(t)
	for _, q := range []string{"limit=0", "limit=201", "limit=", "limit=1&limit=2", "limit=999999999999999999999", "offset=-1", "offset=1000001", "offset=", "offset=x", "unknown=PRIVATE_QUERY", "limit=%zz"} {
		code, body := request(t, h, "GET", "/api/projects/retained/retention/candidates?"+q, adminToken, "")
		if code != 400 || strings.Contains(body, "PRIVATE_QUERY") {
			t.Fatal("非法分页未拒绝", q, code, body)
		}
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
		for _, name := range []string{"retained", "missing-private-project"} {
			code, body := request(t, h, "GET", "/api/projects/"+name+"/retention/candidates", token.Token, "")
			if code != 403 || strings.Contains(body, name) {
				t.Fatal("候选权限未先于存在性", code, body)
			}
		}
	}
	node, err := st.CreateNode(context.Background(), actor, store.NodeInput{Name: "candidate-node", Capacity: 1})
	if err != nil {
		t.Fatal(err)
	}
	code, _ := request(t, h, "GET", "/api/projects/retained/retention/candidates", node.Token, "")
	if code != 401 {
		t.Fatal("节点获得管理查询权", code)
	}
	code, _ = request(t, h, "POST", "/api/projects/retained/retention/candidates", adminToken, "")
	if code != 404 {
		t.Fatal("候选查询接受写方法", code)
	}
}

func retentionCompletionFixture(t *testing.T, registered, adminStop bool) (*Server, *store.Store, *httptest.Server, string, protocol.NodeResourceRegistration) {
	t.Helper()
	s, st, h, token, claim := executionHTTPFixture(t)
	grant := claimHTTP(t, h, token, claim)
	in := protocol.NodeResourceRegistration{Ref: grant.Ref, ID: uuid.NewString(), OwnershipDigest: strings.Repeat("a", 64), HasWorkspace: true}
	if registered {
		code, body := request(t, h, "POST", "/api/agent/resources", token, encodeMessage(t, in))
		if code != 204 {
			t.Fatal(code, body)
		}
	}
	for _, state := range []string{"disable", "enable"} {
		code, body := request(t, h, "POST", "/api/nodes/worker/"+state, adminToken, "")
		if code != 204 {
			t.Fatal(code, body)
		}
	}
	confirmation := protocol.StopConfirmation{Ref: grant.Ref, EvidenceCode: "process_group_reaped", Note: "本HTTP夹具没有启动用户动作，精确原执行已停止"}
	actor, path := token, "/api/agent/stop-confirmation"
	if adminStop {
		confirmation.EvidenceCode = "admin_observed_stopped"
		actor = adminToken
		path = "/api/builds/" + grant.Ref.BuildID + "/stop-confirmation"
	}
	code, body := request(t, h, "POST", path, actor, encodeMessage(t, confirmation))
	if code != 204 {
		t.Fatal(code, body)
	}
	in.Completion = &protocol.NodeResourceCompletion{StopCode: "process_group_reaped"}
	return s, st, h, token, in
}

func TestRetentionResourceCompletionHTTPRequiredFields(t *testing.T) {
	_, _, h, token, in := retentionCompletionFixture(t, true, false)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(encodeMessage(t, in)), &raw); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"last_event_seq", "last_log_seq", "last_log_offset", "last_artifact_seq", "stop_code"} {
		t.Run(field, func(t *testing.T) {
			var c map[string]json.RawMessage
			if err := json.Unmarshal(raw["completion"], &c); err != nil {
				t.Fatal(err)
			}
			delete(c, field)
			nested, _ := json.Marshal(c)
			raw["completion"] = nested
			code, body := request(t, h, "POST", "/api/agent/resources", token, encodeMessage(t, raw))
			if code != 400 {
				t.Fatal("省略完成字段被零值接受", field, code, body)
			}
			raw["completion"] = json.RawMessage(encodeMessage(t, in.Completion))
		})
	}
}

func TestRetentionResourceCompletionHTTPPreciseStopReplayAndBoundaries(t *testing.T) {
	s, st, h, token, in := retentionCompletionFixture(t, true, false)
	before, err := st.GetBuild(context.Background(), in.Ref.BuildID)
	if err != nil {
		t.Fatal(err)
	}
	uncompletedState := retentionResourceRows(t, s)
	for _, body := range []string{
		strings.Replace(encodeMessage(t, in), `"last_event_seq":0`, `"last_event_seq":null`, 1),
		strings.Replace(encodeMessage(t, in), `"last_event_seq":0`, `"last_event_seq":0,"last_event_seq":0`, 1),
		strings.Replace(encodeMessage(t, in), `"last_event_seq":0`, `"last_event_seq":0.0`, 1),
		strings.Replace(encodeMessage(t, in), `"last_event_seq":0`, `"LastEventSeq":0`, 1),
		strings.Replace(encodeMessage(t, in), `"last_event_seq":0`, `"last_event_seq":0,"unknown":"PRIVATE_COMPLETION_PATH"`, 1),
		strings.Replace(encodeMessage(t, in), `"completion":`+encodeMessage(t, in.Completion), `"completion":null`, 1),
	} {
		code, out := request(t, h, "POST", "/api/agent/resources", token, body)
		if code != 400 || strings.Contains(out, "PRIVATE_COMPLETION_PATH") {
			t.Fatal("弱完成JSON未拒绝", code, out)
		}
	}
	for _, changed := range []protocol.NodeResourceCompletion{{StopCode: "process_group_reaped", LastEventSeq: 1}, {StopCode: "process_group_reaped", LastLogSeq: 1}, {StopCode: "process_group_reaped", LastLogSeq: 1, LastLogOffset: 1}, {StopCode: "process_group_reaped", LastArtifactSeq: 1}} {
		wrong := in
		wrong.Completion = &changed
		code, out := request(t, h, "POST", "/api/agent/resources", token, encodeMessage(t, wrong))
		if code != 409 {
			t.Fatal("改变中央游标确认完成", code, out)
		}
	}
	wrong := in
	wrong.Ref.Epoch++
	code, out := request(t, h, "POST", "/api/agent/resources", token, encodeMessage(t, wrong))
	if code != 409 {
		t.Fatal("错完整Ref确认完成", code, out)
	}
	oversize := in
	oversizedCompletion := *in.Completion
	oversizedCompletion.StopCode = strings.Repeat("x", 32*1024)
	oversize.Completion = &oversizedCompletion
	code, out = request(t, h, "POST", "/api/agent/resources", token, encodeMessage(t, oversize))
	if code != 413 {
		t.Fatal("完成JSON限额未拒绝", code, out)
	}
	if retentionResourceRows(t, s) != uncompletedState {
		t.Fatal("拒绝完成声明部分更新归属或时间")
	}
	code, out = request(t, h, "POST", "/api/agent/resources", token, encodeMessage(t, in))
	if code != 204 {
		t.Fatal("真实独立Stop与精确零游标未确认", code, out)
	}
	completionState := retentionResourceRows(t, s)
	var rows [][]any
	if err = json.Unmarshal([]byte(completionState), &rows); err != nil || len(rows) != 1 || rows[0][13] == "" || !strings.Contains(rows[0][14].(string), "process_group_reaped") {
		t.Fatal("实际完成时间和原JSON未保存", err)
	}

	actor, err := st.Authenticate(context.Background(), adminToken)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"admin", "trigger", "approver"} {
		auth := adminToken
		if role != "admin" {
			created, err := st.CreateToken(context.Background(), actor, role)
			if err != nil {
				t.Fatal(err)
			}
			auth = created.Token
		}
		code, _ := request(t, h, "POST", "/api/agent/resources", auth, encodeMessage(t, in))
		if code != 401 {
			t.Fatal("用户确认节点资源完成", role, code)
		}
	}
	rotated, err := st.RotateNodeToken(context.Background(), actor, "worker")
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		code, out = request(t, h, "POST", "/api/agent/resources", rotated.Token, encodeMessage(t, in))
		if code != 204 {
			t.Fatal("当前同Node精确完成重放失败", code, out)
		}
		if retentionResourceRows(t, s) != completionState {
			t.Fatal("完成重放刷新时间/归属/原JSON")
		}
	}
	after, err := st.GetBuild(context.Background(), in.Ref.BuildID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("完成修订原构建停止或游标", err)
	}
	code, _ = request(t, h, "POST", "/api/agent/resources", token, encodeMessage(t, in))
	if code != 401 {
		t.Fatal("旧撤销token确认完成", code)
	}
	if err = st.SetNodeState(context.Background(), actor, "worker", "disabled"); err != nil {
		t.Fatal(err)
	}
	code, _ = request(t, h, "POST", "/api/agent/resources", rotated.Token, encodeMessage(t, in))
	if code != 401 {
		t.Fatal("disabled身份确认完成", code)
	}
}

func TestRetentionResourceCompletionHTTPMissingResourceRunningAndAdminStop(t *testing.T) {
	t.Run("missing-resource", func(t *testing.T) {
		_, _, h, token, in := retentionCompletionFixture(t, false, false)
		code, body := request(t, h, "POST", "/api/agent/resources", token, encodeMessage(t, in))
		if code != 409 {
			t.Fatal("无登记资源创建假完成", code, body)
		}
	})
	t.Run("admin-stop", func(t *testing.T) {
		_, _, h, token, in := retentionCompletionFixture(t, true, true)
		code, body := request(t, h, "POST", "/api/agent/resources", token, encodeMessage(t, in))
		if code != 409 {
			t.Fatal("admin观察冒充节点独立停止完成", code, body)
		}
	})
	t.Run("running", func(t *testing.T) {
		_, _, h, token, claim := executionHTTPFixture(t)
		grant := claimHTTP(t, h, token, claim)
		in := protocol.NodeResourceRegistration{Ref: grant.Ref, ID: uuid.NewString(), OwnershipDigest: strings.Repeat("a", 64), HasWorkspace: true}
		code, body := request(t, h, "POST", "/api/agent/resources", token, encodeMessage(t, in))
		if code != 204 {
			t.Fatal(code, body)
		}
		in.Completion = &protocol.NodeResourceCompletion{StopCode: "process_group_reaped"}
		code, body = request(t, h, "POST", "/api/agent/resources", token, encodeMessage(t, in))
		if code != 409 {
			t.Fatal("running确认完成", code, body)
		}
	})
}
