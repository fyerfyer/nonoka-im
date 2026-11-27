package sdk

import (
	v1 "nonoka-im/api/im/v1"
)

func (c *RealtimeClient) handlePacket(packet *v1.Packet) {
	if packet.GetCmd() == v1.Command_CMD_HEARTBEAT {
		return
	}
	if ch, ok := c.pendingFor(packet.GetSeq()); ok {
		select {
		case ch <- packet:
		default:
		}
		return
	}
	switch packet.GetCmd() {
	case v1.Command_CMD_NOTIFY:
		push := packet.GetNotify()
		if push == nil {
			return
		}
		message := &Message{
			ID: push.GetMsgId(), Topic: push.GetTopic(), SenderID: push.GetSenderId(),
			Type: MessageType(push.GetMsgType()), Content: push.GetContent(),
			Timestamp: push.GetTimestamp(), TopicSeq: push.GetTopicSeq(),
			ClientMessageID: push.GetClientMsgId(), MentionedUserIDs: push.GetMentionedUserIds(),
			Recalled: push.GetRecalled(), Status: MessageStatusDelivered,
		}
		if !c.cfg.Realtime.DisableAutoAck {
			c.sendAck(message.ID, message.Topic, message.TopicSeq)
		}
		c.emit(RealtimeEvent{Type: RealtimeMessage, Message: message})
	case v1.Command_CMD_SEND_RECEIPT:
		receipt := packet.GetSendReceipt()
		if receipt != nil {
			c.emit(RealtimeEvent{Type: RealtimeSendReceipt, SendReceipt: &SendReceipt{
				ClientMessageID: receipt.GetClientMsgId(), MessageID: receipt.GetMsgId(),
				Topic: receipt.GetTopic(), TopicSeq: receipt.GetTopicSeq(), Timestamp: receipt.GetTimestamp(),
			}})
		}
	case v1.Command_CMD_DELIVERY_RECEIPT:
		receipt := packet.GetDeliveryReceipt()
		if receipt != nil {
			c.emit(RealtimeEvent{Type: RealtimeDeliveryReceipt, DeliveryReceipt: &DeliveryReceipt{
				Topic: receipt.GetTopic(), TopicSeq: receipt.GetTopicSeq(), MessageID: receipt.GetMsgId(),
			}})
		}
	case v1.Command_CMD_READ_RECEIPT:
		receipt := packet.GetReadReceipt()
		if receipt != nil {
			c.emit(RealtimeEvent{Type: RealtimeReadReceipt, ReadReceipt: &ReadReceipt{
				Topic: receipt.GetTopic(), UpToSeq: receipt.GetUpToSeq(), ReaderID: receipt.GetReaderId(),
			}})
		}
	case v1.Command_CMD_RECALL:
		notice := packet.GetRecallNotice()
		if notice != nil {
			c.emit(RealtimeEvent{Type: RealtimeRecall, Recall: &RecallNotice{
				Topic: notice.GetTopic(), TopicSeq: notice.GetTopicSeq(), MessageID: notice.GetMsgId(),
				SenderID: notice.GetSenderId(), RecalledAt: notice.GetRecalledAt(),
			}})
		}
	case v1.Command_CMD_AGENT_RUN_EVENT:
		if event := packet.GetAgentRunEvent(); event != nil {
			c.emit(RealtimeEvent{Type: RealtimeAgentRunEvent, RunEvent: runEventFromProto(event)})
		}
	}
}

func runEventFromProto(event *v1.AgentRunEvent) *RunEvent {
	typeMap := map[v1.AgentRunEventType]RunEventType{
		v1.AgentRunEventType_AGENT_RUN_STARTED:           RunEventStarted,
		v1.AgentRunEventType_AGENT_RUN_DELTA:             RunEventDelta,
		v1.AgentRunEventType_AGENT_RUN_TOOL_STARTED:      RunEventToolStarted,
		v1.AgentRunEventType_AGENT_RUN_TOOL_FINISHED:     RunEventToolFinished,
		v1.AgentRunEventType_AGENT_RUN_APPROVAL_REQUIRED: RunEventApprovalRequired,
		v1.AgentRunEventType_AGENT_RUN_APPROVAL_RESOLVED: RunEventApprovalResolved,
		v1.AgentRunEventType_AGENT_RUN_COMPLETED:         RunEventCompleted,
		v1.AgentRunEventType_AGENT_RUN_FAILED:            RunEventFailed,
		v1.AgentRunEventType_AGENT_RUN_CANCELLED:         RunEventCancelled,
		v1.AgentRunEventType_AGENT_RUN_HEARTBEAT:         RunEventHeartbeat,
	}
	return &RunEvent{
		RunID: event.GetRunId(), Seq: event.GetSeq(), Type: typeMap[event.GetType()],
		Topic: event.GetTopic(), AgentID: event.GetAgentId(), BotUserID: event.GetBotUserId(),
		ActorUserID: event.GetActorUserId(), TriggerMsgID: event.GetTriggerMsgId(),
		Delta: event.GetDelta(), Tool: event.GetTool(), ToolCallID: event.GetToolCallId(),
		Summary: event.GetSummary(), Status: event.GetStatus(), ApprovalID: event.GetApprovalId(),
		ReplyMsgID: event.GetReplyMsgId(), ClientMessageID: event.GetClientMsgId(),
		ErrorCode: event.GetErrorCode(), ErrorClass: event.GetErrorClass(),
		Retryable: event.GetRetryable(), InputTokens: int(event.GetInputTokens()),
		OutputTokens: int(event.GetOutputTokens()), TimestampMs: event.GetTimestampMs(),
	}
}
