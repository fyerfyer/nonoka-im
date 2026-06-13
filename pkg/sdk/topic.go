package sdk

import (
	"fmt"
	"strconv"
	"strings"
)

// P2PTopic returns the canonical topic name for a one-to-one conversation.
// The two user IDs are ordered ascending so that both sides map to the same
// topic string, matching the server-side normalization.
func P2PTopic(userID1, userID2 int64) string {
	if userID1 > userID2 {
		userID1, userID2 = userID2, userID1
	}
	return fmt.Sprintf("p2p_%d_%d", userID1, userID2)
}

// ParseP2PTopic parses a p2p topic string and returns the two user IDs.
func ParseP2PTopic(topic string) (uid1, uid2 int64, err error) {
	parts := strings.Split(topic, "_")
	if len(parts) != 3 || parts[0] != "p2p" {
		return 0, 0, fmt.Errorf("invalid p2p topic: %s", topic)
	}
	uid1, err = strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid p2p topic uid1: %w", err)
	}
	uid2, err = strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid p2p topic uid2: %w", err)
	}
	return uid1, uid2, nil
}

// NormalizeTopic canonicalizes a topic string.
// For P2P topics it orders the two user IDs ascending. Group and system topics
// are returned unchanged.
func NormalizeTopic(topic string) (string, error) {
	if !strings.HasPrefix(topic, "p2p_") {
		return topic, nil
	}
	uid1, uid2, err := ParseP2PTopic(topic)
	if err != nil {
		return "", err
	}
	return P2PTopic(uid1, uid2), nil
}
