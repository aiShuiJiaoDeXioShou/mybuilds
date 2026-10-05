package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestResourceRegistrationRequiresExactNoContentACK(t *testing.T) {
	for _, path := range []string{"/api/agent/resources", "/api/agent/stop-confirmation"} {
		t.Run(path, func(t *testing.T) {
			for _, code := range []int{200, 201, 202, 204} {
				t.Run(strconv.Itoa(code), func(t *testing.T) {
					s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(code) }))
					defer s.Close()
					client := &agentHTTP{client: s.Client(), endpoint: s.URL, timeout: time.Second}
					err := client.post(context.Background(), path, struct{}{}, nil)
					if code == 204 && err != nil || code != 204 && (err == nil || err.Error() != "agent_invalid_response") {
						t.Fatal("归属确认必须收到实际204", code, err)
					}
				})
			}
		})
	}
}
