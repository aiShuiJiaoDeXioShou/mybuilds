package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"github.com/google/uuid"
	"io"
	"mime"
	"mybuilds/internal/config"
	"mybuilds/internal/scm"
	"mybuilds/internal/store"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"
)

type WebhookPolicyView struct {
	Enabled       bool     `json:"enabled"`
	Provider      string   `json:"provider"`
	RepositoryKey string   `json:"repository_key"`
	Builds        []string `json:"builds"`
	QuietPeriod   string   `json:"quiet_period"`
	AllowUpload   bool     `json:"allow_upload"`
	PolicyVersion int64    `json:"policy_version"`
}
type WebhookConfigured struct {
	View   WebhookPolicyView `json:"hook"`
	Secret string            `json:"secret,omitempty"`
}

func hookPolicyView(p store.WebhookPolicy) WebhookPolicyView {
	return WebhookPolicyView{Enabled: p.Enabled, Provider: p.Provider, RepositoryKey: p.RepositoryKey, Builds: slices.Clone(p.BuildNames), QuietPeriod: p.QuietPeriod.String(), AllowUpload: p.AllowUpload, PolicyVersion: p.PolicyVersion}
}
func hookFingerprint(secret string) string {
	s := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(s[:])
}

// hookMaterials使用控制端自有固定目录和实际普通fd，不读取宿主环境。
func (s *Server) hookMaterials(project, credential string, create string) (string, error) {
	if uuid.Validate(project) != nil || uuid.Validate(credential) != nil {
		return "", scm.ErrHookUnauthorized
	}
	dataInfo, e := os.Lstat(s.config.DataDir)
	if e != nil || !evidenceInfo(dataInfo, true) {
		return "", errEvidence
	}
	root, e := os.OpenRoot(s.config.DataDir)
	if e != nil {
		return "", errEvidence
	}
	defer root.Close()
	name := "hooks"
	for _, part := range []string{"hooks", project} {
		if part != "hooks" {
			name += "/" + part
		}
		if create != "" {
			if e = root.Mkdir(name, 0700); e != nil && !os.IsExist(e) {
				return "", errEvidence
			}
		}
		info, e := root.Lstat(name)
		if e != nil || !evidenceInfo(info, true) {
			return "", errEvidence
		}
	}
	child, e := root.OpenRoot(name)
	if e != nil {
		return "", errEvidence
	}
	defer child.Close()
	held, e := child.Stat(".")
	current, ce := root.Lstat(name)
	if e != nil || ce != nil || !os.SameFile(held, current) {
		return "", errEvidence
	}
	file := credential + ".key"
	flags := os.O_RDONLY | evidenceOpenFlags()
	if create != "" {
		flags = os.O_WRONLY | os.O_CREATE | os.O_EXCL | evidenceOpenFlags()
	}
	f, e := child.OpenFile(file, flags, 0600)
	if e != nil {
		return "", errEvidence
	}
	defer f.Close()
	opened, e := f.Stat()
	if e != nil || !evidenceInfo(opened, false) {
		return "", errEvidence
	}
	if create != "" {
		if _, e = f.WriteString(create); e != nil {
			return "", errEvidence
		}
		if e = f.Sync(); e != nil {
			return "", errEvidence
		}
		if e = retentionSyncDirectory(child); e != nil {
			return "", errEvidence
		}
	} else {
		data, e := io.ReadAll(io.LimitReader(f, 4097))
		if e != nil || len(data) < 32 || len(data) > 4096 {
			return "", errEvidence
		}
		create = string(data)
	}
	leaf, e := child.Lstat(file)
	current, ce = root.Lstat(name)
	parent, pe := os.Lstat(s.config.DataDir)
	if e != nil || ce != nil || pe != nil || !os.SameFile(leaf, opened) || !os.SameFile(current, held) || !os.SameFile(parent, dataInfo) || !evidenceInfo(parent, true) {
		return "", errEvidence
	}
	return create, nil
}
func (s *Server) hookSecret(p store.WebhookPolicy) (string, error) {
	var secret string
	var e error
	if p.SecretRef != "" {
		name := strings.TrimSuffix(strings.TrimPrefix(p.SecretRef, "${"), "}")
		var values map[string]string
		values, e = config.LoadWebhookSecrets(s.config.WebhookSecretsFile, []string{name})
		secret = values[name]
	} else {
		secret, e = s.hookMaterials(p.ProjectID, p.CredentialID, "")
	}
	if e != nil || hookFingerprint(secret) != p.SecretFingerprint {
		return "", scm.ErrHookUnauthorized
	}
	return secret, nil
}

// ConfigureWebhook只在真实管理员授权后读取仓库及准备私有材料。
func (s *Server) ConfigureWebhook(ctx context.Context, actor store.Actor, project string, settings config.ProjectSettings, rotate bool) (WebhookConfigured, error) {
	if actor.Role != "admin" {
		return WebhookConfigured{}, store.ErrForbidden
	}
	if config.ValidateProjectSettings(settings) != nil {
		return WebhookConfigured{}, store.ErrInvalid
	}
	p, e := s.store.GetProject(ctx, project)
	if e != nil {
		return WebhookConfigured{}, e
	}
	policy, settings, generated, e := s.prepareWebhook(ctx, p, settings, rotate)
	if e != nil {
		return WebhookConfigured{}, e
	}

	policy, e = s.store.ConfigureWebhook(ctx, actor, store.WebhookPolicyInput{Policy: policy, Settings: settings, ExpectedPolicyVersion: p.PolicyVersion})
	if e != nil {
		return WebhookConfigured{}, e
	}
	return WebhookConfigured{View: hookPolicyView(policy), Secret: generated}, nil
}
func hookHTTPError(w http.ResponseWriter, e error) {
	status, code := 400, "hook_invalid"
	switch {
	case errors.Is(e, scm.ErrHookUnauthorized), errors.Is(e, store.ErrNotFound):
		status, code = 401, "hook_unauthorized"
	case errors.Is(e, scm.ErrHookForbidden), errors.Is(e, store.ErrForbidden):
		status, code = 403, "hook_forbidden"
	case errors.Is(e, store.ErrConflict):
		status, code = 409, "hook_conflict"
	case errors.Is(e, scm.ErrHookLimit):
		status, code = 413, "hook_limit"
	case errors.Is(e, store.ErrLockLost):
		status, code = 503, "service_unavailable"
	case !errors.Is(e, scm.ErrHookInvalid) && !errors.Is(e, store.ErrInvalid):
		status, code = 503, "database_error"
	}
	writeJSON(w, status, map[string]string{"error": code})
}
func (s *Server) receiveWebhook(w http.ResponseWriter, r *http.Request, project string) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	if r.Method != http.MethodPost || !validObjectName(project) || r.URL.RawQuery != "" {
		hookHTTPError(w, scm.ErrHookInvalid)
		return
	}
	ct, params, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if e != nil || ct != "application/json" || r.Header.Get("Content-Encoding") != "" || params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8") {
		writeJSON(w, 415, map[string]string{"error": "hook_media_unsupported"})
		return
	}
	p, e := s.store.ReadWebhookPolicy(ctx, project)
	if e != nil || !p.Enabled {
		hookHTTPError(w, scm.ErrHookUnauthorized)
		return
	}
	secret, e := s.hookSecret(p)
	if e != nil {
		hookHTTPError(w, scm.ErrHookUnauthorized)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	defer r.Body.Close()
	raw, e := io.ReadAll(r.Body)
	if e != nil {
		hookHTTPError(w, scm.ErrHookLimit)
		return
	}
	event, e := scm.ParseWebhook(ctx, scm.HookInput{Provider: p.Provider, RepositoryKey: p.RepositoryKey, Header: r.Header, Body: raw, Secret: secret, Generic: p.Generic})
	if e != nil {
		hookHTTPError(w, e)
		return
	}
	result, e := s.store.ReceiveWebhook(ctx, store.WebhookActor{ProjectID: p.ProjectID, CredentialID: p.CredentialID, SecretFingerprint: p.SecretFingerprint, PolicyVersion: p.PolicyVersion}, event)
	if e != nil {
		hookHTTPError(w, e)
		return
	}
	status := 202
	if result.Replayed || result.Status == "ignored" {
		status = 200
	}
	writeJSON(w, status, struct {
		EventID  string `json:"event_id"`
		WindowID string `json:"window_id,omitempty"`
		Status   string `json:"status"`
		Reason   string `json:"reason,omitempty"`
		Replayed bool   `json:"replayed"`
	}{result.EventID, result.WindowID, result.Status, result.Reason, result.Replayed})
}

func (s *Server) prepareWebhook(ctx context.Context, p store.Project, settings config.ProjectSettings, rotate bool) (store.WebhookPolicy, config.ProjectSettings, string, error) {
	var e error
	prior, priorErr := s.store.ReadWebhookPolicy(ctx, p.Name)
	if settings.Hook == nil {
		if priorErr != nil {
			return store.WebhookPolicy{}, settings, "", store.ErrInvalid
		}
		settings.Hook = p.Settings.Hook
	}
	if settings.Triggers == nil {
		settings.Triggers = p.Settings.Triggers
	}
	if settings.Triggers == nil {
		settings.Triggers = &config.TriggerSettings{QuietPeriod: "0s"}
	}
	if settings.Pipeline == nil {
		settings.Pipeline = p.Settings.Pipeline
	}
	if len(settings.Triggers.Builds) == 0 {
		source, e := s.resolvePipeline(ctx, p, p.Branches[0], "")
		if e != nil {
			return store.WebhookPolicy{}, settings, "", errPipeline
		}
		doc := source.Document
		if doc == nil || len(doc.Builds) != 1 {
			return store.WebhookPolicy{}, settings, "", store.ErrInvalid
		}
		for name := range doc.Builds {
			settings.Triggers.Builds = []string{name}
		}
	}
	quiet := time.Duration(0)
	if settings.Triggers.QuietPeriod != "" {
		quiet, _ = time.ParseDuration(settings.Triggers.QuietPeriod)
	}
	policy := store.WebhookPolicy{ProjectID: p.ID, Provider: p.Provider, RepositoryKey: settings.Hook.RepositoryKey, Enabled: settings.Hook.Enabled, BuildNames: settings.Triggers.Builds, QuietPeriod: quiet, AllowUpload: settings.Triggers.AllowUpload}
	generated := ""
	secret := ""
	if settings.Hook.Secret != "" {
		if rotate {
			return store.WebhookPolicy{}, settings, "", store.ErrInvalid
		}
		name := strings.TrimSuffix(strings.TrimPrefix(settings.Hook.Secret, "${"), "}")
		values, e := config.LoadWebhookSecrets(s.config.WebhookSecretsFile, []string{name})
		if e != nil {
			return store.WebhookPolicy{}, settings, "", scm.ErrHookUnauthorized
		}
		secret = values[name]
	} else if priorErr == nil && !rotate && prior.SecretRef == "" {
		secret, e = s.hookSecret(prior)
		if e != nil {
			return store.WebhookPolicy{}, settings, "", e
		}
		policy.CredentialID = prior.CredentialID
	} else {
		buf := make([]byte, 32)
		if _, e = rand.Read(buf); e != nil {
			return store.WebhookPolicy{}, settings, "", errEvidence
		}
		generated = base64.RawURLEncoding.EncodeToString(buf)
		secret = generated
	}
	if policy.CredentialID == "" {
		policy.CredentialID = uuid.NewString()
	}
	if generated != "" {
		if _, e = s.hookMaterials(p.ID, policy.CredentialID, generated); e != nil {
			return store.WebhookPolicy{}, settings, "", e
		}
	}
	policy.SecretFingerprint = hookFingerprint(secret)
	return policy, settings, generated, nil
}

// CreateProject与自产材料一次真实Store创建提交；失败的私有孤儿不是有效凭据。
func (s *Server) CreateProject(ctx context.Context, actor store.Actor, in store.ProjectInput) (store.Project, *WebhookConfigured, error) {
	if actor.Role != "admin" {
		return store.Project{}, nil, store.ErrForbidden
	}
	if config.ValidateProjectSettings(in.Settings) != nil {
		return store.Project{}, nil, store.ErrInvalid
	}
	if !hookSettingsPresent(in.Settings) {
		p, e := s.store.CreateProject(ctx, actor, in)
		return p, nil, e
	}
	if in.Settings.Hook == nil {
		return store.Project{}, nil, store.ErrInvalid
	}
	if in.Provider == "" {
		in.Provider = "generic"
	}
	if len(in.Branches) == 0 {
		in.Branches = []string{"main"}
	}
	p := store.Project{ID: uuid.NewString(), Name: in.Name, Repository: in.Repository, Provider: in.Provider, Branches: in.Branches, Settings: in.Settings, PolicyVersion: 1}
	policy, settings, secret, e := s.prepareWebhook(ctx, p, in.Settings, false)
	if e != nil {
		return store.Project{}, nil, e
	}
	in.ID = p.ID
	in.Settings = settings
	in.PreparedHook = &policy
	created, e := s.store.CreateProject(ctx, actor, in)
	if e != nil {
		return store.Project{}, nil, e
	}
	policy.PolicyVersion = created.PolicyVersion
	configured := &WebhookConfigured{View: hookPolicyView(policy), Secret: secret}
	return created, configured, nil
}
