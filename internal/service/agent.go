package service

import (
	"context"

	"nonoka-im/internal/agent/domain"
)

// VerifyBotSend is called by the HTTP auth middleware for bot credentials.
func (s *AgentHTTP) VerifyBotSend(ctx context.Context, claims map[string]any) error {
	agentID, ok1 := claims["agent_id"].(float64)
	botID, ok2 := claims["user_id"].(float64)
	topic, ok3 := claims["topic"].(string)
	if !ok1 || !ok2 || !ok3 || claims["scope"] != "agent:send" {
		return domain.ErrAgentNotFound
	}
	return s.agents.VerifyBotSend(ctx, int64(agentID), int64(botID), topic)
}
