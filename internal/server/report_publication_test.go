package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
)

// 真实第二SQLite连接只拒绝自己的INSERT；已排他发布的孤立XML不成为可信报告。
func TestReportHTTPPublicationFailedCommitCannotSeal(t *testing.T) {
	data := []byte(`<testsuite tests="1"><testcase/></testsuite>`)
	s, st, api, token, d, evidence := reportHTTPFixture(t, data, nil)
	db, err := sql.Open("sqlite", filepath.Join(s.config.DataDir, "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	const trigger = "owned_junit_commit_failure"
	if _, err = db.Exec("CREATE TRIGGER " + trigger + " BEFORE INSERT ON artifacts BEGIN SELECT * FROM owned_missing_junit_table; END"); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP TRIGGER IF EXISTS " + trigger)
	code, body := uploadArtifactHTTP(t, api, token, d, data)
	if code != 500 {
		t.Fatal("SQL失败未拒绝", code, string(body))
	}
	entries, err := os.ReadDir(filepath.Join(s.config.DataDir, "artifacts"))
	if err != nil || len(entries) != 1 || !canonicalUUID(entries[0].Name()) {
		t.Fatal("未实际发布完整candidate或stage残留", entries, err)
	}
	orphan := filepath.Join(s.config.DataDir, "artifacts", entries[0].Name())
	original, err := os.Lstat(orphan)
	if err != nil || !evidenceInfo(original, false) {
		t.Fatal("孤立XML不属自有普通文件", err)
	}
	actual, err := os.ReadFile(orphan)
	if err != nil || !bytes.Equal(actual, data) {
		t.Fatal("candidate非原XML", err)
	}
	node, err := st.AuthenticateNode(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.FindNodeArtifact(context.Background(), node, d.Ref, d.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("SQL失败获得原文件回执", err)
	}
	var seq, records int64
	if err = db.QueryRow("SELECT last_artifact_seq FROM builds WHERE id=?", d.Ref.BuildID).Scan(&seq); err != nil || seq != 0 {
		t.Fatal("SQL失败推进cursor", seq, err)
	}
	if err = db.QueryRow("SELECT count(*) FROM artifacts WHERE id=?", d.ID).Scan(&records); err != nil || records != 0 {
		t.Fatal("SQL失败保留metadata", records, err)
	}
	for _, path := range []string{"/api/artifacts/" + d.ID, "/api/artifacts/" + d.ID + "/" + d.Name} {
		code, out := request(t, api, "GET", path, adminToken, "")
		if code != 404 {
			t.Fatal("孤立XML可见", code, out)
		}
	}
	sealReportHTTP(t, api, token, d, evidence, 409)
	if _, err = db.Exec("DROP TRIGGER " + trigger); err != nil {
		t.Fatal(err)
	}
	code, body = uploadArtifactHTTP(t, api, token, d, data)
	if code != 200 {
		t.Fatal("同ID重传失败", code, string(body))
	}
	var meta protocol.ArtifactView
	if err = json.Unmarshal(body, &meta); err != nil || meta.ID != d.ID {
		t.Fatal(err, string(body))
	}
	code, repeated := uploadArtifactHTTP(t, api, token, d, data)
	if code != 200 || !bytes.Equal(body, repeated) {
		t.Fatal("同ID回执非canonical", code, string(repeated))
	}
	if err = db.QueryRow("SELECT last_artifact_seq FROM builds WHERE id=?", d.Ref.BuildID).Scan(&seq); err != nil || seq != 1 {
		t.Fatal("重传非幂等cursor", seq, err)
	}
	sealReportHTTP(t, api, token, d, evidence, 200)
	current, err := os.Lstat(orphan)
	if err != nil || !os.SameFile(original, current) {
		t.Fatal("重传删除/替换未知孤立XML", err)
	}
	entries, err = os.ReadDir(filepath.Join(s.config.DataDir, "artifacts"))
	if err != nil || len(entries) != 2 {
		t.Fatal("重传未清自己多余candidate", entries, err)
	}
}
