package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const simpleProfile = "version: 1\nparams:\n  channel: {default: internal}\nsteps:\n  - {kind: run, name: package, run: 'printf ready'}\n"

func TestBuildProfileSingleDefinitionAndStrictInput(t *testing.T) {
	b, _, err := ParseBuildProfile([]byte(simpleProfile))
	if err != nil || b == nil || len(b.Steps) != 1 {
		t.Fatalf("valid profile: %v", err)
	}
	for _, in := range []string{
		"version: 1\nbuilds:\n  default:\n    steps: [{kind: run, name: package, run: echo}]\n",
		simpleProfile + "unknown: true\n", "version: 1\nsteps: null\n", "version: 1\nversion: 1\nsteps: [{kind: run, run: echo}]\n",
		"version: 1\nsteps: &steps [{kind: run, run: echo}]\npost: {always: *steps}\n", simpleProfile + "---\nversion: 1\n",
	} {
		if _, _, e := ParseBuildProfile([]byte(in)); e == nil {
			t.Errorf("accepted invalid profile")
		}
	}
}

func TestLoadBuildProfilesRealFilesAliasesAndIsolation(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "company.yml")
	if err := os.WriteFile(file, []byte(simpleProfile), 0600); err != nil {
		t.Fatal(err)
	}
	builtin := []byte(simpleProfile)
	profiles, err := LoadBuildProfiles(map[string]BuildProfile{"company": {File: file}, "alias": {Template: "native-android"}}, map[string][]byte{"native-android": builtin})
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 3 || profiles["company"].ContentDigest == "" || profiles["alias"].Template != "native-android" {
		t.Fatal("missing source evidence")
	}
	p := profiles["alias"]
	p.Definition.Params["channel"] = Parameter{Default: profileDefault("changed")}
	p.Content[0] = 'x'
	if profiles["native-android"].Definition.Params["channel"].Default == nil || *profiles["native-android"].Definition.Params["channel"].Default != "internal" {
		t.Fatal("definitions alias")
	}
	if !bytes.Equal(profiles["native-android"].Content, builtin) {
		t.Fatal("content alias")
	}
	for _, entries := range []map[string]BuildProfile{
		{"native-android": {File: file}}, {"company": {}}, {"company": {File: file, Template: "native-android"}},
		{"company": {Template: "missing"}}, {"bad/name": {File: file}}, {"company": {File: filepath.Join(dir, "missing")}},
	} {
		if _, e := LoadBuildProfiles(entries, map[string][]byte{"native-android": builtin}); e == nil {
			t.Error("accepted invalid registry")
		}
	}
	link := filepath.Join(dir, "link.yml")
	if err := os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	hard := filepath.Join(dir, "hard.yml")
	if err := os.Link(file, hard); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{link, hard, dir} {
		if _, e := LoadBuildProfiles(map[string]BuildProfile{"company": {File: path}}, nil); e == nil {
			t.Error("accepted nonunique regular file")
		}
	}
	if err := os.Remove(hard); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(strings.Repeat("x", MaxConfigBytes+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, e := LoadBuildProfiles(map[string]BuildProfile{"company": {File: file}}, nil); e == nil {
		t.Fatal("accepted oversized profile")
	}
}

func TestProjectProfilesAndServerRegistryConfiguration(t *testing.T) {
	s, err := ParseProjectSettings([]byte("pipeline:\n  source: profile\n  profile: company\n  params: {channel: beta}\n"))
	if err != nil || s.Pipeline.Profile != "company" {
		t.Fatalf("legacy profile: %v", err)
	}
	for _, in := range []string{
		"pipeline: {source: profile, profile: company, builds: {android: {profile: company}}}",
		"pipeline: {source: profile, builds: {android: {profile: 'bad/name'}}}",
		"pipeline: {source: invalid}", "pipeline: {profile: null}",
	} {
		if _, e := ParseProjectSettings([]byte(in)); e == nil {
			t.Error("accepted invalid binding")
		}
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "server.yml")
	if err := os.WriteFile(file, []byte("build_profiles:\n  company: {file: profiles/company.yml}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadServer(ServerLoadOptions{Filename: file, Explicit: true})
	if err != nil || cfg.BuildProfiles["company"].File != filepath.Join(dir, "profiles/company.yml") {
		t.Fatalf("relative profile path: %v", err)
	}
}

func profileDefault(s string) *string { return &s }
