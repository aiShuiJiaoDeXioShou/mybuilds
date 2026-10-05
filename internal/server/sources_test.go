package server

import (
	"context"
	"mybuilds/internal/config"
	"os"
	"path/filepath"
	"testing"
)

func TestActualProfileSourceFallbackPinnedAndReplay(t *testing.T) {
	s, st, _ := serverFixture(t)
	actor, err := st.Authenticate(context.Background(), adminToken)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	profile := filepath.Join(dir, "company.yml")
	content := []byte("version: 1\nrunner: {platform: android}\nparams:\n  channel: {default: internal}\nsteps: [{kind: run, name: package, run: 'touch MUST_NOT_RUN'}]\n")
	if err := os.WriteFile(profile, content, 0600); err != nil {
		t.Fatal(err)
	}
	s.profiles, err = config.LoadBuildProfiles(map[string]config.BuildProfile{"company": {File: profile}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	settings := config.ProjectSettings{Pipeline: &config.PipelineSettings{Builds: map[string]config.BuildSettings{"android": {Profile: "company"}}}}
	repo, name := triggerProject(t, st, actor, "version: 1\nsteps: [{kind: run, run: echo repo}]\n", settings, "")
	gitCommand(t, repo, "rm", "mybuilds.yml")
	gitCommand(t, repo, "commit", "-m", "missing")
	sha := gitCommand(t, repo, "rev-parse", "HEAD")
	batch, err := s.Trigger(context.Background(), actor, name, "profiles-key", TriggerRequest{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if batch.SHA != sha || len(batch.Builds) != 1 || batch.Builds[0].Name != "android" || batch.Builds[0].Origin == nil || batch.Builds[0].Origin.Kind != "profile" || batch.Builds[0].Origin.Profile != "company" {
		t.Fatal("missing frozen origin")
	}
	if err := os.WriteFile(profile, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	write := filepath.Join(repo, "mybuilds.yml")
	if err := os.WriteFile(write, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, repo, "add", "mybuilds.yml")
	gitCommand(t, repo, "commit", "-m", "bad repository")
	replay, err := s.Trigger(context.Background(), actor, name, "profiles-key", TriggerRequest{All: true})
	if err != nil || !replay.Replayed || replay.ID != batch.ID {
		t.Fatalf("replay reread source: %v", err)
	}
	if _, err := s.Trigger(context.Background(), actor, name, "bad-repo-key", TriggerRequest{All: true}); err == nil {
		t.Fatal("bad repo fell back")
	}
	settings.Pipeline.Source = "profile"
	if _, err = st.SetProjectSettings(context.Background(), actor, name, settings); err != nil {
		t.Fatal(err)
	}
	next, err := s.Trigger(context.Background(), actor, name, "forced-profiles", TriggerRequest{All: true})
	if err != nil || len(next.Builds) != 1 || next.Builds[0].Origin.Kind != "profile" || *next.Builds[0].Number != 2 {
		t.Fatalf("forced profile: %v", err)
	}
	if _, e := os.Stat(filepath.Join(repo, "MUST_NOT_RUN")); !os.IsNotExist(e) {
		t.Fatal("executed build during trigger")
	}
}

func TestActualRepoSourceNeverMergesBindings(t *testing.T) {
	s, st, _ := serverFixture(t)
	actor, _ := st.Authenticate(context.Background(), adminToken)
	_, name := triggerProject(t, st, actor, namedPipeline, config.ProjectSettings{Pipeline: &config.PipelineSettings{Builds: map[string]config.BuildSettings{"unselected-binding": {Profile: "nonexistent"}}}}, "")
	batch, err := s.Trigger(context.Background(), actor, name, "repo-only", TriggerRequest{All: true, Params: map[string]string{"version": "1.0.0"}})
	if err != nil || len(batch.Builds) != 2 || batch.Builds[0].Origin == nil || batch.Builds[0].Origin.Kind != "repo" {
		t.Fatalf("repository replaced/merged: %v", err)
	}
}
