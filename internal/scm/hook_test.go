package scm

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func hookMAC(secret string, body []byte) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(body)
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}
func TestWebhookFourProvidersAndRawAuthentication(t *testing.T) {
	sha := strings.Repeat("a", 40)
	secret := strings.Repeat("s", 32)
	for _, provider := range []string{"github", "gitlab", "gitee", "generic"} {
		t.Run(provider, func(t *testing.T) {
			body := []byte(`{"ref":"refs/heads/main","after":"` + sha + `","before":"` + strings.Repeat("b", 40) + `","repository":{"id":42},"metadata":null}`)
			header := http.Header{}
			switch provider {
			case "github":
				header.Set("X-GitHub-Event", "push")
				header.Set("X-GitHub-Delivery", "owned-delivery")
				header.Set("X-Hub-Signature-256", hookMAC(secret, body))
			case "gitlab":
				body = []byte(`{"object_kind":"push","ref":"refs/heads/main","after":"` + sha + `","before":"` + sha + `","project":{"id":42},"metadata":null}`)
				header.Set("X-Gitlab-Event", "Push Hook")
				header.Set("X-Gitlab-Token", secret)
			case "gitee":
				body = []byte(`{"hook_name":"push_hooks","ref":"refs/heads/main","after":"` + sha + `","repository":{"id":42}}`)
				header.Set("X-Gitee-Event", "Push Hook")
				header.Set("X-Gitee-Token", secret)
			case "generic":
				body = []byte(`{"ref":"refs/heads/main","after":"` + sha + `","repository":"42","metadata":null}`)
				header.Set("X-Mybuilds-Event", "push")
				header.Set("X-Mybuilds-Signature", hookMAC(secret, body))
			}
			input := HookInput{Provider: provider, RepositoryKey: "42", Header: header, Body: body, Secret: secret}
			event, e := ParseWebhook(context.Background(), input)
			if e != nil || event.Kind != "push" || event.Branch != "main" || event.After != sha || event.RepositoryKey != "42" {
				t.Fatalf("实际provider %v %+v", e, event)
			}
			input.Secret = strings.Repeat("x", 32)
			if _, e = ParseWebhook(context.Background(), input); !errors.Is(e, ErrHookUnauthorized) {
				t.Fatal("错误秘密未拒绝")
			}
		})
	}
}
func TestWebhookDuplicatesLimitsAndProviderIsolation(t *testing.T) {
	secret := strings.Repeat("s", 32)
	body := []byte(`{"ref":"refs/heads/main","after":"` + strings.Repeat("a", 40) + `","repository":"42"}`)
	input := HookInput{Provider: "generic", RepositoryKey: "42", Header: http.Header{"X-Mybuilds-Event": []string{"push"}, "X-Mybuilds-Signature": []string{hookMAC(secret, body)}}, Body: body, Secret: secret}
	input.Header.Add("X-Mybuilds-Signature", hookMAC(secret, body))
	if _, e := ParseWebhook(context.Background(), input); !errors.Is(e, ErrHookInvalid) {
		t.Fatal("重复认证头")
	}
	input.Header.Set("X-Mybuilds-Signature", hookMAC(secret, body))
	input.Header.Set("X-Gitlab-Token", secret)
	if _, e := ParseWebhook(context.Background(), input); !errors.Is(e, ErrHookInvalid) {
		t.Fatal("其它provider认证模式")
	}
	input.Header.Del("X-Gitlab-Token")
	input.Body = []byte(`{"ref":"refs/heads/main","ref":"refs/heads/main"}`)
	input.Header.Set("X-Mybuilds-Signature", hookMAC(secret, input.Body))
	if _, e := ParseWebhook(context.Background(), input); !errors.Is(e, ErrHookInvalid) {
		t.Fatal("重复JSON")
	}
	input.Body = make([]byte, (8<<20)+1)
	if _, e := ParseWebhook(context.Background(), input); !errors.Is(e, ErrHookLimit) {
		t.Fatal("超body")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := ParseWebhook(ctx, input); !errors.Is(e, ErrHookCancelled) {
		t.Fatal("取消")
	}
}

func TestWebhookRecognizedIgnoredEventsAndPresence(t *testing.T) {
	secret := strings.Repeat("s", 32)
	body := []byte(`{"repository":"42","metadata":null}`)
	input := HookInput{Provider: "generic", RepositoryKey: "42", Secret: secret, Body: body, Header: http.Header{}}
	input.Header.Set("X-Mybuilds-Event", "ping")
	input.Header.Set("X-Mybuilds-Signature", hookMAC(secret, body))
	event, err := ParseWebhook(context.Background(), input)
	if err != nil || event.Kind != "ignored" {
		t.Fatal("已认证ping", event, err)
	}
	input.Header.Set("X-Mybuilds-Event", "invented-event")
	if _, err = ParseWebhook(context.Background(), input); !errors.Is(err, ErrHookInvalid) {
		t.Fatal("未知事件未拒绝", err)
	}
	input.Header.Del("X-Mybuilds-Signature")
	if _, err = ParseWebhook(context.Background(), input); !errors.Is(err, ErrHookUnauthorized) {
		t.Fatal("缺认证", err)
	}
	input.Header.Set("X-Mybuilds-Event", "push")
	input.Body = []byte(`{"repository":"42","ref":"refs/heads/main","after":"` + strings.Repeat("a", 40) + `","before":null}`)
	input.Header.Set("X-Mybuilds-Signature", hookMAC(secret, input.Body))
	if _, err = ParseWebhook(context.Background(), input); !errors.Is(err, ErrHookInvalid) {
		t.Fatal("显式null不能当before省略", err)
	}
}
