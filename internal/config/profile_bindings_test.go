package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProfileBindingsAndNotificationInheritance(t *testing.T) {
	p, err := BindProfiles("flutter", "ios,android")
	if err != nil || p.Source != "auto" || p.Builds["ios"].Profile != "flutter-ios" || p.Builds["android"].Profile != "flutter-android" {
		t.Fatalf("bindings: %v", err)
	}
	for _, args := range [][2]string{{"flutter", ""}, {"bad", "android"}, {"native", "android,android"}, {"native", "android,"}, {"native", "Android"}} {
		if _, err := BindProfiles(args[0], args[1]); err == nil {
			t.Fatal("accepted invalid binding")
		}
	}
	f := false
	tr := true
	global := &Notifications{Enabled: &tr, On: []string{"success"}, Webhooks: []Webhook{{Type: "generic", URL: "https://example.invalid/global"}}, Template: "global"}
	build := &Notifications{Webhooks: []Webhook{}, Template: "build"}
	project := &Notifications{Enabled: &f, On: []string{"failure"}}
	effective := ResolveNotifications(project, build, global)
	if *effective.Enabled || len(effective.Webhooks) != 0 || len(effective.On) != 1 || effective.On[0] != "failure" || effective.Template != "build" {
		t.Fatal("field precedence/list replacement")
	}
	effective.On[0] = "cancelled"
	if project.On[0] != "failure" {
		t.Fatal("mutable alias")
	}
	if ResolveNotifications(nil, nil, nil) != nil {
		t.Fatal("no configured notification became active")
	}
}

func TestLoadTemplateRejectsLinksAndAllowsNamedBuilds(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "template.yml")
	bytes := []byte("version: 1\nbuilds:\n  first:\n    steps: [{kind: run, run: echo}]\n  second:\n    steps: [{kind: run, run: echo}]\n")
	if err := os.WriteFile(file, bytes, 0600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadTemplate(file)
	if err != nil || string(got) != string(bytes) {
		t.Fatalf("named template: %v", err)
	}
	link := filepath.Join(dir, "link.yml")
	if err := os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadTemplate(link); err == nil {
		t.Fatal("followed template symlink")
	}
}
