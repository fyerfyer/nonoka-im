package service

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"nonoka-im/internal/httpapi"
	"nonoka-im/internal/messaging"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// OutboxAdminHTTP exposes the failed agent-outbox rows for the platform admin
// DLQ view and replay action. It is read/replay only: the msgworker process
// remains the owner of publishing.
type OutboxAdminHTTP struct{ admin *messaging.OutboxAdmin }

func NewOutboxAdminHTTP(admin *messaging.OutboxAdmin) *OutboxAdminHTTP {
	return &OutboxAdminHTTP{admin: admin}
}

func (s *OutboxAdminHTTP) handle(c khttp.Context, fn func(context.Context) (any, error)) error {
	h := c.Middleware(func(ctx context.Context, _ interface{}) (interface{}, error) {
		uid := extractUserIDFromContext(ctx)
		if uid <= 0 {
			writeAgentJSON(c.Response(), http.StatusUnauthorized, map[string]string{"code": "UNAUTHORIZED", "message": "authentication required"})
			return nil, nil
		}
		if !isAgentAdmin(uid) {
			writeAgentJSON(c.Response(), http.StatusForbidden, map[string]string{"code": "FORBIDDEN", "message": "platform admin required"})
			return nil, nil
		}
		body, err := fn(ctx)
		if err != nil {
			status := http.StatusInternalServerError
			code := "INTERNAL"
			if errors.Is(err, bson.ErrInvalidHex) {
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
			return nil, err
		}
		return map[string]any{"status": "replayed", "id": id}, nil
	})
}
