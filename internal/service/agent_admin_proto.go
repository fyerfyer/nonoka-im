package service

import (
	"context"
	"strconv"
	"time"

	agentv1 "nonoka-im/api/agent/v1"
	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/audit"
	"nonoka-im/internal/httpapi"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func quotaProto(q domain.Quota) *agentv1.Quota {
	out := &agentv1.Quota{
		Scope: q.Scope, ScopeKey: q.ScopeKey, UpdatedBy: q.UpdatedBy,
	}
	if q.MaxConcurrentRuns != nil {
		value := int32(*q.MaxConcurrentRuns)
		out.MaxConcurrentRuns = &value
	}
	if q.DailyRuns != nil {
		value := int32(*q.DailyRuns)
		out.DailyRuns = &value
	}
	if q.DailyTokens != nil {
		value := *q.DailyTokens
		out.DailyTokens = &value
	}
	if q.DailyCostUSD != nil {
		value := *q.DailyCostUSD
		out.DailyCostUsd = &value
	}
	if q.MaxToolTurns != nil {
		value := int32(*q.MaxToolTurns)
		out.MaxToolTurns = &value
	}
	if !q.UpdatedAt.IsZero() {
		out.UpdatedAt = timestamppb.New(q.UpdatedAt)
	}
	return out
}

func quotaLimitsProto(limits domain.QuotaLimits) *agentv1.QuotaLimits {
	return &agentv1.QuotaLimits{
		MaxConcurrentRuns: int32(limits.MaxConcurrentRuns),
		DailyRuns:         int32(limits.DailyRuns),
		DailyTokens:       limits.DailyTokens,
		DailyCostUsd:      limits.DailyCostUSD,
		MaxToolTurns:      int32(limits.MaxToolTurns),
	}
}

func usageProto(u domain.Usage, includeCost bool) *agentv1.Usage {
	out := &agentv1.Usage{
		Day: u.Day.Format("2006-01-02"), Scope: u.Scope, ScopeKey: u.ScopeKey,
		Runs: u.Runs, InputTokens: u.InputTokens, OutputTokens: u.OutputTokens,
	}
	if includeCost {
		value := u.CostUSD
		out.CostUsd = &value
	}
	return out
}

func (s *AgentHTTP) ListQuotas(ctx context.Context, req *agentv1.ListQuotasRequest) (*agentv1.ListQuotasReply, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.quota.AdminList(ctx, uid, req.GetScope(), int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		return nil, agentRPCError(err)
	}
	out := make([]*agentv1.Quota, 0, len(rows))
	for i := range rows {
		out = append(out, quotaProto(rows[i]))
	}
	return &agentv1.ListQuotasReply{
		Items: out,
		Defaults: map[string]*agentv1.QuotaLimits{
			domain.QuotaScopeUser:  quotaLimitsProto(s.quota.Cfg.User),
			domain.QuotaScopeAgent: quotaLimitsProto(s.quota.Cfg.Agent),
		},
	}, nil
}

func (s *AgentHTTP) UpdateQuota(ctx context.Context, req *agentv1.UpdateQuotaRequest) (*agentv1.Quota, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	row, err := s.quota.AdminUpdate(ctx, uid, req.GetScope(), req.GetKey(), domain.QuotaLimits{
		MaxConcurrentRuns: int(req.GetMaxConcurrentRuns()),
		DailyRuns:         int(req.GetDailyRuns()),
		DailyTokens:       req.GetDailyTokens(),
		DailyCostUSD:      req.GetDailyCostUsd(),
		MaxToolTurns:      int(req.GetMaxToolTurns()),
	})
	if err != nil {
		return nil, agentRPCError(err)
	}
	return quotaProto(*row), nil
}

func (s *AgentHTTP) ResetQuota(ctx context.Context, req *agentv1.ResetQuotaRequest) (*agentv1.StatusReply, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.quota.AdminReset(ctx, uid, req.GetScope(), req.GetKey()); err != nil {
		return nil, agentRPCError(err)
	}
	return &agentv1.StatusReply{Status: "reset"}, nil
}

func (s *AgentHTTP) TopUsage(ctx context.Context, req *agentv1.TopUsageRequest) (*agentv1.TopUsageReply, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	scope := req.GetScope()
	if scope == "" {
		scope = domain.QuotaScopeAgent
	}
	rows, err := s.quota.AdminTop(ctx, uid, scope, req.GetMetric(), int(req.GetLimit()))
	if err != nil {
		return nil, agentRPCError(err)
	}
	out := make([]*agentv1.Usage, 0, len(rows))
	for i := range rows {
		out = append(out, usageProto(rows[i], true))
	}
	return &agentv1.TopUsageReply{Items: out}, nil
}

func (s *AgentHTTP) GetAgentUsage(ctx context.Context, req *agentv1.GetAgentUsageRequest) (*agentv1.GetAgentUsageReply, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	_, _, manager, _, err := s.agents.Detail(ctx, uid, req.GetId())
	if err != nil {
		return nil, agentRPCError(err)
	}
	days := int(req.GetDays())
	if days <= 0 {
		days = 7
	}
	rows, total, err := s.quota.Usage(ctx, domain.QuotaScopeAgent, strconv.FormatInt(req.GetId(), 10), days)
	if err != nil {
		return nil, agentRPCError(err)
	}
	out := make([]*agentv1.Usage, 0, len(rows))
	for i := range rows {
		out = append(out, usageProto(rows[i], manager))
	}
	return &agentv1.GetAgentUsageReply{
		Days: out, Total: usageProto(total, manager), CanSeeCost: manager,
	}, nil
}

func auditStruct(value map[string]any) *structpb.Struct {
	if len(value) == 0 {
		return nil
	}
	out, err := structpb.NewStruct(value)
	if err != nil {
		return nil
	}
	return out
}

func auditEventProto(ev audit.Event) *agentv1.AuditEvent {
	out := &agentv1.AuditEvent{
		Id: ev.ID, ActorUserId: ev.ActorUserID, Action: ev.Action,
		ResourceType: ev.ResourceType, ResourceId: ev.ResourceID,
		Before: auditStruct(ev.Before), After: auditStruct(ev.After),
		RequestId: ev.RequestID, TraceId: ev.TraceID, Ip: ev.IP,
		UserAgent: ev.UserAgent, Outcome: ev.Outcome, ErrorCode: ev.ErrorCode,
	}
	if !ev.CreatedAt.IsZero() {
		out.CreatedAt = timestamppb.New(ev.CreatedAt)
	}
	return out
}

func (s *AgentHTTP) ListAuditEvents(ctx context.Context, req *agentv1.ListAuditEventsRequest) (*agentv1.ListAuditEventsReply, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	filter := audit.Filter{
		ActorUserID: req.GetActor(), Action: req.GetAction(),
		ResourceType: req.GetResourceType(), ResourceID: req.GetResourceId(),
	}
	if from := req.GetFrom(); from != "" {
		if parsed, parseErr := time.Parse(time.RFC3339, from); parseErr == nil {
			filter.From = &parsed
		}
	}
	if to := req.GetTo(); to != "" {
		if parsed, parseErr := time.Parse(time.RFC3339, to); parseErr == nil {
			filter.To = &parsed
		}
	}
	// Audit is platform-admin only.
	if err := s.quota.RequireAdmin(ctx, uid); err != nil {
		return nil, agentRPCError(err)
	}
	rows, total, err := s.auditor.List(ctx, filter, int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		return nil, agentRPCError(err)
	}
	out := make([]*agentv1.AuditEvent, 0, len(rows))
	for i := range rows {
		out = append(out, auditEventProto(rows[i]))
	}
	return &agentv1.ListAuditEventsReply{Items: out, Total: total}, nil
}

func (s *AgentHTTP) GetAuditEvent(ctx context.Context, req *agentv1.GetAuditEventRequest) (*agentv1.AuditEvent, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.quota.RequireAdmin(ctx, uid); err != nil {
		return nil, agentRPCError(err)
	}
	event, err := s.auditor.Get(ctx, req.GetId())
	if err != nil {
		return nil, agentRPCError(err)
	}
	if event == nil {
		return nil, agentRPCError(httpapi.NotFound("audit event not found"))
	}
	return auditEventProto(*event), nil
}
