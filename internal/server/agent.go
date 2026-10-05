package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
	"net/http"
	"time"
)

// agentRoutes每次重验节点身份，直接接具体Store租约与进度。
func (s *Server) agentRoutes(w http.ResponseWriter, r *http.Request, actor store.NodeActor) {
	if s.nodeArtifactRoute(w, r, actor) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	if s.retentionAgentRoutes(w, r, actor) {
		return
	}
	if _, err := readQuery(r); err != nil {
		writeError(w, err)
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, store.ErrNotFound)
		return
	}
	switch r.URL.Path {
	case "/api/agent/session":
		var in protocol.SessionRequest
		if err := readJSON(r, &in); err != nil {
			writeError(w, err)
			return
		}
		result, err := s.store.OpenNodeSession(r.Context(), actor, in, s.leasePolicy())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 200, result)
	case "/api/agent/terminal-receipt":
		var in protocol.TerminalReceiptRequest
		if err := readJSON(r, &in); err != nil {
			writeError(w, err)
			return
		}
		result, err := s.store.TerminalReceipt(r.Context(), actor, in)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 200, result)
	case "/api/agent/heartbeat":
		var in protocol.HeartbeatRequest
		if err := readJSON(r, &in); err != nil {
			writeError(w, err)
			return
		}
		result, err := s.store.Heartbeat(r.Context(), actor, in, s.leasePolicy())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 200, result)
	case "/api/agent/claim":
		var in protocol.ClaimRequest
		if err := readJSON(r, &in); err != nil {
			writeError(w, err)
			return
		}
		result, err := s.store.Claim(r.Context(), actor, in, s.leasePolicy())
		if err != nil {
			writeError(w, err)
			return
		}
		if result == nil {
			writeJSON(w, 204, nil)
			return
		}
		writeJSON(w, 200, result)
	case "/api/agent/renew":
		var in protocol.LeaseRef
		if err := readJSON(r, &in); err != nil {
			writeError(w, err)
			return
		}
		result, err := s.store.Renew(r.Context(), actor, in, s.leasePolicy())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 200, result)
	case "/api/agent/events":
		var in protocol.ExecutionEvent
		if err := readJSON(r, &in); err != nil {
			writeError(w, err)
			return
		}
		data, err := json.Marshal(in.Progress)
		if err != nil {
			writeError(w, store.ErrInvalid)
			return
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != in.Digest {
			writeError(w, store.ErrInvalid)
			return
		}
		result, err := s.store.ApplyEvent(r.Context(), actor, in)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 200, result)

	case "/api/agent/stop-confirmation":
		var in protocol.StopConfirmation
		if err := readJSON(r, &in); err != nil {
			writeError(w, err)
			return
		}
		if err := s.store.ConfirmNodeStopped(r.Context(), actor, in); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 204, nil)
	case "/api/agent/logs":
		s.ingestLog(w, r, actor)
	default:
		writeError(w, store.ErrNotFound)
	}
}
