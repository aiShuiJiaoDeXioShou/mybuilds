package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func TestCustomOptionalVariantsKeepOriginalReceiptDigest(t *testing.T) {
	// 010原固定字段顺序：nil可选variant不得插入custom键改变既有证据。
	receipt := PublishReceipt{Status: "unknown", StopConfirmed: true, EvidenceCode: "result_unconfirmed"}
	expected := `{"intent_id":"","authorization_digest":"","digest":"","ref":{"node_id":"","session_id":"","build_id":"","attempt_id":"","lease_id":"","epoch":0},"status":"unknown","evidence_code":"result_unconfirmed","mutation_stage":"","started":false,"stop_confirmed":true,"cleanup_failed":false,"remote":{}}`
	data, e := json.Marshal(receipt)
	if e != nil || string(data) != expected {
		t.Fatalf("original receipt encoding changed: %s", data)
	}
	sum := sha256.Sum256([]byte(expected))
	digest, e := PublishReceiptDigest(receipt)
	if e != nil || digest != hex.EncodeToString(sum[:]) {
		t.Fatal("legacy digest changed")
	}
	receipt.Remote.Custom = &CustomPublishEvidence{RemoteID: "owned", Lifecycle: "uploaded", ActionConfirmed: true}
	changed, _ := PublishReceiptDigest(receipt)
	if changed == digest {
		t.Fatal("custom evidence not bound")
	}
	encoded, _ := json.Marshal(PublishQueryTask{})
	if strings.Contains(string(encoded), "custom") {
		t.Fatal("unused query placeholder in original wire")
	}
}
