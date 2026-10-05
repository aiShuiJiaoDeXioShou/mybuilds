package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
)

// retentionRoutes沿真实管理事务推进固定中央事项，不同步启动配置或授节点执行权。
func (s *Server) retentionRoutes(w http.ResponseWriter, r *http.Request, actor store.Actor) bool {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if (len(parts) != 4 && len(parts) != 5) || parts[0] != "api" || parts[1] != "projects" || parts[2] == "" || parts[3] != "retention" || (len(parts) == 5 && parts[4] != "candidates" && parts[4] != "jobs") {
		return false
	}
	// 先拒绝角色，再查询名称，避免向非管理员暴露项目存在性。
	if actor.Role != "admin" {
		writeError(w, store.ErrForbidden)
		return true
	}
	post := len(parts) == 4 && r.Method == http.MethodPost
	if r.Method != http.MethodGet && !post {
		writeError(w, store.ErrNotFound)
		return true
	}
	var page store.Page
	var limit int
	if len(parts) == 5 {
		query, err := readQuery(r, "limit", "offset")
		if err != nil {
			writeError(w, err)
			return true
		}
		page, err = readPage(query)
		if err != nil {
			writeError(w, err)
			return true
		}
	} else {
		if _, err := readQuery(r); err != nil {
			writeError(w, err)
			return true
		}
		if post {
			var in struct {
				Limit int `json:"limit"`
			}
			if err := readJSONLimit(r, &in, 32*1024); err != nil {
				writeError(w, err)
				return true
			}
			// 缺失字段会解为0，明确拒绝；不把省略值解释成默认删除授权。
			if in.Limit < 1 || in.Limit > 100 {
				writeError(w, store.ErrInvalid)
				return true
			}
			limit = in.Limit
		}
	}
	project, err := s.store.GetProject(r.Context(), parts[2])
	if err != nil {
		writeError(w, err)
		return true
	}
	if post {
		if _, err = s.store.ScheduleRetention(r.Context(), actor, project.ID, limit); err != nil {
			writeError(w, err)
			return true
		}
		if err = s.advanceCentralRetention(r.Context(), project.ID, limit); err != nil {
			writeError(w, err)
			return true
		}
		if _, err = s.store.FinalizeRetention(r.Context(), project.ID, limit); err != nil {
			writeError(w, err)
			return true
		}
		view, err := s.store.ListRetention(r.Context(), actor, project.ID, store.Page{Limit: limit})
		if err != nil {
			writeError(w, err)
			return true
		}
		writeJSON(w, http.StatusOK, view)
		return true
	}
	if len(parts) == 5 {
		var view store.RetentionPage
		if parts[4] == "jobs" {
			view, err = s.store.ListRetention(r.Context(), actor, project.ID, page)
		} else {
			view, err = s.store.EvaluateRetention(r.Context(), actor, project.ID, page)
		}
		if err != nil {
			writeError(w, err)
			return true
		}
		writeJSON(w, http.StatusOK, view)
		return true
	}
	view, err := s.store.EffectiveRetention(r.Context(), actor, project.ID)
	if err != nil {
		writeError(w, err)
		return true
	}
	writeJSON(w, http.StatusOK, view)
	return true
}

// 节点归属登记沿当前独立身份与Store完整fence校验，不接收实际路径或执行命令。
func (s *Server) retentionAgentRoutes(w http.ResponseWriter, r *http.Request, actor store.NodeActor) bool {
	if r.URL.Path == "/api/agent/deletions" || strings.HasPrefix(r.URL.Path, "/api/agent/deletions/") {
		s.nodeDeletionRoute(w, r, actor)
		return true
	}
	if r.URL.Path != "/api/agent/resources" {
		return false
	}
	if r.Method != http.MethodPost {
		writeError(w, store.ErrNotFound)
		return true
	}
	if _, err := readQuery(r); err != nil {
		writeError(w, err)
		return true
	}
	var in protocol.NodeResourceRegistration
	// 只为本声明保留已严格校验的有界原文，区分显式零游标与省略字段。
	// 不替换原请求Body，保持net/http对原连接的关闭与deadline所有权。
	var original bytes.Buffer
	request := r.Clone(r.Context())
	request.Body = io.NopCloser(io.TeeReader(r.Body, &original))
	if err := readJSONLimit(request, &in, 32*1024); err != nil {
		writeError(w, err)
		return true
	}
	if in.Completion != nil {
		var fields struct {
			Completion map[string]json.RawMessage `json:"completion"`
		}
		// 已有严格校验拒绝未知键/null/重复；恰好五个合法键才是完整完成声明。
		if json.Unmarshal(original.Bytes(), &fields) != nil || len(fields.Completion) != 5 {
			writeError(w, store.ErrInvalid)
			return true
		}
	}
	if err := s.store.RegisterNodeResource(r.Context(), actor, in); err != nil {
		writeError(w, err)
		return true
	}
	writeJSON(w, http.StatusNoContent, nil)
	return true
}

// 清理管理仅接收固定事项身份，既不下发私有路径，也不授用户动作执行权。
func (s *Server) nodeDeletionRoute(w http.ResponseWriter, r *http.Request, actor store.NodeActor) {
	if r.URL.Path == "/api/agent/deletions" && r.Method == http.MethodGet {
		query, err := readQuery(r, "limit")
		if err != nil {
			writeError(w, err)
			return
		}
		limit := 10
		if values, exists := query["limit"]; exists {
			limit, err = strconv.Atoi(values)
			if err != nil || limit < 1 || limit > 10 || strconv.Itoa(limit) != values {
				writeError(w, store.ErrInvalid)
				return
			}
		}
		items, err := s.store.ClaimNodeDeletions(r.Context(), actor, limit)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, items)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/agent/deletions/"), "/")
	if len(parts) != 2 || r.Method != http.MethodPost || (parts[1] != "authorize" && parts[1] != "confirm") {
		writeError(w, store.ErrNotFound)
		return
	}
	id, err := uuid.Parse(parts[0])
	if err != nil || id.String() != parts[0] {
		writeError(w, store.ErrInvalid)
		return
	}
	if _, err = readQuery(r); err != nil {
		writeError(w, err)
		return
	}
	if parts[1] == "authorize" {
		var empty struct{}
		if err = readJSONLimit(r, &empty, 32*1024); err != nil {
			writeError(w, err)
			return
		}
		authority, err := s.store.AuthorizeNodeDeletion(r.Context(), actor, parts[0])
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, authority)
		return
	}
	var in protocol.NodeDeletionConfirmation
	var original bytes.Buffer
	request := r.Clone(r.Context())
	request.Body = io.NopCloser(io.TeeReader(r.Body, &original))
	if err = readJSONLimit(request, &in, 32*1024); err != nil {
		writeError(w, err)
		return
	}
	var fields map[string]json.RawMessage
	// 严格解码已限制合法键；九个字段必须全显式，包括成功时的空reason。
	if json.Unmarshal(original.Bytes(), &fields) != nil || len(fields) != 9 || in.ID != parts[0] {
		writeError(w, store.ErrInvalid)
		return
	}
	if err = s.store.ConfirmNodeDeletion(r.Context(), actor, in); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, protocol.NodeDeletionReceipt{ID: in.ID, Seq: in.Seq, Digest: in.Digest})
}
