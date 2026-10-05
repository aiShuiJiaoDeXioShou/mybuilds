package scm

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	githubhook "github.com/go-playground/webhooks/v6/github"
	gitlabhook "github.com/go-playground/webhooks/v6/gitlab"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
)

var ErrHookInvalid = errors.New("hook_invalid")
var ErrHookUnauthorized = errors.New("hook_unauthorized")
var ErrHookLimit = errors.New("hook_limit")
var ErrHookCancelled = errors.New("hook_cancelled")
var ErrHookForbidden = errors.New("hook_forbidden")

type HookInput struct {
	Provider, RepositoryKey string
	Header                  http.Header
	Body                    []byte `json:"-"`
	Secret                  string `json:"-"`
	Generic                 *config.GenericHookSettings
}

func hookHeader(headers http.Header, name string, required bool) (string, error) {
	values := []string{}
	for key, rows := range headers {
		if strings.EqualFold(key, name) {
			values = append(values, rows...)
		}
	}
	if len(values) == 0 && !required {
		return "", nil
	}
	if len(values) != 1 || values[0] == "" || len(values[0]) > 4096 || strings.IndexFunc(values[0], unicode.IsControl) >= 0 {
		return "", ErrHookInvalid
	}
	return values[0], nil
}
func hookEqual(a, b string) bool {
	aHash := sha256.Sum256([]byte(a))
	bHash := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(aHash[:], bHash[:]) == 1
}
func hookString(value any) (string, bool) { s, ok := value.(string); return s, ok }
func repositoryKey(value any) (string, bool) {
	if text, ok := value.(string); ok {
		return text, text != "" && len(text) <= 1024 && strings.IndexFunc(text, unicode.IsControl) < 0
	}
	number, ok := value.(json.Number)
	if !ok {
		return "", false
	}
	n, e := strconv.ParseInt(string(number), 10, 64)
	return strconv.FormatInt(n, 10), e == nil && n > 0
}
func hookValue(root map[string]any, pointer string) any {
	value, _ := hookLookup(root, pointer)
	return value
}
func hookLookup(root map[string]any, pointer string) (any, bool) {
	var current any = root
	for _, part := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		mapping, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		var exists bool
		current, exists = mapping[part]
		if !exists {
			return nil, false
		}
	}
	return current, true
}
func hookJSON(ctx context.Context, data []byte) (map[string]any, error) {
	if !utf8.Valid(data) {
		return nil, ErrHookInvalid
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	count := 0
	var parse func(int) (any, error)
	parse = func(depth int) (any, error) {
		count++
		if depth > 64 || count > 200000 {
			return nil, ErrHookLimit
		}
		if count%1024 == 0 && ctx.Err() != nil {
			return nil, ErrHookCancelled
		}
		token, e := d.Token()
		if e != nil {
			return nil, ErrHookInvalid
		}
		switch value := token.(type) {
		case json.Delim:
			switch value {
			case '{':
				out := map[string]any{}
				for d.More() {
					keyToken, e := d.Token()
					key, ok := keyToken.(string)
					if e != nil || !ok {
						return nil, ErrHookInvalid
					}
					if len(key) > 256 {
						return nil, ErrHookLimit
					}
					if _, exists := out[key]; exists {
						return nil, ErrHookInvalid
					}
					item, e := parse(depth + 1)
					if e != nil {
						return nil, e
					}
					out[key] = item
				}
				if end, e := d.Token(); e != nil || end != json.Delim('}') {
					return nil, ErrHookInvalid
				}
				return out, nil
			case '[':
				out := []any{}
				for d.More() {
					item, e := parse(depth + 1)
					if e != nil {
						return nil, e
					}
					out = append(out, item)
				}
				if end, e := d.Token(); e != nil || end != json.Delim(']') {
					return nil, ErrHookInvalid
				}
				return out, nil
			}
		case string:
			if len(value) > 64<<10 {
				return nil, ErrHookLimit
			}
			return value, nil
		case json.Number, bool, nil:
			return value, nil // 可空metadata不能影响必填关键字段。
		}
		return nil, ErrHookInvalid
	}
	value, e := parse(0)
	if e != nil {
		return nil, e
	}
	root, ok := value.(map[string]any)
	if !ok {
		return nil, ErrHookInvalid
	}
	if _, e = d.Token(); e != io.EOF {
		return nil, ErrHookInvalid
	}
	return root, nil
}
func hookBranch(ref string) (string, bool) {
	if !strings.HasPrefix(ref, "refs/heads/") {
		return "", false
	}
	branch := strings.TrimPrefix(ref, "refs/heads/")
	if branch == "" || len(branch) > 1024 || strings.ContainsAny(branch, " ~^:?*[\\") || strings.Contains(branch, "..") || strings.Contains(branch, "@{") || strings.HasSuffix(branch, ".") || strings.IndexFunc(branch, unicode.IsControl) >= 0 {
		return "", false
	}
	for _, part := range strings.Split(branch, "/") {
		if part == "" || strings.HasPrefix(part, ".") || strings.HasSuffix(part, ".lock") {
			return "", false
		}
	}
	return branch, true
}
func hookOID(value string, zero bool) bool {
	if !fullOID.MatchString(value) {
		return false
	}
	if !zero && strings.Trim(value, "0") == "" {
		return false
	}
	return true
}

// ParseWebhook先认证原始bytes，随后仅按登记provider读取可信事件字段，不访问payload URL。
func ParseWebhook(ctx context.Context, in HookInput) (protocol.WebhookEvent, error) {
	var event protocol.WebhookEvent
	if ctx.Err() != nil {
		return event, ErrHookCancelled
	}
	headerBytes := 0
	for k, values := range in.Header {
		headerBytes += len(k)
		for _, v := range values {
			headerBytes += len(v)
		}
		if headerBytes > 32<<10 {
			return event, ErrHookLimit
		}
	}
	if len(in.Body) > 8<<20 {
		return event, ErrHookLimit
	}
	if len(in.Secret) < 32 || len(in.Secret) > 4096 {
		return event, ErrHookUnauthorized
	}
	if !strings.Contains("|github|gitlab|gitee|generic|", "|"+in.Provider+"|") || in.Provider == "" {
		return event, ErrHookInvalid
	}
	g := config.GenericHookDefaults(in.Generic)
	if e := config.ValidateWebhookSettings(&config.HookSettings{Generic: in.Generic}, nil); e != nil {
		return event, ErrHookInvalid
	}
	authHeader, eventHeader, deliveryHeader, push := "", "", "", ""
	switch in.Provider {
	case "github":
		authHeader = "X-Hub-Signature-256"
		eventHeader = "X-GitHub-Event"
		deliveryHeader = "X-GitHub-Delivery"
		push = "push"
	case "gitlab":
		authHeader = "X-Gitlab-Token"
		eventHeader = "X-Gitlab-Event"
		deliveryHeader = "webhook-id"
		push = "Push Hook"
	case "gitee":
		authHeader = "X-Gitee-Token"
		eventHeader = "X-Gitee-Event"
		push = "Push Hook"
	case "generic":
		authHeader = g.AuthHeader
		eventHeader = g.EventHeader
		deliveryHeader = *g.DeliveryHeader
		push = g.PushEvent
	}
	for _, name := range []string{"X-Hub-Signature-256", "X-Gitlab-Token", "X-Gitee-Token", "X-Mybuilds-Signature", "webhook-signature"} {
		if strings.EqualFold(name, authHeader) {
			continue
		}
		for key := range in.Header {
			if strings.EqualFold(key, name) {
				return event, ErrHookInvalid
			}
		}
	}
	foundAuth := false
	for key, values := range in.Header {
		if strings.EqualFold(key, authHeader) && len(values) > 0 {
			foundAuth = true
		}
	}
	if !foundAuth {
		return event, ErrHookUnauthorized
	}
	auth, e := hookHeader(in.Header, authHeader, true)
	if e != nil {
		return event, e
	}
	if in.Provider == "github" || in.Provider == "generic" && g.Auth == "hmac_sha256" {
		prefix := "sha256="
		if in.Provider == "generic" {
			prefix = *g.SignaturePrefix
		}
		if !strings.HasPrefix(auth, prefix) || len(auth) != len(prefix)+64 {
			return event, ErrHookUnauthorized
		}
		given, e := hex.DecodeString(auth[len(prefix):])
		if e != nil {
			return event, ErrHookUnauthorized
		}
		h := hmac.New(sha256.New, []byte(in.Secret))
		h.Write(in.Body)
		if !hmac.Equal(given, h.Sum(nil)) {
			return event, ErrHookUnauthorized
		}
	} else if !hookEqual(auth, in.Secret) {
		return event, ErrHookUnauthorized
	}
	eventType, e := hookHeader(in.Header, eventHeader, true)
	if e != nil {
		return event, e
	}
	delivery := ""
	if deliveryHeader != "" {
		delivery, e = hookHeader(in.Header, deliveryHeader, in.Provider == "github")
		if e != nil {
			return event, e
		}
	}
	if in.Provider == "gitlab" {
		other, e := hookHeader(in.Header, "Idempotency-Key", false)
		if e != nil {
			return event, e
		}
		if delivery != "" && other != "" && delivery != other {
			return event, ErrHookInvalid
		}
		if delivery == "" {
			delivery = other
		}
	}
	if len(delivery) > 128 || strings.IndexFunc(delivery, unicode.IsSpace) >= 0 {
		return event, ErrHookInvalid
	}
	root, e := hookJSON(ctx, in.Body)
	if e != nil {
		return event, e
	}
	sum := sha256.Sum256(in.Body)
	event = protocol.WebhookEvent{Provider: in.Provider, DeliveryID: delivery, RepositoryKey: in.RepositoryKey, BodyDigest: hex.EncodeToString(sum[:])}
	finish := func() (protocol.WebhookEvent, error) {
		event.ReceiptDigest = protocol.WebhookEventDigest(event)
		return event, nil
	}
	var repository any
	switch in.Provider {
	case "github", "gitee":
		if object, ok := root["repository"].(map[string]any); ok {
			repository = object["id"]
		}
	case "gitlab":
		if object, ok := root["project"].(map[string]any); ok {
			repository = object["id"]
		}
	case "generic":
		repository = hookValue(root, *g.RepositoryPointer)
	}
	key, ok := repositoryKey(repository)
	if !ok || key != in.RepositoryKey {
		return protocol.WebhookEvent{}, ErrHookForbidden
	}
	if in.Provider != "generic" {
		number, e := strconv.ParseInt(key, 10, 64)
		if e != nil || number <= 0 {
			return protocol.WebhookEvent{}, ErrHookForbidden
		}
	}
	isPush := eventType == push || in.Provider == "gitee" && eventType == "push_hooks"
	if !isPush {
		recognized := false
		switch in.Provider {
		case "github":
			recognized = slices.Contains([]string{"ping", "create", "delete", "pull_request", "pull_request_review", "issues", "issue_comment", "release", "workflow_run", "workflow_job", "check_run", "check_suite", "repository", "fork", "star", "watch", "deployment", "deployment_status", "status", "member", "membership", "public", "push"}, eventType)
		case "gitlab":
			recognized = slices.Contains([]string{"Tag Push Hook", "Merge Request Hook", "Issue Hook", "Note Hook", "Pipeline Hook", "Job Hook", "Wiki Page Hook", "Deployment Hook", "Release Hook", "Feature Flag Hook", "Subgroup Hook"}, eventType)
		case "gitee":
			recognized = slices.Contains([]string{"Tag Push Hook", "Pull Request Hook", "Issue Hook", "Note Hook", "tag_push_hooks", "pull_request_hooks", "issue_hooks", "note_hooks"}, eventType)
		case "generic":
			recognized = slices.Contains([]string{"ping", "tag", "pull_request", "merge_request"}, eventType)
		}
		if !recognized {
			return event, ErrHookInvalid
		}
		event.Kind = "ignored"
		event.Reason = "hook_event_ignored"
		if in.Provider == "github" && eventType == "ping" {
			event.Reason = "hook_ping"
		}
		return finish()
	}
	if in.Provider == "github" || in.Provider == "gitlab" {
		request, _ := http.NewRequestWithContext(ctx, "POST", "http://127.0.0.1/owned", bytes.NewReader(in.Body))
		request.Header = in.Header.Clone()
		if in.Provider == "github" {
			hook, _ := githubhook.New(githubhook.Options.Secret(in.Secret))
			if _, e := hook.Parse(request, githubhook.PushEvent); e != nil {
				return protocol.WebhookEvent{}, ErrHookInvalid
			}
		}
		if in.Provider == "gitlab" {
			hook, _ := gitlabhook.New(gitlabhook.Options.Secret(in.Secret))
			if _, e := hook.Parse(request, gitlabhook.PushEvents); e != nil {
				return protocol.WebhookEvent{}, ErrHookInvalid
			}
		}
	}
	if in.Provider == "gitlab" && root["object_kind"] != "push" || in.Provider == "gitee" && root["hook_name"] != "push_hooks" {
		return protocol.WebhookEvent{}, ErrHookInvalid
	}
	if in.Provider == "gitee" {
		if password, exists := root["password"]; exists {
			p, ok := password.(string)
			if !ok || !hookEqual(p, in.Secret) {
				return protocol.WebhookEvent{}, ErrHookUnauthorized
			}
		}
	}
	ref, after, before := "", "", ""
	if in.Provider == "generic" {
		ref, _ = hookString(hookValue(root, *g.RefPointer))
		after, _ = hookString(hookValue(root, *g.AfterPointer))
		if raw, exists := hookLookup(root, *g.BeforePointer); exists {
			before, ok = hookString(raw)
			if !ok {
				return protocol.WebhookEvent{}, ErrHookInvalid
			}
		}
	} else {
		ref, _ = hookString(root["ref"])
		after, _ = hookString(root["after"])
		if raw, exists := root["before"]; exists {
			before, ok = hookString(raw)
			if !ok {
				return protocol.WebhookEvent{}, ErrHookInvalid
			}
		}
	}
	if strings.HasPrefix(ref, "refs/tags/") {
		event.Kind = "ignored"
		event.Reason = "hook_event_ignored"
		return finish()
	}
	branch, ok := hookBranch(ref)
	if !ok || !hookOID(after, true) || before != "" && !hookOID(before, true) || before != "" && len(before) != len(after) {
		return protocol.WebhookEvent{}, ErrHookInvalid
	}
	event.Branch = branch
	event.Before = strings.ToLower(before)
	event.After = strings.ToLower(after)
	deleted := strings.Trim(after, "0") == ""
	if flag, exists := root["deleted"]; exists {
		value, ok := flag.(bool)
		if !ok {
			return protocol.WebhookEvent{}, ErrHookInvalid
		}
		deleted = deleted || value
	}
	if deleted {
		event.Kind = "ignored"
		event.Reason = "hook_branch_deleted"
	} else {
		event.Kind = "push"
	}
	return finish()
}
