package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"math"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"path"
	"slices"
	"sort"
	"strings"
	"time"
)

func hookHash(v any) string {
	data, _ := json.Marshal(v)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func hookPolicy(tx *gorm.DB, id string) (WebhookPolicy, error) {
	var row webhookPolicyRecord
	if e := tx.First(&row, "project_id = ?", id).Error; e != nil {
		return WebhookPolicy{}, e
	}
	var p WebhookPolicy
	if json.Unmarshal([]byte(row.PolicyJSON), &p) != nil {
		return p, errDatabase
	}
	var project projectRecord
	if e := tx.First(&project, "id = ?", id).Error; e != nil {
		return p, e
	}
	var settings config.ProjectSettings
	if json.Unmarshal([]byte(project.SettingsJSON), &settings) != nil {
		return p, errDatabase
	}
	p.PolicyVersion = project.PolicyVersion
	if settings.Hook == nil || !settings.Hook.Enabled {
		p.Enabled = false
	}
	p.Params = map[string]string{}
	p.BuildParams = map[string]map[string]string{}
	if settings.Pipeline != nil {
		p.Params = settings.Pipeline.Params
		for n, b := range settings.Pipeline.Builds {
			p.BuildParams[n] = b.Params
		}
	}
	return p, nil
}
func hookActor(tx *gorm.DB, a WebhookActor) (WebhookPolicy, projectRecord, error) {
	p, e := hookPolicy(tx, a.ProjectID)
	var row projectRecord
	if e != nil {
		return p, row, e
	}
	if e = tx.First(&row, "id = ?", a.ProjectID).Error; e != nil {
		return p, row, e
	}
	if !p.Enabled || a.PolicyVersion != p.PolicyVersion || row.PolicyVersion != p.PolicyVersion || a.CredentialID != p.CredentialID || a.SecretFingerprint != p.SecretFingerprint {
		return p, row, ErrForbidden
	}
	return p, row, nil
}

// ConfigureWebhook将实际管理授权、全部settings与密钥身份一次提交。
func (s *Store) ConfigureWebhook(ctx context.Context, actor Actor, in WebhookPolicyInput) (out WebhookPolicy, err error) {
	if !validID(in.Policy.ProjectID) || config.ValidateProjectSettings(in.Settings) != nil || in.Settings.Hook == nil || in.Settings.Triggers == nil {
		return out, ErrInvalid
	}
	err = s.write(ctx, func(tx *gorm.DB) error { var e error; out, e = configureWebhookTx(tx, actor, in); return e })
	return out, err
}
func (s *Store) ReadWebhookPolicy(ctx context.Context, project string) (WebhookPolicy, error) {
	if e := s.CheckLock(ctx); e != nil {
		return WebhookPolicy{}, e
	}
	var row projectRecord
	q := s.db.WithContext(ctx)
	if e := q.First(&row, "name = ?", project).Error; e != nil {
		return WebhookPolicy{}, safeError(e)
	}
	p, e := hookPolicy(q, row.ID)
	return p, safeError(e)
}
func windowView(r webhookWindowRecord) (WebhookWindow, error) {
	w := WebhookWindow{ID: r.ID, ProjectID: r.ProjectID, GroupKey: r.GroupKey, Generation: r.Generation, Revision: r.Revision, PolicyVersion: r.PolicyVersion, State: r.State, Reason: r.Reason, Branch: r.Branch, OpenedAt: r.OpenedAt.UTC(), Deadline: r.Deadline.UTC(), CandidateSHA: r.CandidateSHA, FinalSHA: r.FinalSHA, BatchID: r.BatchID, BuildIDs: []string{}, ReusedBuildIDs: []string{}}
	if json.Unmarshal([]byte(r.PolicyJSON), &w.Policy) != nil {
		return w, errDatabase
	}
	for _, x := range []struct {
		s string
		p *[]string
	}{{r.BuildIDsJSON, &w.BuildIDs}, {r.ReusedBuildIDsJSON, &w.ReusedBuildIDs}} {
		if x.s != "" && json.Unmarshal([]byte(x.s), x.p) != nil {
			return w, errDatabase
		}
	}
	return w, nil
}
func eventView(row webhookEventRecord) (WebhookEvent, error) {
	v := WebhookEvent{ID: row.ID, ProjectID: row.ProjectID, CredentialID: row.CredentialID, WindowID: row.WindowID, ReceivedAt: row.ReceivedAt.UTC()}
	if json.Unmarshal([]byte(row.EventJSON), &v.Event) != nil {
		return v, errDatabase
	}
	return v, nil
}
func receiveView(row webhookEventRecord, replayed bool) (WebhookReceipt, error) {
	v, e := eventView(row)
	if e != nil {
		return WebhookReceipt{}, e
	}
	status := "accepted"
	if v.Event.Kind == "ignored" {
		status = "ignored"
	}
	return WebhookReceipt{EventID: row.ID, WindowID: row.WindowID, Status: status, Reason: v.Event.Reason, Replayed: replayed}, nil
}
func (s *Store) ReceiveWebhook(ctx context.Context, a WebhookActor, event protocol.WebhookEvent) (out WebhookReceipt, err error) {
	if !validID(a.ProjectID) || !validHex(event.BodyDigest, 64) || event.ReceiptDigest != protocol.WebhookEventDigest(event) || len(event.DeliveryID) > 128 || !slices.Contains([]string{"push", "ignored"}, event.Kind) {
		return out, ErrInvalid
	}
	if event.Kind == "push" && (!validBranchPattern(event.Branch) || strings.ContainsAny(event.Branch, "*?[") || !validHex(event.After, 40, 64) || event.Reason != "") {
		return out, ErrInvalid
	}
	if event.Kind == "ignored" && !slices.Contains([]string{"hook_ping", "hook_event_ignored", "hook_branch_deleted"}, event.Reason) {
		return out, ErrInvalid
	}
	err = s.write(ctx, func(tx *gorm.DB) error {
		policy, project, e := hookActor(tx, a)
		if e != nil {
			return e
		}
		if event.Provider != policy.Provider || event.RepositoryKey != policy.RepositoryKey {
			return ErrForbidden
		}
		if event.Kind == "push" {
			var branches []string
			if json.Unmarshal([]byte(project.BranchesJSON), &branches) != nil {
				return errDatabase
			}
			allowed := false
			for _, pattern := range branches {
				hit, _ := path.Match(pattern, event.Branch)
				allowed = allowed || hit
			}
			if !allowed {
				return ErrForbidden
			}
		}
		// delivery和body别名分别持久化；换delivery不能延长窗口。
		if event.DeliveryID != "" {
			var alias webhookAliasRecord
			e = tx.First(&alias, "project_id = ? AND provider = ? AND delivery_id = ?", a.ProjectID, event.Provider, event.DeliveryID).Error
			if e == nil {
				if alias.Digest != event.ReceiptDigest {
					return ErrConflict
				}
				var row webhookEventRecord
				if e = tx.First(&row, "id = ?", alias.EventID).Error; e != nil {
					return e
				}
				out, e = receiveView(row, true)
				return e
			}
			if e != gorm.ErrRecordNotFound {
				return e
			}
		}
		bodyKey := hookHash(struct {
			Credential, Provider, Kind, Branch, After, Body string
			PolicyVersion                                   int64
		}{a.CredentialID, event.Provider, event.Kind, event.Branch, event.After, event.BodyDigest, policy.PolicyVersion})
		var prior webhookEventRecord
		e = tx.Order("received_at DESC, id DESC").First(&prior, "project_id = ? AND body_key = ?", a.ProjectID, bodyKey).Error
		if e == nil {
			reusable, checkErr := hookEventReusable(tx, prior)
			if checkErr != nil {
				return checkErr
			}
			if reusable {
				if event.DeliveryID != "" {
					if e = tx.Create(&webhookAliasRecord{ID: uuid.NewString(), ProjectID: a.ProjectID, Provider: event.Provider, DeliveryID: event.DeliveryID, EventID: prior.ID, Digest: event.ReceiptDigest}).Error; e != nil {
						return e
					}
				}
				out, e = receiveView(prior, true)
				return e
			}
		} else if e != gorm.ErrRecordNotFound {
			return e
		}
		now := time.Now().UTC()
		row := webhookEventRecord{ID: uuid.NewString(), ProjectID: a.ProjectID, CredentialID: a.CredentialID, Provider: event.Provider, BodyKey: bodyKey, ReceivedAt: now}
		row.EventJSON, _ = encode(event)
		if event.Kind == "push" {
			names := slices.Clone(policy.BuildNames)
			sort.Strings(names)
			group := hookHash(struct {
				Project, Provider, Credential, Branch string
				Version                               int64
				Names                                 []string
				Params                                map[string]string
				BuildParams                           map[string]map[string]string
			}{a.ProjectID, policy.Provider, policy.CredentialID, event.Branch, policy.PolicyVersion, names, policy.Params, policy.BuildParams})
			var last webhookWindowRecord
			e = tx.Order("generation DESC").First(&last, "group_key = ?", group).Error
			if e != nil && e != gorm.ErrRecordNotFound {
				return e
			}
			if e == nil && last.State == "pending" && now.Before(last.Deadline) {
				if last.Revision == math.MaxInt64 {
					return ErrConflict
				}
				last.Revision++
				last.CandidateSHA = event.After
				if e = tx.Model(&last).Updates(map[string]any{"revision": last.Revision, "candidate_sha": event.After}).Error; e != nil {
					return e
				}
				row.WindowID = last.ID
			} else {
				gen := int64(1)
				if e == nil {
					if last.Generation == math.MaxInt64 {
						return ErrConflict
					}
					gen = last.Generation + 1
				}
				encoded, _ := encode(policy)
				w := webhookWindowRecord{ID: uuid.NewString(), ProjectID: a.ProjectID, GroupKey: group, Generation: gen, Revision: 1, PolicyVersion: policy.PolicyVersion, State: "pending", Branch: event.Branch, PolicyJSON: encoded, OpenedAt: now, Deadline: now.Add(policy.QuietPeriod), CandidateSHA: event.After}
				if e = tx.Create(&w).Error; e != nil {
					return e
				}
				row.WindowID = w.ID
			}
		}
		if e = tx.Create(&row).Error; e != nil {
			return e
		}
		if event.DeliveryID != "" {
			if e = tx.Create(&webhookAliasRecord{ID: uuid.NewString(), ProjectID: a.ProjectID, Provider: event.Provider, DeliveryID: event.DeliveryID, EventID: row.ID, Digest: event.ReceiptDigest}).Error; e != nil {
				return e
			}
		}
		if _, _, e = hookActor(tx, a); e != nil {
			return e
		}
		out, e = receiveView(row, false)
		return e
	})
	return out, err
}
func (s *Store) ReadWebhookWindow(ctx context.Context, id string) (WebhookWindow, Project, error) {
	if !validID(id) {
		return WebhookWindow{}, Project{}, ErrInvalid
	}
	if e := s.CheckLock(ctx); e != nil {
		return WebhookWindow{}, Project{}, e
	}
	tx := s.db.WithContext(ctx)
	var row webhookWindowRecord
	if e := tx.First(&row, "id = ?", id).Error; e != nil {
		return WebhookWindow{}, Project{}, safeError(e)
	}
	w, e := windowView(row)
	if e != nil {
		return w, Project{}, e
	}
	var project projectRecord
	if e = tx.Preload("Group").First(&project, "id = ?", row.ProjectID).Error; e != nil {
		return w, Project{}, safeError(e)
	}
	p, e := projectView(project)
	return w, p, e
}
func (s *Store) DueWebhookWindows(ctx context.Context, page Page) ([]WebhookWindow, error) {
	page, e := normalizePage(page)
	if e != nil {
		return nil, e
	}
	if e = s.CheckLock(ctx); e != nil {
		return nil, e
	}
	var rows []webhookWindowRecord
	e = s.db.WithContext(ctx).Where("state = ? AND deadline <= ?", "pending", time.Now().UTC()).Order("deadline ASC, id ASC").Limit(page.Limit).Offset(page.Offset).Find(&rows).Error
	if e != nil {
		return nil, safeError(e)
	}
	out := make([]WebhookWindow, 0, len(rows))
	for _, r := range rows {
		v, e := windowView(r)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, nil
}
func (s *Store) ListWebhookEvents(ctx context.Context, actor Actor, project string, page Page) ([]WebhookEvent, error) {
	page, e := normalizePage(page)
	if e != nil {
		return nil, e
	}
	if e = s.CheckLock(ctx); e != nil {
		return nil, e
	}
	tx := s.db.WithContext(ctx)
	if e = authorize(tx, actor, "admin", "approver"); e != nil {
		return nil, safeError(e)
	}
	var p projectRecord
	if e = tx.First(&p, "name = ?", project).Error; e != nil {
		return nil, safeError(e)
	}
	var rows []webhookEventRecord
	if e = tx.Where("project_id = ?", p.ID).Order("received_at DESC, id ASC").Limit(page.Limit).Offset(page.Offset).Find(&rows).Error; e != nil {
		return nil, safeError(e)
	}
	out := make([]WebhookEvent, 0, len(rows))
	for _, r := range rows {
		v, e := eventView(r)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	if e := authorize(tx, actor, "admin", "approver"); e != nil {
		return nil, safeError(e)
	}
	if e := s.CheckLock(ctx); e != nil {
		return nil, e
	}
	return out, nil
}
func (s *Store) ListWebhookWindows(ctx context.Context, actor Actor, project string, page Page) ([]WebhookWindow, error) {
	page, e := normalizePage(page)
	if e != nil {
		return nil, e
	}
	if e = s.CheckLock(ctx); e != nil {
		return nil, e
	}
	tx := s.db.WithContext(ctx)
	if e = authorize(tx, actor, "admin", "approver"); e != nil {
		return nil, safeError(e)
	}
	var p projectRecord
	if e = tx.First(&p, "name = ?", project).Error; e != nil {
		return nil, safeError(e)
	}
	var rows []webhookWindowRecord
	if e = tx.Where("project_id = ?", p.ID).Order("opened_at DESC, id ASC").Limit(page.Limit).Offset(page.Offset).Find(&rows).Error; e != nil {
		return nil, safeError(e)
	}
	out := make([]WebhookWindow, 0, len(rows))
	for _, r := range rows {
		v, e := windowView(r)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	if e := authorize(tx, actor, "admin", "approver"); e != nil {
		return nil, safeError(e)
	}
	if e := s.CheckLock(ctx); e != nil {
		return nil, e
	}
	return out, nil
}

func configureWebhookTx(tx *gorm.DB, actor Actor, in WebhookPolicyInput) (out WebhookPolicy, err error) {
	err = func() error {
		if e := authorize(tx, actor, "admin"); e != nil {
			return e
		}
		var row projectRecord
		if e := tx.First(&row, "id = ?", in.Policy.ProjectID).Error; e != nil {
			return e
		}
		if row.PolicyVersion != in.ExpectedPolicyVersion || row.PolicyVersion == math.MaxInt64 {
			return ErrConflict
		}
		p := in.Policy
		h, tr := in.Settings.Hook, in.Settings.Triggers
		if p.Provider != row.Provider || p.Enabled != h.Enabled || p.RepositoryKey != h.RepositoryKey || p.QuietPeriod < 0 || p.QuietPeriod > 24*time.Hour || !slices.Equal(p.BuildNames, tr.Builds) || p.AllowUpload != tr.AllowUpload {
			return ErrInvalid
		}
		quiet := time.Duration(0)
		if tr.QuietPeriod != "" {
			var e error
			quiet, e = time.ParseDuration(tr.QuietPeriod)
			if e != nil {
				return ErrInvalid
			}
		}
		if quiet != p.QuietPeriod || len(p.BuildNames) == 0 || len(p.BuildNames) > 64 || p.Enabled && (p.CredentialID == "" || !validHex(p.SecretFingerprint, 64) || p.RepositoryKey == "") {
			return ErrInvalid
		}
		if row.Provider != "generic" {
			for _, r := range p.RepositoryKey {
				if r < '0' || r > '9' {
					return ErrInvalid
				}
			}
			if strings.TrimLeft(p.RepositoryKey, "0") == "" || strings.HasPrefix(p.RepositoryKey, "0") {
				return ErrInvalid
			}
		}
		if in.Settings.Pipeline == nil {
			var old config.ProjectSettings
			if json.Unmarshal([]byte(row.SettingsJSON), &old) != nil {
				return errDatabase
			}
			in.Settings.Pipeline = old.Pipeline
		}
		p.Generic = h.Generic
		p.SecretRef = h.Secret
		p.PolicyVersion = row.PolicyVersion + 1
		p.Params = map[string]string{}
		p.BuildParams = map[string]map[string]string{}
		if in.Settings.Pipeline != nil {
			p.Params = in.Settings.Pipeline.Params
			for name, b := range in.Settings.Pipeline.Builds {
				p.BuildParams[name] = b.Params
			}
		}
		settings, e := encode(in.Settings)
		if e != nil {
			return e
		}
		encoded, e := encode(p)
		if e != nil {
			return e
		}
		if e = tx.Model(&projectRecord{}).Where("id = ?", row.ID).Updates(map[string]any{"settings_json": settings, "policy_version": p.PolicyVersion, "updated_at": time.Now().UTC()}).Error; e != nil {
			return e
		}
		var old webhookPolicyRecord
		e = tx.First(&old, "project_id = ?", row.ID).Error
		if e == gorm.ErrRecordNotFound {
			e = tx.Create(&webhookPolicyRecord{ProjectID: row.ID, PolicyJSON: encoded}).Error
		} else if e == nil {
			e = tx.Model(&old).Update("policy_json", encoded).Error
		}
		if e != nil {
			return e
		}
		if e = audit(tx, actor, "webhook_configure", row.ID, "", ""); e != nil {
			return e
		}
		out = p
		return nil
	}()
	return out, err
}

// body别名不能跨已失败的窗口永久拒绝下一次无delivery请求。
func hookEventReusable(tx *gorm.DB, event webhookEventRecord) (bool, error) {
	if event.WindowID == "" {
		return true, nil
	}
	var row webhookWindowRecord
	if err := tx.First(&row, "id = ?", event.WindowID).Error; err != nil {
		return false, err
	}
	if row.State == "pending" {
		return time.Now().UTC().Before(row.Deadline), nil
	}
	if row.State != "closed" {
		return false, nil
	}
	view, err := windowView(row)
	if err != nil {
		return false, err
	}
	if len(view.BuildIDs) == 0 {
		return false, nil
	}
	var count int64
	err = tx.Model(&buildRecord{}).Where("id IN ? AND (status IN ? OR stop_unconfirmed = ?)", view.BuildIDs, []string{"queued", "running", "cancel_requested", "waiting_approval", "approved", "succeeded"}, true).Count(&count).Error
	return count == int64(len(view.BuildIDs)), err
}
