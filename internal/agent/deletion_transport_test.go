package agent

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mybuilds/internal/protocol"
)

func TestDeletionTransportGETAndStrictReceipts(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	digest := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	validItem := `{"id":"` + id + `","resource_id":"` + id + `","build_id":"` + id + `","attempt_id":"` + id + `","ownership_digest":"` + digest + `","has_workspace":true,"has_results":false}`
	validAuthority := `{"id":"` + id + `","node_id":"` + id + `","resource_id":"` + id + `","ownership_digest":"` + digest + `","nonce":"` + id + `","expires_at":"2026-10-05T00:00:00Z"}`
	validReceipt := `{"id":"` + id + `","seq":1,"digest":"` + digest + `"}`
	for _, test := range []struct {
		name, path, body string
		output           func() any
		good             bool
	}{
		{"empty", "/api/agent/deletions?limit=10", "[]", func() any { return new([]protocol.NodeDeletion) }, true},
		{"item", "/api/agent/deletions?limit=10", "[" + validItem + "]", func() any { return new([]protocol.NodeDeletion) }, true},
		{"null", "/api/agent/deletions?limit=10", "null", func() any { return new([]protocol.NodeDeletion) }, false},
		{"itemMissing", "/api/agent/deletions?limit=10", "[{\"id\":\"" + id + "\"}]", func() any { return new([]protocol.NodeDeletion) }, false},
		{"authority", "/api/agent/deletions/" + id + "/authorize", validAuthority, func() any { return new(protocol.DeletionAuthority) }, true},
		{"authorityMissing", "/api/agent/deletions/" + id + "/authorize", "{}", func() any { return new(protocol.DeletionAuthority) }, false},
		{"receipt", "/api/agent/deletions/" + id + "/confirm", validReceipt, func() any { return new(protocol.NodeDeletionReceipt) }, true},
		{"receiptDuplicate", "/api/agent/deletions/" + id + "/confirm", `{"id":"` + id + `","seq":1,"seq":1,"digest":"` + digest + `"}`, func() any { return new(protocol.NodeDeletionReceipt) }, false},
		{"receiptCase", "/api/agent/deletions/" + id + "/confirm", `{"ID":"` + id + `","seq":1,"digest":"` + digest + `"}`, func() any { return new(protocol.NodeDeletionReceipt) }, false},
		{"receiptMissing", "/api/agent/deletions/" + id + "/confirm", `{"id":"` + id + `","digest":"` + digest + `"}`, func() any { return new(protocol.NodeDeletionReceipt) }, false},
		{"receiptFloat", "/api/agent/deletions/" + id + "/confirm", `{"id":"` + id + `","seq":1.0,"digest":"` + digest + `"}`, func() any { return new(protocol.NodeDeletionReceipt) }, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer private-test-identity" {
					t.Error("身份头未保留")
				}
				if r.URL.Path == "/api/agent/deletions" {
					data, err := io.ReadAll(r.Body)
					if err != nil || r.Method != http.MethodGet || len(data) != 0 || r.URL.RawQuery != "limit=10" {
						t.Error("实际GET请求", r.Method, string(data), err)
					}
				} else if r.Method != http.MethodPost {
					t.Error("确认原POST")
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, test.body)
			}))
			defer s.Close()
			client := &agentHTTP{client: s.Client(), endpoint: s.URL, token: "private-test-identity", timeout: time.Second}
			var err error
			if test.path == "/api/agent/deletions?limit=10" {
				err = client.get(context.Background(), test.path, test.output())
			} else {
				err = client.post(context.Background(), test.path, struct{}{}, test.output())
			}
			if test.good && err != nil || !test.good && (err == nil || err.Error() != "agent_invalid_response") {
				t.Fatal("严格管理响应", err)
			}
		})
	}
}
