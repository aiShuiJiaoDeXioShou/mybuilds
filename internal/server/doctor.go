package server

import (
	"mybuilds/internal/store"
	"net/http"
)

// Doctor视图只报告现有控制端和最近节点证据，不在服务端执行工具。
type NodeDoctorDTO struct {
	Status string         `json:"status"`
	Reason string         `json:"reason,omitempty"`
	Node   store.NodeView `json:"node"`
}
type ControlDoctorDTO struct {
	Status  string    `json:"status"`
	Control StatusDTO `json:"control"`
}

func (s *Server) nodeDoctor(w http.ResponseWriter, r *http.Request, actor store.Actor, name string) {
	node, err := s.store.GetNode(r.Context(), actor, name)
	if err != nil {
		writeError(w, err)
		return
	}
	result := NodeDoctorDTO{Status: "passed", Node: node}
	switch {
	case node.Quarantined:
		result.Status = "failed"
		result.Reason = "stop_unconfirmed"
	case node.State == "disabled":
		result.Status = "failed"
		result.Reason = "node_disabled"
	case !node.Healthy:
		result.Status = "failed"
		result.Reason = "node_offline"
	}
	if result.Status == "passed" {
		for _, tool := range node.Tools {
			if tool.Status == "failed" {
				result.Status = "failed"
				result.Reason = "node_tools_failed"
				break
			}
		}
	}
	writeJSON(w, 200, result)
}
func (s *Server) controlDoctor(w http.ResponseWriter, r *http.Request, actor store.Actor) {
	if actor.Role != "admin" {
		writeError(w, store.ErrForbidden)
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, store.ErrNotFound)
		return
	}
	if _, err := readQuery(r); err != nil {
		writeError(w, err)
		return
	}
	status, err := s.store.Status(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, ControlDoctorDTO{Status: "passed", Control: s.statusView(status)})
}
