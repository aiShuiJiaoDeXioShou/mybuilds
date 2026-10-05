package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"mybuilds/internal/config"
	"mybuilds/internal/pipeline"
	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
)

type TriggerRequest struct {
	Branch      string                       `json:"branch"`
	Ref         string                       `json:"ref,omitempty"`
	BuildNames  []string                     `json:"build_names,omitempty"`
	All         bool                         `json:"all"`
	Params      map[string]string            `json:"params,omitempty"`
	BuildParams map[string]map[string]string `json:"build_params,omitempty"`
	AllowUpload bool                         `json:"allow_upload"`
}

var parameterName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var nodeSecret = regexp.MustCompile(`^\$\{[A-Za-z_][A-Za-z0-9_]*\}$`)

func validRequestKey(key string) bool {
	if key == "" || len(key) > 128 {
		return false
	}
	for _, r := range key {
		if unicode.IsSpace(r) || unicode.IsControl(r) || r == '/' || r == '\\' {
			return false
		}
	}
	return true
}
func boundedParams(params map[string]string) bool {
	if len(params) > 128 {
		return false
	}
	for key, value := range params {
		if !parameterName.MatchString(key) || key == "project" || key == "workspace" || len(value) > 4096 {
			return false
		}
	}
	return true
}

// 请求摘要基于业务请求，而非HEAD与最终参数；重放在Git读取前返回原结果。
func requestDigest(project string, request TriggerRequest) (string, error) {
	if !validObjectName(project) || len(request.BuildNames) > 64 || len(request.BuildParams) > 64 || !boundedParams(request.Params) || (request.All && len(request.BuildNames) != 0) {
		return "", store.ErrInvalid
	}
	if request.Branch == "" {
		request.Branch = "main"
	}
	if len(request.Branch) > 1024 || len(request.Ref) > 64 {
		return "", store.ErrInvalid
	}
	for _, value := range []string{request.Branch, request.Ref} {
		for _, r := range value {
			if unicode.IsControl(r) {
				return "", store.ErrInvalid
			}
		}
	}
	seen := map[string]bool{}
	for _, name := range request.BuildNames {
		if len(name) > 64 || !validRequestKey(name) || seen[name] {
			return "", store.ErrInvalid
		}
		seen[name] = true
	}
	for name, params := range request.BuildParams {
		if len(name) > 64 || !validRequestKey(name) || !boundedParams(params) {
			return "", store.ErrInvalid
		}
	}
	if request.BuildNames == nil {
		request.BuildNames = []string{}
	}
	if request.Params == nil {
		request.Params = map[string]string{}
	}
	if request.BuildParams == nil {
		request.BuildParams = map[string]map[string]string{}
	}
	data, err := json.Marshal(struct {
		Project string         `json:"project"`
		Request TriggerRequest `json:"request"`
	}{project, request})
	if err != nil {
		return "", store.ErrInvalid
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}
func (s *Server) Trigger(ctx context.Context, actor store.Actor, project, key string, request TriggerRequest) (store.BatchResult, error) {
	if actor.Role != "admin" && actor.Role != "trigger" {
		return store.BatchResult{}, store.ErrForbidden
	}
	if !validRequestKey(key) {
		return store.BatchResult{}, store.ErrInvalid
	}
	digest, err := requestDigest(project, request)
	if err != nil {
		return store.BatchResult{}, err
	}
	prior, err := s.store.FindRequest(ctx, actor, key, digest)
	if err != nil {
		return store.BatchResult{}, err
	}
	if prior != nil {
		return *prior, nil
	}
	p, err := s.store.GetProject(ctx, project)
	if err != nil {
		return store.BatchResult{}, err
	}
	if request.Branch == "" {
		request.Branch = "main"
	}
	authorized := false
	for _, pattern := range p.Branches {
		match, _ := path.Match(pattern, request.Branch)
		authorized = authorized || match
	}
	if !authorized {
		return store.BatchResult{}, store.ErrForbidden
	}
	source, err := s.resolvePipeline(ctx, p, request.Branch, request.Ref)
	if err != nil {
		return store.BatchResult{}, errPipeline
	}
	prepared, names, hasUpload, err := s.prepareTrigger(ctx, p, source, request, actor.Role == "admin", nil)
	if err != nil {
		return store.BatchResult{}, err
	}
	return s.store.Enqueue(ctx, store.EnqueueInput{Actor: actor, ProjectID: p.ID, ProjectVersion: p.PolicyVersion, Key: key, RequestDigest: digest, SHA: source.SHA, Branch: request.Branch, Source: source.Mode, File: source.File, SourceDigest: source.SourceDigest, HasUpload: hasUpload, AllowUpload: request.AllowUpload, Builds: prepared, SelectedBuilds: names})
}

// prepareTrigger用于手动与Webhook同一冻结来源，不执行任何用户脚本。
func (s *Server) prepareTrigger(ctx context.Context, p store.Project, source resolvedPipeline, request TriggerRequest, admin bool, changes map[string]*protocol.ChangeFacts) ([]store.PreparedBuild, []string, bool, error) {
	document := source.Document
	names, err := document.Select(request.BuildNames, request.All)
	if err != nil || len(names) > 64 {
		return nil, nil, false, errPipeline
	}
	for name := range request.BuildParams {
		if !slices.Contains(names, name) {
			return nil, nil, false, store.ErrInvalid
		}
	}
	hasUpload := false
	for _, name := range names {
		b := document.Builds[name]
		for _, step := range b.Steps {
			hasUpload = hasUpload || step.Kind == "upload"
		}
	}
	// 发布权限依据所有所选定义检查，false when不能豁免。
	if hasUpload && (!admin || !request.AllowUpload) {
		return nil, nil, false, store.ErrForbidden
	}
	prepared := make([]store.PreparedBuild, 0, len(names))
	for _, name := range names {
		b := document.Builds[name]
		if b.Runner == nil && p.DefaultNode == "" {
			return nil, nil, false, errPipeline
		}
		overrides := map[string]string{}
		if settings := p.Settings.Pipeline; settings != nil {
			if name == "default" {
				maps.Copy(overrides, settings.Params)
			}
			maps.Copy(overrides, settings.Builds[name].Params)
		}
		maps.Copy(overrides, request.Params)
		maps.Copy(overrides, request.BuildParams[name])
		params, err := config.ResolveParams(b, overrides)
		if err != nil || !boundedParams(params) {
			return nil, nil, false, errPipeline
		}
		facts := map[string]string{"project": p.Name, "build.name": name, "git.branch": request.Branch, "git.sha": source.SHA}
		plan, err := pipeline.Preview(document, pipeline.PreviewOptions{Names: []string{name}, Params: params, Facts: facts, Changes: changes[name]})
		if err != nil {
			return nil, nil, false, errPipeline
		}
		preview := plan.Builds[0]
		notification := b.Notifications
		if notification == nil {
			notification = document.Notifications
		}
		notification = config.ResolveNotifications(p.Settings.Notifications, notification, s.config.Defaults.Notifications)
		if notification != nil && (notification.Enabled == nil || *notification.Enabled) {
			return nil, nil, false, errUnsupported
		}
		for _, step := range b.Steps {
			if step.Kind == "upload" && step.Credentials != "" && !nodeSecret.MatchString(step.Credentials) {
				return nil, nil, false, errPipeline
			}
		}
		when := pipeline.WhenCondition(b.When, params, facts, changes[name])
		condition, reasons := when.Condition, when.Reasons
		if len(reasons) > 1 {
			reasons = slices.DeleteFunc(slices.Clone(reasons), func(reason string) bool { return reason == "条件已满足" })
		}
		definition := *b
		definition.Notifications = nil
		build := store.PreparedBuild{Name: name, Status: "queued", Snapshot: store.BuildSnapshot{Origin: source.Origins[name], Definition: definition, Changes: changes[name], Params: params, Facts: facts, Condition: condition, Reasons: reasons, AllowedNodes: slices.Clone(p.AllowedNodes), DefaultNode: p.DefaultNode}, PostBudgetNS: int64(2 * time.Minute)}
		if condition == "skipped" {
			build.Status = "skipped"
			build.Reason = "condition_skipped"
		}
		if b.Timeout != "" {
			duration, _ := time.ParseDuration(b.Timeout)
			budget := int64(duration)
			build.InitialBudgetNS = &budget
		}
		if b.Post != nil && b.Post.Timeout != "" {
			duration, _ := time.ParseDuration(b.Post.Timeout)
			build.PostBudgetNS = int64(duration)
		}
		for _, step := range preview.Steps {
			build.Steps = append(build.Steps, stepProgress("ordinary", step))
		}
		if preview.Post != nil {
			for _, phase := range []struct {
				name  string
				steps []pipeline.StepPreview
			}{{"success", preview.Post.Success}, {"failure", preview.Post.Failure}, {"always", preview.Post.Always}} {
				for _, step := range phase.steps {
					build.Steps = append(build.Steps, stepProgress(phase.name, step))
				}
			}
		}
		build.Snapshot.ComparisonKey = store.ComparisonKey(p.ID, request.Branch, name, names, build.Snapshot)
		prepared = append(prepared, build)
	}
	return prepared, names, hasUpload, nil
}

func buildCondition(when *config.When, params map[string]string, branch string) (string, []string) {
	v := pipeline.WhenCondition(when, params, map[string]string{"git.branch": branch}, nil)
	return v.Condition, v.Reasons
}
func stepProgress(phase string, step pipeline.StepPreview) store.StepProgress {
	status := "pending"
	if phase == "ordinary" && step.Condition == "skipped" {
		status = "skipped"
	}
	return store.StepProgress{Phase: phase, Index: step.Index, Name: step.Name, Kind: step.Kind, Condition: step.Condition, Status: status, Reasons: slices.Clone(step.Reasons)}
}

func validObjectName(name string) bool {
	if name == "" || len(name) > 64 || name == "." || name == ".." || strings.TrimSpace(name) != name || strings.ContainsAny(name, "/\\") {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
