package store

import (
	"github.com/google/uuid"
	"mybuilds/internal/protocol"
	"testing"
	"time"
)

func TestCancelExpireProtectionAndIndependentConfirmation(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		actor, grant, policy := claimed(t, s)
		p, err := s.GetProject(testContext, "app")
		if err != nil {
			t.Fatal(err)
		}
		queued := queueBuild(t, s, p, "after", "android", false)
		cancelled, err := s.Cancel(testContext, localAdmin, queued)
		if err != nil || cancelled.Status != "cancelled" || cancelled.NodeID != "" {
			t.Fatal("queued cancel", err)
		}
		pending, err := s.Cancel(testContext, localAdmin, grant.Ref.BuildID)
		if err != nil || pending.Status != "cancel_requested" || !pending.CancelRequested {
			t.Fatal("cancel intent", pending, err)
		}
		renewed, err := s.Renew(testContext, actor, grant.Ref, policy)
		if err != nil || !renewed.CancelRequested {
			t.Fatal("cancel renewal", err)
		}
		filter, err := s.ListBuilds(testContext, BuildFilter{Status: "cancel_requested"})
		if err != nil || len(filter) != 1 {
			t.Fatal("effective status filter", err)
		}
		confirm := protocol.StopConfirmation{Ref: grant.Ref, EvidenceCode: "process_group_reaped", Note: "实际确认本次进程组已经回收"}
		if err = s.ConfirmNodeStopped(testContext, actor, confirm); err != ErrStopUnconfirmed {
			t.Fatal("confirmed running", err)
		}
		if err = s.ExpireLeases(testContext); err != nil {
			t.Fatal(err)
		}
		view, err := s.GetBuild(testContext, grant.Ref.BuildID)
		if err != nil || view.Status != "cancel_requested" || view.StopUnconfirmed {
			t.Fatal("valid lease expired", view, err)
		}
		if err = s.writer.Model(&buildRecord{}).Where("id = ?", grant.Ref.BuildID).Update("lease_expires_at", time.Now().UTC()).Error; err != nil {
			t.Fatal(err)
		}
		if err = s.ExpireLeases(testContext); err != nil {
			t.Fatal(err)
		}
		view, err = s.GetBuild(testContext, grant.Ref.BuildID)
		if err != nil || view.Status != "interrupted" || !view.StopUnconfirmed || view.Reason != "lease_expired" {
			t.Fatal("expiry", view, err)
		}
		node, err := s.GetNode(testContext, localAdmin, "linux")
		if err != nil || !node.Quarantined {
			t.Fatal("quarantine", err)
		}
		if err = s.DeleteNode(testContext, localAdmin, "linux"); err != ErrConflict {
			t.Fatal("delete guard", err)
		}
		queued = queueBuild(t, s, p, "protected", "android", false)
		if next, err := s.Claim(testContext, actor, protocol.ClaimRequest{SessionID: grant.Ref.SessionID, ClaimKey: uuid.NewString()}, policy); err != nil || next != nil {
			t.Fatal("guard lost capacity", err)
		}
		bad := confirm
		bad.Ref.Epoch++
		if err = s.ConfirmNodeStopped(testContext, actor, bad); err != ErrLeaseInvalid {
			t.Fatal("wrong confirmation", err)
		}
		bad = confirm
		bad.Note = ""
		if err = s.ConfirmNodeStopped(testContext, actor, bad); err != ErrInvalid {
			t.Fatal("empty evidence", err)
		}
		other, _ := openSession(t, s, "other", 1)
		if err = s.ConfirmNodeStopped(testContext, other, confirm); err != ErrNodeUnauthorized {
			t.Fatal("other node confirmed", err)
		}
		rotated, err := s.RotateNodeToken(testContext, localAdmin, "linux")
		if err != nil {
			t.Fatal(err)
		}
		fresh, err := s.AuthenticateNode(testContext, rotated.Token)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.ConfirmNodeStopped(testContext, actor, confirm); err != ErrNodeUnauthorized {
			t.Fatal("revoked actor confirmed", err)
		}
		if err = s.ConfirmNodeStopped(testContext, fresh, confirm); err != nil {
			t.Fatal("independent current credential", err)
		}
		if err = s.ConfirmNodeStopped(testContext, fresh, confirm); err != nil {
			t.Fatal("confirmation replay", err)
		}
		view, err = s.GetBuild(testContext, grant.Ref.BuildID)
		if err != nil || view.StopUnconfirmed || view.Status != "interrupted" || view.Reason != "lease_expired" {
			t.Fatal("old result restored", view, err)
		}
		if err = s.CheckExecution(testContext, fresh, grant.Ref); err == nil {
			t.Fatal("expired authority restored")
		}
		if _, err = s.Cancel(testContext, localAdmin, queued); err != nil {
			t.Fatal(err)
		}
		if err = s.DeleteNode(testContext, localAdmin, "linux"); err != nil {
			t.Fatal("history prevents tombstone", err)
		}
	})
}
func TestExpirePreservesFailureAndRestartKeepsValidLease(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		actor, grant, _ := claimed(t, s)
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		reopened, err := Open(testContext, opt)
		if err != nil {
			t.Fatal(err)
		}
		defer reopened.Close()
		if err = reopened.Migrate(testContext); err != nil {
			t.Fatal(err)
		}
		if err = reopened.ExpireLeases(testContext); err != nil {
			t.Fatal(err)
		}
		if err = reopened.CheckExecution(testContext, actor, grant.Ref); err != nil {
			t.Fatal("restart lost valid grant", err)
		}
		if err = reopened.writer.Model(&buildRecord{}).Where("id = ?", grant.Ref.BuildID).Updates(map[string]any{"reason": "exit", "lease_expires_at": time.Now().UTC()}).Error; err != nil {
			t.Fatal(err)
		}
		if err = reopened.ExpireLeases(testContext); err != nil {
			t.Fatal(err)
		}
		view, err := reopened.GetBuild(testContext, grant.Ref.BuildID)
		if err != nil || view.Reason != "exit" || !view.StopUnconfirmed {
			t.Fatal("failure lost", err)
		}
		confirmation := protocol.StopConfirmation{Ref: grant.Ref, EvidenceCode: "admin_observed_stopped", Note: "管理员已核对本次实例停机证据"}
		if err = reopened.ConfirmStopped(testContext, localAdmin, confirmation); err != nil {
			t.Fatal(err)
		}
		view, err = reopened.GetBuild(testContext, grant.Ref.BuildID)
		if err != nil || view.Reason != "exit" || view.Status != "interrupted" || view.StopUnconfirmed {
			t.Fatal("confirm mutated result", err)
		}
	})
}

func TestNodeAuthorityRevocationNeverReenablesOriginalAttempt(t *testing.T) {
	for _, action := range []string{"disable", "revoke", "rotate"} {
		t.Run(action, func(t *testing.T) {
			stores(t, func(t *testing.T, s *Store, opt Options) {
				actor, grant, policy := claimed(t, s)
				p, err := s.GetProject(testContext, "app")
				if err != nil {
					t.Fatal(err)
				}
				queueBuild(t, s, p, "after-revoke", "other", false)
				switch action {
				case "disable":
					if err = s.SetNodeState(testContext, localAdmin, "linux", "disabled"); err != nil {
						t.Fatal(err)
					}
					if err = s.SetNodeState(testContext, localAdmin, "linux", "enabled"); err != nil {
						t.Fatal(err)
					}
				case "revoke":
					if err = s.RevokeNodeToken(testContext, localAdmin, "linux"); err != nil {
						t.Fatal(err)
					}
				case "rotate":
					if _, err = s.RotateNodeToken(testContext, localAdmin, "linux"); err != nil {
						t.Fatal(err)
					}
				}
				if _, err = s.Renew(testContext, actor, grant.Ref, policy); err == nil {
					t.Fatal("original attempt revived")
				}
				view, err := s.GetBuild(testContext, grant.Ref.BuildID)
				if err != nil || view.Status != "interrupted" || view.Reason != "authority_lost" || !view.StopUnconfirmed || view.AttemptID != grant.Ref.AttemptID {
					t.Fatal("revocation lost fence", view, err)
				}
				if action == "disable" {
					if next, err := s.Claim(testContext, actor, protocol.ClaimRequest{SessionID: grant.Ref.SessionID, ClaimKey: uuid.NewString()}, policy); err != nil || next != nil {
						t.Fatal("reenable bypassed guard", err)
					}
				}
				if err = s.DeleteNode(testContext, localAdmin, "linux"); err != ErrConflict {
					t.Fatal("revocation delete guard", err)
				}
				status, err := s.Status(testContext)
				if err != nil || status.Running != 0 || status.Interrupted != 1 || status.HealthyNodes != 0 {
					t.Fatal("revoked statistics", status, err)
				}
			})
		})
	}
}
