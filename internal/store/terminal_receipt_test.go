package store

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"mybuilds/internal/protocol"
)

func terminalReceiptFixture(t *testing.T, s *Store) (NodeActor, protocol.ExecutionEvent) {
	t.Helper()
	a, g, _ := claimed(t, s)
	completeRunEvents(t, s, a, g, false)
	p := protocol.ExecutionProgress{Kind: "build_finished", Status: "succeeded", Started: true, StopConfirmed: true, RemainingPostBudgetNS: int64(2*time.Minute) - 100, ArtifactSteps: []protocol.ArtifactExpectation{}}
	e := event(g.Ref, 8, p)
	accept(t, s, a, e)
	return a, e
}
func TestTerminalReceiptExactReadonlyAndCurrentCredential(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, e := terminalReceiptFixture(t, s)
		req := protocol.TerminalReceiptRequest{Ref: e.Ref, Seq: e.Seq, Digest: e.Digest}
		before := recoveryRows(t, s)
		out, err := s.TerminalReceipt(testContext, a, req)
		if err != nil || out.Ref != req.Ref || out.Seq != req.Seq || out.Digest != req.Digest || out.Status != "succeeded" || !out.StopKnown || out.NodeName != "linux" {
			t.Fatalf("精确回执: %+v %v", out, err)
		}
		if !reflect.DeepEqual(before, recoveryRows(t, s)) {
			t.Fatal("只读查询改变中央记录")
		}
		rotated, err := s.RotateNodeToken(testContext, localAdmin, "linux")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.TerminalReceipt(testContext, a, req); err != ErrNodeUnauthorized {
			t.Fatal("旧凭据未拒绝", err)
		}
		current, err := s.AuthenticateNode(testContext, rotated.Token)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.TerminalReceipt(testContext, current, req); err != nil {
			t.Fatal("新凭据只读核对不应要求旧session", err)
		}
	})
}
func TestTerminalReceiptRejectsUnknownAndWrongEvidence(t *testing.T) {
	cases := []struct {
		name   string
		column string
		value  any
	}{
		{"old-kind", "kind", ""}, {"old-stop", "stop_known", false}, {"wrong-kind", "kind", "finished"},
		{"guard", "stop_unconfirmed", true}, {"active", "status", "running"}, {"last-seq", "last_event_seq", int64(9)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stores(t, func(t *testing.T, s *Store, opt Options) {
				a, e := terminalReceiptFixture(t, s)
				table := "builds"
				id := e.Ref.BuildID
				if c.column == "kind" || c.column == "stop_known" {
					table = "execution_receipts"
					var r executionReceiptRecord
					s.db.First(&r, "build_id = ? AND seq = ?", id, e.Seq)
					id = r.ID
				}
				if err := s.writer.Table(table).Where("id = ?", id).Update(c.column, c.value).Error; err != nil {
					t.Fatal(err)
				}
				before := recoveryRows(t, s)
				if _, err := s.TerminalReceipt(testContext, a, protocol.TerminalReceiptRequest{Ref: e.Ref, Seq: e.Seq, Digest: e.Digest}); err != ErrConflict {
					t.Fatal("未知证据仍确认", err)
				}
				if !reflect.DeepEqual(before, recoveryRows(t, s)) {
					t.Fatal("拒绝仍修改状态")
				}
			})
		})
	}
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, e := terminalReceiptFixture(t, s)
		req := protocol.TerminalReceiptRequest{Ref: e.Ref, Seq: e.Seq, Digest: e.Digest}
		bad := req
		bad.Digest = strings.ToUpper(req.Digest)
		if _, err := s.TerminalReceipt(testContext, a, bad); err != ErrInvalid {
			t.Fatal("大写摘要", err)
		}
		bad = req
		bad.Ref.Epoch++
		if _, err := s.TerminalReceipt(testContext, a, bad); err != ErrConflict {
			t.Fatal("错fence", err)
		}
		bad = req
		bad.Seq--
		if _, err := s.TerminalReceipt(testContext, a, bad); err != ErrConflict {
			t.Fatal("历史收据冒充末次", err)
		}
		bad = req
		bad.Digest = strings.Repeat("a", 64)
		if _, err := s.TerminalReceipt(testContext, a, bad); err != ErrConflict {
			t.Fatal("错摘要", err)
		}
		bad = req
		bad.Ref.BuildID = uuid.NewString()
		if _, err := s.TerminalReceipt(testContext, a, bad); err != ErrNotFound {
			t.Fatal("不存在", err)
		}
		other, _ := openSession(t, s, "other-node", 1)
		if _, err := s.TerminalReceipt(testContext, other, req); err != ErrNodeUnauthorized {
			t.Fatal("跨节点", err)
		}
		if err := s.SetNodeState(testContext, localAdmin, "linux", "disabled"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.TerminalReceipt(testContext, a, req); err != ErrNodeUnauthorized {
			t.Fatal("disabled", err)
		}
	})
}
