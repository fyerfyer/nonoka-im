package service

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"nonoka-im/internal/audit"
	"nonoka-im/internal/authz"
	"nonoka-im/internal/httpapi"
	"nonoka-im/internal/messaging"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

// OutboxAdminHTTP exposes failed agent-outbox rows for the platform admin DLQ
// view and replay action.
type OutboxAdminHTTP struct {
	admin   messaging.OutboxAdmin
	authz   authz.PlatformAuthorizer
	auditor *audit.Service
}

func NewOutboxAdminHTTP(admin messaging.OutboxAdmin, az authz.PlatformAuthorizer, auditor *audit.Service) *OutboxAdminHTTP {
	return &OutboxAdminHTTP{admin: admin, authz: az, auditor: auditor}
}

func (s *OutboxAdminHTTP) handle(c khttp.Context, fn func(context.Context) (any, error)) error {
	h := c.Middleware(func(ctx context.Context, _ interface{}) (interface{}, error) {
		uid := extractUserIDFromContext(ctx)
		if uid <= 0 {
			writeAgentJSON(c.Response(), http.StatusUnauthorized, map[string]string{"code": "UNAUTHORIZED", "message": "authentication required"})
			return nil, nil
		}
		if s.authz == nil || s.authz.CanManagePlatform(ctx, uid) != nil {
			writeAgentJSON(c.Response(), http.StatusForbidden, map[string]string{"code": "FORBIDDEN", "message": "platform admin required"})
			return nil, nil
		}
		body, err := fn(ctx)
		if err != nil {
			status := http.StatusInternalServerError
			code := "INTERNAL"
			if errors.Is(err, errInvalidOutboxID) {
				status, code = http.StatusBadRequest, "INVALID_ARGUMENT"
			}
			writeAgentJSON(c.Response(), status, map[string]string{"code": code, "message": err.Error()})
			return nil, nil
		}
		if body != nil {
			writeAgentJSON(c.Response(), http.StatusOK, body)
		}
		return nil, nil
	})
	_, err := h(c, nil)
	return err
}

var errInvalidOutboxID = errors.New("invalid outbox id")

func (s *OutboxAdminHTTP) ListFailed(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context) (any, error) {
		limit, _ := strconv.Atoi(c.Request().URL.Query().Get("limit"))
		rows, err := s.admin.ListFailed(ctx, limit)
		if err != nil {
			return nil, err
		}
		return httpapi.Page[messaging.FailedEvent]{Items: rows, HasMore: false}, nil
	})
}

func (s *OutboxAdminHTTP) Replay(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context) (any, error) {
		id := c.Vars().Get("id")
		if err := s.admin.Replay(ctx, id); err != nil {
			return nil, errInvalidOutboxID
		}
		s.auditReplay(ctx, []string{id}, 1)
		return map[string]any{"status": "replayed", "id": id}, nil
	})
}

func (s *OutboxAdminHTTP) ReplayMany(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context) (any, error) {
		var input struct {
			IDs []string `json:"ids"`
		}
		if err := decodeJSONBody(c.Request(), &input); err != nil {
			return nil, errInvalidOutboxID
		}
		if len(input.IDs) == 0 || len(input.IDs) > 200 {
			return nil, errInvalidOutboxID
		}
		n, err := s.admin.ReplayMany(ctx, input.IDs)
		if err != nil {
			return nil, errInvalidOutboxID
		}
		s.auditReplay(ctx, input.IDs, n)
		return map[string]any{"status": "replayed", "count": n}, nil
	})
}

func (s *OutboxAdminHTTP) auditReplay(ctx context.Context, ids []string, replayed int) {
	if s.auditor == nil {
		return
	}
	if len(ids) > 50 {
		ids = ids[:50]
	}
	_ = s.auditor.Record(ctx, audit.Event{
		ActorUserID: extractUserIDFromContext(ctx), Action: "outbox.replay",
		ResourceType: "agent_outbox", ResourceID: "batch",
		After: map[string]any{"ids": ids, "replayed": replayed},
	})
}
