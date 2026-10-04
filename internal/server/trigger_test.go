package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"mybuilds/internal/config"
	"mybuilds/internal/store"
)

func gitCommand(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir, "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false"}, args...)...)
	command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid"}
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("实际git夹具: %v %s", err, output)
	}
	return strings.TrimSpace(string(output))
}
func triggerProject(t *testing.T, st *store.Store, actor store.Actor, content string, settings config.ProjectSettings, defaultNode string) (string, string) {
	t.Helper()
	repo := t.TempDir()
	gitCommand(t, repo, "init", "--initial-branch=main", "--template=")
	if err := os.WriteFile(filepath.Join(repo, "mybuilds.yml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, repo, "add", "mybuilds.yml")
	gitCommand(t, repo, "commit", "-m", "fixture")
	p, err := st.CreateProject(context.Background(), actor, store.ProjectInput{Name: "app", Repository: repo, AllowedNodes: []string{"linux"}, DefaultNode: defaultNode, Settings: settings})
	if err != nil {
		t.Fatal(err)
	}
	return repo, p.Name
}

const namedPipeline = `version: 1
builds:
  first:
    runner: {platform: android}
    params:
      version: {required: true}
      channel: {default: internal, choices: [internal, public]}
    when: {changes: ['src/**']}
    env: {PRIVATE_ENV: '${UNREAD_NODE_SECRET}', NUMBER: '{{build.number}}'}
    timeout: 3m
    steps:
      - {kind: run, name: compile, run: 'touch PRIVATE_MUST_NOT_EXECUTE'}
      - {kind: artifact, paths: ['out/{{build.number}}/*.apk']}
    post:
      always: [{kind: run, run: 'touch PRIVATE_POST_MUST_NOT_EXECUTE'}]
  second:
    runner: {platform: android}
    params:
      version: {required: true}
      channel: {default: internal, choices: [internal, public]}
    when: {params: {channel: public}}
    steps: [{kind: run, run: 'touch PRIVATE_MUST_NOT_EXECUTE'}]
`

func TestActualTriggerNamedParamsIndependentWhenAndReplay(t *testing.T) {
	s, st, h := serverFixture(t)
	actor, err := st.Authenticate(context.Background(), adminToken)
	if err != nil {
		t.Fatal(err)
	}
	settings := config.ProjectSettings{Pipeline: &config.PipelineSettings{Builds: map[string]config.BuildSettings{"first": {Params: map[string]string{"version": "project-default"}}}}}
	repo, name := triggerProject(t, st, actor, namedPipeline, settings, "")
	req := TriggerRequest{All: true, Params: map[string]string{"version": "PRIVATE_VERSION_MARKER"}, BuildParams: map[string]map[string]string{"first": {"channel": "public"}}}
	result, err := s.Trigger(context.Background(), actor, name, "same-key", req)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Builds) != 2 || result.Builds[0].Status != "queued" || result.Builds[0].Number == nil || *result.Builds[0].Number != 1 || result.Builds[1].Status != "skipped" || result.Builds[1].Number != nil {
		t.Fatalf("条件和编号: %+v", result.Builds)
	}
	view, err := st.GetBuild(context.Background(), result.Builds[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Condition != "ready" || len(view.Reasons) != 1 || !strings.Contains(view.Reasons[0], "手动") || len(view.Steps) != 2 || view.Steps[0].Condition != "pending" || view.Steps[0].Status != "pending" || len(view.Post) != 1 || view.Post[0].Status != "pending" || view.InitialBudgetNS == nil || *view.InitialBudgetNS != int64(180000000000) {
		t.Fatalf("预算/步骤/条件: %+v", view)
	}
	content := strings.Replace(namedPipeline, "3m", "4m", 1)
	if err := os.WriteFile(filepath.Join(repo, "mybuilds.yml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, repo, "add", "mybuilds.yml")
	gitCommand(t, repo, "commit", "-m", "advance")
	// 来源不可用后同key仍重放原快照，证明不重新读Git。
	if err := os.Rename(repo, repo+".unavailable"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(repo + ".unavailable") })
	again, err := s.Trigger(context.Background(), actor, name, "same-key", req)
	if err != nil || again.ID != result.ID || again.SHA != result.SHA {
		t.Fatal(again, err)
	}
	req.Params["version"] = "changed"
	_, err = s.Trigger(context.Background(), actor, name, "same-key", req)
	if !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	code, body := request(t, h, "GET", "/api/builds", adminToken, "")
	if code != 200 || strings.Contains(body, "PRIVATE_") || strings.Contains(body, "UNREAD_NODE_SECRET") {
		t.Fatal(code, body)
	}
	if _, err := os.Stat(filepath.Join(repo+".unavailable", "PRIVATE_MUST_NOT_EXECUTE")); !os.IsNotExist(err) {
		t.Fatal("执行了流水线")
	}
}
func TestActualTriggerWholeBatchFailuresAndUploadBeforeWhen(t *testing.T) {
	cases := []struct {
		name, config string
		request      TriggerRequest
		role         string
		want         error
	}{
		{"missing required", namedPipeline, TriggerRequest{All: true}, "admin", errPipeline},
		{"bad choice", namedPipeline, TriggerRequest{All: true, Params: map[string]string{"version": "v", "channel": "bad"}}, "admin", errPipeline},
		{"unselected scope", namedPipeline, TriggerRequest{BuildNames: []string{"first"}, Params: map[string]string{"version": "v"}, BuildParams: map[string]map[string]string{"second": {}}}, "admin", store.ErrInvalid},
		{"unknown override", namedPipeline, TriggerRequest{All: true, Params: map[string]string{"unknown": "PRIVATE_VALUE"}}, "admin", errPipeline},
		{"bad post template", "version: 1\nrunner: {platform: android}\nsteps: [{kind: run, run: ':'}]\npost:\n  always: [{kind: artifact, paths: ['{{unknown}}']}]\n", TriggerRequest{}, "admin", errPipeline},
		{"unknown ios signing", "version: 1\nrunner: {platform: ios}\nios_signing: {p12: '${PRIVATE_P12}'}\nsteps: [{kind: run, run: ':'}]\n", TriggerRequest{}, "admin", errPipeline},
		{"missing runner", "version: 1\nsteps: [{kind: run, run: ':'}]\n", TriggerRequest{}, "admin", errPipeline},
		{"enabled notifications", "version: 1\nrunner: {platform: android}\nnotifications: {webhooks: [{type: generic, url: '${PRIVATE_HOOK}'}]}\nsteps: [{kind: run, run: ':'}]\n", TriggerRequest{}, "admin", errUnsupported},
		{"upload false admin", uploadPipeline, TriggerRequest{}, "admin", store.ErrForbidden},
		{"upload false trigger", uploadPipeline, TriggerRequest{AllowUpload: true}, "trigger", store.ErrForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, st, _ := serverFixture(t)
			admin, _ := st.Authenticate(context.Background(), adminToken)
			repo, name := triggerProject(t, st, admin, tc.config, config.ProjectSettings{}, "")
			actor := admin
			if tc.role != "admin" {
				token, err := st.CreateToken(context.Background(), admin, tc.role)
				if err != nil {
					t.Fatal(err)
				}
				actor, err = st.Authenticate(context.Background(), token.Token)
				if err != nil {
					t.Fatal(err)
				}
			}
			_, err := s.Trigger(context.Background(), actor, name, "failure-key", tc.request)
			if !errors.Is(err, tc.want) {
				t.Fatalf("需要%v 得到%v", tc.want, err)
			}
			p, _ := st.GetProject(context.Background(), name)
			rows, err := st.ListBuilds(context.Background(), store.BuildFilter{})
			if err != nil || p.NextNumber != 1 || len(rows) != 0 {
				t.Fatal("失败产生了部分队列", p.NextNumber, rows, err)
			}
			if _, err := os.Stat(filepath.Join(repo, "PRIVATE_MUST_NOT_EXECUTE")); !os.IsNotExist(err) {
				t.Fatal("执行了脚本")
			}
		})
	}
}

const uploadPipeline = `version: 1
runner: {platform: android}
when: {branches: [release]}
steps:
 - {kind: run, run: 'touch PRIVATE_MUST_NOT_EXECUTE'}
 - {kind: upload, target: google_play, file: out.apk, credentials: '${PRIVATE_PLAY_KEY}'}
`

func TestActualHTTPTriggerReplayAndQueryRoles(t *testing.T) {
	s, st, h := serverFixture(t)
	actor, _ := st.Authenticate(context.Background(), adminToken)
	_, name := triggerProject(t, st, actor, uploadPipeline, config.ProjectSettings{}, "")
	// 明确许可后false when记录skipped且不消费构建号。
	res, err := s.Trigger(context.Background(), actor, name, "explicit-upload", TriggerRequest{AllowUpload: true})
	if err != nil || res.Builds[0].Status != "skipped" || res.Builds[0].Number != nil {
		t.Fatal(res, err)
	}
	for _, role := range []string{"trigger", "approver"} {
		token, err := st.CreateToken(context.Background(), actor, role)
		if err != nil {
			t.Fatal(err)
		}
		code, _ := request(t, h, "GET", "/api/builds", token.Token, "")
		want := 200
		if role == "trigger" {
			want = 403
		}
		if code != want {
			t.Fatalf("role %s status %d", role, code)
		}
		code, body := request(t, h, "GET", "/api/builds/"+res.Builds[0].ID, token.Token, "")
		if code != want || strings.Contains(body, "PRIVATE_") {
			t.Fatal(code, body)
		}
	}
}
func TestActualTriggerConcurrentSameKeyAndNoDoubleNumbers(t *testing.T) {
	s, st, _ := serverFixture(t)
	actor, _ := st.Authenticate(context.Background(), adminToken)
	_, name := triggerProject(t, st, actor, "version: 1\nrunner: {platform: android}\nsteps: [{kind: run, run: ':'}]\n", config.ProjectSettings{}, "")
	var wg sync.WaitGroup
	results := make(chan store.BatchResult, 5)
	errs := make(chan error, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := s.Trigger(context.Background(), actor, name, "concurrent-key", TriggerRequest{})
			results <- res
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	id := ""
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for res := range results {
		if id == "" {
			id = res.ID
		}
		if res.ID != id {
			t.Fatal("重复批次")
		}
	}
	p, _ := st.GetProject(context.Background(), name)
	if p.NextNumber != 2 {
		t.Fatal(p.NextNumber)
	}
	for i := 0; i < 3; i++ {
		_, err := s.Trigger(context.Background(), actor, name, fmt.Sprintf("unique-%d", i), TriggerRequest{})
		if err != nil {
			t.Fatal(err)
		}
	}
	rows, err := st.ListBuilds(context.Background(), store.BuildFilter{})
	if err != nil || len(rows) != 4 {
		t.Fatal(rows, err)
	}
	safe, _ := json.Marshal(rows)
	if strings.Contains(string(safe), "run:") {
		t.Fatal("泄露配置")
	}
}

func TestActualHTTPTriggerFirstReplayConflictAndConcurrentStatuses(t *testing.T) {
	_, st, h := serverFixture(t)
	actor, _ := st.Authenticate(context.Background(), adminToken)
	repo, _ := triggerProject(t, st, actor, "version: 1\nrunner: {platform: android}\nsteps: [{kind: run, run: 'touch PRIVATE_MUST_NOT_EXECUTE'}]\n", config.ProjectSettings{}, "")
	code, first := request(t, h, "POST", "/api/projects/app/builds", adminToken, `{}`, "http-key")
	if code != 201 {
		t.Fatal(code, first)
	}
	code, again := request(t, h, "POST", "/api/projects/app/builds", adminToken, `{"branch":"main"}`, "http-key")
	if code != 200 || first != again {
		t.Fatal(code, first, again)
	}
	code, body := request(t, h, "POST", "/api/projects/app/builds", adminToken, `{"allow_upload":true}`, "http-key")
	if code != 409 {
		t.Fatal(code, body)
	}
	for _, input := range []string{`{"ref":null}`, `{"params":{"x":1}}`, `{"params":{"x":"1","x":"2"}}`, `{"all":true,"all":false}`, `{"unknown":"PRIVATE_MARKER"}`} {
		code, body = request(t, h, "POST", "/api/projects/app/builds", adminToken, input, "bad-key")
		if code != 400 || strings.Contains(body, "PRIVATE_") {
			t.Fatal(code, body)
		}
	}
	var wg sync.WaitGroup
	codes := make(chan int, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, _ := request(t, h, "POST", "/api/projects/app/builds", adminToken, `{}`, "concurrent-http-key")
			codes <- code
		}()
	}
	wg.Wait()
	close(codes)
	created, replay := 0, 0
	for code := range codes {
		switch code {
		case 201:
			created++
		case 200:
			replay++
		default:
			t.Fatal(code)
		}
	}
	if created != 1 || replay != 4 {
		t.Fatal(created, replay)
	}
	if _, err := os.Stat(filepath.Join(repo, "PRIVATE_MUST_NOT_EXECUTE")); !os.IsNotExist(err) {
		t.Fatal("HTTP触发运行了仓库命令")
	}
}
func TestActualTriggerDefaultNodeDisabledNotificationsAndBoundaries(t *testing.T) {
	s, st, _ := serverFixture(t)
	actor, _ := st.Authenticate(context.Background(), adminToken)
	_, name := triggerProject(t, st, actor, "version: 1\nnotifications:\n enabled: false\n webhooks: [{type: generic, url: '${PRIVATE_DISABLED_HOOK}'}]\nsteps: [{kind: run, run: ':'}]\n", config.ProjectSettings{}, "linux")
	result, err := s.Trigger(context.Background(), actor, name, "default-node", TriggerRequest{})
	if err != nil || result.Builds[0].Status != "queued" {
		t.Fatal(result, err)
	}
	for _, req := range []TriggerRequest{{All: true, BuildNames: []string{"default"}}, {Params: map[string]string{"project": "forged"}}, {Params: map[string]string{"x": strings.Repeat("v", 4097)}}, {BuildNames: []string{"default", "default"}}, {Branch: "release"}, {Ref: "HEAD~1"}} {
		_, err := s.Trigger(context.Background(), actor, name, "bad-shape", req)
		if err == nil {
			t.Fatal("非法请求被接受", req)
		}
	}
	for _, key := range []string{"", "bad key", strings.Repeat("k", 129)} {
		_, err := s.Trigger(context.Background(), actor, name, key, TriggerRequest{})
		if !errors.Is(err, store.ErrInvalid) {
			t.Fatal(err)
		}
	}
	rows, _ := st.ListBuilds(context.Background(), store.BuildFilter{})
	if len(rows) != 1 {
		t.Fatal(len(rows))
	}
}
