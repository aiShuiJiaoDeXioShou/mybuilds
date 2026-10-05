package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebhookActualHTTPFixedSHAQueueReplayAndSecretIsolation(t *testing.T) {
	s, st, h := serverFixture(t)
	if e := os.Chmod(s.config.DataDir, 0700); e != nil {
		t.Fatal(e)
	}
	a, e := st.Authenticate(context.Background(), adminToken)
	if e != nil {
		t.Fatal(e)
	}
	repo, name := triggerProject(t, st, a, "version: 1\nsteps: [{kind: run, run: 'touch MUST_NOT_WEBHOOK_EXECUTE'}]\n", config.ProjectSettings{}, "linux")
	key, e := s.ConfigureWebhook(context.Background(), a, name, config.ProjectSettings{Hook: &config.HookSettings{Enabled: true, RepositoryKey: "own-repository"}}, false)
	if e != nil {
		t.Fatal(e)
	}
	if len(key.Secret) < 32 {
		t.Fatal("未返回真实自产secret")
	}
	sha := gitCommand(t, repo, "rev-parse", "HEAD")
	body := fmt.Sprintf(`{"ref":"refs/heads/main","after":%q,"before":%q,"repository":"own-repository","metadata":null}`, sha, strings.Repeat("0", 40))
	send := func(secret, delivery string) (int, string) {
		req, _ := http.NewRequest("POST", h.URL+"/hook/"+name, strings.NewReader(body))
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(body))
		req.Header.Set("X-Mybuilds-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		req.Header.Set("X-Mybuilds-Event", "push")
		req.Header.Set("X-Mybuilds-Delivery", delivery)
		req.Header.Set("Content-Type", "application/json")
		resp, e := h.Client().Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	status, raw := send(key.Secret, "one")
	if status != 202 {
		t.Fatalf("接收: %d %s", status, raw)
	}
	var receipt struct {
		WindowID string `json:"window_id"`
	}
	json.Unmarshal([]byte(raw), &receipt)
	if status, raw = send("wrong-key-private-marker-0123456789", "bad"); status != 401 {
		t.Fatal(status, raw)
	}
	w, _, e := st.ReadWebhookWindow(context.Background(), receipt.WindowID)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.closeWebhookWindow(context.Background(), w); e != nil {
		t.Fatal(e)
	}
	w, _, e = st.ReadWebhookWindow(context.Background(), w.ID)
	if e != nil || w.State != "closed" || w.FinalSHA != sha || len(w.BuildIDs) != 1 {
		t.Fatalf("真实关闭: %+v %v", w, e)
	}
	view, e := st.GetBuild(context.Background(), w.BuildIDs[0])
	if e != nil || view.Status != "queued" || view.Number == nil || *view.Number != 1 {
		t.Fatal(view, e)
	}
	if status, raw = send(key.Secret, "one"); status != 200 || !strings.Contains(raw, `"replayed":true`) {
		t.Fatal(status, raw)
	}
	if status, raw = send(key.Secret, "alias"); status != 200 {
		t.Fatal(status, raw)
	}
	code, safe := request(t, h, "GET", "/api/projects/"+name+"/hook/windows", adminToken, "")
	if code != 200 || strings.Contains(safe, key.Secret) || strings.Contains(safe, "Credential") || strings.Contains(safe, "MUST_NOT") {
		t.Fatal("安全视图失败", code, safe)
	}
	for _, marker := range []string{"MUST_NOT_WEBHOOK_EXECUTE"} {
		if _, e = os.Stat(filepath.Join(repo, marker)); !os.IsNotExist(e) {
			t.Fatal("控制端执行用户脚本")
		}
	}
	status, raw = request(t, h, "POST", "/api/projects/"+name+"/hook/rotate", adminToken, "{}")
	if status != 200 {
		t.Fatal(status, raw)
	}
	if status, raw = send(key.Secret, "three"); status != 401 {
		t.Fatal("旧凭据仍有效", status, raw)
	}
	p, _ := st.GetProject(context.Background(), name)
	if p.NextNumber != 2 {
		t.Fatal("重投分号", p.NextNumber)
	}
}
func TestWebhookManagementRoleBeforeLookupAndStrictJSON(t *testing.T) {
	s, st, h := serverFixture(t)
	_ = s
	a, e := st.Authenticate(context.Background(), adminToken)
	if e != nil {
		t.Fatal(e)
	}
	token, e := st.CreateToken(context.Background(), a, "trigger")
	if e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{"/api/projects/does-not-exist/hook", "/api/projects/does-not-exist/hook/events"} {
		code, body := request(t, h, "GET", path, token.Token, "")
		if code != 403 || strings.Contains(body, "does-not") {
			t.Fatal(code, body)
		}
	}
	for _, body := range []string{`{"extra":1}`, `null`, `{} {}`} {
		code, _ := request(t, h, "POST", "/api/projects/does-not-exist/hook/rotate", adminToken, body)
		if code != 400 {
			t.Fatal("管理JSON未拒", code)
		}
	}
}
func TestWebhookNewProjectAtomicSettingsAndCurrentPolicyDisable(t *testing.T) {
	s, st, h := serverFixture(t)
	if e := os.Chmod(s.config.DataDir, 0700); e != nil {
		t.Fatal(e)
	}
	for _, body := range []string{`{"name":"bad","repo":"/tmp/own","nodes":["linux"],"settings":{"hook":{"enabled":true,"repository_key":"42"},"triggers":{"builds":[],"quiet_period":"0s"}}}`, `{"name":"bad","repo":"/tmp/own","nodes":["linux"],"settings":{"hook":{"enabled":true,"repository_key":"42","secret":"literal-private-key"},"triggers":{"builds":["default"]}}}`} {
		code, _ := request(t, h, "POST", "/api/projects", adminToken, body)
		if code != 400 {
			t.Fatal("非法创建未拒", code)
		}
		if _, e := st.GetProject(context.Background(), "bad"); e == nil {
			t.Fatal("失败部分创建项目")
		}
	}
	a, e := st.Authenticate(context.Background(), adminToken)
	if e != nil {
		t.Fatal(e)
	}
	_, name := triggerProject(t, st, a, "version: 1\nsteps: [{kind: run, run: 'true'}]\n", config.ProjectSettings{}, "linux")
	v, e := s.ConfigureWebhook(context.Background(), a, name, config.ProjectSettings{Hook: &config.HookSettings{Enabled: true, RepositoryKey: "own"}, Triggers: &config.TriggerSettings{Builds: []string{"default"}}}, false)
	if e != nil {
		t.Fatal(e)
	}
	before, e := st.ReadWebhookPolicy(context.Background(), name)
	if e != nil {
		t.Fatal(e)
	}
	code, raw := request(t, h, "POST", "/api/projects/"+name+"/hook/disable", adminToken, "{}")
	if code != 200 {
		t.Fatal(code, raw)
	}
	event := protocol.WebhookEvent{Provider: "generic", Kind: "push", RepositoryKey: "own", Branch: "main", After: strings.Repeat("a", 40), BodyDigest: strings.Repeat("b", 64)}
	event.ReceiptDigest = protocol.WebhookEventDigest(event)
	_, e = st.ReceiveWebhook(context.Background(), store.WebhookActor{ProjectID: before.ProjectID, CredentialID: before.CredentialID, SecretFingerprint: before.SecretFingerprint, PolicyVersion: before.PolicyVersion}, event)
	if !errors.Is(e, store.ErrForbidden) {
		t.Fatal("撤权后仍接受", e)
	}
	code, raw = request(t, h, "POST", "/api/projects/"+name+"/hook/enable", adminToken, "{}")
	if code != 200 || strings.Contains(raw, v.Secret) || strings.Contains(raw, `"secret"`) {
		t.Fatal("重启泄旧密钥", code, raw)
	}
}
