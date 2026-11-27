package service

import (
	"context"

	agentv1 "nonoka-im/api/agent/v1"
	"nonoka-im/internal/audit"
	"nonoka-im/internal/authz"
	"nonoka-im/internal/messaging"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// OutboxAdminHTTP exposes failed agent-outbox rows for the platform admin DLQ
// view and replay action.
type OutboxAdminHTTP struct {
	agentv1.UnimplementedAgentOutboxAdminServiceServer

	admin   messaging.OutboxAdmin
	authz   authz.PlatformAuthorizer
	auditor *audit.Service
}

func NewOutboxAdminHTTP(admin messaging.OutboxAdmin, az authz.PlatformAuthorizer, auditor *audit.Service) *OutboxAdminHTTP {
	return &OutboxAdminHTTP{admin: admin, authz: az, auditor: auditor}
}

func (s *OutboxAdminHTTP) requireAdmin(ctx context.Context) (int64, error) {
	uid := extractUserIDFromContext(ctx)
	if uid <= 0 {
		return 0, kerrors.Unauthorized("UNAUTHORIZED", "authentication required")
	}
	if s.authz == nil || s.authz.CanManagePlatform(ctx, uid) != nil {
		return 0, kerrors.Forbidden("FORBIDDEN", "platform admin required")
	}
	return uid, nil
}

func (s *OutboxAdminHTTP) ListFailedOutbox(ctx context.Context, req *agentv1.ListFailedOutboxRequest) (*agentv1.ListFailedOutboxReply, error) {
	if _, err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	rows, err := s.admin.ListFailed(ctx, int(req.GetLimit()))
	if err != nil {
		return nil, kerrors.InternalServer("INTERNAL", err.Error())
	}
	out := make([]*agentv1.FailedOutboxEvent, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		out = append(out, &agentv1.FailedOutboxEvent{
			Id: row.ID, EventId: row.EventID, Topic: row.Topic, TopicSeq: row.TopicSeq,
			Attempts: int32(row.Attempts), LastError: row.LastError, CreatedAt: row.CreatedAt,
		})
	}
	return &agentv1.ListFailedOutboxReply{Items: out}, nil
}

func (s *OutboxAdminHTTP) ReplayOutbox(ctx context.Context, req *agentv1.ReplayOutboxRequest) (*agentv1.ReplayOutboxReply, error) {
	uid, err := s.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	id := req.GetId()
	if err := s.admin.Replay(ctx, id); err != nil {
		return nil, kerrors.BadRequest("INVALID_ARGUMENT", "invalid outbox id")
	}
	s.auditReplay(ctx, uid, []string{id}, 1)
	return &agentv1.ReplayOutboxReply{Status: "replayed", Id: id}, nil
}

func (s *OutboxAdminHTTP) ReplayOutboxMany(ctx context.Context, req *agentv1.ReplayOutboxManyRequest) (*agentv1.ReplayOutboxManyReply, error) {
	uid, err := s.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	ids := req.GetIds()
	if len(ids) == 0 || len(ids) > 200 {
		return nil, kerrors.BadRequest("INVALID_ARGUMENT", "invalid outbox id list")
	}
	n, err := s.admin.ReplayMany(ctx, ids)
	if err != nil {
		return nil, kerrors.BadRequest("INVALID_ARGUMENT", "invalid outbox id")
	}
	s.auditReplay(ctx, uid, ids, n)
	return &agentv1.ReplayOutboxManyReply{Status: "replayed", Count: int32(n)}, nil
}

func (s *OutboxAdminHTTP) auditReplay(ctx context.Context, userID int64, ids []string, replayed int) {
	if s.auditor == nil {
		return
	}
	if len(ids) > 50 {
		ids = ids[:50]
	}
	_ = s.auditor.Record(ctx, audit.Event{
		ActorUserID: userID, Action: "outbox.replay",
		ResourceType: "agent_outbox", ResourceID: "batch",
		After: map[string]any{"ids": ids, "replayed": replayed},
	})
}
