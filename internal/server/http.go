package server

import (
	"net/http"
	"strings"
	"time"

	"mybuilds/internal/config"
	"mybuilds/internal/store"
)

type ProjectView struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	GroupID        string    `json:"group_id"`
	Group          string    `json:"group"`
	Provider       string    `json:"provider"`
	Branches       []string  `json:"branches"`
	Nodes          []string  `json:"nodes"`
	DefaultNode    string    `json:"default_node"`
	PipelineSource string    `json:"pipeline_source"`
	PipelineFile   string    `json:"pipeline_file"`
	NextNumber     int64     `json:"next_number"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func ProjectSummary(p store.Project) ProjectView {
	source, file := "auto", "mybuilds.yml"
	if p.Settings.Pipeline != nil {
		if p.Settings.Pipeline.Source != "" {
			source = p.Settings.Pipeline.Source
		}
		if p.Settings.Pipeline.File != "" {
			file = p.Settings.Pipeline.File
		}
	}
	return ProjectView{ID: p.ID, Name: p.Name, GroupID: p.GroupID, Group: p.GroupName, Provider: p.Provider, Branches: p.Branches, Nodes: p.AllowedNodes, DefaultNode: p.DefaultNode, PipelineSource: source, PipelineFile: file, NextNumber: p.NextNumber, CreatedAt: p.CreatedAt.UTC(), UpdatedAt: p.UpdatedAt.UTC()}
}

type ProjectRequest struct {
	Name             string                 `json:"name"`
	Repository       string                 `json:"repo"`
	Provider         string                 `json:"provider,omitempty"`
	Group            string                 `json:"group,omitempty"`
	Branches         []string               `json:"branches,omitempty"`
	Nodes            []string               `json:"nodes"`
	DefaultNode      string                 `json:"default_node,omitempty"`
	BuildNumberStart int64                  `json:"build_number_start,omitempty"`
	Settings         config.ProjectSettings `json:"settings,omitempty"`
}

func (s *Server) management(w http.ResponseWriter, r *http.Request, actor store.Actor) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(parts) < 2 || parts[0] != "api" {
		writeError(w, store.ErrNotFound)
		return
	}
	route := parts[1]
	if route != "groups" && route != "projects" && route != "tokens" {
		writeError(w, store.ErrNotFound)
		return
	}
	name := ""
	if len(parts) == 3 && parts[2] != "" {
		name = parts[2]
	} else if len(parts) != 2 {
		writeError(w, store.ErrNotFound)
		return
	}
	if actor.Role != "admin" {
		writeError(w, store.ErrForbidden)
		return
	}
	allowed := []string{}
	if r.Method == http.MethodGet && name == "" {
		allowed = []string{"limit", "offset"}
		if route == "projects" {
			allowed = append(allowed, "group")
		}
	}
	query, err := readQuery(r, allowed...)
	if err != nil {
		writeError(w, err)
		return
	}
	page, err := readPage(query)
	if err != nil {
		writeError(w, err)
		return
	}
	if r.Method == http.MethodGet && name == "" {
		switch route {
		case "groups":
			items, err := s.store.ListGroups(r.Context(), page)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, 200, map[string]any{"items": items, "limit": page.Limit, "offset": page.Offset})
		case "projects":
			items, err := s.store.ListProjects(r.Context(), store.ProjectFilter{Group: query["group"], Page: page})
			if err != nil {
				writeError(w, err)
				return
			}
			views := make([]ProjectView, 0, len(items))
			for _, item := range items {
				views = append(views, ProjectSummary(item))
			}
			writeJSON(w, 200, map[string]any{"items": views, "limit": page.Limit, "offset": page.Offset})
		case "tokens":
			items, err := s.store.ListTokens(r.Context(), actor, page)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, 200, map[string]any{"items": items, "limit": page.Limit, "offset": page.Offset})
		}
		return
	}
	if route == "groups" {
		switch r.Method {
		case http.MethodPost, http.MethodPatch:
			if (r.Method == http.MethodPost) != (name == "") {
				writeError(w, store.ErrNotFound)
				return
			}
			var input struct {
				Name string `json:"name"`
			}
			if err := readJSON(r, &input); err != nil {
				writeError(w, err)
				return
			}
			var group store.Group
			status := 200
			if name == "" {
				group, err = s.store.CreateGroup(r.Context(), actor, input.Name)
				status = 201
			} else {
				group, err = s.store.RenameGroup(r.Context(), actor, name, input.Name)
			}
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, status, group)
			return
		case http.MethodDelete:
			if name != "" {
				err = s.store.DeleteGroup(r.Context(), actor, name)
				if err != nil {
					writeError(w, err)
					return
				}
				writeJSON(w, 204, nil)
				return
			}
		}
	}
	if route == "projects" {
		switch r.Method {
		case http.MethodPost:
			if name == "" {
				// 缺省为1，显式0不能被Store的Go零值默认规则吞掉。
				input := ProjectRequest{BuildNumberStart: 1}
				if err := readJSON(r, &input); err != nil {
					writeError(w, err)
					return
				}
				if input.BuildNumberStart <= 0 {
					writeError(w, store.ErrInvalid)
					return
				}
				project, err := s.store.CreateProject(r.Context(), actor, store.ProjectInput{Name: input.Name, Repository: input.Repository, Provider: input.Provider, Group: input.Group, Branches: input.Branches, AllowedNodes: input.Nodes, DefaultNode: input.DefaultNode, BuildNumberStart: input.BuildNumberStart, Settings: input.Settings})
				if err != nil {
					writeError(w, err)
					return
				}
				writeJSON(w, 201, ProjectSummary(project))
				return
			}
		case http.MethodPatch:
			if name != "" {
				var input struct {
					Group    *string                 `json:"group"`
					Settings *config.ProjectSettings `json:"settings"`
				}
				if err := readJSON(r, &input); err != nil {
					writeError(w, err)
					return
				}
				if (input.Group == nil) == (input.Settings == nil) {
					writeError(w, store.ErrInvalid)
					return
				}
				var project store.Project
				if input.Group != nil {
					project, err = s.store.MoveProject(r.Context(), actor, name, *input.Group)
				} else {
					project, err = s.store.SetProjectSettings(r.Context(), actor, name, *input.Settings)
				}
				if err != nil {
					writeError(w, err)
					return
				}
				writeJSON(w, 200, ProjectSummary(project))
				return
			}
		case http.MethodDelete:
			if name != "" {
				err = s.store.DeleteProject(r.Context(), actor, name)
				if err != nil {
					writeError(w, err)
					return
				}
				writeJSON(w, 204, nil)
				return
			}
		}
	}
	if route == "tokens" {
		switch r.Method {
		case http.MethodPost:
			if name == "" {
				var input struct {
					Role string `json:"role"`
				}
				if err := readJSON(r, &input); err != nil {
					writeError(w, err)
					return
				}
				token, err := s.store.CreateToken(r.Context(), actor, input.Role)
				if err != nil {
					writeError(w, err)
					return
				}
				writeJSON(w, 201, token)
				return
			}
		case http.MethodDelete:
			if name != "" {
				err = s.store.RevokeToken(r.Context(), actor, name)
				if err != nil {
					writeError(w, err)
					return
				}
				writeJSON(w, 204, nil)
				return
			}
		}
	}
	writeError(w, store.ErrNotFound)
}

// 批次响应只返回队列摘要，不编码内部Definition、参数或事务标记。
type BatchView struct {
	ID     string         `json:"batch_id"`
	SHA    string         `json:"sha"`
	Builds []BuildSummary `json:"builds"`
}
type BuildSummary struct {
	ID     string `json:"id"`
	Name   string `json:"build_name"`
	Number *int64 `json:"number"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

func batchView(batch store.BatchResult) BatchView {
	result := BatchView{ID: batch.ID, SHA: batch.SHA, Builds: make([]BuildSummary, 0, len(batch.Builds))}
	for _, build := range batch.Builds {
		result.Builds = append(result.Builds, BuildSummary{ID: build.ID, Name: build.Name, Number: build.Number, Status: build.Status, Reason: build.Reason})
	}
	return result
}
func (s *Server) buildRoutes(w http.ResponseWriter, r *http.Request, actor store.Actor) bool {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(parts) == 4 && parts[0] == "api" && parts[1] == "projects" && parts[2] != "" && parts[3] == "builds" && r.Method == http.MethodPost {
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
		var input TriggerRequest
		if err := readJSON(r, &input); err != nil {
			writeError(w, err)
			return true
		}
		result, err := s.Trigger(r.Context(), actor, parts[2], keys[0], input)
		if err != nil {
			writeError(w, err)
			return true
		}
		status := 201
		if result.Replayed {
			status = 200
		}
		writeJSON(w, status, batchView(result))
		return true
	}
	if len(parts) < 2 || parts[0] != "api" || parts[1] != "builds" {
		return false
	}
	if (len(parts) != 2 && len(parts) != 3) || r.Method != http.MethodGet {
		writeError(w, store.ErrNotFound)
		return true
	}
	if actor.Role != "admin" && actor.Role != "approver" {
		writeError(w, store.ErrForbidden)
		return true
	}
	if len(parts) == 3 {
		if parts[2] == "" {
			writeError(w, store.ErrNotFound)
			return true
		}
		if _, err := readQuery(r); err != nil {
			writeError(w, err)
			return true
		}
		result, err := s.store.GetBuild(r.Context(), parts[2])
		if err != nil {
			writeError(w, err)
			return true
		}
		writeJSON(w, 200, result)
		return true
	}
	values, err := readQuery(r, "project", "group", "build_name", "batch_id", "status", "limit", "offset")
	if err != nil {
		writeError(w, err)
		return true
	}
	page, err := readPage(values)
	if err != nil {
		writeError(w, err)
		return true
	}
	items, err := s.store.ListBuilds(r.Context(), store.BuildFilter{Project: values["project"], Group: values["group"], BuildName: values["build_name"], BatchID: values["batch_id"], Status: values["status"], Page: page})
	if err != nil {
		writeError(w, err)
		return true
	}
	writeJSON(w, 200, map[string]any{"items": items, "limit": page.Limit, "offset": page.Offset})
	return true
}
