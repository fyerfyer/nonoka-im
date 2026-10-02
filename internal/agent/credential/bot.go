package credential

import (
	"fmt"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
)

func Sign(secret []byte, agentID, botID int64, topic string) (string, error) {
	if len(secret) < 32 || agentID <= 0 || botID <= 0 || topic == "" {
		return "", fmt.Errorf("invalid bot credential configuration")
	}
	now := time.Now()
	claims := jwt.MapClaims{"user_id": botID, "agent_id": agentID, "topic": topic, "scope": "agent:send", "iat": now.Unix(), "exp": now.Add(2 * time.Minute).Unix()}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token.Header["typ"] = "agent-bot"
	return token.SignedString(secret)
}
