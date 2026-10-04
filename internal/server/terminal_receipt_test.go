package server

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"mybuilds/internal/protocol"
)

func TestActualTerminalReceiptHTTPReadOnlyCurrentNode(t *testing.T) {
	_, st, h, token, in := executionHTTPFixture(t)
	grant := claimHTTP(t, h, token, in)
	p := protocol.ExecutionProgress{Kind: "build_finished", Status: "succeeded", Started: true, StopConfirmed: true, At: time.Now().UTC(), RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}
	query := protocol.TerminalReceiptRequest{Ref: grant.Ref, Seq: 5, Digest: messageDigest(t, p)}
	code, body := request(t, h, "POST", "/api/agent/terminal-receipt", token, encodeMessage(t, query))
	if code != 409 {
		t.Fatal("active回执应拒绝", code, body)
	}
	step := protocol.ExecutionProgress{Kind: "intent", Phase: "ordinary", Index: 1, Name: "shell", StepKind: "run", At: time.Now().UTC(), RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}
	sequence := []protocol.ExecutionProgress{step}
	step.Kind = "started"
	step.Started = true
	sequence = append(sequence, step)
	step.Kind = "finished"
	step.Status = "succeeded"
	step.StopConfirmed = true
	sequence = append(sequence, step)
	sequence = append(sequence, protocol.ExecutionProgress{Kind: "post_selected", PostPhase: "success", At: time.Now().UTC(), RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}, p)
	for i, progress := range sequence {
		ev := protocol.ExecutionEvent{Ref: grant.Ref, Seq: int64(i + 1), Digest: messageDigest(t, progress), Progress: progress}
		code, body = request(t, h, "POST", "/api/agent/events", token, encodeMessage(t, ev))
		if code != 200 {
			t.Fatal("实际终态提交失败", i, code, body)
		}
	}
	before, err := st.GetBuild(context.Background(), grant.Ref.BuildID)
	if err != nil {
		t.Fatal(err)
	}
	code, body = request(t, h, "POST", "/api/agent/terminal-receipt", token, encodeMessage(t, query))
	var receipt protocol.TerminalReceipt
	if err = json.Unmarshal([]byte(body), &receipt); err != nil || code != 200 || receipt.Ref != query.Ref || receipt.Seq != query.Seq || receipt.Digest != query.Digest || !receipt.StopKnown || receipt.Status != "succeeded" || receipt.NodeName != "worker" {
		t.Fatal(code, body, err)
	}
	after, err := st.GetBuild(context.Background(), grant.Ref.BuildID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("只读回执改变状态/预算", err)
	}
	wrong := query
	wrong.Seq++
	code, _ = request(t, h, "POST", "/api/agent/terminal-receipt", token, encodeMessage(t, wrong))
	if code != 409 {
		t.Fatal("错seq被接受", code)
	}
	code, _ = request(t, h, "POST", "/api/agent/terminal-receipt", adminToken, encodeMessage(t, query))
	if code != 401 {
		t.Fatal("用户访问节点只读", code)
	}
	code, _ = request(t, h, "POST", "/api/agent/terminal-receipt", token, `{"ref":null}`)
	if code != 400 {
		t.Fatal("弱JSON被接受", code)
	}
	// 查receipt不允许补写原终态。
	ev := protocol.ExecutionEvent{Ref: query.Ref, Seq: query.Seq, Digest: query.Digest, Progress: p}
	code, _ = request(t, h, "POST", "/api/agent/events", token, encodeMessage(t, ev))
	if code != 409 {
		t.Fatal("旧终态重放被接受", code)
	}
	admin, _ := st.Authenticate(context.Background(), adminToken)
	rotated, err := st.RotateNodeToken(context.Background(), admin, "worker")
	if err != nil {
		t.Fatal(err)
	}
	code, body = request(t, h, "POST", "/api/agent/terminal-receipt", rotated.Token, encodeMessage(t, query))
	if code != 200 {
		t.Fatal("当前新凭据必须可只读核对旧session", code, body)
	}
	code, _ = request(t, h, "POST", "/api/agent/terminal-receipt", token, encodeMessage(t, query))
	if code != 401 {
		t.Fatal("撤销旧凭据仍可读", code)
	}
}
