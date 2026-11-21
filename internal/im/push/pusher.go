// Package push adapts realtime gateway pushers to the IM application ports.
package push

import (
	"context"

	"nonoka-im/internal/im/app"
	"nonoka-im/internal/im/domain"
	"nonoka-im/internal/realtime"

	pb "nonoka-im/api/im/v1"
)

// Pusher maps domain push payloads to the protobuf wire contract.
type Pusher struct{ Impl realtime.Pusher }

func NewPusher(impl realtime.Pusher) *Pusher { return &Pusher{Impl: impl} }

func (p *Pusher) BatchPushToUsers(ctx context.Context, userIDs []int64, msg *domain.PushMessage) (int32, []int64, error) {
	return p.Impl.BatchPushToUsers(ctx, userIDs, toProtoPush(msg))
}

func (p *Pusher) PushReceiptToUser(ctx context.Context, userID int64, receipt *domain.SendReceipt) (int32, error) {
	return p.Impl.PushReceiptToUser(ctx, userID, toProtoReceipt(receipt))
}

func (p *Pusher) BatchPushReceiptsToUsers(ctx context.Context, items []domain.ReceiptBatchItem) (int32, []domain.ReceiptFailure, error) {
	protoItems := make([]*pb.ReceiptBatchItem, 0, len(items))
	for i := range items {
		protoItems = append(protoItems, &pb.ReceiptBatchItem{UserId: items[i].UserID, Receipt: toProtoReceipt(&items[i].Receipt)})
	}
	delivered, failed, err := p.Impl.BatchPushReceiptsToUsers(ctx, protoItems)
	out := make([]domain.ReceiptFailure, 0, len(failed))
	for _, item := range failed {
		out = append(out, domain.ReceiptFailure{
			UserID: item.GetUserId(),
			Receipt: domain.SendReceipt{
				ClientMsgID: item.GetReceipt().GetClientMsgId(), MsgID: item.GetReceipt().GetMsgId(),
				Topic: item.GetReceipt().GetTopic(), TopicSeq: item.GetReceipt().GetTopicSeq(), Timestamp: item.GetReceipt().GetTimestamp(),
			},
		})
	}
	return delivered, out, err
}

func (p *Pusher) Close() error { return p.Impl.Close() }

func toProtoPush(msg *domain.PushMessage) *pb.MessagePush {
	if msg == nil {
		return nil
	}
	return &pb.MessagePush{
		MsgId: msg.MsgID, Topic: msg.Topic, SenderId: msg.SenderID, MsgType: msg.MsgType,
		Content: msg.Content, Timestamp: msg.Timestamp, TopicSeq: msg.TopicSeq,
		ClientMsgId: msg.ClientMsgID, MentionedUserIds: msg.MentionedUserIDs, Recalled: msg.Recalled,
	}
}

func toProtoReceipt(receipt *domain.SendReceipt) *pb.SendReceipt {
	if receipt == nil {
		return nil
	}
	return &pb.SendReceipt{
		ClientMsgId: receipt.ClientMsgID, MsgId: receipt.MsgID, Topic: receipt.Topic,
		TopicSeq: receipt.TopicSeq, Timestamp: receipt.Timestamp,
	}
}

var _ app.Pusher = (*Pusher)(nil)
