// Package audit records append-only, redacted events for sensitive platform
// and agent operations. It is a shared usecase package: transports and
// application services depend on it, while the PostgreSQL adapter implements
// its Store port.
package audit

import (
	"context"
	"encoding/json"
	"time"
	"unicode/utf8"
)

// RequestMeta is the HTTP correlation metadata attached by the transport layer.
type RequestMeta struct {
	RequestID string
	TraceID   string
	IP        string
	UserAgent string
}

type metaKey struct{}

func WithRequestMeta(ctx context.Context, meta RequestMeta) context.Context {
	return context.WithValue(ctx, metaKey{}, meta)
}

func RequestMetaFromContext(ctx context.Context) RequestMeta {
	meta, _ := ctx.Value(metaKey{}).(RequestMeta)
	return meta
}

// Event is one immutable audit record. Before/After must contain only
// allowlisted, redacted fields; prompts, message bodies and credentials are
// never recorded.
type Event struct {
	ID           uint64
	ActorUserID  int64
	Action       string
	ResourceType string
	ResourceID   string
	Before       map[string]any
	After        map[string]any
	RequestID    string
	TraceID      string
	IP           string
	UserAgent    string
	Outcome      string
	ErrorCode    string
	CreatedAt    time.Time
}

// Store is the persistence port.
type Store interface {
	RecordAuditEvent(ctx context.Context, event Event) error
	ListAuditEvents(ctx context.Context, filter Filter, limit, offset int) ([]Event, int64, error)
	GetAuditEvent(ctx context.Context, id uint64) (*Event, error)
}

type Filter struct {
	ActorUserID  int64
	Action       string
	ResourceType string
	ResourceID   string
	From         *time.Time
	To           *time.Time
}

// Service enriches events with request metadata and writes them.
type Service struct {
	Store Store
}

func NewService(store Store) *Service { return &Service{Store: store} }

func (s *Service) Record(ctx context.Context, event Event) error {
	if s == nil || s.Store == nil {
		return nil
	}
	meta := RequestMetaFromContext(ctx)
	if event.RequestID == "" {
		event.RequestID = meta.RequestID
	}
	if event.TraceID == "" {
		event.TraceID = meta.TraceID
	}
	if event.IP == "" {
		event.IP = meta.IP
	}
	if event.UserAgent == "" {
		event.UserAgent = meta.UserAgent
	}
	if event.Outcome == "" {
		event.Outcome = "success"
	}
	event.IP = truncate(event.IP, 64)
	event.UserAgent = truncate(event.UserAgent, 512)
	event.RequestID = truncate(event.RequestID, 128)
	event.TraceID = truncate(event.TraceID, 64)
	event.ResourceID = truncate(event.ResourceID, 160)
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}
	return s.recordWithRetry(ctx, event)
}

func (s *Service) recordWithRetry(ctx context.Context, event Event) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		err = s.Store.RecordAuditEvent(ctx, event)
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(time.Duration(attempt+1) * 50 * time.Millisecond):
		}
	}
	return err
}

func truncate(v string, max int) string {
	if len(v) <= max {
		return v
	}
	v = v[:max]
	for !utf8.ValidString(v) && len(v) > 0 {
		v = v[:len(v)-1]
	}
	return v
}

func (s *Service) List(ctx context.Context, filter Filter, limit, offset int) ([]Event, int64, error) {
	if s == nil || s.Store == nil {
		return nil, 0, nil
	}
	return s.Store.ListAuditEvents(ctx, filter, limit, offset)
}

func (s *Service) Get(ctx context.Context, id uint64) (*Event, error) {
	if s == nil || s.Store == nil {
		return nil, nil
	}
	return s.Store.GetAuditEvent(ctx, id)
}

// RecordFailure records a failed sensitive attempt. The error code is stable
// and never contains the raw error text.
func (s *Service) RecordFailure(ctx context.Context, actor int64, action, resourceType, resourceID, code string, details map[string]any) error {
	return s.Record(ctx, Event{
		ActorUserID: actor, Action: action, ResourceType: resourceType, ResourceID: resourceID,
		After: details, Outcome: "failure", ErrorCode: code,
	})
}

// MarshalDetails is a small helper for adapters that store JSONB.
func MarshalDetails(v map[string]any) ([]byte, error) {
	if len(v) == 0 {
		return nil, nil
	}
	return json.Marshal(v)
}
