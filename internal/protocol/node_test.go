package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestExecutionWireEvidence(t *testing.T) {
	now := time.Date(2026, 10, 4, 11, 0, 0, 123456789, time.UTC)
	p := ExecutionProgress{Kind: "build_finished", Status: "succeeded", At: now,
		RemainingPostBudgetNS: 123456789, ArtifactSteps: []ArtifactExpectation{},
		PID: 7391, PGID: 7391, LocalResultDir: "private-result-marker",
		LocalArtifacts: []CollectedArtifact{{SnapshotPath: "private-artifact-marker"}}}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"last_log_seq", "last_log_offset", "last_artifact_seq"} {
		if wire[key] != float64(0) {
			t.Fatalf("终态必须明确编码零位置：%s", key)
		}
	}
	if items, ok := wire["artifact_steps"].([]any); !ok || len(items) != 0 {
		t.Fatal("终态空清单必须是数组")
	}
	if _, exists := wire["remaining_budget_ns"]; exists {
		t.Fatal("无限普通预算应省略")
	}
	if wire["remaining_post_budget_ns"] != float64(123456789) {
		t.Fatal("纳秒预算丢失")
	}
	if wire["at"] != "2026-10-04T11:00:00.123456789Z" {
		t.Fatal("UTC精度丢失")
	}
	for _, forbidden := range []string{"7391", "private-result-marker", "private-artifact-marker", "PID", "pgid", "local_artifacts"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("私有证据编码：%s", forbidden)
		}
	}
	zero := int64(0)
	p.RemainingBudgetNS = &zero
	data, _ = json.Marshal(p)
	if !strings.Contains(string(data), `"remaining_budget_ns":0`) {
		t.Fatal("耗尽不能编码为无限")
	}
}

func TestSnapshotDigestAndIdentityEncoding(t *testing.T) {
	a := TaskSnapshot{Parameters: map[string]string{"b": "2", "a": "1"}, Facts: map[string]string{"git.sha": "abc", "project": "test"}}
	b := TaskSnapshot{Parameters: map[string]string{"a": "1", "b": "2"}, Facts: map[string]string{"project": "test", "git.sha": "abc"}}
	first, _ := json.Marshal(a)
	second, _ := json.Marshal(b)
	if string(first) != string(second) {
		t.Fatal("快照映射编码不确定")
	}
	ref := LeaseRef{NodeID: "bb7647b6-1746-4bda-a3b3-d945ce47925d", SessionID: "a29ad81f-a57c-48cd-9c61-176c101e4274", BuildID: "36b85ed3-805c-4785-ab76-2f4dc1981e81", AttemptID: "1781764e-c0f7-485a-9c57-7bc385fae746", LeaseID: "c8cdd81f-1c74-4e1a-8d2c-932987877954", Epoch: 1}
	progress := ExecutionProgress{Kind: "finished", Phase: "ordinary", Index: 1, ArtifactIDs: []string{"a"}, ArtifactSteps: []ArtifactExpectation{}}
	canonical, _ := json.Marshal(progress)
	sum := sha256.Sum256(canonical)
	event := ExecutionEvent{Ref: ref, Seq: 1, Digest: hex.EncodeToString(sum[:]), Progress: progress}
	data, _ := json.Marshal(event)
	var restored ExecutionEvent
	if err := json.Unmarshal(data, &restored); err != nil || restored.Ref != ref {
		t.Fatal("完整归属丢失")
	}
	canonical, _ = json.Marshal(restored.Progress)
	sum = sha256.Sum256(canonical)
	if restored.Digest != hex.EncodeToString(sum[:]) {
		t.Fatal("真实进度摘要不稳定")
	}
	data, _ = json.Marshal(SessionGrant{NodeID: ref.NodeID, NodeName: "node-a", SessionID: ref.SessionID})
	if !strings.Contains(string(data), `"node_name":"node-a"`) {
		t.Fatal("握手缺少实际节点名")
	}
	if err := json.Unmarshal([]byte(`{"seq":9223372036854775808}`), &restored); err == nil {
		t.Fatal("序号溢出必须拒绝")
	}
}

func TestCanonicalLogDigest(t *testing.T) {
	records := []LogRecord{{UTC: time.Date(2026, 10, 4, 11, 0, 0, 1, time.UTC), Build: "app", Phase: "ordinary", Index: 1, Step: "compile", Stream: "stdout", Text: "已脱敏"}}
	data, _ := json.Marshal(records)
	sum := sha256.Sum256(data)
	chunk := LogChunk{Seq: 1, Offset: 0, Digest: hex.EncodeToString(sum[:]), Records: records}
	encoded, _ := json.Marshal(chunk)
	var decoded LogChunk
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	data, _ = json.Marshal(decoded.Records)
	sum = sha256.Sum256(data)
	if decoded.Digest != hex.EncodeToString(sum[:]) || decoded.Records[0].Index != 1 || decoded.Records[0].Phase != "ordinary" {
		t.Fatal("日志位置或摘要改变")
	}
}
