package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func tokenDigest(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}
func validToken(token string) bool {
	if len(token) < 32 || len(token) > 4096 {
		return false
	}
	for _, c := range token {
		if c < 33 || c > 126 {
			return false
		}
	}
	return true
}
func validRole(role string) bool { return role == "admin" || role == "trigger" || role == "approver" }
func authorize(db *gorm.DB, actor Actor, roles ...string) error {
	if actor.ID == "local-admin" && actor.Role == "admin" {
		return nil
	}
	var identity identityRecord
	if err := db.Where("id = ? AND revoked_at IS NULL", actor.ID).First(&identity).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return ErrUnauthorized
		}
		return err
	}
	if identity.Role != actor.Role {
		return ErrUnauthorized
	}
	for _, role := range roles {
		if identity.Role == role {
			return nil
		}
	}
	return ErrForbidden
}
func (s *Store) Bootstrap(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	if !validToken(token) {
		return ErrInvalid
	}
	return s.write(ctx, func(tx *gorm.DB) error {
		var metadata metadataRecord
		if err := tx.First(&metadata, 1).Error; err != nil {
			return err
		}
		if metadata.IdentityInitialized {
			return nil
		}
		identity := identityRecord{ID: uuid.NewString(), Digest: tokenDigest(token), Role: "admin", CreatedAt: time.Now().UTC()}
		if err := tx.Create(&identity).Error; err != nil {
			return err
		}
		return tx.Model(&metadata).Update("identity_initialized", true).Error
	})
}
func (s *Store) Authenticate(ctx context.Context, token string) (Actor, error) {
	if err := s.CheckLock(ctx); err != nil {
		return Actor{}, err
	}
	if !validToken(token) {
		return Actor{}, ErrUnauthorized
	}
	var identity identityRecord
	if err := s.db.WithContext(ctx).Where("digest = ? AND revoked_at IS NULL", tokenDigest(token)).First(&identity).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return Actor{}, ErrUnauthorized
		}
		return Actor{}, safeError(err)
	}
	return Actor{ID: identity.ID, Role: identity.Role}, nil
}
func (s *Store) CreateToken(ctx context.Context, actor Actor, role string) (TokenCreated, error) {
	if !validRole(role) {
		return TokenCreated{}, ErrInvalid
	}
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return TokenCreated{}, errDatabase
	}
	token := base64.RawURLEncoding.EncodeToString(bytes)
	row := identityRecord{ID: uuid.NewString(), Role: role, Digest: tokenDigest(token), CreatedAt: time.Now().UTC()}
	err := s.write(ctx, func(tx *gorm.DB) error {
		if err := authorize(tx, actor, "admin"); err != nil {
			return err
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		if err := tx.Model(&metadataRecord{}).Where("id = ?", 1).Update("identity_initialized", true).Error; err != nil {
			return err
		}
		return audit(tx, actor, "token_create", row.ID, "", role)
	})
	if err != nil {
		return TokenCreated{}, err
	}
	return TokenCreated{ID: row.ID, Role: role, Token: token}, nil
}
func (s *Store) ListTokens(ctx context.Context, actor Actor, page Page) ([]TokenView, error) {
	page, err := normalizePage(page)
	if err != nil {
		return nil, err
	}
	if err = s.CheckLock(ctx); err != nil {
		return nil, err
	}
	db := s.db.WithContext(ctx)
	if err = authorize(db, actor, "admin"); err != nil {
		return nil, safeError(err)
	}
	var rows []identityRecord
	if err = db.Order("created_at DESC, id DESC").Limit(page.Limit).Offset(page.Offset).Find(&rows).Error; err != nil {
		return nil, safeError(err)
	}
	views := make([]TokenView, 0, len(rows))
	for _, row := range rows {
		if row.RevokedAt != nil {
			utc := row.RevokedAt.UTC()
			row.RevokedAt = &utc
		}
		views = append(views, TokenView{ID: row.ID, Role: row.Role, CreatedAt: row.CreatedAt.UTC(), RevokedAt: row.RevokedAt})
	}
	return views, nil
}
func (s *Store) RevokeToken(ctx context.Context, actor Actor, id string) error {
	if !validID(id) {
		return ErrInvalid
	}
	return s.write(ctx, func(tx *gorm.DB) error {
		if err := authorize(tx, actor, "admin"); err != nil {
			return err
		}
		var row identityRecord
		if err := tx.First(&row, "id = ?", id).Error; err != nil {
			return err
		}
		if row.RevokedAt != nil {
			return nil
		}
		if err := tx.Model(&row).Update("revoked_at", time.Now().UTC()).Error; err != nil {
			return err
		}
		return audit(tx, actor, "token_revoke", id, row.Role, "")
	})
}
