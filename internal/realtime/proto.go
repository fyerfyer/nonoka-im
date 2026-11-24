package realtime

import (
	pb "nonoka-im/api/im/v1"
)

var runEventTypeToProto = map[RunEventType]pb.AgentRunEventType{
	RunStarted:          pb.AgentRunEventType_AGENT_RUN_STARTED,
	RunDelta:            pb.AgentRunEventType_AGENT_RUN_DELTA,
	RunToolStarted:      pb.AgentRunEventType_AGENT_RUN_TOOL_STARTED,
	RunToolFinished:     pb.AgentRunEventType_AGENT_RUN_TOOL_FINISHED,
	RunApprovalNeeded:   pb.AgentRunEventType_AGENT_RUN_APPROVAL_REQUIRED,
	RunApprovalResolved: pb.AgentRunEventType_AGENT_RUN_APPROVAL_RESOLVED,
	RunCompleted:        pb.AgentRunEventType_AGENT_RUN_COMPLETED,
	RunFailed:           pb.AgentRunEventType_AGENT_RUN_FAILED,
	RunCancelled:        pb.AgentRunEventType_AGENT_RUN_CANCELLED,
	RunHeartbeat:        pb.AgentRunEventType_AGENT_RUN_HEARTBEAT,
}

var protoToRunEventType = func() map[pb.AgentRunEventType]RunEventType {
	out := make(map[pb.AgentRunEventType]RunEventType, len(runEventTypeToProto))
	for key, value := range runEventTypeToProto {
		out[value] = key
	}
	return out
}()

// ToProto converts the shared event contract to the WebSocket payload.
func (e RunEvent) ToProto() *pb.AgentRunEvent {
	eventType, ok := runEventTypeToProto[e.Type]
	if !ok {
		eventType = pb.AgentRunEventType_AGENT_RUN_EVENT_UNKNOWN
	}
	return &pb.AgentRunEvent{
		RunId: e.RunID, Seq: e.Seq, Type: eventType, Topic: e.Topic,
		AgentId: e.AgentID, BotUserId: e.BotUserID, ActorUserId: e.ActorUserID,
		TriggerMsgId: e.TriggerMsgID, Test: e.Test, Delta: e.Delta, Tool: e.Tool,
		ToolCallId: e.ToolCallID, Summary: e.Summary, Status: e.Status,
		ApprovalId: e.ApprovalID, ReplyMsgId: e.ReplyMsgID, ErrorCode: e.ErrorCode,
		ErrorClass: e.ErrorClass, Retryable: e.Retryable, InputTokens: int32(e.InputTokens),
		OutputTokens: int32(e.OutputTokens), TimestampMs: e.TimestampMs, ClientMsgId: e.ClientMessageID,
	}
}

// FromProto converts a WebSocket payload to the shared event contract.
func FromProto(event *pb.AgentRunEvent) RunEvent {
	if event == nil {
		return RunEvent{}
	}
	eventType, ok := protoToRunEventType[event.GetType()]
	if !ok {
		eventType = RunEventType("unknown")
	}
	return RunEvent{
		RunID: event.GetRunId(), Seq: event.GetSeq(), Type: eventType, Topic: event.GetTopic(),
		AgentID: event.GetAgentId(), BotUserID: event.GetBotUserId(), ActorUserID: event.GetActorUserId(),
		TriggerMsgID: event.GetTriggerMsgId(), Test: event.GetTest(), Delta: event.GetDelta(),
		Tool: event.GetTool(), ToolCallID: event.GetToolCallId(), Summary: event.GetSummary(),
		Status: event.GetStatus(), ApprovalID: event.GetApprovalId(), ReplyMsgID: event.GetReplyMsgId(),
		ErrorCode: event.GetErrorCode(), ErrorClass: event.GetErrorClass(), Retryable: event.GetRetryable(),
		InputTokens: int(event.GetInputTokens()), OutputTokens: int(event.GetOutputTokens()),
		TimestampMs: event.GetTimestampMs(), ClientMessageID: event.GetClientMsgId(),
	}
}
