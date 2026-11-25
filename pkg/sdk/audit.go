package sdk

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type AuditEvent struct {
	ID           uint64         `json:"id"`
	ActorUserID  int64          `json:"actorUserId"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resourceType"`
	ResourceID   string         `json:"resourceId"`
	Before       map[string]any `json:"before,omitempty"`
	After        map[string]any `json:"after,omitempty"`
	RequestID    string         `json:"requestId,omitempty"`
	TraceID      string         `json:"traceId,omitempty"`
	IP           string         `json:"ip,omitempty"`
	UserAgent    string         `json:"userAgent,omitempty"`
	Outcome      string         `json:"outcome"`
	ErrorCode    string         `json:"errorCode,omitempty"`
	CreatedAt    time.Time      `json:"createdAt"`
}

type AuditFilter struct {
	ActorUserID  int64
	Action       string
	ResourceType string
	ResourceID   string
	From         string
	To           string
	Limit        int
	Offset       int
}

type AuditResource struct {
	http *apiTransport
}

func (r *AuditResource) List(ctx context.Context, filter AuditFilter) ([]AuditEvent, int64, error) {
	query := url.Values{}
	if filter.ActorUserID > 0 {
		query.Set("actor", strconv.FormatInt(filter.ActorUserID, 10))
	}
	if filter.Action != "" {
		query.Set("action", filter.Action)
	}
	if filter.ResourceType != "" {
		query.Set("resourceType", filter.ResourceType)
	}
	if filter.ResourceID != "" {
		query.Set("resourceId", filter.ResourceID)
	}
	if filter.From != "" {
		query.Set("from", filter.From)
	}
	if filter.To != "" {
		query.Set("to", filter.To)
	}
	if filter.Limit > 0 {
		query.Set("limit", strconv.Itoa(filter.Limit))
	}
	if filter.Offset > 0 {
		query.Set("offset", strconv.Itoa(filter.Offset))
	}
	var reply struct {
		Items []AuditEvent `json:"items"`
		Total int64        `json:"total"`
	}
	if err := r.http.doJSON(ctx, http.MethodGet, "/v1/admin/audit-events", query, nil, &reply, true); err != nil {
		return nil, 0, err
	}
	return reply.Items, reply.Total, nil
}

func (r *AuditResource) Get(ctx context.Context, id uint64) (*AuditEvent, error) {
	var out AuditEvent
	if err := r.http.doJSON(ctx, http.MethodGet, "/v1/admin/audit-events/"+strconv.FormatUint(id, 10), nil, nil, &out, true); err != nil {
		return nil, err
	}
	return &out, nil
}
