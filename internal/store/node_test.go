package store

import (
	"encoding/json"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"strings"
	"testing"
)

func TestNodeIdentityManagement(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		trigger, err := s.CreateToken(testContext, localAdmin, "trigger")
		if err != nil {
			t.Fatal(err)
		}
		actor, err := s.Authenticate(testContext, trigger.Token)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.CreateNode(testContext, actor, NodeInput{Name: "linux", Capacity: 1}); err != ErrForbidden {
			t.Fatal("trigger created node", err)
		}
		for _, in := range []NodeInput{{Name: "../secret-marker", Capacity: 1}, {Name: "linux", Capacity: 0}, {Name: "linux", Capacity: 33}, {Name: "linux", Capacity: 1, Labels: []string{"duplicate", "duplicate"}}} {
			if _, err = s.CreateNode(testContext, localAdmin, in); err != ErrInvalid {
				t.Fatal("invalid node", err)
			}
		}
		created, err := s.CreateNode(testContext, localAdmin, NodeInput{Name: "linux", Capacity: 2, Labels: []string{"android-sdk", "中文"}})
		if err != nil {
			t.Fatal(err)
		}
		if len(created.Token) < 32 || created.Node.State != "enabled" || created.Node.Healthy || created.Node.EffectiveCapacity != 0 {
			t.Fatal("created view", created.Node)
		}
		node, err := s.AuthenticateNode(testContext, created.Token)
		if err != nil || node.ID != created.Node.ID {
			t.Fatal("node authenticate", err)
		}
		if _, err = s.Authenticate(testContext, created.Token); err != ErrUnauthorized {
			t.Fatal("node became user", err)
		}
		if _, err = s.AuthenticateNode(testContext, trigger.Token); err != ErrNodeUnauthorized {
			t.Fatal("user became node", err)
		}
		view, err := s.GetNode(testContext, localAdmin, "linux")
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(view)
		if strings.Contains(string(b), created.Token) || strings.Contains(string(b), tokenDigest(created.Token)) {
			t.Fatal("token leaked")
		}
		var cred nodeCredentialRecord
		if err = s.db.First(&cred, "id = ?", node.CredentialID).Error; err != nil || cred.Digest != tokenDigest(created.Token) {
			t.Fatal("digest not persisted")
		}
		if _, err = s.GetNode(testContext, Actor{ID: node.ID, Role: "admin"}, "linux"); err != ErrUnauthorized {
			t.Fatal("forged role accepted", err)
		}
		for _, state := range []string{"draining", "disabled", "enabled"} {
			if err = s.SetNodeState(testContext, localAdmin, "linux", state); err != nil {
				t.Fatal(err)
			}
		}
		if err = s.SetNodeState(testContext, localAdmin, "linux", "deleted"); err != ErrInvalid {
			t.Fatal("state bypass", err)
		}
		rotated, err := s.RotateNodeToken(testContext, localAdmin, "linux")
		if err != nil || rotated.Token == created.Token {
			t.Fatal("rotate", err)
		}
		if _, err = s.AuthenticateNode(testContext, created.Token); err != ErrNodeUnauthorized {
			t.Fatal("old token alive", err)
		}
		if _, err = s.AuthenticateNode(testContext, rotated.Token); err != nil {
			t.Fatal(err)
		}
		if err = s.RevokeNodeToken(testContext, localAdmin, "linux"); err != nil {
			t.Fatal(err)
		}
		if _, err = s.AuthenticateNode(testContext, rotated.Token); err != ErrNodeUnauthorized {
			t.Fatal("revoked alive", err)
		}
		if err = s.DeleteNode(testContext, localAdmin, "linux"); err != nil {
			t.Fatal(err)
		}
		if _, err = s.CreateNode(testContext, localAdmin, NodeInput{Name: "linux", Capacity: 1}); err != ErrConflict {
			t.Fatal("name reused", err)
		}
		if _, err = s.GetNode(testContext, localAdmin, "linux"); err != ErrNotFound {
			t.Fatal("tombstone public", err)
		}
		for i := 0; i < 3; i++ {
			if _, err = s.CreateNode(testContext, localAdmin, NodeInput{Name: "node-" + uuid.NewString()[:8], Capacity: 1}); err != nil {
				t.Fatal(err)
			}
		}
		nodes, err := s.ListNodes(testContext, localAdmin, NodeFilter{Page: Page{Limit: 2}})
		if err != nil || len(nodes) != 2 {
			t.Fatal("page", err)
		}
		if _, err = s.ListNodes(testContext, actor, NodeFilter{}); err != ErrForbidden {
			t.Fatal("node list trigger", err)
		}
		if _, err = s.ListNodes(testContext, localAdmin, NodeFilter{Page: Page{Limit: 201}}); err != ErrInvalid {
			t.Fatal("unbounded page", err)
		}
		status, err := s.Status(testContext)
		if err != nil || status.Nodes != 3 || status.HealthyNodes != 0 {
			t.Fatal("node counts", status, err)
		}
		if err = s.writer.Delete(&nodeRecord{}, "id = ?", created.Node.ID).Error; safeError(err) != ErrConflict {
			t.Fatal("node credential FK", safeError(err))
		}
		err = s.write(testContext, func(tx *gorm.DB) error {
			return tx.Create(&nodeCredentialRecord{ID: uuid.NewString(), NodeID: uuid.NewString(), Digest: strings.Repeat("a", 64)}).Error
		})
		if err != ErrConflict {
			t.Fatal("orphan FK", err)
		}
	})
}
