//go:build darwin || linux

package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"mybuilds/internal/config"
	"mybuilds/internal/store"
)

func retentionManagementFixture(t *testing.T) (*Server, *store.Store, *httptest.Server, store.Actor, string) {
	t.Helper()
	s, st, h := retentionHTTPFixture(t)
	actor, err := st.Authenticate(context.Background(), adminToken)
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.GetProject(context.Background(), "retained")
	if err != nil {
		t.Fatal(err)
	}
	if err = st.SyncGlobalRetention(context.Background(), config.Retention{Builds: 1, Days: 30}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"old", "newer"} {
		in := store.PreparedBuild{Name: name, Status: "queued", PostBudgetNS: int64(2 * time.Minute), Snapshot: store.BuildSnapshot{Definition: config.Build{Steps: []config.Step{{Name: "shell", Kind: "run", Run: "echo PRIVATE_NO_EXECUTION"}}}, Params: map[string]string{}, Facts: map[string]string{}, Condition: "ready", AllowedNodes: p.AllowedNodes}, Steps: []store.StepProgress{{Phase: "ordinary", Index: 1, Name: "shell", Kind: "run", Status: "pending", Condition: "ready"}}}
		batch, err := st.Enqueue(context.Background(), store.EnqueueInput{Actor: actor, ProjectID: p.ID, ProjectVersion: p.PolicyVersion, Key: name, RequestDigest: strings.Repeat("a", 64), SHA: strings.Repeat("b", 40), Branch: "main", Source: "repo", File: "mybuilds.yml", SourceDigest: strings.Repeat("c", 64), Builds: []store.PreparedBuild{in}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = st.Cancel(context.Background(), actor, batch.Builds[0].ID); err != nil {
			t.Fatal(err)
		}
	}
	return s, st, h, actor, p.ID
}
func TestRetentionManagementActualScheduleJobsAndReplay(t *testing.T) {
	s, st, h, actor, id := retentionManagementFixture(t)
	before, err := st.ListRetention(context.Background(), actor, id, store.Page{})
	if err != nil || len(before.Items) != 0 {
		t.Fatal(err, before)
	}
	code, body := request(t, h, "POST", "/api/projects/retained/retention", adminToken, `{"limit":100}`)
	var page store.RetentionPage
	if code != 200 || json.Unmarshal([]byte(body), &page) != nil || len(page.Items) != 1 || page.Items[0].CentralState != "completed" || page.Items[0].NodeState != "not_applicable" || page.Items[0].HistoryState != "cleaned" || page.Items[0].CleanedAt == nil || page.Items[0].JobID == "" {
		t.Fatal("管理POST未返回真实分态", code, body)
	}
	job := page.Items[0].JobID
	for _, query := range []string{"?limit=1&offset=0", "?limit=200&offset=0", "?limit=20&offset=1000000"} {
		code, body = request(t, h, "GET", "/api/projects/retained/retention/jobs"+query, adminToken, "")
		if code != 200 || strings.Contains(body, "PRIVATE_") || strings.Contains(body, s.config.DataDir) || strings.Contains(body, "storage_id") || json.Unmarshal([]byte(body), &page) != nil || page.Items == nil {
			t.Fatal("事项查询失败/泄露", code, body)
		}
	}
	code, body = request(t, h, "POST", "/api/projects/retained/retention", adminToken, `{"limit":1}`)
	if code != 200 || json.Unmarshal([]byte(body), &page) != nil || len(page.Items) != 1 || page.Items[0].JobID != job {
		t.Fatal("重复推进新建删除身份", code, body)
	}
	actual, err := st.ListRetention(context.Background(), actor, id, store.Page{Limit: 1})
	if err != nil || !reflect.DeepEqual(actual, page) {
		t.Fatal("返回非实际Store事项页", err, actual, page)
	}
}
func TestRetentionManagementStrictInputAndRoles(t *testing.T) {
	s, st, h, actor, id := retentionManagementFixture(t)
	for _, body := range []string{`{}`, `{"limit":0}`, `{"limit":101}`, `{"limit":-1}`, `{"limit":null}`, `{"limit":"1"}`, `{"limit":1.0}`, `{"limit":1e0}`, `{"Limit":1}`, `{"limit":1,"limit":2}`, `{"limit":1,"force":true}`, `{"limit":1} {"limit":1}`, `null`} {
		code, out := request(t, h, "POST", "/api/projects/retained/retention", adminToken, body)
		if code != 400 {
			t.Fatal("非法管理声明未拒", body, code, out)
		}
	}
	for _, path := range []string{"/api/projects/retained/retention?force=true", "/api/projects/retained/retention/jobs?limit=0", "/api/projects/retained/retention/jobs?limit=201", "/api/projects/retained/retention/jobs?offset=-1", "/api/projects/retained/retention/jobs?limit=1&limit=2", "/api/projects/retained/retention/jobs?unknown=1"} {
		method, body := "GET", ""
		if strings.Contains(path, "force=") {
			method, body = "POST", `{"limit":1}`
		}
		code, out := request(t, h, method, path, adminToken, body)
		if code != 400 {
			t.Fatal("非法query未拒", path, code, out)
		}
	}
	code, body := request(t, h, "POST", "/api/projects/retained/retention", adminToken, `{"limit":1,"unknown":"`+strings.Repeat("x", 32*1024)+`"}`)
	if code != 413 {
		t.Fatal("超过32KiB未拒", code, body)
	}
	for _, role := range []string{"trigger", "approver"} {
		token, err := st.CreateToken(context.Background(), actor, role)
		if err != nil {
			t.Fatal(err)
		}
		for _, project := range []string{"retained", "PRIVATE_MISSING"} {
			for _, suffix := range []string{"", "/jobs"} {
				method, body := "GET", ""
				if suffix == "" {
					method, body = "POST", `{"limit":1}`
				}
				code, out := request(t, h, method, "/api/projects/"+project+"/retention"+suffix, token.Token, body)
				if code != 403 || strings.Contains(out, project) {
					t.Fatal("角色查存在性或启动管理", role, code, out)
				}
			}
		}
	}
	node, err := st.CreateNode(context.Background(), actor, store.NodeInput{Name: "private-node", Capacity: 1})
	if err != nil {
		t.Fatal(err)
	}
	code, body = request(t, h, "POST", "/api/projects/retained/retention", node.Token, `{"limit":1}`)
	if code != 401 {
		t.Fatal("Node身份进入管理", code, body)
	}
	jobs, err := st.ListRetention(context.Background(), actor, id, store.Page{})
	if err != nil || len(jobs.Items) != 0 {
		t.Fatal("拒绝输入仍登记删除事项", err, jobs)
	}
	_ = s
}
func TestRetentionManagementActualPhysicalAndFixedError(t *testing.T) {
	for _, change := range []string{"normal", "foreign"} {
		t.Run(change, func(t *testing.T) {
			s, actor, project, o := retentionCleanupFixture(t, "artifact")
			if change == "foreign" {
				if err := os.Chmod(retentionCleanupPath(s, o), 0644); err != nil {
					t.Fatal(err)
				}
			}
			h := httptest.NewServer(s.Handler())
			defer h.Close()
			code, body := request(t, h, "POST", "/api/projects/app/retention", adminToken, `{"limit":100}`)
			if change == "normal" {
				var page store.RetentionPage
				if code != 200 || json.Unmarshal([]byte(body), &page) != nil || len(page.Items) != 1 || page.Items[0].CentralState != "completed" || page.Items[0].NodeState != "pending" {
					t.Fatal("物理完成/离线节点分态不真实", code, body)
				}
				if _, err := os.Lstat(retentionCleanupPath(s, o)); !os.IsNotExist(err) {
					t.Fatal("真实文件仍存在", err)
				}
			} else {
				if code != 409 || !strings.Contains(body, "retention_ownership_unknown") || strings.Contains(body, filepath.Dir(retentionCleanupPath(s, o))) {
					t.Fatal("物理错误未沿固定响应", code, body)
				}
				if _, err := os.Lstat(retentionCleanupPath(s, o)); err != nil {
					t.Fatal("unsafe仍删除", err)
				}
				jobs, err := s.store.ListRetention(context.Background(), actor, project, store.Page{})
				if err != nil || len(jobs.Items) != 1 || jobs.Items[0].CentralState == "completed" {
					t.Fatal("未完成伪装完成", err, jobs)
				}
			}
		})
	}
}
