package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"mybuilds/internal/protocol"
)

func TestActualNodeHTTPManagementIdentityAndSession(t *testing.T) {
	_, st, h := serverFixture(t)
	code, body := request(t, h, "POST", "/api/nodes", adminToken, `{"name":"node-a","labels":["generic"],"capacity":2}`)
	if code != 201 {
		t.Fatal("节点创建失败", code, body)
	}
	var created struct {
		Node  struct{ ID, Name string }
		Token string
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil || len(created.Token) < 32 || created.Node.Name != "node-a" {
		t.Fatal("一次节点凭据无效", body, err)
	}
	code, body = request(t, h, "GET", "/api/nodes?limit=1&offset=0", adminToken, "")
	if code != 200 || !strings.Contains(body, "node-a") || strings.Contains(body, created.Token) || strings.Contains(body, "digest") {
		t.Fatal("节点列表不安全", code, body)
	}
	for _, path := range []string{"/api/nodes", "/api/status", "/api/tokens"} {
		code, _ := request(t, h, "GET", path, created.Token, "")
		if code != 401 {
			t.Fatal("节点凭据获得用户权限", path, code)
		}
	}
	actor, err := st.Authenticate(context.Background(), adminToken)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"trigger", "approver"} {
		tok, err := st.CreateToken(context.Background(), actor, role)
		if err != nil {
			t.Fatal(err)
		}
		code, _ := request(t, h, "GET", "/api/nodes", tok.Token, "")
		if code != 403 {
			t.Fatal("非管理员节点管理", role, code)
		}
	}
	report := protocol.SessionRequest{SessionID: "5ad5f16a-37e7-4e09-8bda-c5d616551959", HeartbeatNS: 5e9, LeaseNS: 30e9, Report: protocol.NodeReport{OS: "linux", Arch: "arm64", Capacity: 1, Tools: []protocol.ToolCheck{{Name: "shell", Status: "passed", Version: "1.0"}, {Name: "git", Status: "passed", Version: "2.47.1"}}}}
	encoded, _ := json.Marshal(report)
	code, body = request(t, h, "POST", "/api/agent/session", created.Token, string(encoded))
	if code != 200 {
		t.Fatal("节点握手失败", code, body)
	}
	var grant protocol.SessionGrant
	if err := json.Unmarshal([]byte(body), &grant); err != nil || grant.NodeName != "node-a" || grant.NodeID != created.Node.ID || grant.SessionID != report.SessionID {
		t.Fatal("实际节点身份不匹配", body, err)
	}
	code, _ = request(t, h, "POST", "/api/agent/session", adminToken, string(encoded))
	if code != 401 {
		t.Fatal("用户token取得节点权限", code)
	}
	heartbeat, _ := json.Marshal(protocol.HeartbeatRequest{SessionID: report.SessionID, Report: report.Report})
	code, _ = request(t, h, "POST", "/api/agent/heartbeat", created.Token, string(heartbeat))
	if code != 200 {
		t.Fatal("节点心跳失败", code)
	}
	for _, action := range []string{"drain", "enable", "disable"} {
		code, body = request(t, h, "POST", "/api/nodes/node-a/"+action, adminToken, "")
		if code != 204 {
			t.Fatal(action, code, body)
		}
	}
	code, _ = request(t, h, "POST", "/api/agent/heartbeat", created.Token, string(heartbeat))
	if code != 401 {
		t.Fatal("禁用后仍授权心跳", code)
	}
	code, body = request(t, h, "POST", "/api/nodes/node-a/token/rotate", adminToken, "")
	if code != 200 || strings.Contains(body, created.Token) {
		t.Fatal("轮换节点凭据失败", code, body)
	}
	code, _ = request(t, h, "POST", "/api/agent/session", created.Token, string(encoded))
	if code != 401 {
		t.Fatal("旧节点凭据仍有效", code)
	}
	code, body = request(t, h, "GET", "/api/doctor", adminToken, "")
	if code != 200 || strings.Contains(body, created.Token) {
		t.Fatal("控制端doctor失败", code, body)
	}
}
func TestActualNodeHTTPStrictBoundaries(t *testing.T) {
	_, _, h := serverFixture(t)
	for _, body := range []string{`{"name":"n","capacity":0}`, `{"name":"n","capacity":33}`, `{"name":null}`, `{"name":"n","Name":"other"}`, `{"name":"n","name":"other"}`, `{"name":"n","unknown":"PRIVATE"}`, `{"name":"n","capacity":"1"}`, `{"name":"n","labels":[null]}`} {
		code, out := request(t, h, "POST", "/api/nodes", adminToken, body)
		if code != 400 || strings.Contains(out, "PRIVATE") {
			t.Fatal("非法节点消息未拒绝", code, out)
		}
	}
	for _, query := range []string{"limit=0", "limit=201", "offset=-1", "limit=1&limit=2", "unknown=PRIVATE"} {
		code, body := request(t, h, "GET", "/api/nodes?"+query, adminToken, "")
		if code != 400 || strings.Contains(body, "PRIVATE") {
			t.Fatal("分页不严格", code, body)
		}
	}
	code, _ := request(t, h, "POST", "/api/nodes", adminToken, `{"name":"retired"}`)
	if code != 201 {
		t.Fatal(code)
	}
	code, _ = request(t, h, "DELETE", "/api/nodes/retired", adminToken, "")
	if code != 204 {
		t.Fatal(code)
	}
	code, _ = request(t, h, "POST", "/api/nodes", adminToken, `{"name":"retired"}`)
	if code != 409 {
		t.Fatal("墓碑名复用", code)
	}
}

func TestNodeDoctorDoesNotTreatHeartbeatAsToolSuccess(t *testing.T) {
	_, _, h := serverFixture(t)
	code, body := request(t, h, "POST", "/api/nodes", adminToken, `{"name":"limited","capacity":1}`)
	if code != 201 {
		t.Fatal(code, body)
	}
	var created struct{ Token string }
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatal(err)
	}
	in := protocol.SessionRequest{SessionID: "9d06f1be-9197-444b-a18b-c04aa9d8e49a", HeartbeatNS: 5e9, LeaseNS: 30e9, Report: protocol.NodeReport{OS: "linux", Arch: "arm64", Capacity: 1, Tools: []protocol.ToolCheck{{Name: "shell", Status: "passed"}, {Name: "git", Status: "passed", Version: "2.47.1"}, {Name: "android_aapt2", Status: "failed", Reason: "tool_missing"}}}}
	encoded, _ := json.Marshal(in)
	code, body = request(t, h, "POST", "/api/agent/session", created.Token, string(encoded))
	if code != 200 {
		t.Fatal(code, body)
	}
	code, body = request(t, h, "GET", "/api/nodes/limited/doctor", adminToken, "")
	var doctor NodeDoctorDTO
	if err := json.Unmarshal([]byte(body), &doctor); err != nil {
		t.Fatal(err)
	}
	if code != 200 || !doctor.Node.Healthy || doctor.Status != "failed" || doctor.Reason != "node_tools_failed" {
		t.Fatal("心跳掩盖真实工具失败", code, body)
	}
}
