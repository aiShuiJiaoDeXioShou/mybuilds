package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
)

func retentionNodeHTTPFixture(t *testing.T) (*store.Store, *httptest.Server, string, protocol.NodeDeletion) {
	t.Helper()
	_, st, h, token, registration := retentionCompletionFixture(t, true, false)
	code, body := request(t, h, "POST", "/api/agent/resources", token, encodeMessage(t, registration))
	if code != 204 {
		t.Fatal(code, body)
	}
	ctx := context.Background()
	admin, err := st.Authenticate(ctx, adminToken)
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.GetProject(ctx, "app")
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.Enqueue(ctx, store.EnqueueInput{Actor: admin, ProjectID: p.ID, ProjectVersion: p.PolicyVersion, Key: "later-skipped", RequestDigest: strings.Repeat("d", 64), SHA: strings.Repeat("e", 40), Branch: "main", Source: "repo", File: "mybuilds.yml", SourceDigest: strings.Repeat("f", 64), Builds: []store.PreparedBuild{{Name: "compile", Status: "skipped", Snapshot: store.BuildSnapshot{Definition: config.Build{Steps: []config.Step{{Name: "shell", Kind: "run", Run: "echo NOT_RUN"}}}, Params: map[string]string{}, Facts: map[string]string{}, Condition: "skipped"}, PostBudgetNS: int64(2 * time.Minute), Steps: []store.StepProgress{{Phase: "ordinary", Index: 1, Name: "shell", Kind: "run", Condition: "skipped", Status: "skipped"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = st.SyncGlobalRetention(ctx, config.Retention{Builds: 1, Days: 30}); err != nil {
		t.Fatal(err)
	}
	if _, err = st.ScheduleRetention(ctx, admin, p.ID, 100); err != nil {
		t.Fatal(err)
	}
	actor, err := st.AuthenticateNode(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	items, err := st.ClaimNodeDeletions(ctx, actor, 10)
	if err != nil || len(items) != 1 {
		t.Fatal("真实完整资源与清理事项", err, len(items))
	}
	return st, h, token, items[0]
}

func TestRetentionNodeHTTPActualAuthorityConfirmationReplay(t *testing.T) {
	st, h, token, item := retentionNodeHTTPFixture(t)
	code, body := request(t, h, "GET", "/api/agent/deletions?limit=10", token, "")
	var claimed []protocol.NodeDeletion
	if json.Unmarshal([]byte(body), &claimed) != nil || code != 200 || len(claimed) != 1 || claimed[0] != item {
		t.Fatal(code, body)
	}
	path := "/api/agent/deletions/" + item.ID
	code, body = request(t, h, "POST", path+"/authorize", token, "{}")
	var authority protocol.DeletionAuthority
	if json.Unmarshal([]byte(body), &authority) != nil || code != 200 || authority.ID != item.ID || authority.Nonce == "" || !authority.ExpiresAt.After(time.Now().UTC()) {
		t.Fatal(code, body)
	}
	in := protocol.NodeDeletionConfirmation{ID: item.ID, ResourceID: item.ResourceID, OwnershipDigest: item.OwnershipDigest, Nonce: authority.Nonce, Seq: 1, WorkspaceState: "partial", ResultsState: "not_applicable", Reason: "partial"}
	in.Digest, _ = protocol.NodeDeletionDigest(in)
	admin, _ := st.Authenticate(context.Background(), adminToken)
	rotated, err := st.RotateNodeToken(context.Background(), admin, "worker")
	if err != nil {
		t.Fatal(err)
	}
	code, _ = request(t, h, "POST", path+"/confirm", token, encodeMessage(t, in))
	if code != 401 {
		t.Fatal("撤销身份仍确认", code)
	}
	for range 20 {
		code, body = request(t, h, "POST", path+"/confirm", rotated.Token, encodeMessage(t, in))
		var receipt protocol.NodeDeletionReceipt
		if json.Unmarshal([]byte(body), &receipt) != nil || code != 200 || receipt != (protocol.NodeDeletionReceipt{ID: in.ID, Seq: in.Seq, Digest: in.Digest}) {
			t.Fatal("精确原回执", code, body)
		}
	}
	in.WorkspaceState = "deleted"
	in.Reason = ""
	in.Digest, _ = protocol.NodeDeletionDigest(in)
	code, body = request(t, h, "POST", path+"/confirm", rotated.Token, encodeMessage(t, in))
	if code != 409 || !strings.Contains(body, "retention_receipt_conflict") {
		t.Fatal("同序不同事实", code, body)
	}
}

func TestRetentionNodeHTTPStrictBoundaryAndIdentity(t *testing.T) {
	st, h, token, item := retentionNodeHTTPFixture(t)
	path := "/api/agent/deletions/" + item.ID
	for _, query := range []string{"?limit=0", "?limit=11", "?limit=-1", "?limit=1&limit=2", "?limit=1.0", "?unknown=1", "?limit="} {
		code, body := request(t, h, "GET", "/api/agent/deletions"+query, token, "")
		if code != 400 {
			t.Fatal("非法领取参数", query, code, body)
		}
	}
	for _, body := range []string{"", "null", "[]", `{"id":"x"}`, `{} {}`, `{"x":null}`} {
		code, response := request(t, h, "POST", path+"/authorize", token, body)
		if code != 400 {
			t.Fatal("严格空对象", body, code, response)
		}
	}
	code, _ := request(t, h, "POST", path+"/authorize?limit=1", token, "{}")
	if code != 400 {
		t.Fatal("授权接受参数", code)
	}
	for _, endpoint := range []string{"", "/" + item.ID + "/authorize", "/" + item.ID + "/confirm"} {
		method, body := "POST", "{}"
		if endpoint == "" {
			method = "GET"
			body = ""
		}
		code, _ = request(t, h, method, "/api/agent/deletions"+endpoint, adminToken, body)
		if code != 401 {
			t.Fatal("管理用户成为节点", code)
		}
	}
	admin, _ := st.Authenticate(context.Background(), adminToken)
	other, err := st.CreateNode(context.Background(), admin, store.NodeInput{Name: "other-cleaner", Capacity: 1})
	if err != nil {
		t.Fatal(err)
	}
	code, _ = request(t, h, "POST", path+"/authorize", other.Token, "{}")
	if code != 401 {
		t.Fatal("转派别节点", code)
	}
	code, body := request(t, h, "POST", path+"/authorize", token, "{}")
	var authority protocol.DeletionAuthority
	json.Unmarshal([]byte(body), &authority)
	if code != 200 {
		t.Fatal(code, body)
	}
	in := protocol.NodeDeletionConfirmation{ID: item.ID, ResourceID: item.ResourceID, OwnershipDigest: item.OwnershipDigest, Nonce: authority.Nonce, Seq: 1, WorkspaceState: "partial", ResultsState: "not_applicable", Reason: "partial"}
	in.Digest, _ = protocol.NodeDeletionDigest(in)
	var fields map[string]json.RawMessage
	json.Unmarshal([]byte(encodeMessage(t, in)), &fields)
	for key, value := range fields {
		delete(fields, key)
		code, body = request(t, h, "POST", path+"/confirm", token, encodeMessage(t, fields))
		if code != 400 {
			t.Fatal("省略固定确认字段", key, code, body)
		}
		fields[key] = value
	}
	wrong := in
	wrong.ID = uuid.NewString()
	wrong.Digest, _ = protocol.NodeDeletionDigest(wrong)
	code, _ = request(t, h, "POST", path+"/confirm", token, encodeMessage(t, wrong))
	if code != 400 {
		t.Fatal("URL与body身份不一致", code)
	}
	for _, raw := range []string{strings.Replace(encodeMessage(t, in), `"seq":1`, `"seq":1,"seq":1`, 1), strings.Replace(encodeMessage(t, in), `"seq":1`, `"seq":1.0`, 1), strings.Replace(encodeMessage(t, in), `"reason":"partial"`, `"reason":null`, 1), strings.Replace(encodeMessage(t, in), `"seq":1`, `"Seq":1`, 1)} {
		code, body = request(t, h, "POST", path+"/confirm", token, raw)
		if code != 400 {
			t.Fatal("严格wire确认", code, body)
		}
	}
	code, _ = request(t, h, "POST", "/api/nodes/worker/disable", adminToken, "")
	if code != 204 {
		t.Fatal(code)
	}
	code, _ = request(t, h, "GET", "/api/agent/deletions", token, "")
	if code != 401 {
		t.Fatal("禁用节点仍领取", code)
	}
}
