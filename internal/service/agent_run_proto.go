package service

import (
	"context"
	"errors"

	agentv1 "nonoka-im/api/agent/v1"
	"nonoka-im/internal/agent/app"
	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/httpapi"
	"nonoka-im/internal/realtime"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *AgentHTTP) agentRPCUser(ctx context.Context) (int64, error) {
	uid := extractUserIDFromContext(ctx)
	if uid <= 0 {
		return 0, kerrors.Unauthorized("UNAUTHORIZED", "authentication required")
	}
	return uid, nil
}

func agentRPCError(err error) error {
	if err == nil {
		return nil
	}
	adapted := agentHTTPError(err)
	var e *httpapi.Error
	if errors.As(adapted, &e) {
		return kerrors.New(e.Status, e.Code, e.Message)
	}
	return kerrors.InternalServer("INTERNAL", "internal error")
}

func agentRunProto(run *domain.AgentRun) *agentv1.AgentRun {
	if run == nil {
		return nil
	}
	out := &agentv1.AgentRun{
		Id: run.ID, EventId: run.EventID, AgentId: run.AgentID, AgentVersion: run.AgentVersion,
		SessionId: run.SessionID, ActorUserId: run.ActorUserID, Topic: run.Topic,
		MsgId: run.MsgID, TopicSeq: run.TopicSeq, Status: run.Status,
		ErrorCode: run.ErrorCode, ErrorClass: run.ErrorClass, Attempts: int32(run.Attempts),
		ReplyMsgId: run.ReplyMsgID, ReplyClientMsgId: run.ReplyClientMsgID,
		CheckpointId: run.CheckpointID, InputTokens: int32(run.InputTokens),
		OutputTokens: int32(run.OutputTokens), TotalTokens: int32(run.TotalTokens),
		ModelKey: run.ModelKey, EstimatedCostUsd: run.EstimatedCostUSD, TraceId: run.TraceID,
		LastError: app.PublicFailureMessage(run.ErrorCode, run.LastError),
	}
	if run.LeaseUntil != nil {
		out.LeaseUntil = timestamppb.New(*run.LeaseUntil)
	}
	if run.NextAttemptAt != nil {
		out.NextAttemptAt = timestamppb.New(*run.NextAttemptAt)
	}
	if run.SideEffectAt != nil {
		out.SideEffectAt = timestamppb.New(*run.SideEffectAt)
	}
	if run.RetryAfterAt != nil {
		out.RetryAfterAt = timestamppb.New(*run.RetryAfterAt)
	}
	if !run.CreatedAt.IsZero() {
		out.CreatedAt = timestamppb.New(run.CreatedAt)
	}
	if run.StartedAt != nil {
		out.StartedAt = timestamppb.New(*run.StartedAt)
	}
	if run.FinishedAt != nil {
		out.FinishedAt = timestamppb.New(*run.FinishedAt)
	}
	return out
}

func agentRunStepProto(step *domain.AgentRunStep) *agentv1.AgentRunStep {
	if step == nil {
		return nil
	}
	out := &agentv1.AgentRunStep{
		Id: step.ID, Seq: step.Seq, Type: step.Type, Tool: step.Tool,
		ToolCallId: step.ToolCallID, Summary: step.Summary, Status: step.Status,
	}
	if !step.CreatedAt.IsZero() {
		out.CreatedAt = timestamppb.New(step.CreatedAt)
	}
	return out
}

func toolInvocationProto(t *domain.ToolInvocation) *agentv1.ToolInvocation {
	return &agentv1.ToolInvocation{
		Id: t.ID, ToolCallId: t.ToolCallID, Tool: t.Tool, Status: t.Status,
		ResultSummary: t.ResultSummary, ErrorCode: t.ErrorCode,
		CreatedAt: timestamppb.New(t.CreatedAt), UpdatedAt: timestamppb.New(t.UpdatedAt),
	}
}

func citationProto(c *domain.AgentRunCitation) *agentv1.Citation {
	return &agentv1.Citation{
		MsgId: c.MsgID, Topic: c.Topic, TopicSeq: c.TopicSeq,
		SenderId: c.SenderID, Snippet: c.Snippet,
	}
}

func approvalProto(a *domain.AgentApproval) *agentv1.AgentApproval {
	if a == nil {
		return nil
	}
	out := &agentv1.AgentApproval{
		Id: a.ID, RunId: a.RunID, ToolCallId: a.ToolCallID, Tool: a.Tool,
		ArgsSummary: a.ArgsSummary, ArgsHash: a.ArgsHash, ApproverId: a.ApproverID,
		Status: a.Status, DecidedBy: a.DecidedBy,
	}
	if !a.ExpiresAt.IsZero() {
		out.ExpiresAt = timestamppb.New(a.ExpiresAt)
	}
	if a.DecidedAt != nil {
		out.DecidedAt = timestamppb.New(*a.DecidedAt)
	}
	if a.ConsumedAt != nil {
		out.ConsumedAt = timestamppb.New(*a.ConsumedAt)
	}
	if !a.CreatedAt.IsZero() {
		out.CreatedAt = timestamppb.New(a.CreatedAt)
	}
	if !a.UpdatedAt.IsZero() {
		out.UpdatedAt = timestamppb.New(a.UpdatedAt)
	}
	return out
}

func runEventProto(event realtime.RunEvent) *agentv1.RunEvent {
	out := &agentv1.RunEvent{
		RunId: event.RunID, Seq: event.Seq, Type: string(event.Type), Topic: event.Topic,
		AgentId: event.AgentID, BotUserId: event.BotUserID, ActorUserId: event.ActorUserID,
		TriggerMsgId: event.TriggerMsgID, Delta: event.Delta, Tool: event.Tool,
		ToolCallId: event.ToolCallID, Summary: event.Summary, Status: event.Status,
		ApprovalId: event.ApprovalID, ReplyMsgId: event.ReplyMsgID,
		ClientMessageId: event.ClientMessageID, ErrorCode: event.ErrorCode,
		ErrorClass: event.ErrorClass, ErrorMessage: event.ErrorMessage,
		RetryAfterAt: event.RetryAfterAt, Retryable: event.Retryable,
		InputTokens: int32(event.InputTokens), OutputTokens: int32(event.OutputTokens),
		TimestampMs: event.TimestampMs,
	}
	for i := range event.Citations {
		out.Citations = append(out.Citations, &agentv1.Citation{
			MsgId: event.Citations[i].MsgID, Topic: event.Citations[i].Topic,
			TopicSeq: event.Citations[i].TopicSeq, SenderId: event.Citations[i].SenderID,
			Snippet: event.Citations[i].Snippet,
		})
	}
	return out
}

func (s *AgentHTTP) GetRun(ctx context.Context, req *agentv1.GetRunRequest) (*agentv1.AgentRun, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	run, steps, tools, citations, err := s.runs.Get(ctx, uid, req.GetId())
	if err != nil {
		return nil, agentRPCError(err)
	}
	out := agentRunProto(run)
	for i := range steps {
		out.Steps = append(out.Steps, agentRunStepProto(&steps[i]))
	}
	for i := range tools {
		out.ToolInvocations = append(out.ToolInvocations, toolInvocationProto(&tools[i]))
	}
	for i := range citations {
		out.Citations = append(out.Citations, citationProto(&citations[i]))
	}
	return out, nil
}

func (s *AgentHTTP) GetRunByReply(ctx context.Context, req *agentv1.GetRunByReplyRequest) (*agentv1.AgentRun, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	run, citations, err := s.runs.GetByReply(ctx, uid, req.GetMsgId(), req.GetClientMsgId())
	if err != nil {
		return nil, agentRPCError(err)
	}
	out := agentRunProto(run)
	for i := range citations {
		out.Citations = append(out.Citations, citationProto(&citations[i]))
	}
	return out, nil
}

func (s *AgentHTTP) GetRunByClient(ctx context.Context, req *agentv1.GetRunByClientRequest) (*agentv1.AgentRun, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetClientMsgId() == "" || len(req.GetClientMsgId()) > 128 {
		return nil, kerrors.BadRequest("INVALID_ARGUMENT", "invalid client message id")
	}
	run, citations, err := s.runs.GetByReply(ctx, uid, 0, req.GetClientMsgId())
	if err != nil {
		return nil, agentRPCError(err)
	}
	out := agentRunProto(run)
	for i := range citations {
		out.Citations = append(out.Citations, citationProto(&citations[i]))
	}
	return out, nil
}

func (s *AgentHTTP) GetRunEvents(ctx context.Context, req *agentv1.GetRunEventsRequest) (*agentv1.GetRunEventsReply, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	events, err := s.runs.Events(ctx, uid, req.GetId(), req.GetAfterSeq())
	if err != nil {
		return nil, agentRPCError(err)
	}
	out := &agentv1.GetRunEventsReply{NextSeq: req.GetAfterSeq()}
	for i := range events {
		out.Events = append(out.Events, runEventProto(events[i]))
		if events[i].Seq > out.NextSeq {
			out.NextSeq = events[i].Seq
		}
	}
	return out, nil
}

func (s *AgentHTTP) runAction(ctx context.Context, req *agentv1.RunActionRequest, action string) (*agentv1.RunActionReply, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.runs.Act(ctx, uid, req.GetId(), action, req.GetAcknowledgeSideEffect()); err != nil {
		return nil, agentRPCError(err)
	}
	return &agentv1.RunActionReply{Action: action}, nil
}

func (s *AgentHTTP) CancelRun(ctx context.Context, req *agentv1.RunActionRequest) (*agentv1.RunActionReply, error) {
	return s.runAction(ctx, req, "cancel")
}

func (s *AgentHTTP) RetryRun(ctx context.Context, req *agentv1.RunActionRequest) (*agentv1.RunActionReply, error) {
	return s.runAction(ctx, req, "retry")
}

func (s *AgentHTTP) SkipRun(ctx context.Context, req *agentv1.RunActionRequest) (*agentv1.RunActionReply, error) {
	return s.runAction(ctx, req, "skip")
}

func (s *AgentHTTP) ListApprovals(ctx context.Context, req *agentv1.ListApprovalsRequest) (*agentv1.ListApprovalsReply, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	limit := int(req.GetLimit())
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset, err := httpapi.Offset(req.GetCursor())
	if err != nil {
		return nil, kerrors.BadRequest("INVALID_CURSOR", "invalid cursor")
	}
	rows, total, err := s.approvals.List(ctx, uid, req.GetStatus(), limit, offset)
	if err != nil {
		return nil, agentRPCError(err)
	}
	items := make([]*agentv1.AgentApproval, 0, len(rows))
	for i := range rows {
		items = append(items, approvalProto(&rows[i]))
	}
	hasMore := int64(offset+len(rows)) < total
	return &agentv1.ListApprovalsReply{
		Items: items, Total: int32(total), HasMore: hasMore,
		NextCursor: agentNextCursor(offset, len(rows), total),
	}, nil
}

func (s *AgentHTTP) GetApproval(ctx context.Context, req *agentv1.GetApprovalRequest) (*agentv1.AgentApproval, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	approval, err := s.approvals.Get(ctx, uid, req.GetId())
	if err != nil {
		return nil, agentRPCError(err)
	}
	return approvalProto(approval), nil
}

func (s *AgentHTTP) approvalAction(ctx context.Context, req *agentv1.ApprovalActionRequest, action string) (*agentv1.ApprovalActionReply, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.approvals.Decide(ctx, uid, req.GetId(), action); err != nil {
		return nil, agentRPCError(err)
	}
	status := "approved"
	if action == "reject" {
		status = "rejected"
	}
	return &agentv1.ApprovalActionReply{Status: status}, nil
}

func (s *AgentHTTP) ApproveApproval(ctx context.Context, req *agentv1.ApprovalActionRequest) (*agentv1.ApprovalActionReply, error) {
	return s.approvalAction(ctx, req, "approve")
}

func (s *AgentHTTP) RejectApproval(ctx context.Context, req *agentv1.ApprovalActionRequest) (*agentv1.ApprovalActionReply, error) {
	return s.approvalAction(ctx, req, "reject")
}

func agentNextCursor(offset, count int, total int64) string {
	if int64(offset+count) >= total {
		return ""
	}
	return httpapi.EncodeCursor(offset + count)
}
