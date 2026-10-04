package server

import (
	"net/http"
	"strings"

	"mybuilds/internal/store"
)

// retryRoute只接受原构建与显式key，快照、授权、停止和编号由同一Store事务核对。
func (s *Server) retryRoute(w http.ResponseWriter, r *http.Request, actor store.Actor) bool {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(parts) != 4 || parts[0] != "api" || parts[1] != "builds" || parts[2] == "" || parts[3] != "retry" || r.Method != http.MethodPost {
		return false
	}
	if actor.Role != "admin" && actor.Role != "trigger" {
		writeError(w, store.ErrForbidden)
		return true
	}
	if _, err := readQuery(r); err != nil {
		writeError(w, err)
		return true
	}
	keys := r.Header.Values("Idempotency-Key")
	if len(keys) != 1 || !validRequestKey(keys[0]) {
		writeError(w, store.ErrInvalid)
		return true
	}
	var input struct {
		AllowUpload bool `json:"allow_upload"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, err)
		return true
	}
	result, err := s.store.Retry(r.Context(), actor, store.RetryInput{BuildID: parts[2], Key: keys[0], AllowUpload: input.AllowUpload})
	if err != nil {
		writeError(w, err)
		return true
	}
	status := http.StatusCreated
	if result.Replayed {
		status = http.StatusOK
	}
	writeJSON(w, status, batchView(result))
	return true
}
