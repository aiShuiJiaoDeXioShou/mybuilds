package config

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
)

type TriggerSettings struct {
	Builds      []string `yaml:"builds,omitempty" json:"builds,omitempty"`
	QuietPeriod string   `yaml:"quiet_period,omitempty" json:"quiet_period,omitempty"`
	AllowUpload bool     `yaml:"allow_upload" json:"allow_upload"`
}
type HookSettings struct {
	Enabled       bool                 `yaml:"enabled" json:"enabled"`
	RepositoryKey string               `yaml:"repository_key,omitempty" json:"repository_key,omitempty"`
	Secret        string               `yaml:"secret,omitempty" json:"secret,omitempty"`
	Generic       *GenericHookSettings `yaml:"generic,omitempty" json:"generic,omitempty"`
}
type GenericHookSettings struct {
	Auth              string  `yaml:"auth,omitempty" json:"auth,omitempty"`
	AuthHeader        string  `yaml:"auth_header,omitempty" json:"auth_header,omitempty"`
	SignaturePrefix   *string `yaml:"signature_prefix,omitempty" json:"signature_prefix,omitempty"`
	EventHeader       string  `yaml:"event_header,omitempty" json:"event_header,omitempty"`
	PushEvent         string  `yaml:"push_event,omitempty" json:"push_event,omitempty"`
	DeliveryHeader    *string `yaml:"delivery_header,omitempty" json:"delivery_header,omitempty"`
	RefPointer        *string `yaml:"ref_pointer,omitempty" json:"ref_pointer,omitempty"`
	AfterPointer      *string `yaml:"after_pointer,omitempty" json:"after_pointer,omitempty"`
	BeforePointer     *string `yaml:"before_pointer,omitempty" json:"before_pointer,omitempty"`
	RepositoryPointer *string `yaml:"repository_pointer,omitempty" json:"repository_pointer,omitempty"`
}

var headerName = regexp.MustCompile(`^[!#$%&'*+.^_` + "`" + `|~0-9A-Za-z-]{1,128}$`)
var secretRef = regexp.MustCompile(`^\$\{[A-Za-z_][A-Za-z0-9_]*\}$`)

func GenericHookDefaults(in *GenericHookSettings) GenericHookSettings {
	prefix, delivery := "sha256=", "X-Mybuilds-Delivery"
	ref, after, before, repository := "/ref", "/after", "/before", "/repository"
	out := GenericHookSettings{Auth: "hmac_sha256", AuthHeader: "X-Mybuilds-Signature", SignaturePrefix: &prefix, EventHeader: "X-Mybuilds-Event", PushEvent: "push", DeliveryHeader: &delivery, RefPointer: &ref, AfterPointer: &after, BeforePointer: &before, RepositoryPointer: &repository}
	if in == nil {
		return out
	}
	if in.Auth != "" {
		out.Auth = in.Auth
	}
	if in.AuthHeader != "" {
		out.AuthHeader = in.AuthHeader
	}
	if in.SignaturePrefix != nil {
		out.SignaturePrefix = in.SignaturePrefix
	}
	if in.EventHeader != "" {
		out.EventHeader = in.EventHeader
	}
	if in.PushEvent != "" {
		out.PushEvent = in.PushEvent
	}
	if in.DeliveryHeader != nil {
		out.DeliveryHeader = in.DeliveryHeader
	}
	if in.RefPointer != nil {
		out.RefPointer = in.RefPointer
	}
	if in.AfterPointer != nil {
		out.AfterPointer = in.AfterPointer
	}
	if in.BeforePointer != nil {
		out.BeforePointer = in.BeforePointer
	}
	if in.RepositoryPointer != nil {
		out.RepositoryPointer = in.RepositoryPointer
	}
	if out.Auth == "token" && in.SignaturePrefix == nil {
		empty := ""
		out.SignaturePrefix = &empty
	}
	return out
}
func safeHookText(value string, max int) bool {
	return value != "" && len(value) <= max && strings.IndexFunc(value, func(r rune) bool { return unicode.IsControl(r) }) < 0
}
func hookPointer(value string) bool {
	if !strings.HasPrefix(value, "/") || len(value) > 1024 {
		return false
	}
	parts := strings.Split(value[1:], "/")
	if len(parts) > 16 {
		return false
	}
	for _, part := range parts {
		if part == "" || len(part) > 256 || strings.IndexFunc(part, unicode.IsControl) >= 0 {
			return false
		}
		for i := 0; i < len(part); i++ {
			if part[i] == '~' {
				if i+1 >= len(part) || (part[i+1] != '0' && part[i+1] != '1') {
					return false
				}
				i++
			}
		}
	}
	return true
}
func ValidateWebhookSettings(hook *HookSettings, triggers *TriggerSettings) error {
	fail := func() error { return invalid("hook", "来源设置不合法") }
	if triggers != nil {
		if triggers.Builds != nil && len(triggers.Builds) == 0 || len(triggers.Builds) > 64 {
			return fail()
		}
		seen := map[string]bool{}
		for _, name := range triggers.Builds {
			if !safeName(name) || len(name) > 64 || seen[name] {
				return fail()
			}
			seen[name] = true
		}
		if triggers.QuietPeriod != "" {
			period, e := time.ParseDuration(triggers.QuietPeriod)
			if e != nil || period < 0 || period > 24*time.Hour {
				return fail()
			}
		}
	}
	if hook == nil {
		return nil
	}
	if hook.RepositoryKey != "" && !safeHookText(hook.RepositoryKey, 1024) {
		return fail()
	}
	if hook.Enabled && hook.RepositoryKey == "" {
		return fail()
	}
	if hook.Secret != "" && !secretRef.MatchString(hook.Secret) {
		return fail()
	}
	if hook.Generic == nil {
		return nil
	}
	original := hook.Generic
	g := GenericHookDefaults(original)
	if g.Auth != "hmac_sha256" && g.Auth != "token" || !safeHookText(g.PushEvent, 128) {
		return fail()
	}
	if g.SignaturePrefix == nil || (*g.SignaturePrefix != "" && *g.SignaturePrefix != "sha256=") || g.Auth == "token" && *g.SignaturePrefix != "" {
		return fail()
	}
	seen := map[string]bool{}
	for _, header := range []string{g.AuthHeader, g.EventHeader, *g.DeliveryHeader} {
		if header == "" {
			continue
		}
		name := strings.ToLower(header)
		if !headerName.MatchString(header) || seen[name] || strings.HasPrefix(name, "content-") || strings.Contains(name, "gitee") || strings.Contains(name, "gitlab") || strings.Contains(name, "github") || strings.HasPrefix(name, "x-hub-") || name == "authorization" || name == "cookie" || name == "host" {
			return fail()
		}
		seen[name] = true
	}
	for _, pointer := range []string{*g.RefPointer, *g.AfterPointer, *g.BeforePointer, *g.RepositoryPointer} {
		if !hookPointer(pointer) {
			return fail()
		}
	}
	return nil
}

// LoadWebhookSecrets只取显式声明值；不读宿主env或解释shell/模板。
func LoadWebhookSecrets(filename string, names []string) (map[string]string, error) {
	fail := func() (map[string]string, error) { return nil, invalid("hook材料", "私有材料无效") }
	wanted := map[string]bool{}
	if len(names) > 128 {
		return fail()
	}
	for _, name := range names {
		if !identifier.MatchString(name) || wanted[name] {
			return fail()
		}
		wanted[name] = true
	}
	if len(names) == 0 {
		return map[string]string{}, nil
	}
	f, e := openConfiguration(filename)
	if e != nil {
		return fail()
	}
	defer f.Close()
	before, e := f.Stat()
	if e != nil || !before.Mode().IsRegular() || before.Mode().Perm() != 0600 || !configurationOwned(before) || !configurationSingleLink(before) || before.Size() > MaxConfigBytes {
		return fail()
	}
	data, e := io.ReadAll(io.LimitReader(f, MaxConfigBytes+1))
	if e != nil || int64(len(data)) != before.Size() || len(data) > MaxConfigBytes {
		return fail()
	}
	after, e := f.Stat()
	leaf, le := os.Lstat(filename)
	if e != nil || le != nil || !os.SameFile(before, after) || !os.SameFile(after, leaf) || !before.ModTime().Equal(after.ModTime()) || bytes.IndexByte(data, 0) >= 0 {
		return fail()
	}
	parent, e := os.Lstat(filepath.Dir(filename))
	if e != nil || !parent.IsDir() || parent.Mode().Perm() != 0700 || !configurationOwned(parent) {
		return fail()
	}
	out := map[string]string{}
	seen := map[string]bool{}
	count := 0
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		count++
		if !ok || !identifier.MatchString(key) || seen[key] || count > 128 || len(value) > 4096 {
			return fail()
		}
		seen[key] = true
		if wanted[key] {
			if len(value) < 32 || !safeHookText(value, 4096) {
				return fail()
			}
			out[key] = value
		}
	}
	if len(out) != len(wanted) {
		return fail()
	}
	return out, nil
}
