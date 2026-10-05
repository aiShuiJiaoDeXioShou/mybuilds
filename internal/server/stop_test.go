package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
)

func TestActualStopHTTPPersistentCancelAndIndependentConfirmation(t *testing.T) {
	_, st, h, token, in := executionHTTPFixture(t)
	g := claimHTTP(t, h, token, in)
	path := "/api/builds/" + g.Ref.BuildID
	code, body := request(t, h, "POST", path+"/cancel", adminToken, "")
	var view store.BuildView
	if e := json.Unmarshal([]byte(body), &view); e != nil || code != 200 || view.Status != "cancel_requested" || !view.CancelRequested || view.StopUnconfirmed {
		t.Fatal("取消意图伪报回收", code, body, e)
	}
	code, body = request(t, h, "POST", "/api/agent/renew", token, encodeMessage(t, g.Ref))
	var renewed protocol.LeaseGrant
	if e := json.Unmarshal([]byte(body), &renewed); e != nil || code != 200 || !renewed.CancelRequested {
		t.Fatal("续租缺持久取消", code, body, e)
	}
	confirm := protocol.StopConfirmation{Ref: g.Ref, EvidenceCode: "admin_observed_stopped", Note: "管理员已确认该进程组停止"}
	code, _ = request(t, h, "POST", path+"/stop-confirmation", adminToken, encodeMessage(t, confirm))
	if code != 409 {
		t.Fatal("未经interrupted提前解guard", code)
	}
	code, body = request(t, h, "POST", "/api/nodes/worker/disable", adminToken, "")
	if code != 204 {
		t.Fatal(code, body)
	}
	view, e := st.GetBuild(context.Background(), g.Ref.BuildID)
	if e != nil || view.Status != "interrupted" || !view.StopUnconfirmed {
		t.Fatal("禁用未闭锁", view, e)
	}
	wrong := confirm
	wrong.Ref.Epoch++
	code, _ = request(t, h, "POST", path+"/stop-confirmation", adminToken, encodeMessage(t, wrong))
	if code != 409 {
		t.Fatal("错epoch确认", code)
	}
	wrong = confirm
	wrong.Note = "PRIVATE\n"
	code, body = request(t, h, "POST", path+"/stop-confirmation", adminToken, encodeMessage(t, wrong))
	if code != 400 || strings.Contains(body, "PRIVATE") {
		t.Fatal("控制字符证据", code, body)
	}
	code, body = request(t, h, "POST", path+"/stop-confirmation", adminToken, encodeMessage(t, confirm))
	if code != 204 {
		t.Fatal(code, body)
	}
	view, e = st.GetBuild(context.Background(), g.Ref.BuildID)
	if e != nil || view.StopUnconfirmed || view.Status != "interrupted" {
		t.Fatal("停止确认伪报成功", view, e)
	}
	code, _ = request(t, h, "POST", "/api/agent/stop-confirmation", adminToken, encodeMessage(t, confirm))
	if code != 401 {
		t.Fatal("用户越权节点确认", code)
	}
	// 审批路由已实现，但空body不能取得任何精确审批决定。
	for _, action := range []string{"approve", "reject"} {
		code, body = request(t, h, "POST", path+"/"+action, adminToken, "")
		if code != 400 {
			t.Fatal("审批缺少精确字段未拒绝", action, code, body)
		}
	}
	for _, future := range []string{"upload-resolution"} {
		code, _ = request(t, h, "POST", path+"/"+future, adminToken, "")
		if code != 404 {
			t.Fatal("未来路由被注册", future, code)
		}
	}
}
func TestActualStopHTTPQueuedAndRoles(t *testing.T) {
	_, st, h, token, _ := executionHTTPFixture(t)
	ctx := context.Background()
	builds, e := st.ListBuilds(ctx, store.BuildFilter{})
	if e != nil || len(builds) != 1 {
		t.Fatal(e)
	}
	path := "/api/builds/" + builds[0].ID + "/cancel"
	admin, _ := st.Authenticate(ctx, adminToken)
	for _, role := range []string{"trigger", "approver"} {
		tok, e := st.CreateToken(ctx, admin, role)
		if e != nil {
			t.Fatal(e)
		}
		code, _ := request(t, h, "POST", path, tok.Token, "")
		if code != 403 {
			t.Fatal(role, code)
		}
	}
	code, _ := request(t, h, "POST", path, token, "")
	if code != 401 {
		t.Fatal("节点取消用户build", code)
	}
	code, body := request(t, h, "POST", path, adminToken, "")
	var view store.BuildView
	if e = json.Unmarshal([]byte(body), &view); e != nil || code != 200 || view.Status != "cancelled" || view.AttemptID != "" {
		t.Fatal(code, body, e)
	}
}
