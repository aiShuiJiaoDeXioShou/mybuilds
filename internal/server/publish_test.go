package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"mybuilds/internal/store"
)

func TestPublishHTTPRolesPrivateFieldsAndStrictInput(t *testing.T) {
	_, st, h, nodeToken, _ := executionHTTPFixture(t)
	admin, err := st.Authenticate(context.Background(), adminToken)
	if err != nil {
		t.Fatal(err)
	}
	roles := map[string]string{"admin": adminToken, "node": nodeToken}
	for _, role := range []string{"trigger", "approver"} {
		token, err := st.CreateToken(context.Background(), admin, role)
		if err != nil {
			t.Fatal(err)
		}
		roles[role] = token.Token
	}
	for role, token := range roles {
		code, _ := request(t, h, "GET", "/api/publishes", token, "")
		want := 200
		if role == "trigger" {
			want = 403
		}
		if role == "node" {
			want = 401
		}
		if code != want {
			t.Fatal("发布读取权限", role, code)
		}
		code, _ = request(t, h, "POST", "/api/projects/app/applications", token, "{}")
		want = 400
		if role != "admin" {
			want = 403
		}
		if role == "node" {
			want = 401
		}
		if code != want {
			t.Fatal("发布写入权限", role, code)
		}
	}
	for _, body := range []string{`{"store":null}`, `{"store":"google_play","store":"app_store"}`, `{"Unknown":"PRIVATE"}`, `{"store":"google_play","extra":"PRIVATE"}`, strings.Repeat("x", 65537)} {
		code, output := request(t, h, "POST", "/api/projects/app/applications", adminToken, body)
		if code != 400 || strings.Contains(output, "PRIVATE") {
			t.Fatal("严格绑定输入", code, output)
		}
	}
	input := store.BindApplicationInput{NodeID: "worker", Store: "google_play", AppIdentifier: "com.example.httppublish", CredentialRef: "${PRIVATE_PLAY_MATERIAL}", AllowedTracks: []string{"internal"}, UploadCertificateSHA256: strings.Repeat("a", 64)}
	code, body := request(t, h, "POST", "/api/projects/app/applications", adminToken, encodeMessage(t, input))
	var app store.ApplicationView
	if err := json.Unmarshal([]byte(body), &app); err != nil || code != 202 || app.Status != "pending" || strings.Contains(body, "PRIVATE") || strings.Contains(body, "credential_ref") {
		t.Fatal("公开绑定脱敏", code, body, err)
	}
	code, body = request(t, h, "GET", "/api/projects/app/applications", roles["approver"], "")
	if code != 200 || strings.Contains(body, "PRIVATE") {
		t.Fatal("审批者只读公开绑定", code, body)
	}
	code, _ = request(t, h, "GET", "/api/publishes?limit=101", adminToken, "")
	if code != 400 {
		t.Fatal("无限列表", code)
	}
	for _, path := range []string{"publishes/authorize", "publishes/lookup", "publishes/preflight", "publish-queries/claim"} {
		code, _ = request(t, h, "POST", "/api/agent/"+path, adminToken, "{}")
		if code != 401 {
			t.Fatal("管理员进入节点发布API", path, code)
		}
	}
	for _, body := range []string{`{"session_id":null}`, `{"session_id":"a","session_id":"b"}`, `{"SessionID":"PRIVATE"}`} {
		code, output := request(t, h, "POST", "/api/agent/publish-queries/claim", nodeToken, body)
		if code != 400 || strings.Contains(output, "PRIVATE") {
			t.Fatal("节点管理请求严格输入", code, output)
		}
	}
}
