//go:build darwin || linux

package server

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
)

// 原始文件由真实HTTP上传、事件终态和Schedule登记，不手填清理业务行。
func retentionCleanupFixture(t *testing.T, kind string) (*Server, store.Actor, string, store.RetentionObject) {
	t.Helper()
	ctx := context.Background()
	var s *Server
	var st *store.Store
	var d protocol.ArtifactDeclaration
	if kind == "junit" {
		data := []byte(`<testsuite tests="1"><testcase/></testsuite>`)
		srv, db, h, token, decl, evidence := reportHTTPFixture(t, data, nil)
		s, st, d = srv, db, decl
		registration := protocol.NodeResourceRegistration{Ref: d.Ref, ID: uuid.NewString(), OwnershipDigest: strings.Repeat("a", 64), HasWorkspace: true, HasResults: true}
		code, body := request(t, h, "POST", "/api/agent/resources", token, encodeMessage(t, registration))
		if code != 204 {
			t.Fatal(code, body)
		}
		code, out := uploadArtifactHTTP(t, h, token, d, data)
		if code != 200 {
			t.Fatal(code, string(out))
		}
		seal := sealReportHTTP(t, h, token, d, evidence, 200)
		postReportEvent(t, h, token, d.Ref, 7, protocol.ExecutionProgress{Kind: "post_selected", PostPhase: "success", At: time.Now().UTC(), RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}, 200)
		postReportEvent(t, h, token, d.Ref, 8, protocol.ExecutionProgress{Kind: "build_finished", Status: "succeeded", Started: true, StopConfirmed: true, At: time.Now().UTC(), RemainingPostBudgetNS: int64(2 * time.Minute), LastArtifactSeq: 1, ArtifactSteps: []protocol.ArtifactExpectation{}, ReportManifest: &protocol.ReportManifest{SealDigest: seal, IDs: []string{d.ID}}}, 200)
	} else {
		srv, db, h, token, decl, data := artifactHTTPFixture(t)
		s, st, d = srv, db, decl
		registration := protocol.NodeResourceRegistration{Ref: d.Ref, ID: uuid.NewString(), OwnershipDigest: strings.Repeat("a", 64), HasWorkspace: true, HasResults: true}
		code, body := request(t, h, "POST", "/api/agent/resources", token, encodeMessage(t, registration))
		if code != 204 {
			t.Fatal(code, body)
		}
		code, out := uploadArtifactHTTP(t, h, token, d, data)
		if code != 200 {
			t.Fatal(code, string(out))
		}
		var logSeq, logOffset int64
		if kind == "log" {
			chunk := logHTTPMessage(t, d.Ref)
			chunk.Records[0].Step = "collect"
			chunk.Records[0].Stream = "system"
			chunk.Digest = messageDigest(t, chunk.Records)
			code, body = request(t, h, "POST", "/api/agent/logs", token, encodeMessage(t, chunk))
			if code != 200 {
				t.Fatal(code, body)
			}
			logSeq = 1
			logOffset = int64(len(encodeMessage(t, chunk.Records)))
		}
		expected := []protocol.ArtifactExpectation{{Phase: "ordinary", Index: 1, Count: 1, IDs: []string{d.ID}}}
		postReportEvent(t, h, token, d.Ref, 4, protocol.ExecutionProgress{Kind: "post_selected", PostPhase: "success", At: time.Now().UTC(), RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}, 200)
		postReportEvent(t, h, token, d.Ref, 5, protocol.ExecutionProgress{Kind: "build_finished", Status: "succeeded", Started: true, StopConfirmed: true, At: time.Now().UTC(), RemainingPostBudgetNS: int64(2 * time.Minute), LastLogSeq: logSeq, LastLogOffset: logOffset, LastArtifactSeq: 1, ArtifactSteps: expected}, 200)
	}
	actor, err := st.Authenticate(ctx, adminToken)
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.GetProject(ctx, "app")
	if err != nil {
		t.Fatal(err)
	}
	newer := store.PreparedBuild{Name: "newer", Status: "skipped", PostBudgetNS: int64(2 * time.Minute), Snapshot: store.BuildSnapshot{Definition: config.Build{Steps: []config.Step{{Name: "shell", Kind: "run", Run: "true"}}}, Params: map[string]string{}, Facts: map[string]string{}, Condition: "skipped", AllowedNodes: p.AllowedNodes, DefaultNode: p.DefaultNode}, Steps: []store.StepProgress{{Phase: "ordinary", Index: 1, Name: "shell", Kind: "run", Status: "pending", Condition: "ready"}}}
	_, err = st.Enqueue(ctx, store.EnqueueInput{Actor: actor, ProjectID: p.ID, ProjectVersion: p.PolicyVersion, Key: "retention-newer", RequestDigest: strings.Repeat("d", 64), SHA: strings.Repeat("b", 40), Branch: "main", Source: "repo", File: "mybuilds.yml", SourceDigest: strings.Repeat("c", 64), Builds: []store.PreparedBuild{newer}})
	if err != nil {
		t.Fatal(err)
	}
	if err = st.SyncGlobalRetention(ctx, config.Retention{Builds: 1, Days: 30}); err != nil {
		t.Fatal(err)
	}
	if _, err = st.ScheduleRetention(ctx, actor, p.ID, 100); err != nil {
		t.Fatal(err)
	}
	objects, err := st.AdvanceRetention(ctx, p.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range objects {
		if o.Kind == kind {
			return s, actor, p.ID, o
		}
	}
	t.Fatal("真实终态没有对应中央事项", kind)
	return nil, store.Actor{}, "", store.RetentionObject{}
}
func retentionCleanupPath(s *Server, o store.RetentionObject) string {
	kind := "artifacts"
	if o.Kind == "log" {
		kind = "logs"
	}
	return filepath.Join(s.config.DataDir, kind, o.StorageID)
}
func TestRetentionCleanupActualFilesAndMetadata(t *testing.T) {
	for _, kind := range []string{"log", "artifact", "junit"} {
		t.Run(kind, func(t *testing.T) {
			s, _, project, o := retentionCleanupFixture(t, kind)
			before, err := s.store.GetBuild(context.Background(), o.BuildID)
			if err != nil {
				t.Fatal(err)
			}
			control := filepath.Join(s.config.DataDir, "UNKNOWN_CONTROL")
			if err = os.WriteFile(control, []byte("unchanged"), 0600); err != nil {
				t.Fatal(err)
			}
			if err = s.advanceCentralRetention(context.Background(), project, 100); err != nil {
				t.Fatal(err)
			}
			if _, err = os.Lstat(retentionCleanupPath(s, o)); !os.IsNotExist(err) {
				t.Fatal("原文件未删除", err)
			}
			if _, err = os.Lstat(filepath.Join(s.config.DataDir, "retention", "quarantine", o.JobID, o.ObjectID)); !os.IsNotExist(err) {
				t.Fatal("隔离文件未删除", err)
			}
			after, err := s.store.GetBuild(context.Background(), o.BuildID)
			if err != nil || after.Status != before.Status || after.Reason != before.Reason || after.SHA != before.SHA || after.ReportSealDigest != before.ReportSealDigest {
				t.Fatal("物理清理改原终态", err)
			}
			remaining, err := s.store.AdvanceRetention(context.Background(), project, 100)
			if err != nil || len(remaining) != 0 {
				t.Fatal("deleted仍待处理", err, len(remaining))
			}
			got, err := os.ReadFile(control)
			if err != nil || string(got) != "unchanged" {
				t.Fatal("未知控制文件遭修改", err)
			}
		})
	}
}
func TestRetentionCleanupUnknownMissingKeepsIntent(t *testing.T) {
	s, _, project, o := retentionCleanupFixture(t, "artifact")
	if err := os.Remove(retentionCleanupPath(s, o)); err != nil {
		t.Fatal(err)
	}
	if err := s.advanceCentralRetention(context.Background(), project, 100); !errors.Is(err, store.ErrRetentionOwnershipUnknown) {
		t.Fatal("初次双方缺失被假确认", err)
	}
	rows, err := s.store.AdvanceRetention(context.Background(), project, 100)
	if err != nil || len(rows) != 1 || rows[0].Identity != "" || rows[0].State != "failed" || rows[0].Reason != "retention_ownership_unknown" || rows[0].QuarantineSlot != "" {
		t.Fatal("初次失败未保存原因或伪造身份", err, rows)
	}
}

func TestRetentionCleanupRecoversExactPhysicalGaps(t *testing.T) {
	for _, gap := range []string{"authorized", "renamed", "quarantined", "unlinked"} {
		t.Run(gap, func(t *testing.T) {
			s, actor, project, o := retentionCleanupFixture(t, "artifact")
			r, err := s.retentionCleanupRoots(o)
			if err != nil {
				t.Fatal(err)
			}
			f, err := r.source.OpenFile(o.StorageID, os.O_RDONLY|evidenceOpenFlags(), 0)
			if err != nil {
				t.Fatal(err)
			}
			identity, err := retentionExclusiveLock(context.Background(), f)
			if err != nil {
				t.Fatal(err)
			}
			observed, err := retentionObserve(context.Background(), f, o)
			if err != nil {
				t.Fatal(err)
			}
			saved, err := s.store.AuthorizeRetentionObject(context.Background(), o.ID, observed)
			if err != nil || saved.Identity != identity || saved.QuarantineSlot == "" {
				t.Fatal("授权未绑定真实出生身份/槽", err)
			}
			if gap != "authorized" {
				if err = retentionRenameExclusive(r.source, r.quarantine, o.StorageID, o.ObjectID); err != nil {
					t.Fatal(err)
				}
				if err = retentionSyncDirectory(r.source); err != nil {
					t.Fatal(err)
				}
				if err = retentionSyncDirectory(r.quarantine); err != nil {
					t.Fatal(err)
				}
			}
			if gap == "quarantined" || gap == "unlinked" {
				if err = s.store.ConfirmRetentionObject(context.Background(), o.ID, store.RetentionObjectResult{State: "quarantined", Identity: identity}); err != nil {
					t.Fatal(err)
				}
			}
			if gap == "unlinked" {
				if err = r.quarantine.Remove(o.ObjectID); err != nil {
					t.Fatal(err)
				}
				if err = retentionSyncDirectory(r.quarantine); err != nil {
					t.Fatal(err)
				}
			}
			if err = f.Close(); err != nil {
				t.Fatal(err)
			}
			r.close()
			before, err := s.store.ListRetention(context.Background(), actor, project, store.Page{})
			if err != nil {
				t.Fatal(err)
			}
			if err = s.advanceCentralRetention(context.Background(), project, 100); err != nil {
				t.Fatal("实际间隙不能恢复", gap, err)
			}
			after, err := s.store.ListRetention(context.Background(), actor, project, store.Page{})
			if err != nil || len(after.Items) != 1 || after.Items[0].JobID != before.Items[0].JobID || after.Items[0].CentralState != "completed" || after.Items[0].NodeState != "pending" {
				t.Fatal("恢复重造任务或错误宣称节点已清理", err, after)
			}
		})
	}
}

func TestRetentionCleanupSharedLockAndCurrentPolicy(t *testing.T) {
	t.Run("真实SH", func(t *testing.T) {
		s, _, project, o := retentionCleanupFixture(t, "artifact")
		f := retentionTestOpen(t, retentionCleanupPath(s, o))
		if _, err := retentionSharedLock(context.Background(), f); err != nil {
			t.Fatal(err)
		}
		if err := s.advanceCentralRetention(context.Background(), project, 100); !errors.Is(err, store.ErrRetentionReadersActive) {
			t.Fatal("SH未阻止物理迁移", err)
		}
		if _, err := os.Lstat(retentionCleanupPath(s, o)); err != nil {
			t.Fatal("SH期间原路径被修改", err)
		}
		rows, err := s.store.AdvanceRetention(context.Background(), project, 100)
		if err != nil || len(rows) != 1 || rows[0].State != "retired" || rows[0].Reason != "retention_readers_active" || rows[0].Identity != "" || rows[0].QuarantineSlot != "" {
			t.Fatal("初次SH等待未保存真实安全原因", err, rows)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		if err := s.advanceCentralRetention(context.Background(), project, 100); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("最新策略", func(t *testing.T) {
		s, _, project, o := retentionCleanupFixture(t, "artifact")
		if err := s.store.SyncGlobalRetention(context.Background(), config.Retention{Builds: 100, Days: 30}); err != nil {
			t.Fatal(err)
		}
		if err := s.cleanCentralRetentionObject(context.Background(), o); !errors.Is(err, store.ErrRetentionProtected) {
			t.Fatal("陈旧候选绕过策略", err)
		}
		if _, err := os.Lstat(retentionCleanupPath(s, o)); err != nil {
			t.Fatal("新保护后原路径被修改", err)
		}
		if err := s.store.SyncGlobalRetention(context.Background(), config.Retention{Builds: 1, Days: 30}); err != nil {
			t.Fatal(err)
		}
		if err := s.advanceCentralRetention(context.Background(), project, 100); err != nil {
			t.Fatal(err)
		}
	})
}

func TestRetentionCleanupUnsafeLeavesAndRoots(t *testing.T) {
	for _, change := range []string{"sha", "mode", "symlink", "fifo", "hardlink", "directory", "foreign-target", "source-parent-symlink", "quarantine-parent-symlink"} {
		t.Run(change, func(t *testing.T) {
			s, _, project, o := retentionCleanupFixture(t, "artifact")
			path := retentionCleanupPath(s, o)
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			control := filepath.Join(t.TempDir(), "FOREIGN_CONTROL")
			if err = os.WriteFile(control, original, 0600); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "sha":
				if err = os.WriteFile(path, []byte(strings.Repeat("x", len(original))), 0600); err != nil {
					t.Fatal(err)
				}
			case "mode":
				if err = os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err = os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err = os.Symlink(control, path); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				if err = os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err = unix.Mkfifo(path, 0600); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err = os.Link(path, path+"-foreign"); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err = os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err = os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "foreign-target":
				dir := filepath.Join(s.config.DataDir, "retention", "quarantine", o.JobID)
				if err = os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(filepath.Join(dir, o.ObjectID), []byte("unknown"), 0600); err != nil {
					t.Fatal(err)
				}
			case "source-parent-symlink":
				parent := filepath.Dir(path)
				if err = os.Rename(parent, parent+"-original"); err != nil {
					t.Fatal(err)
				}
				if err = os.Symlink(parent+"-original", parent); err != nil {
					t.Fatal(err)
				}
			case "quarantine-parent-symlink":
				if err = os.Symlink(t.TempDir(), filepath.Join(s.config.DataDir, "retention")); err != nil {
					t.Fatal(err)
				}
			}
			started := time.Now()
			if err = s.advanceCentralRetention(context.Background(), project, 100); !errors.Is(err, store.ErrRetentionOwnershipUnknown) {
				t.Fatal("unsafe未拒绝或错误泄漏", change, err)
			}
			if time.Since(started) > time.Second {
				t.Fatal("unsafe叶打开阻塞")
			}
			got, err := os.ReadFile(control)
			if err != nil || string(got) != string(original) {
				t.Fatal("外部未知文件被修改", err)
			}
			if change == "foreign-target" {
				got, err = os.ReadFile(filepath.Join(s.config.DataDir, "retention", "quarantine", o.JobID, o.ObjectID))
				if err != nil || string(got) != "unknown" {
					t.Fatal("陌生隔离槽被覆盖", err)
				}
			}
		})
	}
}

func TestRetentionCleanupOriginalBirthAndExclusiveRename(t *testing.T) {
	t.Run("替换同内容叶", func(t *testing.T) {
		s, _, project, o := retentionCleanupFixture(t, "artifact")
		r, err := s.retentionCleanupRoots(o)
		if err != nil {
			t.Fatal(err)
		}
		defer r.close()
		f := retentionTestOpen(t, retentionCleanupPath(s, o))
		if _, err = retentionExclusiveLock(context.Background(), f); err != nil {
			t.Fatal(err)
		}
		observed, err := retentionObserve(context.Background(), f, o)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.store.AuthorizeRetentionObject(context.Background(), o.ID, observed); err != nil {
			t.Fatal(err)
		}
		bytes, err := os.ReadFile(retentionCleanupPath(s, o))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.Remove(retentionCleanupPath(s, o)); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(retentionCleanupPath(s, o), bytes, 0600); err != nil {
			t.Fatal(err)
		}
		if err = s.advanceCentralRetention(context.Background(), project, 100); !errors.Is(err, store.ErrRetentionOwnershipUnknown) {
			t.Fatal("内容相同但出生不同仍删除", err)
		}
		if _, err = os.Stat(retentionCleanupPath(s, o)); err != nil {
			t.Fatal("新叶被删除", err)
		}
	})
	t.Run("原生EXCL拒覆盖", func(t *testing.T) {
		s, _, _, o := retentionCleanupFixture(t, "artifact")
		r, err := s.retentionCleanupRoots(o)
		if err != nil {
			t.Fatal(err)
		}
		defer r.close()
		target, err := r.quarantine.OpenFile(o.ObjectID, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = target.Write([]byte("foreign")); err != nil {
			t.Fatal(err)
		}
		if err = target.Close(); err != nil {
			t.Fatal(err)
		}
		if err = retentionRenameExclusive(r.source, r.quarantine, o.StorageID, o.ObjectID); !errors.Is(err, store.ErrRetentionOwnershipUnknown) {
			t.Fatal("原生覆盖目标", err)
		}
		got, err := os.ReadFile(filepath.Join(s.config.DataDir, "retention", "quarantine", o.JobID, o.ObjectID))
		if err != nil || string(got) != "foreign" {
			t.Fatal("原生拒绝后目标改变", err)
		}
		if _, err = r.source.Lstat(o.StorageID); err != nil {
			t.Fatal("原生拒绝后源消失", err)
		}
	})
	t.Run("已持目录根被替换", func(t *testing.T) {
		s, _, _, o := retentionCleanupFixture(t, "artifact")
		r, err := s.retentionCleanupRoots(o)
		if err != nil {
			t.Fatal(err)
		}
		defer r.close()
		path := r.quarantine.Name()
		if err = os.Rename(path, path+"-original"); err != nil {
			t.Fatal(err)
		}
		if err = os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
		if err = retentionRenameExclusive(r.source, r.quarantine, o.StorageID, o.ObjectID); !errors.Is(err, store.ErrRetentionOwnershipUnknown) {
			t.Fatal("已替换目录根仍迁移", err)
		}
		if _, err = r.source.Lstat(o.StorageID); err != nil {
			t.Fatal("未知根情况下源被修改", err)
		}
	})
}

func TestRetentionCleanupSQLConfirmationFailureKeepsExactIntent(t *testing.T) {
	for _, state := range []string{"quarantined", "deleted"} {
		t.Run(state, func(t *testing.T) {
			s, actor, project, o := retentionCleanupFixture(t, "artifact")
			db, err := sql.Open("sqlite", filepath.Join(s.config.DataDir, "control.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			// 只注入自有DB的真实确认事务失败，不手填/更新对象状态。
			trigger := "owned_retention_confirmation_fault"
			if _, err = db.Exec("CREATE TRIGGER " + trigger + " BEFORE UPDATE OF state ON retention_objects WHEN NEW.state='" + state + "' BEGIN SELECT json_extract('OWNED_INVALID_JSON', '$'); END"); err != nil {
				t.Fatal(err)
			}
			if err = s.advanceCentralRetention(context.Background(), project, 100); err == nil || err.Error() != "database_error" {
				t.Fatal("真实SQL确认失败未保留固定错误", err)
			}
			rows, err := s.store.AdvanceRetention(context.Background(), project, 100)
			if err != nil || len(rows) != 1 || rows[0].ID != o.ID || rows[0].JobID != o.JobID || rows[0].Identity == "" || rows[0].QuarantineSlot == "" {
				t.Fatal("失败后原意图/身份丢失", err, rows)
			}
			target := filepath.Join(s.config.DataDir, rows[0].QuarantineSlot)
			if state == "quarantined" {
				if _, err = os.Lstat(target); err != nil {
					t.Fatal("隔离确认失败丢失原文件", err)
				}
			} else {
				if rows[0].State != "quarantined" {
					t.Fatal("unlink确认失败不能丢原已确认隔离", rows[0].State)
				}
				if _, err = os.Lstat(target); !os.IsNotExist(err) {
					t.Fatal("真实unlink未发生", err)
				}
			}
			if _, err = db.Exec("DROP TRIGGER " + trigger); err != nil {
				t.Fatal(err)
			}
			if err = s.advanceCentralRetention(context.Background(), project, 100); err != nil {
				t.Fatal("同意图恢复失败", err)
			}
			jobs, err := s.store.ListRetention(context.Background(), actor, project, store.Page{})
			if err != nil || len(jobs.Items) != 1 || jobs.Items[0].JobID != o.JobID || jobs.Items[0].CentralState != "completed" {
				t.Fatal("恢复未沿原任务完成", err, jobs)
			}
		})
	}
}

func TestRetentionCleanupCanceledAndInvalidLimit(t *testing.T) {
	s, _, project, o := retentionCleanupFixture(t, "artifact")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.advanceCentralRetention(ctx, project, 100); err == nil {
		t.Fatal("已取消仍成功")
	}
	if _, err := os.Lstat(retentionCleanupPath(s, o)); err != nil {
		t.Fatal("取消仍删除", err)
	}
	if err := s.advanceCentralRetention(context.Background(), project, 101); !errors.Is(err, store.ErrRetentionLimit) {
		t.Fatal("单轮超100未拒", err)
	}
	if _, err := os.Lstat(retentionCleanupPath(s, o)); err != nil {
		t.Fatal("超限仍删除", err)
	}
}
