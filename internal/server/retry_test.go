package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"mybuilds/internal/store"
)

func TestActualRetryHTTPExplicitKeyRolesAndOriginalEvidence(t *testing.T) {
	_, st, h, nodeToken, _ := executionHTTPFixture(t)
	ctx := context.Background()
	builds, err := st.ListBuilds(ctx, store.BuildFilter{})
	if err != nil || len(builds) != 1 {
		t.Fatal(err)
	}
	original := builds[0]
	path := "/api/builds/" + original.ID + "/retry"
	code, body := request(t, h, "POST", path, adminToken, `{}`, "retry-first")
	if code != 409 {
		t.Fatal("queued不得retry", code, body)
	}
	cancelled, err := st.Cancel(ctx, func() store.Actor {
		a, e := st.Authenticate(ctx, adminToken)
		if e != nil {
			t.Fatal(e)
		}
		return a
	}(), original.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{``, `[]`, `null`, `{"allow_upload":null}`, `{"allow_upload":false,"allow_upload":true}`, `{"AllowUpload":false}`, `{"params":{"x":"PRIVATE"}}`, `{"branch":"PRIVATE"}`, `{"allow_upload":false} {}`} {
		code, body = request(t, h, "POST", path, adminToken, input, "invalid-body")
		if code != 400 || strings.Contains(body, "PRIVATE") {
			t.Fatal(code, body)
		}
	}
	code, _ = request(t, h, "POST", path, adminToken, `{}`)
	if code != 400 {
		t.Fatal("无显式key被接受", code)
	}
	for _, key := range []string{"", "contains/slash", "contains\\slash", strings.Repeat("a", 129)} {
		code, _ = request(t, h, "POST", path, adminToken, `{}`, key)
		if code != 400 {
			t.Fatal("非法key", code)
		}
	}
	code, _ = request(t, h, "POST", path+"?params=PRIVATE", adminToken, `{}`, "bad-query")
	if code != 400 {
		t.Fatal(code)
	}
	code, body = request(t, h, "POST", path, adminToken, `{"allow_upload":false}`, "retry-original")
	var result BatchView
	if err = json.Unmarshal([]byte(body), &result); err != nil || code != 201 || len(result.Builds) != 1 || result.Builds[0].ID == original.ID || result.Builds[0].Number == nil || *result.Builds[0].Number != *original.Number+1 {
		t.Fatal(code, body, err)
	}
	if strings.Contains(body, "PRIVATE") {
		t.Fatal("公开脚本/秘密")
	}
	code, repeated := request(t, h, "POST", path, adminToken, `{"allow_upload":false}`, "retry-original")
	if code != 200 || repeated != body {
		t.Fatal("同请求未原样重放", code, repeated)
	}
	code, _ = request(t, h, "POST", path, adminToken, `{"allow_upload":true}`, "retry-original")
	if code != 409 {
		t.Fatal("不同输入未冲突", code)
	}
	admin, _ := st.Authenticate(ctx, adminToken)
	approver, err := st.CreateToken(ctx, admin, "approver")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		token string
		want  int
	}{{"", 401}, {nodeToken, 401}, {approver.Token, 403}} {
		code, body = request(t, h, "POST", path, tc.token, `{}`, "forbidden")
		if code != tc.want || strings.Contains(body, "PRIVATE") {
			t.Fatal(code, body)
		}
	}
	trigger, err := st.CreateToken(ctx, admin, "trigger")
	if err != nil {
		t.Fatal(err)
	}
	code, body = request(t, h, "POST", path, trigger.Token, `{}`, "trigger-retry")
	if code != 201 {
		t.Fatal("合法trigger无法retry", code, body)
	}
	after, err := st.GetBuild(ctx, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	beforeBytes, _ := json.Marshal(cancelled)
	afterBytes, _ := json.Marshal(after)
	if string(beforeBytes) != string(afterBytes) {
		t.Fatal("原任务证据被改写")
	}
	// 不含完整配置，安全关系仍能从响应和查询确认。
	var relation struct {
		Builds []struct {
			RetryOf string `json:"retry_of"`
		}
	}
	if err = json.Unmarshal([]byte(repeated), &relation); err != nil || len(relation.Builds) != 1 || relation.Builds[0].RetryOf != original.ID {
		t.Fatal("缺安全关联", err)
	}
	code, body = request(t, h, "GET", "/api/builds/"+result.Builds[0].ID, adminToken, "")
	if code != 200 || !strings.Contains(body, `"retry_of":"`+original.ID+`"`) || strings.Contains(body, "PRIVATE") {
		t.Fatal(code, body)
	}
}
