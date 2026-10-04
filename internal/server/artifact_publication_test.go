package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/glebarez/go-sqlite"
	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
)

// 实际第二连接在INSERT边界拒绝事务，不能把先发布的孤立文件当确认产物。
func TestActualArtifactPublicationBeforeFailedSQLCommitIsInvisible(t *testing.T) {
	s, st, api, token, declaration, data := artifactHTTPFixture(t)
	db, err := sql.Open("sqlite", filepath.Join(s.config.DataDir, "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	const trigger = "mybuilds_owned_artifact_commit_failure"
	if _, err = db.ExecContext(ctx, "CREATE TRIGGER "+trigger+" BEFORE INSERT ON artifacts BEGIN SELECT * FROM mybuilds_owned_missing_artifact_table; END"); err != nil {
		t.Fatal(err)
	}
	defer db.ExecContext(context.Background(), "DROP TRIGGER IF EXISTS "+trigger)
	code, body := uploadArtifactHTTP(t, api, token, declaration, data)
	if code != 500 || strings.Contains(string(body), "mybuilds_owned_missing_artifact_table") {
		t.Fatal("事务错误未安全拒绝", code, string(body))
	}
	directory := filepath.Join(s.config.DataDir, "artifacts")
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 || !canonicalUUID(entries[0].Name()) {
		t.Fatal("未实际排他发布或stage残留", entries, err)
	}
	orphan := filepath.Join(directory, entries[0].Name())
	owned, err := os.Lstat(orphan)
	if err != nil || !evidenceInfo(owned, false) {
		t.Fatal("孤立文件属性无效", err)
	}
	orphanData, err := os.ReadFile(orphan)
	if err != nil || !bytes.Equal(orphanData, data) {
		t.Fatal("孤立文件非实际完整数据", err)
	}
	admin, err := st.Authenticate(ctx, adminToken)
	if err != nil {
		t.Fatal(err)
	}
	items, err := st.ListArtifacts(ctx, admin, declaration.Ref.BuildID, store.Page{Limit: 200})
	if err != nil || len(items) != 0 {
		t.Fatal("失败事务产物可见", items, err)
	}
	var sequence int64
	if err = db.QueryRowContext(ctx, "SELECT last_artifact_seq FROM builds WHERE id=?", declaration.Ref.BuildID).Scan(&sequence); err != nil || sequence != 0 {
		t.Fatal("失败事务推进产物cursor", sequence, err)
	}
	for _, path := range []string{"/api/artifacts/" + declaration.ID, "/api/artifacts/" + declaration.ID + "/" + declaration.Name} {
		code, out := request(t, api, "GET", path, adminToken, "")
		if code != 404 || strings.Contains(out, string(data)) {
			t.Fatal("孤立文件被公开", code, out)
		}
	}
	node, err := st.AuthenticateNode(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.FindNodeArtifact(ctx, node, declaration.Ref, declaration.ID); err == nil {
		t.Fatal("未确认孤立文件获得node回执")
	}
	if _, err = db.ExecContext(ctx, "DROP TRIGGER "+trigger); err != nil {
		t.Fatal(err)
	}
	code, canonical := uploadArtifactHTTP(t, api, token, declaration, data)
	var view protocol.ArtifactView
	if err = json.Unmarshal(canonical, &view); err != nil || code != 200 || view.ID != declaration.ID || view.SHA256 != declaration.SHA256 {
		t.Fatal("同ID合法重传未确认", code, string(canonical), err)
	}
	code, repeated := uploadArtifactHTTP(t, api, token, declaration, data)
	if code != 200 || !bytes.Equal(canonical, repeated) {
		t.Fatal("重传不返回原canonical", code, string(repeated))
	}
	if err = db.QueryRowContext(ctx, "SELECT last_artifact_seq FROM builds WHERE id=?", declaration.Ref.BuildID).Scan(&sequence); err != nil || sequence != 1 {
		t.Fatal("重传cursor未幂等", sequence, err)
	}
	items, err = st.ListArtifacts(ctx, admin, declaration.Ref.BuildID, store.Page{Limit: 200})
	if err != nil || len(items) != 1 {
		t.Fatal("确认元数据数量错误", len(items), err)
	}
	current, err := os.Lstat(orphan)
	if err != nil || !os.SameFile(current, owned) {
		t.Fatal("重传删除或替换旧孤立文件", err)
	}
	entries, err = os.ReadDir(directory)
	if err != nil || len(entries) != 2 {
		t.Fatal("合法重传未清自己多余候选", entries, err)
	}
	stored, err := st.GetArtifact(ctx, admin, declaration.ID)
	if err != nil || stored.StorageID == filepath.Base(orphan) {
		t.Fatal("确认复用了旧孤立文件", err)
	}
}
