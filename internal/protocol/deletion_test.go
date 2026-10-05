package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestNodeDeletionDigestCanonicalExcludesDigest(t *testing.T) {
	in := NodeDeletionConfirmation{ID: "id", ResourceID: "resource", OwnershipDigest: "ownership", Nonce: "nonce", Seq: 1, Digest: "ignored", WorkspaceState: "deleted", ResultsState: "not_applicable"}
	canonical := `{"id":"id","resource_id":"resource","ownership_digest":"ownership","nonce":"nonce","seq":1,"workspace_state":"deleted","results_state":"not_applicable","reason":""}`
	hash := sha256.Sum256([]byte(canonical))
	got, err := NodeDeletionDigest(in)
	if err != nil || got != hex.EncodeToString(hash[:]) {
		t.Fatal("固定当前字段顺序并完全省略Digest", got, err)
	}
	in.Digest = "different"
	if again, err := NodeDeletionDigest(in); err != nil || again != got {
		t.Fatal("Digest自身不能改变摘要", err)
	}
	in.Seq++
	if changed, err := NodeDeletionDigest(in); err != nil || changed == got {
		t.Fatal("真实新确认必须不同摘要", err)
	}
}
