package server

import (
	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) publishRoutes(w http.ResponseWriter, r *http.Request, actor store.Actor) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	binding := len(parts) >= 4 && parts[0] == "api" && parts[1] == "projects" && parts[3] == "applications"
	if !binding && (len(parts) < 2 || parts[0] != "api" || parts[1] != "publishes" && parts[1] != "publish-queries" && parts[1] != "applications") {
		return false
	}
	if actor.Role != "admin" && actor.Role != "approver" || r.Method != http.MethodGet && actor.Role != "admin" {
		writeError(w, store.ErrForbidden)
		return true
	}
	allowed := []string{}
	if len(parts) == 2 && parts[1] == "publishes" && r.Method == http.MethodGet {
		allowed = []string{"project", "limit", "after"}
	}
	query, err := readQuery(r, allowed...)
	if err != nil {
		writeError(w, err)
		return true
	}
	var result any
	code := 200
	if binding && len(parts) == 4 {
		var project store.Project
		project, err = s.store.GetProject(r.Context(), parts[2])
		if err != nil {
			writeError(w, err)
			return true
		}
		switch r.Method {
		case http.MethodPost:
			var input store.BindApplicationInput
			err = readJSON(r, &input)
			if err == nil {
				if input.ProjectID != "" && input.ProjectID != project.ID {
					err = store.ErrInvalid
				} else {
					input.ProjectID = project.ID
					result, err = s.store.BindApplication(r.Context(), actor, input)
					code = 202
				}
			}
		case http.MethodGet:
			result, err = s.store.ListApplications(r.Context(), project.ID)
		default:
			err = store.ErrNotFound
		}
	} else if len(parts) == 4 && parts[1] == "applications" && parts[3] == "doctor" && r.Method == http.MethodPost {
		if err = emptyBody(r); err == nil {
			result, err = s.store.RequestApplicationDoctor(r.Context(), actor, parts[2])
			code = 202
		}
	} else if len(parts) == 2 && parts[1] == "publishes" && r.Method == http.MethodGet {
		limit := 20
		if query["limit"] != "" {
			limit, err = strconv.Atoi(query["limit"])
		}
		projectID := query["project"]
		if err == nil && projectID != "" {
			var project store.Project
			project, err = s.store.GetProject(r.Context(), projectID)
			projectID = project.ID
		}
		if err == nil {
			var items []store.PublishView
			items, err = s.store.ListPublishes(r.Context(), projectID, limit, query["after"])
			result = map[string]any{"items": items, "limit": limit}
		}
	} else if len(parts) == 3 && parts[1] == "publishes" && r.Method == http.MethodGet {
		result, err = s.store.GetPublish(r.Context(), parts[2])
	} else if len(parts) == 3 && parts[1] == "publish-queries" && r.Method == http.MethodGet {
		result, err = s.store.GetPublishQuery(r.Context(), parts[2])
	} else if len(parts) == 4 && parts[1] == "publishes" && r.Method == http.MethodPost {
		switch parts[3] {
		case "query":
			if err = emptyBody(r); err == nil {
				result, err = s.store.RequestPublishQuery(r.Context(), actor, parts[2])
				code = 202
			}
		case "confirm":
			var input store.ConfirmPublishInput
			err = readJSON(r, &input)
			if err == nil {
				if input.IntentID != parts[2] {
					err = store.ErrInvalid
				} else {
					result, err = s.store.ConfirmPublish(r.Context(), actor, input)
				}
			}
		default:
			err = store.ErrNotFound
		}
	} else {
		err = store.ErrNotFound
	}
	if err != nil {
		writeError(w, err)
	} else {
		writeJSON(w, code, result)
	}
	return true
}
func (s *Server) publishAgentRoutes(w http.ResponseWriter, r *http.Request, actor store.NodeActor) bool {
	if !strings.HasPrefix(r.URL.Path, "/api/agent/publishes/") && !strings.HasPrefix(r.URL.Path, "/api/agent/publish-queries/") {
		return false
	}
	if _, err := readQuery(r); err != nil {
		writeError(w, err)
		return true
	}
	if r.Method != http.MethodPost {
		writeError(w, store.ErrNotFound)
		return true
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var result any
	var err error
	code := 200
	switch r.URL.Path {
	case "/api/agent/publishes/authorize":
		var input protocol.PublishAuthorization
		err = readJSON(r, &input)
		if err == nil {
			result, err = s.store.AuthorizePublish(r.Context(), actor, input)
			code = 201
		}
	case "/api/agent/publishes/lookup":
		var input protocol.PublishLookup
		err = readJSON(r, &input)
		if err == nil {
			result, err = s.store.FindNodePublish(r.Context(), actor, input)
		}
	case "/api/agent/publishes/preflight":
		var input struct {
			Ref           protocol.LeaseRef `json:"ref"`
			Target        string            `json:"target"`
			AppIdentifier string            `json:"app_identifier"`
		}
		err = readJSON(r, &input)
		if err == nil {
			result, err = s.store.FindNodeApplication(r.Context(), actor, input.Ref, input.Target, input.AppIdentifier)
		}
	case "/api/agent/publish-queries/claim":
		var input struct {
			SessionID string `json:"session_id"`
		}
		err = readJSON(r, &input)
		if err == nil {
			var task *protocol.PublishQueryTask
			task, err = s.store.ClaimPublishQuery(r.Context(), actor, input.SessionID)
			result = task
			if task == nil {
				code = 204
			}
		}
	default:
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/agent/"), "/")
		if len(parts) == 3 && parts[0] == "publishes" && parts[2] == "receipt" {
			var input protocol.PublishReceipt
			err = readJSON(r, &input)
			if err == nil {
				if input.IntentID != parts[1] {
					err = store.ErrInvalid
				} else {
					result, err = s.store.RecordPublish(r.Context(), actor, input)
				}
			}
		} else if len(parts) == 3 && parts[0] == "publish-queries" && parts[2] == "result" {
			var input protocol.PublishQueryResult
			err = readJSON(r, &input)
			if err == nil {
				if input.ID != parts[1] {
					err = store.ErrInvalid
				} else {
					result, err = s.store.CompletePublishQuery(r.Context(), actor, input)
				}
			}
		} else {
			err = store.ErrNotFound
		}
	}
	if err != nil {
		writeError(w, err)
	} else if code == 204 {
		writeJSON(w, code, nil)
	} else {
		writeJSON(w, code, result)
	}
	return true
}
