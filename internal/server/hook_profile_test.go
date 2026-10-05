package server

import (
	"context"
	"mybuilds/internal/config"
	"os"
	"path/filepath"
	"testing"
)

func TestWebhookProfileOnlyInfersFrozenBuild(t *testing.T) {
	s, st, _ := serverFixture(t)
	if err := os.Chmod(s.config.DataDir, 0700); err != nil {
		t.Fatal(err)
	}
	actor, err := st.Authenticate(context.Background(), adminToken)
	if err != nil {
		t.Fatal(err)
	}
	profile := filepath.Join(t.TempDir(), "company.yml")
	if err := os.WriteFile(profile, []byte("version: 1\nsteps: [{kind: run, run: 'true'}]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s.profiles, err = config.LoadBuildProfiles(map[string]config.BuildProfile{"company": {File: profile}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	settings := config.ProjectSettings{Pipeline: &config.PipelineSettings{Source: "profile", Builds: map[string]config.BuildSettings{"package": {Profile: "company"}}}}
	repo, name := triggerProject(t, st, actor, "version: 1\nsteps: [{kind: run, run: 'true'}]\n", settings, "linux")
	gitCommand(t, repo, "rm", "mybuilds.yml")
	gitCommand(t, repo, "commit", "-m", "profile-only")
	configured, err := s.ConfigureWebhook(context.Background(), actor, name, config.ProjectSettings{Hook: &config.HookSettings{Enabled: true, RepositoryKey: "owned-profile"}}, false)
	if err != nil {
		t.Fatal("无仓库YAML的实际profile不能启用", err)
	}
	if len(configured.View.Builds) != 1 || configured.View.Builds[0] != "package" {
		t.Fatal("推导未沿唯一profile来源", configured.View.Builds)
	}
}
