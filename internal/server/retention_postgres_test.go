//go:build darwin || linux

package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
)

// 每个用例仅使用自有数据库内的新schema；同库控制锁意味着这些用例必须串行。
func retentionPostgresFixture(t *testing.T, kind string) (*Server, *httptest.Server, *sql.DB, store.Options, store.Actor, string, string, string, string, []byte) {
	t.Helper()
	dsn := os.Getenv("MYBUILDS_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("未提供自有PostgreSQL DSN，本次未验证PG真实文件门")
	}
	ctx := context.Background()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal("自有PG夹具连接失败")
	}
	schema := "retention_server_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = db.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		db.Close()
		t.Fatal("自有schema创建失败")
	}
	t.Cleanup(func() {
		if _, e := db.Exec("DROP SCHEMA " + schema + " CASCADE"); e != nil {
			t.Error("自有schema清理失败")
		}
		if e := db.Close(); e != nil {
			t.Error("自有诊断连接关闭失败")
		}
	})
	opt := store.Options{Driver: "postgres", DSN: dsn + " search_path=" + schema + " application_name=" + schema}
	st, err := store.Open(ctx, opt)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err = st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err = st.Bootstrap(ctx, adminToken); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err = os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	s := New(st, config.ServerConfig{Retention: config.Retention{Builds: 100, Days: 30}, DataDir: dir, Concurrency: 3, HeartbeatInterval: 5 * time.Second, LeaseDuration: 30 * time.Second})
	if s.evidenceReadError != nil {
		t.Fatal(s.evidenceReadError)
	}
	if err = st.SyncGlobalRetention(ctx, s.config.Retention); err != nil {
		t.Fatal(err)
	}
	h := httptest.NewServer(s.Handler())
	t.Cleanup(h.Close)
	a, err := st.Authenticate(ctx, adminToken)
	if err != nil {
		t.Fatal(err)
	}
	node, err := st.CreateNode(ctx, a, store.NodeInput{Name: "worker", Capacity: 1})
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.CreateProject(ctx, a, store.ProjectInput{Name: "app", Repository: "https://example.org/repo.git", AllowedNodes: []string{"worker"}, DefaultNode: "worker"})
	if err != nil {
		t.Fatal(err)
	}
	step := config.Step{Name: "collect", Kind: "artifact", Paths: []string{"out/*"}}
	definition := config.Build{Steps: []config.Step{step}}
	if kind == "junit" {
		step = config.Step{Name: "tests", Kind: "run", Run: "printf test"}
		definition = config.Build{Steps: []config.Step{step}, Reports: &config.Reports{JUnit: &config.JUnitReport{Paths: []string{"results/*.xml"}}}}
	}
	input := store.EnqueueInput{Actor: a, ProjectID: p.ID, ProjectVersion: p.PolicyVersion, Key: "pg-original", RequestDigest: strings.Repeat("a", 64), SHA: strings.Repeat("b", 40), Branch: "main", Source: "repo", File: "mybuilds.yml", SourceDigest: strings.Repeat("c", 64), Builds: []store.PreparedBuild{{Name: "compile", Status: "queued", PostBudgetNS: int64(2 * time.Minute), Snapshot: store.BuildSnapshot{Definition: definition, Params: map[string]string{}, Facts: map[string]string{}, Condition: "ready", AllowedNodes: p.AllowedNodes, DefaultNode: p.DefaultNode}, Steps: []store.StepProgress{{Phase: "ordinary", Index: 1, Name: step.Name, Kind: step.Kind, Condition: "ready", Status: "pending"}}}}}
	if _, err = st.Enqueue(ctx, input); err != nil {
		t.Fatal(err)
	}
	session := protocol.SessionRequest{SessionID: uuid.NewString(), HeartbeatNS: int64(5 * time.Second), LeaseNS: int64(30 * time.Second), Report: protocol.NodeReport{OS: "linux", Arch: "amd64", Capacity: 1, Tools: []protocol.ToolCheck{{Name: "shell", Status: "passed"}, {Name: "git", Status: "passed"}, {Name: "node_journal", Status: "passed"}}}}
	if code, out := request(t, h, "POST", "/api/agent/session", node.Token, encodeMessage(t, session)); code != 200 {
		t.Fatal(code, out)
	}
	g := claimHTTP(t, h, node.Token, protocol.ClaimRequest{SessionID: session.SessionID, ClaimKey: uuid.NewString()})
	data := []byte("actual PG artifact bytes\x00\xff")
	if kind == "junit" {
		data = []byte(`<testsuite tests="1"><testcase name="pg-real-read"/></testsuite>`)
	}
	hash := sha256.Sum256(data)
	d := protocol.ArtifactDeclaration{Ref: g.Ref, ID: uuid.NewString(), Seq: 1, Phase: "ordinary", Step: step.Name, Index: 1, Name: "app.apk", Size: int64(len(data)), SHA256: hex.EncodeToString(hash[:])}
	for i, k := range []string{"intent", "started", "finished"} {
		progress := protocol.ExecutionProgress{Kind: k, Phase: "ordinary", Name: step.Name, StepKind: step.Kind, Index: 1, At: time.Now().UTC(), ExitCode: -1, RemainingPostBudgetNS: g.RemainingPostBudgetNS, ArtifactSteps: []protocol.ArtifactExpectation{}}
		if k != "intent" {
			progress.Started = true
		}
		if k == "finished" {
			progress.Status = "succeeded"
			progress.StopConfirmed = true
			progress.ExitCode = 0
			if kind != "junit" {
				progress.ArtifactIDs = []string{d.ID}
			}
		}
		postReportEvent(t, h, node.Token, g.Ref, int64(i+1), progress, 200)
	}
	var evidence protocol.ReportEvidence
	if kind == "junit" {
		pathKey := sha256.Sum256([]byte("results/result.xml"))
		d.Name, d.Purpose, d.ReportRevision, d.ReportKey = "result.xml", "junit", 2, hex.EncodeToString(pathKey[:])
		counts := protocol.JUnitCounts{Tests: 1}
		evidence = protocol.ReportEvidence{Revision: 1, Outcome: "pending", Required: true, Counts: counts, Diagnostics: []protocol.JUnitDiagnostic{}, Files: []protocol.ReportFile{{Key: d.ReportKey, Path: "results/result.xml", ArtifactID: d.ID, SourceIndex: 1, SourceStep: step.Name, Size: d.Size, SHA256: d.SHA256, Counts: counts}}}
		progress := protocol.ExecutionProgress{Kind: "reports_checked", Phase: "ordinary", Name: step.Name, StepKind: "run", Index: 1, At: time.Now().UTC(), ExitCode: -1, RemainingPostBudgetNS: g.RemainingPostBudgetNS, ArtifactSteps: []protocol.ArtifactExpectation{}, Reports: &evidence}
		postReportEvent(t, h, node.Token, g.Ref, 4, progress, 200)
		evidence.Revision, evidence.Outcome = 2, "passed"
		progress.Phase, progress.Name, progress.StepKind, progress.Index, progress.At = "", "", "", 0, time.Now().UTC()
		postReportEvent(t, h, node.Token, g.Ref, 5, progress, 200)
	}
	if code, out := uploadArtifactHTTP(t, h, node.Token, d, data); code != 200 {
		t.Fatal(code, string(out))
	}
	registration := protocol.NodeResourceRegistration{Ref: g.Ref, ID: uuid.NewString(), OwnershipDigest: strings.Repeat("a", 64), HasWorkspace: true, HasResults: true}
	if code, out := request(t, h, "POST", "/api/agent/resources", node.Token, encodeMessage(t, registration)); code != 204 {
		t.Fatal(code, out)
	}
	var logSeq, logOffset int64
	if kind == "log" {
		chunk := logHTTPMessage(t, g.Ref)
		chunk.Records[0].Step, chunk.Records[0].Stream = "collect", "system"
		chunk.Digest = messageDigest(t, chunk.Records)
		if code, out := request(t, h, "POST", "/api/agent/logs", node.Token, encodeMessage(t, chunk)); code != 200 {
			t.Fatal(code, out)
		}
		logSeq, data = 1, []byte(encodeMessage(t, chunk.Records))
		logOffset = int64(len(data))
	}
	seq := int64(4)
	expected := []protocol.ArtifactExpectation{{Phase: "ordinary", Index: 1, Count: 1, IDs: []string{d.ID}}}
	var manifest *protocol.ReportManifest
	if kind == "junit" {
		manifest = &protocol.ReportManifest{SealDigest: sealReportHTTP(t, h, node.Token, d, evidence, 200), IDs: []string{d.ID}}
		seq = 7
		expected = []protocol.ArtifactExpectation{}
	}
	postReportEvent(t, h, node.Token, g.Ref, seq, protocol.ExecutionProgress{Kind: "post_selected", PostPhase: "success", At: time.Now().UTC(), RemainingPostBudgetNS: g.RemainingPostBudgetNS, ArtifactSteps: []protocol.ArtifactExpectation{}}, 200)
	postReportEvent(t, h, node.Token, g.Ref, seq+1, protocol.ExecutionProgress{Kind: "build_finished", Status: "succeeded", Started: true, StopConfirmed: true, At: time.Now().UTC(), RemainingPostBudgetNS: g.RemainingPostBudgetNS, LastArtifactSeq: 1, LastLogSeq: logSeq, LastLogOffset: logOffset, ArtifactSteps: expected, ReportManifest: manifest}, 200)
	newer := store.PreparedBuild{Name: "newer-read", Status: "skipped", Snapshot: store.BuildSnapshot{Definition: config.Build{Steps: []config.Step{{Name: "shell", Kind: "run", Run: "true"}}}, Condition: "skipped", Params: map[string]string{}, Facts: map[string]string{}}, Steps: []store.StepProgress{{Phase: "ordinary", Index: 1, Name: "shell", Kind: "run", Condition: "ready", Status: "pending"}}}
	input.Key, input.RequestDigest, input.Builds = "pg-newer", strings.Repeat("d", 64), []store.PreparedBuild{newer}
	if _, err = st.Enqueue(ctx, input); err != nil {
		t.Fatal(err)
	}
	if err = st.SyncGlobalRetention(ctx, config.Retention{Builds: 1, Days: 30}); err != nil {
		t.Fatal(err)
	}
	id, url, path := d.ID, "/api/artifacts/"+d.ID+"/"+d.Name, ""
	if kind == "log" {
		rows, e := st.ListLogChunks(ctx, a, g.Ref.BuildID, 0, store.Page{})
		if e != nil || len(rows) != 1 {
			t.Fatal(e, len(rows))
		}
		id = rows[0].ID
		path = filepath.Join(dir, "logs", rows[0].StorageID)
		url = "/api/builds/" + g.Ref.BuildID + "/log?follow=1"
	} else {
		file, e := st.GetArtifact(ctx, a, id)
		if e != nil {
			t.Fatal(e)
		}
		path = filepath.Join(dir, "artifacts", file.StorageID)
	}
	return s, h, db, opt, a, p.ID, id, url, path, data
}

func retentionPostgresSchema(t *testing.T, opt store.Options) string {
	t.Helper()
	for _, word := range strings.Fields(opt.DSN) {
		if strings.HasPrefix(word, "search_path=retention_server_") {
			return strings.TrimPrefix(word, "search_path=")
		}
	}
	t.Fatal("不是本夹具独立schema")
	return ""
}

func TestRetentionPostgresActualSocketReadRetireRace100(t *testing.T) {
	for _, kind := range []string{"artifact", "junit", "log"} {
		t.Run(kind, func(t *testing.T) {
			s, h, _, _, actor, pid, id, url, path, original := retentionPostgresFixture(t, kind)
			readKind := kind
			if kind == "junit" {
				readKind = "artifact"
			}
			held, err := s.openRetentionEvidence(context.Background(), actor, readKind, id)
			if err != nil {
				t.Fatal(err)
			}
			defer held.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			baseline, e := http.NewRequestWithContext(ctx, "GET", h.URL+url, nil)
			if e != nil {
				t.Fatal(e)
			}
			baseline.Header.Set("Authorization", "Bearer "+adminToken)
			res, e := h.Client().Do(baseline)
			if e != nil {
				t.Fatal(e)
			}
			baselineData, e := io.ReadAll(res.Body)
			res.Body.Close()
			if e != nil || res.StatusCode != 200 {
				t.Fatal("退役前实际TCP读取", res.StatusCode, e)
			}
			if kind == "log" {
				if !bytes.Contains(baselineData, []byte("event: log\n")) || !bytes.Contains(baselineData, []byte(`"next_seq":1`)) {
					t.Fatal("退役前真实SSE缺原chunk")
				}
			} else {
				digest := sha256.Sum256(baselineData)
				if !bytes.Equal(baselineData, original) || hex.EncodeToString(digest[:]) != res.Header.Get("X-Content-SHA256") {
					t.Fatal("退役前TCP字节或摘要不完整")
				}
			}
			type outcome struct {
				status int
				data   []byte
				digest string
				err    error
			}
			results, start := make(chan outcome, 100), make(chan struct{})
			var workers sync.WaitGroup
			for range 100 {
				workers.Add(1)
				go func() {
					defer workers.Done()
					<-start
					req, e := http.NewRequestWithContext(ctx, "GET", h.URL+url, nil)
					if e != nil {
						results <- outcome{err: e}
						return
					}
					req.Header.Set("Authorization", "Bearer "+adminToken)
					res, e := h.Client().Do(req)
					if e != nil {
						results <- outcome{err: e}
						return
					}
					data, e := io.ReadAll(res.Body)
					res.Body.Close()
					results <- outcome{status: res.StatusCode, data: data, digest: res.Header.Get("X-Content-SHA256"), err: e}
				}()
			}
			close(start)
			if _, err = s.store.ScheduleRetention(ctx, actor, pid, 100); err != nil {
				cancel()
				workers.Wait()
				t.Fatal(err)
			}
			if err = s.advanceCentralRetention(ctx, pid, 100); !errors.Is(err, store.ErrRetentionReadersActive) {
				cancel()
				workers.Wait()
				t.Fatal("真实held SH必须挡EX", err)
			}
			data, e := os.ReadFile(path)
			if e != nil || !bytes.Equal(data, original) {
				cancel()
				workers.Wait()
				t.Fatal("活动原文件变更", e)
			}
			workers.Wait()
			close(results)
			succeeded, retired := 0, 0
			for result := range results {
				if result.err != nil {
					t.Fatal(result.err)
				}
				if result.status == 410 {
					retired++
					if !bytes.Contains(result.data, []byte("retention_retired")) {
						t.Fatal("非固定退役错误")
					}
					continue
				}
				if result.status != 200 {
					t.Fatal("读取竞争错误", result.status)
				}
				succeeded++
				if kind == "log" {
					if !bytes.Contains(result.data, []byte("event: log\n")) || !bytes.Contains(result.data, []byte(`"next_seq":1`)) {
						t.Fatal("SSE未完成原chunk")
					}
				} else {
					digest := sha256.Sum256(result.data)
					if !bytes.Equal(result.data, original) || hex.EncodeToString(digest[:]) != result.digest {
						t.Fatal("非完整原字节/SHA")
					}
				}
			}
			t.Logf("真实TCP竞争：200=%d，410=%d，退役前完整200=1", succeeded, retired)
			if err = held.Close(); err != nil {
				t.Fatal(err)
			}
			if err = s.advanceCentralRetention(ctx, pid, 100); err != nil {
				t.Fatal(err)
			}
			if _, err = os.Lstat(path); !os.IsNotExist(err) {
				t.Fatal("原文件未真unlink", err)
			}
			for range 3 {
				req, e := http.NewRequestWithContext(ctx, "GET", h.URL+url, nil)
				if e != nil {
					t.Fatal(e)
				}
				req.Header.Set("Authorization", "Bearer "+adminToken)
				res, e := h.Client().Do(req)
				if e != nil {
					t.Fatal(e)
				}
				data, e = io.ReadAll(res.Body)
				res.Body.Close()
				if e != nil || res.StatusCode != 410 || !bytes.Contains(data, []byte("retention_retired")) {
					t.Fatal("退役后新读", res.StatusCode, e)
				}
			}
		})
	}
}

func TestRetentionPostgresReadOwnerReopenRequiresOriginalFDReleased(t *testing.T) {
	s, h, db, opt, actor, pid, id, _, path, original := retentionPostgresFixture(t, "artifact")
	held, err := s.openRetentionEvidence(context.Background(), actor, "artifact", id)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	if other, e := store.Open(context.Background(), opt); e != store.ErrLocked || other != nil {
		if other != nil {
			other.Close()
		}
		t.Fatal("持控制独占时第二Store未拒", e)
	}
	if _, err = s.store.ScheduleRetention(context.Background(), actor, pid, 100); err != nil {
		t.Fatal(err)
	}
	h.Close()
	if err = s.store.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := store.Open(context.Background(), opt)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	current := New(next, s.config)
	if current.evidenceReadError != nil || current.evidenceReadOwner == s.evidenceReadOwner {
		t.Fatal("新控制实例未换owner", current.evidenceReadError)
	}
	if err = current.advanceCentralRetention(context.Background(), pid, 100); !errors.Is(err, store.ErrRetentionReadersActive) {
		t.Fatal("新owner错误代替原同fd EX", err)
	}
	data, err := io.ReadAll(held.File)
	if err != nil || !bytes.Equal(data, original) {
		t.Fatal("原fd字节变更", err)
	}
	originalIdentity, err := retentionFileIdentity(held.File)
	if err != nil {
		t.Fatal(err)
	}
	if err = held.Close(); err == nil {
		t.Fatal("旧Store关闭后假清登记")
	}
	schema := retentionPostgresSchema(t, opt)
	var state, owner, identity string
	if err = db.QueryRow("SELECT state,owner,identity FROM "+schema+".evidence_reads WHERE id=$1", held.Read.ID).Scan(&state, &owner, &identity); err != nil || state != "active" || owner != s.evidenceReadOwner || identity != originalIdentity {
		t.Fatal("旧active登记提前清", err)
	}
	if err = current.advanceCentralRetention(context.Background(), pid, 100); err != nil {
		t.Fatal("同inode实际EX恢复", err)
	}
	if err = db.QueryRow("SELECT state,owner FROM "+schema+".evidence_reads WHERE id=$1", held.Read.ID).Scan(&state, &owner); err != nil || state != "closed" || owner != s.evidenceReadOwner {
		t.Fatal("原owner绑定登记未闭合", err)
	}
	if _, err = os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("恢复未真unlink", err)
	}
}

func TestRetentionPostgresControlBackendLostStopsNewFileIO(t *testing.T) {
	s, h, db, opt, actor, pid, id, url, path, original := retentionPostgresFixture(t, "artifact")
	ctx := context.Background()
	if other, err := store.Open(ctx, opt); err != store.ErrLocked || other != nil {
		if other != nil {
			other.Close()
		}
		t.Fatal("原控制PID存活时第二Store未拒", err)
	}
	if _, err := s.store.ScheduleRetention(ctx, actor, pid, 100); err != nil {
		t.Fatal(err)
	}
	// 精确观测本Open持有的唯一session advisory锁，不按用户、库名通配或扫描结果盲杀。
	if err := s.store.CheckLock(ctx); err != nil {
		t.Fatal(err)
	}
	schema := retentionPostgresSchema(t, opt)
	query := `SELECT activity.pid FROM pg_stat_activity activity JOIN pg_locks locks ON locks.pid=activity.pid WHERE activity.application_name=$1 AND activity.datname=current_database() AND locks.locktype='advisory' AND locks.classid=1973481521 AND locks.objid=6 AND locks.objsubid=2 AND locks.granted`
	rows, err := db.QueryContext(ctx, query, schema)
	if err != nil {
		t.Fatal("本控制backend观测失败")
	}
	var pids []int
	for rows.Next() {
		var p int
		if err = rows.Scan(&p); err != nil {
			rows.Close()
			t.Fatal("本控制backend读取失败")
		}
		pids = append(pids, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil || len(pids) != 1 || pids[0] <= 0 {
		t.Fatal("不是本Store唯一advisory backend")
	}
	if err = s.store.CheckLock(ctx); err != nil {
		t.Fatal(err)
	}
	var terminated bool
	if err = db.QueryRowContext(ctx, "SELECT pg_terminate_backend($1)", pids[0]).Scan(&terminated); err != nil || !terminated {
		t.Fatal("本控制backend未终止")
	}
	if err = s.store.CheckLock(ctx); err != store.ErrLockLost {
		t.Fatal("原控制失锁未闭锁", err)
	}
	if read, e := s.openRetentionEvidence(ctx, actor, "artifact", id); e != store.ErrLockLost || read != nil {
		if read != nil {
			read.Close()
		}
		t.Fatal("失锁后新读开始IO", e)
	}
	if err = s.advanceCentralRetention(ctx, pid, 100); err != store.ErrLockLost {
		t.Fatal("失锁后删除未停止", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, original) {
		t.Fatal("失锁后原文件变更", err)
	}
	if _, err = os.Lstat(filepath.Join(s.config.DataDir, "retention")); !os.IsNotExist(err) {
		t.Fatal("失锁后新建了清理目录", err)
	}
	if code, body := request(t, h, "GET", url, adminToken, ""); code != 503 || !strings.Contains(body, "control_lock_lost") {
		t.Fatal("失锁HTTP未停止", code)
	}
	// 原conn锁已真消失后新Store可独占，旧Store即使新owner退出也不能复活。
	next, err := store.Open(ctx, opt)
	if err != nil {
		t.Fatal("失锁后独立新控制不可打开", err)
	}
	if err = next.CheckLock(ctx); err != nil {
		next.Close()
		t.Fatal(err)
	}
	if err = next.Close(); err != nil {
		t.Fatal(err)
	}
	if err = s.store.CheckLock(ctx); err != store.ErrLockLost {
		t.Fatal("旧控制实例复活", err)
	}
}

func TestRetentionPostgresSQLConfirmFaultRecoversExactPhysicalObject(t *testing.T) {
	for _, target := range []string{"quarantined", "deleted"} {
		t.Run(target, func(t *testing.T) {
			s, _, db, opt, actor, pid, _, _, path, original := retentionPostgresFixture(t, "artifact")
			ctx := context.Background()
			if _, err := s.store.ScheduleRetention(ctx, actor, pid, 100); err != nil {
				t.Fatal(err)
			}
			objects, err := s.store.AdvanceRetention(ctx, pid, 100)
			if err != nil || len(objects) != 1 {
				t.Fatal(err, len(objects))
			}
			o := objects[0]
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			identity, err := retentionFileIdentity(f)
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
			schema := retentionPostgresSchema(t, opt)
			if _, err = db.Exec(`CREATE FUNCTION ` + schema + `.reject_confirm() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.state='` + target + `' THEN RAISE EXCEPTION 'private confirm fault'; END IF; RETURN NEW; END $$`); err != nil {
				t.Fatal("本schema故障function创建失败")
			}
			if _, err = db.Exec(`CREATE TRIGGER reject_confirm BEFORE UPDATE OF state ON ` + schema + `.retention_objects FOR EACH ROW EXECUTE FUNCTION ` + schema + `.reject_confirm()`); err != nil {
				t.Fatal("本schema故障trigger创建失败")
			}
			if err = s.cleanCentralRetentionObject(ctx, o); err == nil || strings.Contains(err.Error(), "private confirm fault") {
				t.Fatal("SQL确认故障假成功或泄漏", err)
			}
			remaining, err := s.store.AdvanceRetention(ctx, pid, 100)
			if err != nil || len(remaining) != 1 || remaining[0].ID != o.ID || remaining[0].StorageID != o.StorageID || remaining[0].Identity != identity {
				t.Fatal("实际故障换ID/身份或丢事项", err)
			}
			held := remaining[0]
			quarantine := filepath.Join(s.config.DataDir, "retention", "quarantine", o.JobID, o.ObjectID)
			if _, err = os.Lstat(path); !os.IsNotExist(err) {
				t.Fatal("原叶没有实际隔离", err)
			}
			if target == "quarantined" {
				data, e := os.ReadFile(quarantine)
				if e != nil || !bytes.Equal(data, original) || held.State != "failed" {
					t.Fatal("隔离确认失败未保留真原叶", e, held.State)
				}
			} else {
				if _, err = os.Lstat(quarantine); !os.IsNotExist(err) || held.State != "quarantined" {
					t.Fatal("unlink后确认失败未保留真隔离凭据", err, held.State)
				}
			}
			if _, err = db.Exec("DROP TRIGGER reject_confirm ON " + schema + ".retention_objects"); err != nil {
				t.Fatal("本schema故障移除失败")
			}
			if err = s.advanceCentralRetention(ctx, pid, 100); err != nil {
				t.Fatal("撤故障后原ID恢复失败", err)
			}
			for _, name := range []string{path, quarantine} {
				if _, err = os.Lstat(name); !os.IsNotExist(err) {
					t.Fatal("恢复未真unlink", err)
				}
			}
			var state, keptIdentity, storageID string
			var confirmedAt, quarantinedAt sql.NullTime
			if err = db.QueryRow("SELECT state,identity,storage_id,confirmed_at,quarantined_at FROM "+schema+".retention_objects WHERE id=$1", o.ID).Scan(&state, &keptIdentity, &storageID, &confirmedAt, &quarantinedAt); err != nil || state != "deleted" || keptIdentity != identity || storageID != o.StorageID || !confirmedAt.Valid || !quarantinedAt.Valid || confirmedAt.Time.Before(quarantinedAt.Time) {
				t.Fatal("原事项完整确认未保存", err)
			}
		})
	}
}
