package client

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"mybuilds/internal/config"
	"mybuilds/internal/store"
)

func TestRetentionCLIActualPolicyAndSettings(t *testing.T) {
	st, address := realRemoteAPI(t)
	if err := st.SyncGlobalRetention(context.Background(), config.Retention{Builds: 100, Days: 30}); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	file := filepath.Join(directory, "settings.yml")
	if err := os.WriteFile(file, []byte("pipeline: {params: {channel: PRIVATE_RETENTION_PARAMETER}}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	call := func(args ...string) string {
		t.Helper()
		out, err := executeRemote(t, append([]string{"--server-url", address}, args...)...)
		if err != nil || strings.Contains(out, "PRIVATE_") || strings.Contains(out, directory) {
			t.Fatal("实际CLI失败或泄露", out, err)
		}
		return out
	}
	call("project", "init", "retained", "--repo", directory, "--nodes", "linux", "--settings", file, "--json")
	initial, err := st.GetProject(context.Background(), "retained")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		body         string
		builds, days int64
		bs, ds       string
	}{
		{"retention: {builds: 200}\n", 200, 30, "project", "global"},
		{"retention: {days: 45}\n", 100, 45, "global", "project"},
		{"retention: {}\n", 100, 30, "global", "global"},
	} {
		if err := os.WriteFile(file, []byte(item.body), 0600); err != nil {
			t.Fatal(err)
		}
		call("project", "set", "retained", "--settings", file, "--json")
		out := call("retention", "show", "retained", "--json")
		var view store.EffectiveRetention
		if err := json.Unmarshal([]byte(out), &view); err != nil || view.Builds != item.builds || view.Days != item.days || view.BuildsSource != item.bs || view.DaysSource != item.ds {
			t.Fatal("有效策略JSON错误", out, err)
		}
		current, err := st.GetProject(context.Background(), "retained")
		if err != nil || current.Settings.Pipeline == nil || current.Settings.Pipeline.Params["channel"] != initial.Settings.Pipeline.Params["channel"] {
			t.Fatal("retention-only改了原pipeline", err)
		}
	}
	out := call("retention", "show", "retained")
	if !strings.Contains(out, "BUILDS") || !strings.Contains(out, "DAYS") || !strings.Contains(out, "global") {
		t.Fatal("表格缺少策略来源", out)
	}
	before, err := st.GetProject(context.Background(), "retained")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("retention: {builds: 0}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := executeRemote(t, "--server-url", address, "project", "set", "retained", "--settings", file); err == nil {
		t.Fatal("非法settings被发送并接受")
	}
	after, err := st.GetProject(context.Background(), "retained")
	if err != nil || after.PolicyVersion != before.PolicyVersion {
		t.Fatal("非法CLI导入部分写入", err)
	}
}

func TestRetentionCLIHelpDoesNotLoadClient(t *testing.T) {
	file := filepath.Join(t.TempDir(), "broken-client.yml")
	if err := os.WriteFile(file, []byte("unknown: PRIVATE_RETENTION_CLIENT\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"retention", "--help"}, {"retention", "show", "--help"}, {"version"}, {"--help"}} {
		out, err := executeRemote(t, append([]string{"--config", file}, args...)...)
		if err != nil || strings.Contains(out, "PRIVATE_RETENTION_CLIENT") {
			t.Fatal("help读取坏client配置", out, err)
		}
	}
	if _, err := executeRemote(t, "--config", file, "retention", "show", "retained"); err == nil {
		t.Fatal("远程命令未读取客户端配置")
	}
}

func TestRetentionCandidatesCLIActualPageAndTable(t *testing.T) {
	st, address := realRemoteAPI(t)
	ctx := context.Background()
	actor, err := st.Authenticate(ctx, os.Getenv("MYBUILDS_CLIENT_TOKEN"))
	if err != nil {
		t.Fatal(err)
	}
	if err = st.SyncGlobalRetention(ctx, config.Retention{Builds: 1, Days: 30}); err != nil {
		t.Fatal(err)
	}
	project, err := st.CreateProject(ctx, actor, store.ProjectInput{Name: "candidate-app", Repository: "/tmp/PRIVATE_RETENTION_REPO", AllowedNodes: []string{"linux"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"first", "second", "active"} {
		batch, err := st.Enqueue(ctx, store.EnqueueInput{Actor: actor, ProjectID: project.ID, ProjectVersion: project.PolicyVersion, Key: name, RequestDigest: strings.Repeat("a", 64), SHA: strings.Repeat("b", 40), Branch: "main", Source: "repo", File: "mybuilds.yml", SourceDigest: strings.Repeat("c", 64), Builds: []store.PreparedBuild{{Name: name, Status: "queued", Snapshot: store.BuildSnapshot{Definition: config.Build{Steps: []config.Step{{Name: "shell", Kind: "run", Run: "echo PRIVATE_RETENTION_SCRIPT"}}}, Params: map[string]string{}, Facts: map[string]string{}, Condition: "ready", AllowedNodes: project.AllowedNodes}, PostBudgetNS: int64(2 * time.Minute), Steps: []store.StepProgress{{Phase: "ordinary", Index: 1, Name: "shell", Kind: "run", Condition: "ready", Status: "pending"}}}}})
		if err != nil {
			t.Fatal(err)
		}
		if name != "active" {
			if _, err = st.Cancel(ctx, actor, batch.Builds[0].ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, page := range []store.Page{{Limit: 20}, {Limit: 1, Offset: 1}, {Limit: 200}, {Limit: 20, Offset: 1000000}} {
		expected, err := st.EvaluateRetention(ctx, actor, project.ID, page)
		if err != nil {
			t.Fatal(err)
		}
		out, err := executeRemote(t, "--server-url", address, "retention", "ls", project.Name, "--candidates", "--json", "--limit", strconv.Itoa(page.Limit), "--offset", strconv.Itoa(page.Offset))
		var actual store.RetentionPage
		if err != nil || strings.Contains(out, "PRIVATE_") || json.Unmarshal([]byte(out), &actual) != nil || actual.Items == nil {
			t.Fatal("候选CLI失败", out, err)
		}
		for i := range expected.Items {
			expected.Items[i].EvaluatedAt = nil
		}
		for i := range actual.Items {
			actual.Items[i].EvaluatedAt = nil
		}
		if !reflect.DeepEqual(actual, expected) {
			t.Fatal("CLI过滤或改变Store页", actual, expected)
		}
	}
	out, err := executeRemote(t, "--server-url", address, "retention", "ls", project.Name, "--candidates")
	if err != nil || !strings.Contains(out, "CANDIDATE") || !strings.Contains(out, "PROTECT_REASONS") || !strings.Contains(out, "false") || !strings.Contains(out, "true") || !strings.Contains(out, "active") {
		t.Fatal("表格未保留候选和保护", out, err)
	}
	// 尚无登记时返回真实Store空页；查询本身不能登记或推进清理。
	out, err = executeRemote(t, "--server-url", address, "retention", "ls", project.Name, "--json")
	var jobs store.RetentionPage
	if err != nil || json.Unmarshal([]byte(out), &jobs) != nil || jobs.Items == nil || len(jobs.Items) != 0 {
		t.Fatal("真实空事项查询失败", out, err)
	}

}

func TestRetentionCandidatesCLIPaginationAndHelpIsolation(t *testing.T) {
	for _, args := range [][]string{{"--limit", "0"}, {"--limit", "201"}, {"--offset", "-1"}, {"--offset", "1000001"}} {
		_, err := executeRemote(t, append([]string{"--server-url", "http://127.0.0.1:1", "retention", "ls", "app", "--candidates"}, args...)...)
		if err == nil || err.Error() != "分页参数不合法" {
			t.Fatal("分页未在传输前校验", err)
		}
	}
	file := filepath.Join(t.TempDir(), "bad-client.yml")
	if err := os.WriteFile(file, []byte("unknown: PRIVATE_CANDIDATE_CLIENT\n"), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := executeRemote(t, "--config", file, "retention", "ls", "--help")
	if err != nil || strings.Contains(out, "PRIVATE_CANDIDATE_CLIENT") {
		t.Fatal("help读取坏client配置", out, err)
	}
}

func TestRetentionCLIActualRunAndJobs(t *testing.T) {
	st, address := realRemoteAPI(t)
	ctx := context.Background()
	actor, err := st.Authenticate(ctx, os.Getenv("MYBUILDS_CLIENT_TOKEN"))
	if err != nil {
		t.Fatal(err)
	}
	if err = st.SyncGlobalRetention(ctx, config.Retention{Builds: 1, Days: 30}); err != nil {
		t.Fatal(err)
	}
	p, err := st.CreateProject(ctx, actor, store.ProjectInput{Name: "jobs-app", Repository: "/tmp/PRIVATE_RETENTION_NO_RUN", AllowedNodes: []string{"linux"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"old", "newer"} {
		in := store.PreparedBuild{Name: name, Status: "queued", PostBudgetNS: int64(2 * time.Minute), Snapshot: store.BuildSnapshot{Definition: config.Build{Steps: []config.Step{{Name: "shell", Kind: "run", Run: "echo PRIVATE_NOT_EXECUTED"}}}, Params: map[string]string{}, Facts: map[string]string{}, Condition: "ready", AllowedNodes: p.AllowedNodes}, Steps: []store.StepProgress{{Phase: "ordinary", Index: 1, Name: "shell", Kind: "run", Status: "pending", Condition: "ready"}}}
		batch, err := st.Enqueue(ctx, store.EnqueueInput{Actor: actor, ProjectID: p.ID, ProjectVersion: p.PolicyVersion, Key: name, RequestDigest: strings.Repeat("a", 64), SHA: strings.Repeat("b", 40), Branch: "main", Source: "repo", File: "mybuilds.yml", SourceDigest: strings.Repeat("c", 64), Builds: []store.PreparedBuild{in}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = st.Cancel(ctx, actor, batch.Builds[0].ID); err != nil {
			t.Fatal(err)
		}
	}
	out, err := executeRemote(t, "--server-url", address, "retention", "run", p.Name, "--json")
	var page store.RetentionPage
	if err != nil || json.Unmarshal([]byte(out), &page) != nil || len(page.Items) != 1 || page.Items[0].JobID == "" || page.Items[0].CentralState != "completed" || page.Items[0].NodeState != "not_applicable" || page.Items[0].HistoryState != "cleaned" || page.Items[0].CleanedAt == nil || strings.Contains(out, "PRIVATE_") {
		t.Fatal("实际run失败/伪完成/泄露", out, err)
	}
	job := page.Items[0].JobID
	for _, args := range [][]string{{"retention", "run", p.Name, "--limit", "1", "--json"}, {"retention", "ls", p.Name, "--limit", "1", "--json"}} {
		out, err = executeRemote(t, append([]string{"--server-url", address}, args...)...)
		if err != nil || json.Unmarshal([]byte(out), &page) != nil || len(page.Items) != 1 || page.Items[0].JobID != job {
			t.Fatal("run/ls改变原删除身份", out, err)
		}
	}
	out, err = executeRemote(t, "--server-url", address, "retention", "run", p.Name)
	if err != nil || !strings.Contains(out, "CENTRAL_STATE") || !strings.Contains(out, "NODE_STATE") || !strings.Contains(out, "completed") {
		t.Fatal("run表格缺真实分态", out, err)
	}
	for _, role := range []string{"trigger", "approver"} {
		token, err := st.CreateToken(ctx, actor, role)
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv("MYBUILDS_CLIENT_TOKEN", token.Token)
		for _, args := range [][]string{{"retention", "run", p.Name}, {"retention", "ls", p.Name}} {
			if out, err = executeRemote(t, append([]string{"--server-url", address}, args...)...); err == nil || strings.Contains(out, "PRIVATE_") {
				t.Fatal("非admin启动/查询清理", out, err)
			}
		}
	}
}
func TestRetentionCLIRunLimitsAndHelp(t *testing.T) {
	for _, limit := range []string{"0", "-1", "101", "999999999999999999999"} {
		_, err := executeRemote(t, "--server-url", "http://127.0.0.1:1", "retention", "run", "app", "--limit", limit)
		if err == nil || strings.Contains(err.Error(), "远程") {
			t.Fatal("非法limit进入远程传输", err)
		}
	}
	file := filepath.Join(t.TempDir(), "bad-client.yml")
	if err := os.WriteFile(file, []byte("unknown: PRIVATE_RETENTION_RUN\n"), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := executeRemote(t, "--config", file, "retention", "run", "--help")
	if err != nil || strings.Contains(out, "PRIVATE_RETENTION_RUN") {
		t.Fatal("help加载坏配置", out, err)
	}
}
