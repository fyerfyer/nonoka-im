package platformdata

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"nonoka-im/internal/httpapi"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// IdempotencyKey is the persisted Idempotency-Key record.
type IdempotencyKey struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement"`
	ScopeUserID  int64  `gorm:"uniqueIndex:idx_idempotency_user_key,priority:1;not null"`
	Key          string `gorm:"uniqueIndex:idx_idempotency_user_key,priority:2;size:255;not null"`
	Fingerprint  string `gorm:"size:64;not null"`
	Status       string `gorm:"size:16;not null"`
	ResponseCode int
	// ResponseBody is text; API responses are JSON.
	ResponseBody string `gorm:"type:text"`
	LockedUntil  *time.Time
	ExpiresAt    *time.Time `gorm:"index"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type IdempotencyStore struct {
	db  *gorm.DB
	TTL time.Duration
}

func NewIdempotencyStore(db *gorm.DB) *IdempotencyStore { return &IdempotencyStore{db: db} }

var _ httpapi.IdempotencyStore = (*IdempotencyStore)(nil)

const defaultIdempotencyTTL = 24 * time.Hour

var idempotencyPruneCounter atomic.Uint64

func (s *IdempotencyStore) ttl() time.Duration {
	if s.TTL > 0 {
		return s.TTL
	}
	return defaultIdempotencyTTL
}

// Begin inserts a new in-progress key or returns the existing record. An
// abandoned in-progress row can be taken over after its lock expires, and a
// record past its retention TTL is treated as absent.
func (s *IdempotencyStore) Begin(ctx context.Context, userID int64, key, fingerprint string, lockUntil time.Time) (httpapi.IdempotencyRecord, bool, error) {
	// Opportunistic cleanup keeps the table bounded without a dedicated cron.
	if idempotencyPruneCounter.Add(1)%256 == 0 {
		_, _ = s.PruneExpired(ctx, time.Now(), 500)
	}
	var out httpapi.IdempotencyRecord
	acquired := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		expiresAt := now.Add(s.ttl())
		row := IdempotencyKey{
			ScopeUserID: userID, Key: key, Fingerprint: fingerprint, Status: "in_progress",
			LockedUntil: &lockUntil, ExpiresAt: &expiresAt, CreatedAt: now, UpdatedAt: now,
		}
		res := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "scope_user_id"}, {Name: "key"}},
			DoNothing: true,
		}).Create(&row)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 1 {
			acquired = true
			return nil
		}
		var existing IdempotencyKey
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("scope_user_id = ? AND key = ?", userID, key).
			First(&existing).Error; err != nil {
			return err
		}
		expired := existing.ExpiresAt != nil && existing.ExpiresAt.Before(now)
		if !expired {
			out = httpapi.IdempotencyRecord{
				Status: existing.Status, Fingerprint: existing.Fingerprint,
				ResponseCode: existing.ResponseCode, ResponseBody: []byte(existing.ResponseBody),
			}
			if existing.Status == "completed" {
				return nil
			}
		}
		if expired || existing.LockedUntil == nil || existing.LockedUntil.Before(now) {
			if err := tx.Model(&IdempotencyKey{}).
				Where("id = ?", existing.ID).
				Updates(map[string]interface{}{
					"fingerprint": fingerprint, "status": "in_progress", "response_code": 0,
					"response_body": "", "locked_until": lockUntil, "expires_at": expiresAt, "updated_at": now,
				}).Error; err != nil {
				return err
			}
			acquired = true
			out = httpapi.IdempotencyRecord{Status: "in_progress", Fingerprint: fingerprint}
		}
		return nil
	})
	return out, acquired, err
}

// Complete stores the response of the winning request. A completion for a
// key that was taken over by another attempt is a no-op.
func (s *IdempotencyStore) Complete(ctx context.Context, userID int64, key, fingerprint string, code int, body []byte) error {
	now := time.Now()
	expiresAt := now.Add(s.ttl())
	res := s.db.WithContext(ctx).Model(&IdempotencyKey{}).
		Where("scope_user_id = ? AND key = ? AND fingerprint = ? AND status = ?", userID, key, fingerprint, "in_progress").
		Updates(map[string]interface{}{
			"status": "completed", "response_code": code, "response_body": string(body),
			"locked_until": nil, "expires_at": expiresAt, "updated_at": now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("idempotency record no longer owned by this attempt")
	}
	return nil
}

// PruneExpired deletes idempotency records past their retention TTL.
func (s *IdempotencyStore) PruneExpired(ctx context.Context, now time.Time, limit int) (int64, error) {
	if limit <= 0 {
		limit = 500
	}
	res := s.db.WithContext(ctx).Exec(`DELETE FROM idempotency_keys WHERE id IN (
		SELECT id FROM idempotency_keys WHERE expires_at IS NOT NULL AND expires_at < ? ORDER BY expires_at LIMIT ?
	)`, now, limit)
	return res.RowsAffected, res.Error
}
