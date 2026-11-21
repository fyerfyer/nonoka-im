package domain

import "nonoka-im/internal/topic"

// Topic is a validated conversation topic. The raw string form is the only
// representation persisted in MongoDB, Kafka and the WebSocket protocol.
type Topic struct {
	raw string
	typ topic.Type
}

// NewTopic validates and normalizes a topic string.
func NewTopic(raw string) (Topic, error) {
	normalized, err := topic.NormalizeTopic(raw)
	if err != nil {
		return Topic{}, err
	}
	return Topic{raw: normalized, typ: topic.ParseType(normalized)}, nil
}

func (t Topic) String() string   { return t.raw }
func (t Topic) Type() topic.Type { return t.typ }
func (t Topic) IsZero() bool     { return t.raw == "" }

// Topic type aliases keep the IM domain self-describing without duplicating
// the dependency-free topic parser shared with authz and the agent layer.
type TopicType = topic.Type

const (
	TopicTypeUnknown = topic.TypeUnknown
	TopicTypeP2P     = topic.TypeP2P
	TopicTypeGroup   = topic.TypeGroup
	TopicTypeSystem  = topic.TypeSystem
)

func ParseTopicType(raw string) TopicType { return topic.ParseType(raw) }

func NormalizeTopic(raw string) (string, error) { return topic.NormalizeTopic(raw) }

func ExtractUserIDsFromP2PTopic(raw string) (uid1, uid2 int64, err error) {
	return topic.ExtractUserIDsFromP2PTopic(raw)
}

func ExtractGroupID(raw string) (string, error) { return topic.ExtractGroupID(raw) }
