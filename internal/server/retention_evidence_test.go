//go:build darwin || linux

package server

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
	"mybuilds/internal/store"
)

func retentionEvidenceFixture(t *testing.T, kind string) (*Server, store.Actor, string, string, []byte) {
	t.Helper()
	if kind == "artifact" {
		s, st, h, token, d, data := artifactHTTPFixture(t)
		code, body := uploadArtifactHTTP(t, h, token, d, data)
		if code != 200 {
			t.Fatal(code, string(body))
		}
		actor, err := st.Authenticate(context.Background(), adminToken)
		if err != nil {
			t.Fatal(err)
		}
		stored, err := st.GetArtifact(context.Background(), actor, d.ID)
		if err != nil {
			t.Fatal(err)
		}
		return s, actor, d.ID, filepath.Join(s.config.DataDir, "artifacts", stored.StorageID), data
	}
	s, st, h, token, claim := executionHTTPFixture(t)
	grant := claimHTTP(t, h, token, claim)
	chunk := logHTTPMessage(t, grant.Ref)
	code, body := request(t, h, "POST", "/api/agent/logs", token, encodeMessage(t, chunk))
	if code != 200 {
		t.Fatal(code, body)
	}
	actor, err := st.Authenticate(context.Background(), adminToken)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := st.ListLogChunks(context.Background(), actor, grant.Ref.BuildID, 0, store.Page{})
	if err != nil || len(rows) != 1 || rows[0].ID == "" {
		t.Fatal(err)
	}
	return s, actor, rows[0].ID, filepath.Join(s.config.DataDir, "logs", rows[0].StorageID), []byte(encodeMessage(t, chunk.Records))
}

type retentionReadState struct{ state, identity, owner, closedAt string }

// 仅观察真实Begin/Activate/End写入结果，不自行创建或修改读登记。
func retentionEvidenceRows(t *testing.T, s *Server) map[string]retentionReadState {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(s.config.DataDir, "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query("SELECT id,state,identity,owner,COALESCE(CAST(closed_at AS TEXT),'') FROM evidence_reads ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	result := map[string]retentionReadState{}
	for rows.Next() {
		var id string
		var row retentionReadState
		if err = rows.Scan(&id, &row.state, &row.identity, &row.owner, &row.closedAt); err != nil {
			t.Fatal(err)
		}
		result[id] = row
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestRetentionEvidenceActualReadersAndClose(t *testing.T) {
	for _, kind := range []string{"log", "artifact"} {
		t.Run(kind, func(t *testing.T) {
			s, actor, id, path, expected := retentionEvidenceFixture(t, kind)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			first, err := s.openRetentionEvidence(ctx, actor, kind, id)
			if err != nil {
				t.Fatal(err)
			}
			second, err := s.openRetentionEvidence(ctx, actor, kind, id)
			if err != nil {
				t.Fatal(err)
			}
			defer first.Close()
			defer second.Close()
			if first.Read.ID == second.Read.ID || first.Read.StorageID == id || first.Read.ObjectID != id || first.Read.Kind != kind {
				t.Fatal("读取对象身份与StorageID混淆")
			}
			states := retentionEvidenceRows(t, s)
			for _, held := range []*retentionEvidence{first, second} {
				row := states[held.Read.ID]
				identity, err := retentionFileIdentity(held.File)
				if err != nil || row.state != "active" || row.identity != identity || row.owner != s.evidenceReadOwner {
					t.Fatal("持有SH的实际fd与active登记未绑定", err)
				}
			}
			bytes, err := io.ReadAll(first.File)
			if err != nil || string(bytes) != string(expected) {
				t.Fatal("实际字节不同", err)
			}
			exclusive := retentionTestOpen(t, path)
			if _, err = retentionExclusiveLock(context.Background(), exclusive); !errors.Is(err, store.ErrRetentionReadersActive) {
				t.Fatal("读者活跃期间取得EX", err)
			}
			cancel()
			if err = first.Close(); err != nil {
				t.Fatal("原ctx取消妨碍独立End", err)
			}
			if _, err = retentionExclusiveLock(context.Background(), exclusive); !errors.Is(err, store.ErrRetentionReadersActive) {
				t.Fatal("另一个真实SH仍应保护", err)
			}
			if err = second.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err = retentionExclusiveLock(context.Background(), exclusive); err != nil {
				t.Fatal("全部fd关闭后EX未释放", err)
			}
			for _, row := range retentionEvidenceRows(t, s) {
				if row.state != "closed" || row.closedAt == "" {
					t.Fatal("正常读取后遗留pending/active")
				}
			}
			if err = first.Close(); err != nil {
				t.Fatal("Close不幂等", err)
			}
		})
	}
}

func TestRetentionEvidenceFailedOpenEndsPending(t *testing.T) {
	for _, kind := range []string{"log", "artifact"} {
		t.Run(kind, func(t *testing.T) {
			for _, change := range []string{"missing", "mode", "symlink", "fifo", "hardlink", "directory", "size"} {
				t.Run(change, func(t *testing.T) {
					s, actor, id, path, data := retentionEvidenceFixture(t, kind)
					switch change {
					case "missing":
						if err := os.Remove(path); err != nil {
							t.Fatal(err)
						}
					case "mode":
						if err := os.Chmod(path, 0644); err != nil {
							t.Fatal(err)
						}
					case "symlink":
						target := filepath.Join(t.TempDir(), "PRIVATE_EXTERNAL_CONTROL")
						if err := os.WriteFile(target, data, 0600); err != nil {
							t.Fatal(err)
						}
						if err := os.Remove(path); err != nil {
							t.Fatal(err)
						}
						if err := os.Symlink(target, path); err != nil {
							t.Fatal(err)
						}
					case "fifo":
						if err := os.Remove(path); err != nil {
							t.Fatal(err)
						}
						if err := unix.Mkfifo(path, 0600); err != nil {
							t.Fatal(err)
						}
					case "hardlink":
						if err := os.Link(path, path+"-alias"); err != nil {
							t.Fatal(err)
						}
					case "directory":
						if err := os.Remove(path); err != nil {
							t.Fatal(err)
						}
						if err := os.Mkdir(path, 0700); err != nil {
							t.Fatal(err)
						}
					case "size":
						if err := os.WriteFile(path, append(data, 'x'), 0600); err != nil {
							t.Fatal(err)
						}
					}
					start := time.Now()
					held, err := s.openRetentionEvidence(context.Background(), actor, kind, id)
					if held != nil || err == nil || err.Error() != store.ErrRetentionOwnershipUnknown.Error() {
						t.Fatal("不安全文件仍开放或错误泄漏", err)
					}
					if time.Since(start) > time.Second {
						t.Fatal("unsafe打开发生阻塞")
					}
					rows := retentionEvidenceRows(t, s)
					if len(rows) != 1 {
						t.Fatal("未经过真实Begin", len(rows))
					}
					for _, row := range rows {
						if row.state != "closed" || row.identity != "" || row.closedAt == "" {
							t.Fatal("打开失败遗留活跃登记")
						}
					}
				})
			}
		})
	}
}

func TestRetentionEvidenceExclusiveAndRoleBoundary(t *testing.T) {
	s, actor, id, path, _ := retentionEvidenceFixture(t, "artifact")
	exclusive := retentionTestOpen(t, path)
	if _, err := retentionExclusiveLock(context.Background(), exclusive); err != nil {
		t.Fatal(err)
	}
	held, err := s.openRetentionEvidence(context.Background(), actor, "artifact", id)
	if held != nil || !errors.Is(err, store.ErrRetentionReadersActive) {
		t.Fatal("实际EX未拒新SH", err)
	}
	for _, row := range retentionEvidenceRows(t, s) {
		if row.state != "closed" || row.identity != "" {
			t.Fatal("SH失败没有End真实pending")
		}
	}
	if err = exclusive.Close(); err != nil {
		t.Fatal(err)
	}
	bad, err := s.store.CreateToken(context.Background(), actor, "trigger")
	if err != nil {
		t.Fatal(err)
	}
	denied, err := s.store.Authenticate(context.Background(), bad.Token)
	if err != nil {
		t.Fatal(err)
	}
	count := len(retentionEvidenceRows(t, s))
	if _, err = s.openRetentionEvidence(context.Background(), denied, "artifact", id); !errors.Is(err, store.ErrForbidden) {
		t.Fatal("trigger读取登记", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = s.openRetentionEvidence(ctx, actor, "artifact", id); !errors.Is(err, store.ErrRetentionCancelled) {
		t.Fatal("取消请求仍开始读登记", err)
	}
	if len(retentionEvidenceRows(t, s)) != count {
		t.Fatal("鉴权/取消失败创建登记")
	}
	again := New(s.store, s.config)
	if again.evidenceReadOwner != s.evidenceReadOwner || again.evidenceReadError != nil {
		t.Fatal("同Store重复Server.New改owner")
	}
}

func TestRetentionEvidenceCloseFailureRetainsProtection(t *testing.T) {
	t.Run("bad-fd", func(t *testing.T) {
		s, actor, id, _, _ := retentionEvidenceFixture(t, "artifact")
		held, err := s.openRetentionEvidence(context.Background(), actor, "artifact", id)
		if err != nil {
			t.Fatal(err)
		}
		if err = held.File.Close(); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if err = held.Close(); !errors.Is(err, store.ErrRetentionIO) {
				t.Fatal("fd关闭错误假解除登记", err)
			}
			row := retentionEvidenceRows(t, s)[held.Read.ID]
			if row.state != "active" || row.closedAt != "" {
				t.Fatal("不确定关闭丢失保护")
			}
		}
	})
	t.Run("end-sql-error", func(t *testing.T) {
		s, actor, id, path, _ := retentionEvidenceFixture(t, "log")
		held, err := s.openRetentionEvidence(context.Background(), actor, "log", id)
		if err != nil {
			t.Fatal(err)
		}
		db, err := sql.Open("sqlite", filepath.Join(s.config.DataDir, "control.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		// 只在自有数据库增加测试触发器制造真实SQL失败，不伪造读登记或替换Store。
		_, err = db.Exec("CREATE TRIGGER owned_retention_end_fault BEFORE UPDATE OF state ON evidence_reads WHEN NEW.state='closed' BEGIN SELECT value FROM owned_absent_retention_fault; END")
		if err != nil {
			t.Fatal(err)
		}
		if err = held.Close(); err == nil {
			t.Fatal("实际End SQL失败被忽略")
		}
		row := retentionEvidenceRows(t, s)[held.Read.ID]
		if row.state != "active" || row.closedAt != "" {
			t.Fatal("失败End更新登记")
		}
		exclusive := retentionTestOpen(t, path)
		if _, err = retentionExclusiveLock(context.Background(), exclusive); err != nil {
			t.Fatal("End之前未关闭实际SH", err)
		}
		if _, err = db.Exec("DROP TRIGGER owned_retention_end_fault"); err != nil {
			t.Fatal(err)
		}
		if err = held.Close(); err != nil {
			t.Fatal("原登记不能在实际fd关闭后有限重试End", err)
		}
		row = retentionEvidenceRows(t, s)[held.Read.ID]
		if row.state != "closed" || row.closedAt == "" {
			t.Fatal("重试End未持久关闭")
		}
	})
}
