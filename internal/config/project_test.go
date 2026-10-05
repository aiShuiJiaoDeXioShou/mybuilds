package config

import (
	"strings"
	"testing"
)

func TestProjectSettingsStrictAndDefaults(t *testing.T) {
	for _, body := range []string{"pipeline: {source: profile}\n", "pipeline: {file: ../secret}\n", "pipeline: {file: a/../b}\n", "pipeline: {file: '*.yml'}\n", "pipeline: {file: 'C:\\secret'}\n", "pipeline: {source: 7}\n", "pipeline: {params: {channel: 7}}\n", "pipeline: {params: {channel: a}, builds: {default: {params: {channel: b}}}}\n", "pipeline: {builds: {default: {profile: TOKEN_SECRET}}}\n", "pipeline: null\n", "pipeline: {source: repo, source: auto}\n"} {
		if _, err := ParseProjectSettings([]byte(body)); err == nil || strings.Contains(err.Error(), "TOKEN_SECRET") {
			t.Errorf("非法settings: %q %v", body, err)
		}
	}
	cfg, err := ParseProjectSettings([]byte("pipeline: {params: {channel: internal}}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Pipeline == nil || cfg.Pipeline.Source != "auto" || cfg.Pipeline.File != "mybuilds.yml" || cfg.Pipeline.Params["channel"] != "internal" {
		t.Fatal("旧default简写错误")
	}
	cfg, err = ParseProjectSettings([]byte("pipeline: {source: repo, builds: {android: {params: {channel: internal}}}}\n"))
	if err != nil || cfg.Pipeline.Builds["android"].Params["channel"] != "internal" {
		t.Fatal("命名参数错误", err)
	}
}
