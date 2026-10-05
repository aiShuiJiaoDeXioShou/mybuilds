package server

import (
	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
	"net/http"
	"strings"
)

func (s *Server) approvalRoutes(w http.ResponseWriter, r *http.Request, actor store.Actor) bool {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	list := len(parts) >= 2 && parts[0] == "api" && parts[1] == "approvals"
	decision := len(parts) == 4 && parts[0] == "api" && parts[1] == "builds" && (parts[3] == "approve" || parts[3] == "reject")
	if !list && !decision {
		return false
	}
	if actor.Role != "admin" && actor.Role != "approver" {
		writeError(w, store.ErrForbidden)
		return true
	}
	if list && r.Method == http.MethodGet && len(parts) == 2 {
		query, err := readQuery(r, "project_id", "state", "limit", "offset")
		if err != nil {
			writeError(w, err)
			return true
		}
		page, err := readPage(query)
		if err != nil {
			writeError(w, err)
			return true
		}
		items, err := s.store.ListApprovals(r.Context(), actor, store.ApprovalFilter{ProjectID: query["project_id"], State: query["state"], Page: page})
		if err != nil {
			writeError(w, err)
		} else {
			writeJSON(w, 200, items)
		}
	} else if list && r.Method == http.MethodGet && len(parts) == 3 {
		if _, err := readQuery(r); err != nil {
			writeError(w, err)
			return true
		}
		result, err := s.store.GetApproval(r.Context(), actor, parts[2])
		if err != nil {
			writeError(w, err)
		} else {
			writeJSON(w, 200, result)
		}
	} else if decision && r.Method == http.MethodPost {
		if _, err := readQuery(r); err != nil {
			writeError(w, err)
			return true
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		var input struct {
			ApprovalID       string `json:"approval_id"`
			Revision         int64  `json:"revision"`
			CheckpointDigest string `json:"checkpoint_digest"`
			Note             string `json:"note"`
		}
		if err := readJSON(r, &input); err != nil {
			writeError(w, err)
			return true
		}
		result, err := s.store.DecideApproval(r.Context(), actor, parts[2], store.ApprovalDecision{ApprovalID: input.ApprovalID, Revision: input.Revision, CheckpointDigest: input.CheckpointDigest, Note: input.Note, Decision: parts[3]})
		if err != nil {
			writeError(w, err)
		} else {
			writeJSON(w, 200, result)
		}
	} else {
		writeError(w, store.ErrNotFound)
	}
	return true
}
func (s *Server) approvalAgentRoute(w http.ResponseWriter, r *http.Request, actor store.NodeActor) bool {
	if r.URL.Path != "/api/agent/approval-checkpoint" {
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
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var in protocol.ApprovalCheckpointLookup
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return true
	}
	result, err := s.store.ReadApprovalCheckpoint(r.Context(), actor, in)
	if err != nil {
		writeError(w, err)
	} else {
		writeJSON(w, 200, result)
	}
	return true
}
