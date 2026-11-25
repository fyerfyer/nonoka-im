package platformdata

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"nonoka-im/internal/audit"

	"gorm.io/gorm"
)

type AuditEventRow struct {
	ID           uint64          `gorm:"column:id;primaryKey;autoIncrement"`
	ActorUserID  int64           `gorm:"column:actor_user_id;not null"`
	Action       string          `gorm:"column:action;size:80;not null"`
	ResourceType string          `gorm:"column:resource_type;size:40;not null"`
	ResourceID   string          `gorm:"column:resource_id;size:160;not null"`
	Before       json.RawMessage `gorm:"column:before;type:jsonb"`
	After        json.RawMessage `gorm:"column:after;type:jsonb"`
	RequestID    string          `gorm:"column:request_id;size:128;not null"`
	TraceID      string          `gorm:"column:trace_id;size:64;not null"`
	IP           string          `gorm:"column:ip;size:64;not null"`
	UserAgent    string          `gorm:"column:user_agent;size:512;not null"`
	Outcome      string          `gorm:"column:outcome;size:16;not null"`
	ErrorCode    string          `gorm:"column:error_code;size:64;not null"`
	CreatedAt    time.Time       `gorm:"column:created_at;not null"`
}

func (AuditEventRow) TableName() string { return "audit_events" }

func (r AuditEventRow) toDomain() audit.Event {
	ev := audit.Event{
		ID: r.ID, ActorUserID: r.ActorUserID, Action: r.Action, ResourceType: r.ResourceType,
		ResourceID: r.ResourceID, RequestID: r.RequestID, TraceID: r.TraceID, IP: r.IP,
		UserAgent: r.UserAgent, Outcome: r.Outcome, ErrorCode: r.ErrorCode, CreatedAt: r.CreatedAt,
	}
	if len(r.Before) > 0 {
		_ = json.Unmarshal(r.Before, &ev.Before)
	}
	if len(r.After) > 0 {
		_ = json.Unmarshal(r.After, &ev.After)
	}
	return ev
}

func (s *Store) RecordAuditEvent(ctx context.Context, event audit.Event) error {
	before, _ := audit.MarshalDetails(event.Before)
	after, _ := audit.MarshalDetails(event.After)
	row := AuditEventRow{
		ActorUserID: event.ActorUserID, Action: event.Action, ResourceType: event.ResourceType,
		ResourceID: event.ResourceID, Before: before, After: after,
		RequestID: event.RequestID, TraceID: event.TraceID, IP: event.IP,
		UserAgent: event.UserAgent, Outcome: event.Outcome, ErrorCode: event.ErrorCode,
		CreatedAt: event.CreatedAt,
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = time.Now()
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return transient(err)
	}
	return nil
}

func (s *Store) ListAuditEvents(ctx context.Context, filter audit.Filter, limit, offset int) ([]audit.Event, int64, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	q := s.db.WithContext(ctx).Model(&AuditEventRow{})
	if filter.ActorUserID > 0 {
		q = q.Where("actor_user_id = ?", filter.ActorUserID)
	}
	if filter.Action != "" {
		q = q.Where("action = ?", filter.Action)
	}
	if filter.ResourceType != "" {
		q = q.Where("resource_type = ?", filter.ResourceType)
	}
	if filter.ResourceID != "" {
		q = q.Where("resource_id = ?", filter.ResourceID)
	}
	if filter.From != nil {
		q = q.Where("created_at >= ?", *filter.From)
	}
	if filter.To != nil {
		q = q.Where("created_at <= ?", *filter.To)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, transient(err)
	}
	var rows []AuditEventRow
	if err := q.Order("created_at desc, id desc").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, transient(err)
	}
	out := make([]audit.Event, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, total, nil
}

func (s *Store) GetAuditEvent(ctx context.Context, id uint64) (*audit.Event, error) {
	var row AuditEventRow
	err := s.db.WithContext(ctx).First(&row, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, transient(err)
	}
	ev := row.toDomain()
	return &ev, nil
}

// PurgeAuditEvents deletes rows older than cutoff. The append-only trigger
// only permits deletes inside a transaction with nonoka.audit_purge=on.
func (s *Store) PurgeAuditEvents(ctx context.Context, cutoff time.Time, batch int) (int64, error) {
	if batch <= 0 || batch > 10000 {
		batch = 5000
	}
	var total int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL nonoka.audit_purge = 'on'").Error; err != nil {
			return err
		}
		res := tx.Exec(`DELETE FROM audit_events WHERE id IN (
			SELECT id FROM audit_events WHERE created_at < ? ORDER BY id LIMIT ?
		)`, cutoff, batch)
		if res.Error != nil {
			return res.Error
		}
		total = res.RowsAffected
		return nil
	})
	return total, transient(err)
}
