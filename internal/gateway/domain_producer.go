package gateway

import (
	"context"

	v1 "nonoka-im/api/im/v1"
	"nonoka-im/internal/im/app"
	"nonoka-im/internal/im/domain"
)

// DomainProducer adapts the gateway Kafka producer to the IM application port.
type DomainProducer struct{ Producer MessageProducer }

func NewDomainProducer(producer MessageProducer) *DomainProducer {
	return &DomainProducer{Producer: producer}
}

func (p *DomainProducer) Produce(ctx context.Context, msg *domain.Message) error {
	if msg == nil {
		return nil
	}
	return p.Producer.Produce(ctx, &v1.UpstreamMessage{
		SenderId: msg.SenderID, Topic: msg.Topic, MsgType: int32(msg.Type),
		Content: msg.Content, ClientMsgId: msg.ClientMsgID, Timestamp: msg.Timestamp,
		MentionedUserIds: msg.MentionedUserIDs,
	})
}

var _ app.MessageProducer = (*DomainProducer)(nil)
