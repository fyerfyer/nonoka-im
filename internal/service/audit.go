package service

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"nonoka-im/internal/audit"
	"nonoka-im/internal/httpapi"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

type AuditEventView struct {
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

func (s *AgentHTTP) ListAuditEvents(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		offset, _ := strconv.Atoi(q.Get("offset"))
		actor, _ := strconv.ParseInt(q.Get("actor"), 10, 64)
		filter := audit.Filter{
			ActorUserID: actor, Action: q.Get("action"),
			ResourceType: q.Get("resourceType"), ResourceID: q.Get("resourceId"),
		}
		if from := q.Get("from"); from != "" {
			if t, err := time.Parse(time.RFC3339, from); err == nil {
				filter.From = &t
			}
		}
		if to := q.Get("to"); to != "" {
			if t, err := time.Parse(time.RFC3339, to); err == nil {
				filter.To = &t
			}
		}
		// Audit is platform-admin only.
		if err := s.quota.RequireAdmin(ctx, extractUserIDFromContext(ctx)); err != nil {
			return err
		}
		rows, total, err := s.auditor.List(ctx, filter, limit, offset)
		if err != nil {
			return err
		}
		out := make([]AuditEventView, 0, len(rows))
		for i := range rows {
			out = append(out, auditEventView(rows[i]))
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{"items": out, "total": total})
		return nil
	})
}

func (s *AgentHTTP) GetAuditEvent(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		id, err := strconv.ParseUint(c.Vars().Get("id"), 10, 64)
		if err != nil || id == 0 {
			return invalidAgentInput("invalid audit event id")
		}
		if err := s.quota.RequireAdmin(ctx, extractUserIDFromContext(ctx)); err != nil {
			return err
		}
		ev, err := s.auditor.Get(ctx, id)
		if err != nil {
			return err
		}
		if ev == nil {
			return httpapi.NotFound("audit event not found")
		}
		writeAgentJSON(w, http.StatusOK, auditEventView(*ev))
		return nil
	})
}

func auditEventView(ev audit.Event) AuditEventView {
	return AuditEventView{
		ID: ev.ID, ActorUserID: ev.ActorUserID, Action: ev.Action,
		ResourceType: ev.ResourceType, ResourceID: ev.ResourceID,
		Before: ev.Before, After: ev.After, RequestID: ev.RequestID, TraceID: ev.TraceID,
		IP: ev.IP, UserAgent: ev.UserAgent, Outcome: ev.Outcome, ErrorCode: ev.ErrorCode,
		CreatedAt: ev.CreatedAt,
	}
}
