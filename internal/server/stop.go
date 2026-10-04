package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
)

func (s *Server) stopRoute(w http.ResponseWriter, r *http.Request, actor store.Actor) bool {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(parts) != 4 || parts[0] != "api" || parts[1] != "builds" || (parts[3] != "cancel" && parts[3] != "stop-confirmation") {
		return false
	}
	if actor.Role != "admin" {
		writeError(w, store.ErrForbidden)
		return true
	}
	if r.Method != "POST" {
		writeError(w, store.ErrNotFound)
		return true
	}
	if _, e := readQuery(r); e != nil {
		writeError(w, e)
		return true
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	if parts[3] == "cancel" {
		if e := emptyBody(r); e != nil {
			writeError(w, e)
			return true
		}
		result, e := s.store.Cancel(ctx, actor, parts[2])
		if e != nil {
			writeError(w, e)
			return true
		}
		writeJSON(w, 200, result)
		return true
	}
	var in protocol.StopConfirmation
	if e := readJSON(r, &in); e != nil {
		writeError(w, e)
		return true
	}
	if in.Ref.BuildID != parts[2] {
		writeError(w, store.ErrInvalid)
		return true
	}
	if e := s.store.ConfirmStopped(ctx, actor, in); e != nil {
		writeError(w, e)
		return true
	}
	writeJSON(w, 204, nil)
	return true
}
