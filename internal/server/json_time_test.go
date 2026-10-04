package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mybuilds/internal/store"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mybuilds/internal/protocol"
)

func TestNodeMessageStrictTime(t *testing.T) {
	progress := protocol.ExecutionProgress{Kind: "intent", Phase: "ordinary", Index: 1, At: time.Now().UTC(), ArtifactSteps: []protocol.ArtifactExpectation{}}
	body, err := json.Marshal(progress)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/api/agent/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	var got protocol.ExecutionProgress
	if err := readJSON(req, &got); err != nil || !got.At.Equal(progress.At) {
		t.Fatal("合法UTC time被误拒", err)
	}
	for _, body := range []string{`{"at":null}`, `{"at":7}`, `{"at":{}}`, `{"at":"SECRET"}`, `{"at":"2026-10-04T00:00:00+08:00"}`, `{"at":"2026-10-04T00:00:00Z","at":"2026-10-04T00:00:00Z"}`, `{"At":"2026-10-04T00:00:00Z"}`, `{"at":"2026-10-04T00:00:00Z","unknown":1}`} {
		req := httptest.NewRequest("POST", "/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if err := readJSON(req, &got); err == nil {
			t.Fatalf("非法time消息被接受: %s", body)
		}
	}
}

func TestNodeErrorsNeverExposeWrappedText(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, fmt.Errorf("PRIVATE_SECRET: %w", store.ErrLeaseExpired))
	if w.Code != 409 || !strings.Contains(w.Body.String(), "lease_expired") || strings.Contains(w.Body.String(), "PRIVATE_SECRET") {
		t.Fatal(w.Code, w.Body.String())
	}
}
