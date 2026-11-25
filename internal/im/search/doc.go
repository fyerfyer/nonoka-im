// Package search contains the optional Meilisearch-backed message index. The
// IM app depends only on the narrow SearchIndex/IndexTask ports in im/app;
// this package provides the adapter and the durable relay.
package search

import (
	"encoding/json"
	"strconv"
	"strings"

	"nonoka-im/internal/im/domain"
)

// Document is the Meilisearch document for one message. Topic-scoped search
// means no per-user audience list is needed; the API authorizes the topic
// before querying.
type Document struct {
	ID        string `json:"id"`
	Topic     string `json:"topic"`
	MsgID     int64  `json:"msg_id"`
	TopicSeq  uint64 `json:"topic_seq"`
	SenderID  int64  `json:"sender_id"`
	Timestamp int64  `json:"timestamp"`
	MsgType   int32  `json:"msg_type"`
	HasFile   bool   `json:"has_file"`
	Content   string `json:"content"`
	Recalled  bool   `json:"recalled"`
}

// ContentText extracts the searchable text of a message. Text messages use
// their body; image/file messages use the attachment name so file search and
// the has_file filter remain useful without indexing binary content.
func ContentText(msgType int32, content []byte) string {
	switch msgType {
	case int32(domain.MessageTypeText):
		return strings.TrimSpace(strings.ToValidUTF8(string(content), "�"))
	case int32(domain.MessageTypeImage), int32(domain.MessageTypeFile):
		var meta struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(content, &meta) == nil {
			return strings.TrimSpace(meta.Name)
		}
	}
	return ""
}

// PlainText is kept as an alias for callers that only need text-message bodies.
func PlainText(msgType int32, content []byte) string {
	return ContentText(msgType, content)
}

// HasFile reports whether the message references an attachment.
func HasFile(msgType int32) bool {
	return msgType == int32(domain.MessageTypeImage) || msgType == int32(domain.MessageTypeFile)
}

// BuildDocument converts one message into an index document. It returns false
// for messages that should not be searchable (currently recalled messages and
// non-text messages with no text).
func BuildDocument(msg *domain.Message, contentText string) (Document, bool) {
	if msg == nil || msg.Recalled {
		return Document{}, false
	}
	doc := Document{
		ID:        docID(msg.ID),
		Topic:     msg.Topic,
		MsgID:     msg.ID,
		TopicSeq:  msg.TopicSeq,
		SenderID:  msg.SenderID,
		Timestamp: msg.Timestamp,
		MsgType:   int32(msg.Type),
		HasFile:   HasFile(int32(msg.Type)),
		Content:   contentText,
		Recalled:  msg.Recalled,
	}
	if doc.Content == "" && !doc.HasFile {
		return Document{}, false
	}
	return doc, true
}

// MarshalDocument serializes a document for the durable index outbox.
func MarshalDocument(doc Document) ([]byte, error) {
	return json.Marshal(doc)
}

func docID(msgID int64) string {
	return strconv.FormatInt(msgID, 10)
}
