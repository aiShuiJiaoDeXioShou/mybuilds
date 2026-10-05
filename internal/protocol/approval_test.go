package protocol

import (
	"encoding/json"
	"github.com/google/uuid"
	"reflect"
	"strings"
	"testing"
	"time"
)

// 可选字段不能改变原无审批消息的JSON或摘要。
func TestApprovalOptionalWirePreservesLegacy(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeOf(ExecutionProgress{}), reflect.TypeOf(TaskSnapshot{})} {
		name := "Approval"
		if typ == reflect.TypeOf(TaskSnapshot{}) {
			name = "Resume"
		}
		field, ok := typ.FieldByName(name)
		if !ok || field.Tag.Get("json") != map[string]string{"Approval": "approval,omitempty", "Resume": "resume,omitempty"}[name] {
			t.Fatalf("缺少兼容的可选审批字段 %s", name)
		}
	}
	p := ExecutionProgress{Kind: "finished", ArtifactSteps: []ArtifactExpectation{}}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]json.RawMessage
	json.Unmarshal(data, &wire)
	if _, ok := wire["approval"]; ok {
		t.Fatal("nil审批改变旧wire")
	}
	data, err = json.Marshal(TaskSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(data, &wire)
	if _, ok := wire["resume"]; ok {
		t.Fatal("nil恢复改变旧wire")
	}
}

func approvalProtocolFixture() (LeaseRef, ExecutionProgress) {
	ref := LeaseRef{NodeID: uuid.NewString(), SessionID: uuid.NewString(), BuildID: uuid.NewString(), AttemptID: uuid.NewString(), LeaseID: uuid.NewString(), Epoch: 1}
	evidence := &ApprovalCheckpointEvidence{ID: uuid.NewString(), Revision: 1, SnapshotDigest: strings.Repeat("a", 64), WorkspaceID: uuid.NewString(), ResultID: uuid.NewString(), NextOrdinaryIndex: 3, Artifacts: []ArtifactExpectation{}, PublishIntents: []PublishExpectation{}, SystemResourcesClosed: true}
	p := ExecutionProgress{Kind: "approval_checkpoint", Phase: "ordinary", Name: "review", StepKind: "approval", Index: 2, PostPhase: "none", StopConfirmed: true, ExitCode: -1, At: time.Now().UTC(), Approval: evidence, ArtifactSteps: []ArtifactExpectation{}, RemainingPostBudgetNS: 100}
	return ref, p
}
func TestApprovalCheckpointDigestBindsExactEvidence(t *testing.T) {
	ref, p := approvalProtocolFixture()
	hash, err := ApprovalCheckpointDigest(ref, 4, p)
	if err != nil || len(hash) != 64 {
		t.Fatal("合法证据摘要失败", err)
	}
	p.Approval.CheckpointDigest = hash
	again, err := ApprovalCheckpointDigest(ref, 4, p)
	if err != nil || again != hash {
		t.Fatal("摘要存在循环依赖")
	}
	changed := ref
	changed.Epoch++
	different, _ := ApprovalCheckpointDigest(changed, 4, p)
	if different == hash {
		t.Fatal("新epoch未被绑定")
	}
	p.LastLogOffset++
	different, _ = ApprovalCheckpointDigest(ref, 4, p)
	if different == hash {
		t.Fatal("日志游标未被绑定")
	}
}
func TestApprovalCheckpointDigestRejectsUnsafeShape(t *testing.T) {
	for _, mode := range []string{"ref", "closed", "stop", "cleanup", "started", "budget", "nil_manifest", "nil_reports", "next", "phase"} {
		t.Run(mode, func(t *testing.T) {
			ref, p := approvalProtocolFixture()
			switch mode {
			case "ref":
				ref.NodeID = "invalid"
			case "closed":
				p.Approval.SystemResourcesClosed = false
			case "stop":
				p.StopConfirmed = false
			case "cleanup":
				p.CleanupFailed = true
			case "started":
				p.Started = true
			case "budget":
				n := int64(-1)
				p.RemainingBudgetNS = &n
			case "nil_manifest":
				p.ArtifactSteps = nil
			case "nil_reports":
				p.Approval.Reports = &ReportEvidence{Revision: 1, Outcome: "pending", Files: nil, Diagnostics: []JUnitDiagnostic{}}
			case "next":
				p.Approval.NextOrdinaryIndex = 1
			case "phase":
				p.Phase = "always"
			}
			if _, err := ApprovalCheckpointDigest(ref, 4, p); err == nil {
				t.Fatal("非法检查点被规范化成有效摘要")
			}
		})
	}
}
