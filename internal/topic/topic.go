// Package topic provides topic parsing and normalization helpers shared across
// the backend. It is intentionally dependency-free so that both the data and
// msgworker packages can use it without import cycles.
package topic

import (
	"fmt"
	"strconv"
	"strings"
)

// Type determines the type of a topic.
type Type int

const (
	TypeUnknown Type = iota
	TypeP2P
	TypeGroup
	TypeSystem
)

const (
	// Prefixes used in topic strings.
	PrefixP2P    = "p2p_"
	PrefixGroup  = "grp_"
	PrefixSystem = "sys_"
)

// ParseType parses a topic string to determine its type.
// Format: p2p_uid1_uid2, grp_groupid, sys_uid
func ParseType(topic string) Type {
	switch {
	case strings.HasPrefix(topic, PrefixP2P):
		return TypeP2P
	case strings.HasPrefix(topic, PrefixGroup):
		return TypeGroup
	case strings.HasPrefix(topic, PrefixSystem):
		return TypeSystem
	default:
		return TypeUnknown
	}
}

// ExtractUserIDsFromP2PTopic extracts the two user IDs from a P2P topic.
func ExtractUserIDsFromP2PTopic(topic string) (uid1, uid2 int64, err error) {
	parts := strings.Split(topic, "_")
	if len(parts) != 3 {
		return 0, 0, fmt.Errorf("invalid p2p topic format: %s", topic)
	}
	_, err = fmt.Sscanf(parts[1]+" "+parts[2], "%d %d", &uid1, &uid2)
	if err != nil {
		return 0, 0, fmt.Errorf("parse p2p topic user IDs: %w", err)
	}
	return uid1, uid2, nil
}

// NormalizeTopic canonicalizes a topic string.
// For P2P topics it orders the two user IDs ascending so that the same
// conversation always maps to a single topic regardless of which side initiates
// the message. Group and system topics are returned unchanged.
func NormalizeTopic(topic string) (string, error) {
	topicType := ParseType(topic)
	switch topicType {
	case TypeP2P:
		uid1, uid2, err := ExtractUserIDsFromP2PTopic(topic)
		if err != nil {
			return "", err
		}
		if uid1 > uid2 {
			uid1, uid2 = uid2, uid1
		}
		return fmt.Sprintf("p2p_%d_%d", uid1, uid2), nil
	case TypeGroup, TypeSystem:
		return topic, nil
	default:
		return "", fmt.Errorf("invalid topic format: %s", topic)
	}
}

// ExtractGroupID extracts the group ID from a group topic string.
func ExtractGroupID(topic string) (string, error) {
	if !strings.HasPrefix(topic, PrefixGroup) {
		return "", fmt.Errorf("invalid group topic format: %s", topic)
	}
	return strings.TrimPrefix(topic, PrefixGroup), nil
}

// AuthorizeDirect applies deterministic ACLs for topics whose membership is
// encoded in the name. P2P participants and system topic owners do not need an
// external relationship tuple; group topics return handled=false so callers can
// consult OpenFGA, which remains the source of truth for group membership.
func AuthorizeDirect(userID int64, raw string) (allowed bool, handled bool) {
	if userID <= 0 {
		return false, false
	}
	normalized, err := NormalizeTopic(raw)
	if err != nil {
		return false, false
	}
	switch ParseType(normalized) {
	case TypeP2P:
		left, right, err := ExtractUserIDsFromP2PTopic(normalized)
		if err != nil {
			return false, false
		}
		return userID == left || userID == right, true
	case TypeSystem:
		owner, err := strconv.ParseInt(strings.TrimPrefix(normalized, PrefixSystem), 10, 64)
		if err != nil {
			return false, false
		}
		return userID == owner, true
	default:
		return false, false
	}
}
