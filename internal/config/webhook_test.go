package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebhookSettingsStrictBoundary(t *testing.T) {
	for _, raw := range []string{`hook: {enabled: true, repository_key: '42'}\ntriggers: {builds: [one], quiet_period: 30s}`, `hook: {enabled: false}`} {
		raw = strings.ReplaceAll(raw, `\n`, "\n")
		s, e := ParseProjectSettings([]byte(raw))
		if e != nil || ValidateWebhookSettings(s.Hook, s.Triggers) != nil {
			t.Fatalf("合法设置 %v", e)
		}
	}
	for _, raw := range []string{`hook: null`, `hook: {enabled: true, secret: plaintext}`, `triggers: {builds: []}`, `triggers: {quiet_period: -1s}`, `triggers: {quiet_period: 25h}`, `triggers: {allow_upload: 'false'}`, `hook: {enabled: true, enabled: false}`, `hook: {generic: {auth: script}}`, `hook: {generic: {auth_header: Authorization}}`, `hook: {generic: {ref_pointer: ''}}`} {
		if _, e := ParseProjectSettings([]byte(raw)); e == nil {
			t.Fatalf("非法设置接受 %s", raw)
		}
	}
}
func TestWebhookSecretExplicitFileOnly(t *testing.T) {
	dir := t.TempDir()
	os.Chmod(dir, 0700)
	file := filepath.Join(dir, "hooks.env")
	value := strings.Repeat("a", 32)
	os.WriteFile(file, []byte("OWN_HOOK="+value+"\nUNKNOWN=kept_out\n"), 0600)
	t.Setenv("OWN_HOOK", strings.Repeat("b", 32))
	got, e := LoadWebhookSecrets(file, []string{"OWN_HOOK"})
	if e != nil || got["OWN_HOOK"] != value || len(got) != 1 {
		t.Fatalf("显式材料 %v", e)
	}
	for _, body := range []string{"OWN_HOOK=short\n", "OWN_HOOK=" + value + "\nOWN_HOOK=" + value + "\n", "OWN_HOOK=" + value + "\x00\n"} {
		os.WriteFile(file, []byte(body), 0600)
		if _, e := LoadWebhookSecrets(file, []string{"OWN_HOOK"}); e == nil {
			t.Fatal("非法材料")
		}
	}
	os.WriteFile(file, []byte("OWN_HOOK="+value), 0644)
	os.Chmod(file, 0644)
	if _, e := LoadWebhookSecrets(file, []string{"OWN_HOOK"}); e == nil {
		t.Fatal("公共材料")
	}
}
