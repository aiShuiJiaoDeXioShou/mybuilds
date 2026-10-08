package distribute

import (
	"context"
	"mybuilds/internal/protocol"
	"os"
	"path/filepath"
	"testing"
)

func TestGrantDigestBindsServerChoices(t *testing.T) {
	a := protocol.PublishGrant{IntentID: "i", ReportIDs: []string{}, ReleaseStatus: "draft"}
	x, _ := protocol.PublishGrantDigest(a)
	a.ReleaseStatus = "completed"
	y, _ := protocol.PublishGrantDigest(a)
	if x == y {
		t.Fatal("派生状态未绑定")
	}
	r := protocol.PublishReceipt{Digest: "old"}
	x, _ = protocol.PublishReceiptDigest(r)
	r.Digest = "other"
	y, _ = protocol.PublishReceiptDigest(r)
	if x != y {
		t.Fatal("自身摘要参与")
	}
}
func TestRestrictedMaterialBoundaries(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "material")
	if err := os.WriteFile(p, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	b, err := readMaterial(context.Background(), p, 64, true)
	if err != nil || string(b) != "private" {
		t.Fatalf("%v", err)
	}
	if err := os.Chmod(p, 0400); err != nil {
		t.Fatal(err)
	}
	if data, err := readMaterial(context.Background(), p, 64, true); err != nil || string(data) != "private" {
		t.Fatal("只读发布材料被拒绝", err)
	}
	for _, name := range []string{"link", "hard"} {
		q := filepath.Join(dir, name)
		if name == "link" {
			os.Symlink(p, q)
		} else {
			os.Link(p, q)
		}
		if _, err := readMaterial(context.Background(), q, 64, true); err == nil {
			t.Fatal(name)
		}
	}
	os.Remove(filepath.Join(dir, "hard"))
	os.Chmod(p, 0644)
	if _, err := readMaterial(context.Background(), p, 64, true); err == nil {
		t.Fatal("公开凭据")
	}
	if _, err := readMaterial(context.Background(), p, 3, false); err == nil {
		t.Fatal("超限")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := readMaterial(ctx, p, 64, false); err == nil {
		t.Fatal("取消")
	}
}
func TestStrictCredentialJSON(t *testing.T) {
	for _, raw := range []string{`{"type":"service_account","type":"service_account"}`, `{"type":null}`, `{"type":"service_account","unknown":"private"}`, `{"type":"authorized_user"}`} {
		if _, err := googleCredential([]byte(raw)); err == nil {
			t.Fatal("非法凭据被接受")
		}
	}
}
