package server

import (
	"mybuilds/internal/config"
	"mybuilds/internal/store"
	"net/http"
	"strings"
	"time"
)

type WebhookEventView struct {
	ID            string    `json:"id"`
	Provider      string    `json:"provider"`
	Kind          string    `json:"kind"`
	DeliveryID    string    `json:"delivery_id,omitempty"`
	WindowID      string    `json:"window_id,omitempty"`
	Branch        string    `json:"branch,omitempty"`
	After         string    `json:"after,omitempty"`
	ReceivedAt    time.Time `json:"received_at"`
	Status        string    `json:"status"`
	Reason        string    `json:"reason,omitempty"`
	BodyDigest    string    `json:"body_digest"`
	ReceiptDigest string    `json:"receipt_digest"`
}
type WebhookWindowView struct {
	ID             string    `json:"id"`
	Generation     int64     `json:"generation"`
	Revision       int64     `json:"revision"`
	Provider       string    `json:"provider"`
	Branch         string    `json:"branch"`
	BuildNames     []string  `json:"build_names"`
	OpenedAt       time.Time `json:"opened_at"`
	Deadline       time.Time `json:"deadline"`
	State          string    `json:"state"`
	Reason         string    `json:"reason,omitempty"`
	CandidateSHA   string    `json:"candidate_sha,omitempty"`
	FinalSHA       string    `json:"final_sha,omitempty"`
	BatchID        string    `json:"batch_id,omitempty"`
	BuildIDs       []string  `json:"build_ids"`
	ReusedBuildIDs []string  `json:"reused_build_ids"`
}

func (s *Server) hookRoutes(w http.ResponseWriter, r *http.Request, actor store.Actor) bool {
	p := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(p) < 4 || p[0] != "api" || p[1] != "projects" || p[3] != "hook" {
		return false
	}
	if actor.Role != "admin" && (actor.Role != "approver" || r.Method != http.MethodGet) {
		writeError(w, store.ErrForbidden)
		return true
	}
	if len(p) > 5 || !validObjectName(p[2]) {
		writeError(w, store.ErrNotFound)
		return true
	}
	action := ""
	if len(p) == 5 {
		action = p[4]
	}
	allowed := []string{}
	if r.Method == http.MethodGet && (action == "events" || action == "windows") {
		allowed = []string{"limit", "offset"}
	}
	q, e := readQuery(r, allowed...)
	if e != nil {
		writeError(w, e)
		return true
	}
	if r.Method == http.MethodPost && (action == "rotate" || action == "disable" || action == "enable") {
		var empty struct{}
		if e = readJSON(r, &empty); e != nil {
			writeError(w, e)
			return true
		}
		project, e := s.store.GetProject(r.Context(), p[2])
		if e != nil {
			writeError(w, e)
			return true
		}
		settings := project.Settings
		if settings.Hook == nil {
			writeError(w, store.ErrNotFound)
			return true
		}
		hook := *settings.Hook
		settings.Hook = &hook
		if action == "enable" {
			settings.Hook.Enabled = true
		}
		if action == "disable" {
			settings.Hook.Enabled = false
		}
		v, e := s.ConfigureWebhook(r.Context(), actor, p[2], settings, action == "rotate")
		if e != nil {
			writeError(w, e)
			return true
		}
		writeJSON(w, 200, v)
		return true
	}
	if r.Method != http.MethodGet {
		writeError(w, store.ErrNotFound)
		return true
	}
	if action == "" {
		v, e := s.store.ReadWebhookPolicy(r.Context(), p[2])
		if e != nil {
			writeError(w, e)
			return true
		}
		writeJSON(w, 200, hookPolicyView(v))
		return true
	}
	page, e := readPage(q)
	if e != nil {
		writeError(w, e)
		return true
	}
	switch action {
	case "events":
		rows, e := s.store.ListWebhookEvents(r.Context(), actor, p[2], page)
		if e != nil {
			writeError(w, e)
			return true
		}
		views := make([]WebhookEventView, 0, len(rows))
		for _, v := range rows {
			views = append(views, WebhookEventSummary(v))
		}
		writeJSON(w, 200, map[string]any{"items": views, "limit": page.Limit, "offset": page.Offset})
	case "windows":
		rows, e := s.store.ListWebhookWindows(r.Context(), actor, p[2], page)
		if e != nil {
			writeError(w, e)
			return true
		}
		views := make([]WebhookWindowView, 0, len(rows))
		for _, v := range rows {
			views = append(views, WebhookWindowSummary(v))
		}
		writeJSON(w, 200, map[string]any{"items": views, "limit": page.Limit, "offset": page.Offset})
	default:
		writeError(w, store.ErrNotFound)
	}
	return true
}

// 单独配置构造只共享既有settings，不能暴露密钥响应到普通项目视图。
func hookSettingsPresent(v config.ProjectSettings) bool { return v.Hook != nil || v.Triggers != nil }

// WebhookEventSummary 供HTTP和本机CLI共用同一脱敏投影。
func WebhookEventSummary(v store.WebhookEvent) WebhookEventView {
	status := "accepted"
	if v.Event.Kind == "ignored" {
		status = "ignored"
	}
	return WebhookEventView{ID: v.ID, Provider: v.Event.Provider, Kind: v.Event.Kind, DeliveryID: v.Event.DeliveryID, WindowID: v.WindowID, Branch: v.Event.Branch, After: v.Event.After, ReceivedAt: v.ReceivedAt, Status: status, Reason: v.Event.Reason, BodyDigest: v.Event.BodyDigest, ReceiptDigest: v.Event.ReceiptDigest}
}

// WebhookWindowSummary 不公开冻结参数、私有材料或完整定义。
func WebhookWindowSummary(v store.WebhookWindow) WebhookWindowView {
	return WebhookWindowView{ID: v.ID, Generation: v.Generation, Revision: v.Revision, Provider: v.Policy.Provider, Branch: v.Branch, BuildNames: v.Policy.BuildNames, OpenedAt: v.OpenedAt, Deadline: v.Deadline, State: v.State, Reason: v.Reason, CandidateSHA: v.CandidateSHA, FinalSHA: v.FinalSHA, BatchID: v.BatchID, BuildIDs: v.BuildIDs, ReusedBuildIDs: v.ReusedBuildIDs}
}
