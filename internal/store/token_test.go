package store

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestBootstrapAuthenticationSticky(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		token := "bootstrap-unique-secret-marker-0123456789abcdef"
		if err := s.Bootstrap(testContext, ""); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Authenticate(testContext, token); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("empty bootstrap: %v", err)
		}
		if err := s.Bootstrap(testContext, token); err != nil {
			t.Fatal(err)
		}
		actor, err := s.Authenticate(testContext, token)
		if err != nil || actor.Role != "admin" {
			t.Fatalf("auth: %v %v", actor, err)
		}
		if _, err = s.Authenticate(testContext, "different-secret-marker-0123456789"); !errors.Is(err, ErrUnauthorized) {
			t.Fatal("unknown token accepted")
		}
		var row identityRecord
		if err = s.db.First(&row, "id = ?", actor.ID).Error; err != nil {
			t.Fatal(err)
		}
		if row.Digest == token || len(row.Digest) != 64 {
			t.Fatal("token plaintext persisted")
		}
		if err = s.RevokeToken(testContext, localAdmin, actor.ID); err != nil {
			t.Fatal(err)
		}
		if _, err = s.Authenticate(testContext, token); !errors.Is(err, ErrUnauthorized) {
			t.Fatal("revoked accepted")
		}
		if err = s.Close(); err != nil {
			t.Fatal(err)
		}
		s, err = Open(testContext, opt)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		if err = s.Migrate(testContext); err != nil {
			t.Fatal(err)
		}
		if err = s.Bootstrap(testContext, token); err != nil {
			t.Fatal(err)
		}
		if _, err = s.Authenticate(testContext, token); !errors.Is(err, ErrUnauthorized) {
			t.Fatal("bootstrap resurrected")
		}
	})
}

func TestTokenRolesAndSafeViews(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		for _, role := range []string{"admin", "trigger", "approver"} {
			created, err := s.CreateToken(testContext, localAdmin, role)
			if err != nil {
				t.Fatal(err)
			}
			actor, err := s.Authenticate(testContext, created.Token)
			if err != nil || actor.Role != role {
				t.Fatal("new token invalid")
			}
			views, err := s.ListTokens(testContext, localAdmin, Page{})
			if err != nil {
				t.Fatal(err)
			}
			b, _ := json.Marshal(views)
			if strings.Contains(string(b), created.Token) || strings.Contains(string(b), "digest") {
				t.Fatal("view secret leak")
			}
			if role != "admin" {
				if _, err = s.CreateToken(testContext, actor, "admin"); !errors.Is(err, ErrForbidden) {
					t.Fatalf("role escalation: %v", err)
				}
			}
			if role == "trigger" {
				forged := Actor{ID: actor.ID, Role: "admin"}
				if _, err = s.CreateToken(testContext, forged, "admin"); !errors.Is(err, ErrUnauthorized) {
					t.Fatal("forged actor trusted")
				}
			}
		}
		if _, err := s.CreateToken(testContext, localAdmin, "secret-role-marker"); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid role")
		}
		if err := s.Bootstrap(testContext, "weak"); !errors.Is(err, ErrInvalid) {
			t.Fatal("weak bootstrap")
		}
	})
}
