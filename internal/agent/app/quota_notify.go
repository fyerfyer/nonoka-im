package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"nonoka-im/internal/agent/tools"
)

// QuotaNotifier sends the single, deduplicated "agent is over quota" message
// for a conversation. The run row remains the durable source of truth for
// operators even when a notice is suppressed.
type QuotaNotifier struct {
	Quota  *QuotaService
	Sender ReplySender
}

func (n *QuotaNotifier) Notify(ctx context.Context, agentID, botID, actorID int64, topic, eventID, code, message string) {
	if n == nil || n.Quota == nil || n.Sender == nil || agentID <= 0 || topic == "" {
		return
	}
	ok, err := n.Quota.ShouldNotify(ctx, agentID, topic, code)
	if err != nil || !ok {
		return
	}
	if message == "" {
		message = "This agent is temporarily unavailable because it reached a usage limit."
	}
	sendCtx := tools.WithRunContext(ctx, tools.RunContext{
		AgentID: agentID, BotID: botID, ActorUserID: actorID, Topic: topic, EventID: eventID,
	})
	sendCtx, cancel := context.WithTimeout(context.WithoutCancel(sendCtx), 10*time.Second)
	defer cancel()
	_ = n.Sender.Send(sendCtx, topic, message, quotaNoticeClientID(eventID, agentID, code))
}

func quotaNoticeClientID(eventID string, agentID int64, code string) string {
	h := sha256.Sum256(fmt.Appendf(nil, "%s:%d:quota:%s", eventID, agentID, code))
	return "agent-quota-" + hex.EncodeToString(h[:16])
}
