package realtime

import (
	"time"

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
		TriggerMsgId: e.TriggerMsgID, Delta: e.Delta, Tool: e.Tool,
		ToolCallId: e.ToolCallID, Summary: e.Summary, Status: e.Status,
		ApprovalId: e.ApprovalID, ReplyMsgId: e.ReplyMsgID, ErrorCode: e.ErrorCode,
		ErrorClass: e.ErrorClass, ErrorMessage: e.ErrorMessage, Retryable: e.Retryable,
		InputTokens: int32(e.InputTokens), OutputTokens: int32(e.OutputTokens),
		TimestampMs: e.TimestampMs, ClientMsgId: e.ClientMessageID,
		RetryAfterAtMs: parseRetryAt(e.RetryAfterAt),
		Citations:      citationsToProto(e.Citations),
	}
}

func citationsToProto(citations []RunCitation) []*pb.AgentRunCitation {
	if len(citations) == 0 {
		return nil
	}
	out := make([]*pb.AgentRunCitation, 0, len(citations))
	for _, c := range citations {
		out = append(out, &pb.AgentRunCitation{
			MsgId: c.MsgID, Topic: c.Topic, TopicSeq: c.TopicSeq,
			SenderId: c.SenderID, Snippet: c.Snippet,
		})
	}
	return out
}

func citationsFromProto(citations []*pb.AgentRunCitation) []RunCitation {
	if len(citations) == 0 {
		return nil
	}
	out := make([]RunCitation, 0, len(citations))
	for _, c := range citations {
		if c == nil {
			continue
		}
		out = append(out, RunCitation{
			MsgID: c.GetMsgId(), Topic: c.GetTopic(), TopicSeq: c.GetTopicSeq(),
			SenderID: c.GetSenderId(), Snippet: c.GetSnippet(),
		})
	}
	return out
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
		TriggerMsgID: event.GetTriggerMsgId(), Delta: event.GetDelta(),
		Tool: event.GetTool(), ToolCallID: event.GetToolCallId(), Summary: event.GetSummary(),
		Status: event.GetStatus(), ApprovalID: event.GetApprovalId(), ReplyMsgID: event.GetReplyMsgId(),
		ErrorCode: event.GetErrorCode(), ErrorClass: event.GetErrorClass(), ErrorMessage: event.GetErrorMessage(),
		Retryable: event.GetRetryable(), RetryAfterAt: formatRetryAt(event.GetRetryAfterAtMs()),
		InputTokens: int(event.GetInputTokens()), OutputTokens: int(event.GetOutputTokens()),
		TimestampMs: event.GetTimestampMs(), ClientMessageID: event.GetClientMsgId(),
		Citations: citationsFromProto(event.GetCitations()),
	}
}

func parseRetryAt(raw string) int64 {
	if raw == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return 0
	}
	return t.UnixMilli()
}

func formatRetryAt(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}
