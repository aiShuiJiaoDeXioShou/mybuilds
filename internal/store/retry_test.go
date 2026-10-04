package store

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/google/uuid"
)

func cancelledQueued(t *testing.T, s *Store) (Project, string) {
	t.Helper()
	p, err := s.CreateProject(testContext, localAdmin, projectInput("retry-app"))
	if err != nil {
		t.Fatal(err)
	}
	id := queueBuild(t, s, p, "original", "build", false)
	if _, err = s.Cancel(testContext, localAdmin, id); err != nil {
		t.Fatal(err)
	}
	return p, id
}
func TestRetryOriginalSnapshotConcurrentAndRelation(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		p, id := cancelledQueued(t, s)
		before := recoveryRows(t, s)
		// 当前参数默认与default改变不影响原快照；原default仍在当前授权交集中。
		if err := s.writer.Model(&projectRecord{}).Where("id = ?", p.ID).Updates(map[string]any{"settings_json": "{}", "default_node": "other", "nodes_json": "[\"linux\",\"other\"]"}).Error; err != nil {
			t.Fatal(err)
		}
		type answer struct {
			out BatchResult
			err error
		}
		results := make(chan answer, 20)
		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				o, e := s.Retry(testContext, localAdmin, RetryInput{BuildID: id, Key: "retry-key"})
				results <- answer{o, e}
			}()
		}
		wg.Wait()
		close(results)
		var target string
		fresh := 0
		for r := range results {
			if r.err != nil {
				t.Fatal(r.err)
			}
			if !r.out.Replayed {
				fresh++
			}
			if len(r.out.Builds) != 1 || r.out.Builds[0].RetryOf != id || *r.out.Builds[0].Number != 2 {
				t.Fatal("关系/编号", r.out)
			}
			if target != "" && target != r.out.Builds[0].ID {
				t.Fatal("重复新构建")
			}
			target = r.out.Builds[0].ID
		}
		if fresh != 1 {
			t.Fatal("首次响应数量", fresh)
		}
		var original, child buildRecord
		s.db.First(&original, "id = ?", id)
		s.db.First(&child, "id = ?", target)
		if !reflect.DeepEqual(before[0], original) {
			t.Fatal("原构建被改写")
		}
		var a, b BuildSnapshot
		json.Unmarshal([]byte(original.SnapshotJSON), &a)
		json.Unmarshal([]byte(child.SnapshotJSON), &b)
		if b.Facts["build.id"] != child.ID || b.Facts["build.number"] != "2" {
			t.Fatal("新身份事实")
		}
		b.Facts["build.id"] = a.Facts["build.id"]
		b.Facts["build.number"] = a.Facts["build.number"]
		if !reflect.DeepEqual(a, b) {
			t.Fatal("原定义/参数/when/事实变动")
		}
		var count int64
		s.db.Model(&attemptRecord{}).Where("build_id = ?", target).Count(&count)
		if count != 0 || buildRef(child).Epoch != 0 || child.LastEventSeq != 0 || child.CancelRequested || child.StopUnconfirmed {
			t.Fatal("继承旧执行证据")
		}
		current, _ := s.GetProject(testContext, p.Name)
		if current.NextNumber != 3 {
			t.Fatal("重放占号")
		}
		if _, err := s.Retry(testContext, localAdmin, RetryInput{BuildID: id, Key: "retry-key", AllowUpload: true}); err != ErrConflict {
			t.Fatal("不同输入同key", err)
		}
		in := enqueueInput(current, "retry-key")
		if _, err := s.Enqueue(testContext, in); err != ErrConflict {
			t.Fatal("trigger跨操作同key", err)
		}
	})
}
func TestRetryRejectsWithoutAllocationAndRechecksReplay(t *testing.T) {
	for _, name := range []string{"queued", "skipped", "guard", "range", "branch", "facts", "params", "step", "budget", "missing-attempt"} {
		t.Run(name, func(t *testing.T) {
			stores(t, func(t *testing.T, s *Store, opt Options) {
				p, id := cancelledQueued(t, s)
				switch name {
				case "queued":
					s.writer.Model(&buildRecord{}).Where("id = ?", id).Updates(map[string]any{"status": "queued", "cancel_requested": false})
				case "skipped":
					s.writer.Model(&buildRecord{}).Where("id = ?", id).Update("status", "skipped")
				case "guard":
					s.writer.Model(&buildRecord{}).Where("id = ?", id).Update("stop_unconfirmed", true)
				case "range":
					s.writer.Model(&projectRecord{}).Where("id = ?", p.ID).Update("nodes_json", "[\"other\"]")
				case "branch":
					s.writer.Model(&projectRecord{}).Where("id = ?", p.ID).Update("branches_json", "[\"private\"]")
				case "facts", "params":
					var row buildRecord
					s.db.First(&row, "id = ?", id)
					var snapshot BuildSnapshot
					json.Unmarshal([]byte(row.SnapshotJSON), &snapshot)
					if name == "facts" {
						delete(snapshot.Facts, "build.number")
					} else {
						snapshot.Params = map[string]string{}
					}
					data, _ := encode(snapshot)
					s.writer.Model(&row).Update("snapshot_json", data)
				case "step":
					s.writer.Model(&stepRecord{}).Where("build_id = ? AND phase = ?", id, "ordinary").Update("name", "forged")
				case "budget":
					s.writer.Model(&buildRecord{}).Where("id = ?", id).Update("post_budget_ns", int64(1))
				case "missing-attempt":
					s.writer.Model(&buildRecord{}).Where("id = ?", id).Update("status", "failed")
				}
				before := recoveryRows(t, s)
				if _, err := s.Retry(testContext, localAdmin, RetryInput{BuildID: id, Key: "invalid"}); err == nil {
					t.Fatal("无效retry仍接受")
				}
				if !reflect.DeepEqual(before, recoveryRows(t, s)) {
					t.Fatal("拒绝时创建row/修改原构建")
				}
				current, _ := s.GetProject(testContext, p.Name)
				if current.NextNumber != 2 {
					t.Fatal("拒绝占号")
				}
			})
		})
	}
	stores(t, func(t *testing.T, s *Store, opt Options) {
		p, id := cancelledQueued(t, s)
		req := RetryInput{BuildID: id, Key: "ok"}
		if _, err := s.Retry(testContext, localAdmin, req); err != nil {
			t.Fatal(err)
		}
		if err := s.writer.Model(&projectRecord{}).Where("id = ?", p.ID).Update("nodes_json", "[\"other\"]").Error; err != nil {
			t.Fatal(err)
		}
		if _, err := s.Retry(testContext, localAdmin, req); err != ErrForbidden {
			t.Fatal("重放未复核当前授权", err)
		}
	})
}
func TestRetryFinishedResetsAllExecutionEvidence(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		_, terminal := terminalReceiptFixture(t, s)
		id := terminal.Ref.BuildID
		before := recoveryRows(t, s)
		out, err := s.Retry(testContext, localAdmin, RetryInput{BuildID: id, Key: "finished"})
		if err != nil {
			t.Fatal(err)
		}
		var original buildRecord
		s.db.First(&original, "id = ?", id)
		if !reflect.DeepEqual(before[0], original) {
			t.Fatal("原证据改写")
		}
		var rows []stepRecord
		s.db.Where("build_id = ?", out.Builds[0].ID).Find(&rows)
		for _, step := range rows {
			if step.Status != "pending" || step.Intent || step.Started || step.StopConfirmed || step.CleanupFailed || step.ElapsedNS != 0 || step.ExitCode != 0 || step.ArtifactIDsJSON != "" {
				t.Fatalf("旧执行证据继承: %s", step.Phase)
			}
		}
	})
}
func TestRetryInputAndRoles(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		_, id := cancelledQueued(t, s)
		for _, in := range []RetryInput{{BuildID: "invalid", Key: "ok"}, {BuildID: id}, {BuildID: id, Key: "bad/key"}} {
			if _, err := s.Retry(testContext, localAdmin, in); err != ErrInvalid {
				t.Fatal("输入未拒绝", err)
			}
		}
		for _, role := range []string{"approver", "trigger"} {
			token, err := s.CreateToken(testContext, localAdmin, role)
			if err != nil {
				t.Fatal(err)
			}
			actor, _ := s.Authenticate(testContext, token.Token)
			out, err := s.Retry(testContext, actor, RetryInput{BuildID: id, Key: fmt.Sprintf("role-%s", role)})
			if role == "approver" && err != ErrForbidden {
				t.Fatal("角色", err)
			}
			if role == "trigger" && (err != nil || out.ID == "") {
				t.Fatal("trigger", err)
			}
		}
		if _, err := s.Retry(testContext, localAdmin, RetryInput{BuildID: uuid.NewString(), Key: "none"}); err != ErrNotFound {
			t.Fatal("不存在", err)
		}
	})
}
