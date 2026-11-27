package botdata

import (
	"context"
	"fmt"
	"time"

	"nonoka-im/internal/bot/domain"

	"gorm.io/gorm"
)

func (s *Store) EnqueueDelivery(ctx context.Context, delivery *domain.Delivery) (bool, error) {
	if delivery.EventID == "" || delivery.BotID <= 0 {
		return false, fmt.Errorf("delivery bot and event id are required")
	}
	now := time.Now()
	if delivery.CreatedAt.IsZero() {
		delivery.CreatedAt = now
	}
	if delivery.NextAttemptAt.IsZero() {
		delivery.NextAttemptAt = now
	}
	if delivery.Status == "" {
		delivery.Status = domain.DeliveryPending
	}
	res := s.db.WithContext(ctx).Exec(`
		INSERT INTO bot_deliveries
			(bot_id, event_id, event_type, topic, msg_id, status, attempts, next_attempt_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?)
		ON CONFLICT (bot_id, event_id) DO NOTHING`,
		delivery.BotID, delivery.EventID, delivery.EventType, delivery.Topic, delivery.MsgID,
		delivery.Status, delivery.NextAttemptAt, delivery.CreatedAt)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// ClaimDelivery atomically claims one due row. It returns (nil, nil) when no
// row is currently due.
func (s *Store) ClaimDelivery(ctx context.Context, lock time.Duration) (*domain.Delivery, error) {
	if lock <= 0 {
		lock = time.Minute
	}
	var row deliveryRow
	res := s.db.WithContext(ctx).Raw(`
		WITH claimed AS (
			SELECT id FROM bot_deliveries
			WHERE (status = 'pending' AND next_attempt_at <= now())
			   OR (status = 'delivering' AND locked_until < now())
			ORDER BY next_attempt_at ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE bot_deliveries d
		SET status = 'delivering',
		    locked_until = now() + ($1 * interval '1 second'),
		    attempts = attempts + 1
		FROM claimed
		WHERE d.id = claimed.id
		RETURNING d.*`, lock.Seconds()).Scan(&row)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, nil
	}
	delivery := toDomainDelivery(row)
	return &delivery, nil
}

func markDelivery(ctx context.Context, db *gorm.DB, id int64, attempt int, values map[string]any) error {
	res := db.WithContext(ctx).Table(tableDeliveries).
		Where("id = ? AND status = ? AND attempts = ?", id, domain.DeliveryDelivering, attempt).
		Updates(values)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		// The lease expired and another worker claimed a newer attempt. A
		// stale worker must not overwrite that worker's result.
		return domain.ErrDeliveryLease
	}
	return nil
}

func (s *Store) MarkDeliverySucceeded(ctx context.Context, id int64, attempt, statusCode int, snippet string) error {
	now := time.Now()
	return markDelivery(ctx, s.db, id, attempt, map[string]any{
		"status": domain.DeliverySucceeded, "delivered_at": now, "locked_until": nil,
		"last_status_code": statusCode, "response_snippet": snippet, "last_error": "",
	})
}

func (s *Store) MarkDeliveryRetry(ctx context.Context, id int64, attempt int, next time.Time, statusCode int, errMsg, snippet string) error {
	return markDelivery(ctx, s.db, id, attempt, map[string]any{
		"status": domain.DeliveryPending, "next_attempt_at": next, "locked_until": nil,
		"last_status_code": statusCode, "last_error": errMsg, "response_snippet": snippet,
	})
}

func (s *Store) MarkDeliveryFailed(ctx context.Context, id int64, attempt, statusCode int, errMsg, snippet string) error {
	return markDelivery(ctx, s.db, id, attempt, map[string]any{
		"status": domain.DeliveryFailed, "locked_until": nil,
		"last_status_code": statusCode, "last_error": errMsg, "response_snippet": snippet,
	})
}

func (s *Store) ListDeliveries(ctx context.Context, botID int64, status string, limit, offset int) ([]domain.Delivery, int64, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	q := s.db.WithContext(ctx).Table(tableDeliveries).Where("bot_id = ?", botID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []deliveryRow
	if err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]domain.Delivery, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDomainDelivery(row))
	}
	return out, total, nil
}

func (s *Store) ReplayDelivery(ctx context.Context, botID, id int64) error {
	now := time.Now()
	res := s.db.WithContext(ctx).Table(tableDeliveries).
		Where("id = ? AND bot_id = ? AND status = ?", id, botID, domain.DeliveryFailed).
		Updates(map[string]any{
			"status": domain.DeliveryPending, "attempts": 0, "next_attempt_at": now,
			"locked_until": nil, "last_error": "",
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) ResetStaleDeliveries(ctx context.Context, older time.Duration) (int64, error) {
	if older <= 0 {
		older = time.Minute
	}
	cutoff := time.Now().Add(-older)
	res := s.db.WithContext(ctx).Table(tableDeliveries).
		Where("status = ? AND locked_until < ?", domain.DeliveryDelivering, cutoff).
		Updates(map[string]any{"status": domain.DeliveryPending, "locked_until": nil})
	return res.RowsAffected, res.Error
}

func (s *Store) CleanupDeliveries(ctx context.Context, before time.Time, limit int) (int64, error) {
	if limit <= 0 {
		limit = 5000
	}
	res := s.db.WithContext(ctx).Exec(`
		DELETE FROM bot_deliveries
		WHERE id IN (
			SELECT id FROM bot_deliveries
			WHERE created_at < ? AND status IN ('succeeded', 'failed')
			LIMIT ?
		)`, before, limit)
	return res.RowsAffected, res.Error
}

func (s *Store) DeliveryStats(ctx context.Context, botID int64, since time.Time) (map[string]int64, error) {
	type statRow struct {
		Status string `gorm:"column:status"`
		Count  int64  `gorm:"column:count"`
	}
	var rows []statRow
	if err := s.db.WithContext(ctx).Table(tableDeliveries).
		Select("status, count(*) AS count").
		Where("bot_id = ? AND created_at >= ?", botID, since).
		Group("status").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, row := range rows {
		out[row.Status] = row.Count
	}
	return out, nil
}
