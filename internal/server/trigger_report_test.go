package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"mybuilds/internal/config"
)

func TestActualTriggerReportsKeepsFrozenDefinitionAndSkippedBuild(t *testing.T) {
	s, st, _ := serverFixture(t)
	actor, err := st.Authenticate(context.Background(), adminToken)
	if err != nil {
		t.Fatal(err)
	}
	source := `version: 1
builds:
  tests:
    reports: {junit: {paths: ['results/{{build.id}}/*.xml']}}
    steps: [{kind: run, run: 'touch must-not-run'}]
  skipped:
    when: {params: {channel: release}}
    params: {channel: debug}
    reports: {junit: {paths: ['results/*.xml']}}
    steps: [{kind: run, run: 'touch must-not-run'}]
`
	repo, project := triggerProject(t, st, actor, source, config.ProjectSettings{}, "linux")
	batch, err := s.Trigger(context.Background(), actor, project, "reports-frozen", TriggerRequest{All: true})
	if err != nil || len(batch.Builds) != 2 {
		t.Fatal("严格报告定义未实际入队", err)
	}
	for _, build := range batch.Builds {
		view, err := st.GetBuild(context.Background(), build.ID)
		if err != nil || view.Reports != nil || view.ReportSealDigest != "" {
			t.Fatal("入队或跳过不能伪造报告结果", err)
		}
		if build.Name == "tests" && (view.Status != "queued" || view.Number == nil) || build.Name == "skipped" && (view.Status != "skipped" || view.Number != nil) {
			t.Fatal("报告模板待定不等同条件跳过")
		}
	}
	if err := os.WriteFile(filepath.Join(repo, "mybuilds.yml"), []byte("version: 1\nsteps: [{kind: run, run: ':'}]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, repo, "add", "mybuilds.yml")
	gitCommand(t, repo, "commit", "-m", "advance")
	replayed, err := s.Trigger(context.Background(), actor, project, "reports-frozen", TriggerRequest{All: true})
	if err != nil || !replayed.Replayed || replayed.ID != batch.ID || replayed.SHA != batch.SHA {
		t.Fatal("报告原快照重放不应重新读取HEAD", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "must-not-run")); !os.IsNotExist(err) {
		t.Fatal("控制端触发不能执行用户脚本")
	}
}
