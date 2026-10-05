package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mybuilds/internal/config"
)

func TestPublishHTTPActualResponse64KiBPreservesOrdinaryBudget(t *testing.T) {
	body := `{"value":"` + strings.Repeat("a", 65525) + `"}`
	if len(body) != 65537 {
		t.Fatal("测试边界长度错误", len(body))
	}
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
	defer control.Close()
	client, err := newAgentHTTP(config.AgentConfig{Server: control.URL, RuntimeToken: strings.Repeat("t", 32), HeartbeatInterval: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer client.close()
	var out struct {
		Value string `json:"value"`
	}
	for _, path := range []string{"/api/agent/publish-queries/claim", "/api/agent/publishes/lookup"} {
		if err := client.get(context.Background(), path, &out); err == nil {
			t.Fatal("超限发布响应被接受", path)
		}
	}
	if err := client.get(context.Background(), "/api/agent/example-ordinary", &out); err != nil || out.Value != strings.Repeat("a", 65525) {
		t.Fatal("原普通响应预算被缩小", err)
	}
}
