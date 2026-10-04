package server

import (
	"io"
	"net/http"
	"strings"

	"mybuilds/internal/store"
)

type NodeRequest struct {
	Name     string   `json:"name"`
	Labels   []string `json:"labels"`
	Capacity int      `json:"capacity"`
}

func (s *Server) leasePolicy() store.LeasePolicy {
	return store.LeasePolicy{Concurrency: s.config.Concurrency, Heartbeat: s.config.HeartbeatInterval, Duration: s.config.LeaseDuration}
}
func emptyBody(r *http.Request) error {
	if r.Body == nil {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1))
	if err != nil || len(body) != 0 {
		return store.ErrInvalid
	}
	return nil
}

// nodeRoutes只接管理员的真实节点操作；节点凭据不能进入此入口。
func (s *Server) nodeRoutes(w http.ResponseWriter, r *http.Request, actor store.Actor) bool {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(parts) < 2 || parts[0] != "api" || parts[1] != "nodes" {
		return false
	}
	if actor.Role != "admin" {
		writeError(w, store.ErrForbidden)
		return true
	}
	allowed := []string{}
	if len(parts) == 2 && r.Method == http.MethodGet {
		allowed = []string{"limit", "offset"}
	}
	query, err := readQuery(r, allowed...)
	if err != nil {
		writeError(w, err)
		return true
	}
	if len(parts) == 2 {
		switch r.Method {
		case http.MethodPost:
			input := NodeRequest{Capacity: 1}
			if err := readJSON(r, &input); err != nil {
				writeError(w, err)
				return true
			}
			result, err := s.store.CreateNode(r.Context(), actor, store.NodeInput{Name: input.Name, Labels: input.Labels, Capacity: input.Capacity})
			if err != nil {
				writeError(w, err)
				return true
			}
			writeJSON(w, 201, result)
			return true
		case http.MethodGet:
			page, err := readPage(query)
			if err != nil {
				writeError(w, err)
				return true
			}
			items, err := s.store.ListNodes(r.Context(), actor, store.NodeFilter{Page: page})
			if err != nil {
				writeError(w, err)
				return true
			}
			writeJSON(w, 200, map[string]any{"items": items, "limit": page.Limit, "offset": page.Offset})
			return true
		}
	}
	if len(parts) >= 3 && parts[2] != "" {
		name := parts[2]
		if len(parts) == 3 && r.Method == http.MethodGet {
			result, err := s.store.GetNode(r.Context(), actor, name)
			if err != nil {
				writeError(w, err)
				return true
			}
			writeJSON(w, 200, result)
			return true
		}
		if len(parts) == 4 && parts[3] == "doctor" && r.Method == http.MethodGet {
			s.nodeDoctor(w, r, actor, name)
			return true
		}
		if err := emptyBody(r); err != nil {
			writeError(w, err)
			return true
		}
		if len(parts) == 3 && r.Method == http.MethodDelete {
			if err := s.store.DeleteNode(r.Context(), actor, name); err != nil {
				writeError(w, err)
				return true
			}
			writeJSON(w, 204, nil)
			return true
		}
		if len(parts) == 4 && r.Method == http.MethodPost {
			state := map[string]string{"drain": "draining", "enable": "enabled", "disable": "disabled"}[parts[3]]
			if state != "" {
				if err := s.store.SetNodeState(r.Context(), actor, name, state); err != nil {
					writeError(w, err)
					return true
				}
				writeJSON(w, 204, nil)
				return true
			}
		}
		if len(parts) == 5 && parts[3] == "token" && r.Method == http.MethodPost {
			switch parts[4] {
			case "rotate":
				result, err := s.store.RotateNodeToken(r.Context(), actor, name)
				if err != nil {
					writeError(w, err)
					return true
				}
				writeJSON(w, 200, result)
				return true
			case "revoke":
				if err := s.store.RevokeNodeToken(r.Context(), actor, name); err != nil {
					writeError(w, err)
					return true
				}
				writeJSON(w, 204, nil)
				return true
			}
		}
	}
	writeError(w, store.ErrNotFound)
	return true
}
