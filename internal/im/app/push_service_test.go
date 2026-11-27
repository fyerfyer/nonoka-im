package app

import (
	"context"
	"sync"
	"testing"

	"nonoka-im/internal/im/domain"
)

type recordingPusher struct {
	mu    sync.Mutex
	users []int64
}

func (p *recordingPusher) BatchPushToUsers(_ context.Context, userIDs []int64, _ *domain.PushMessage) (int32, []int64, error) {
	p.mu.Lock()
	p.users = append(p.users, userIDs...)
	p.mu.Unlock()
	return int32(len(userIDs)), nil, nil
}
func (p *recordingPusher) PushReceiptToUser(context.Context, int64, *domain.SendReceipt) (int32, error) {
	return 1, nil
}
func (p *recordingPusher) BatchPushReceiptsToUsers(context.Context, []domain.ReceiptBatchItem) (int32, []domain.ReceiptFailure, error) {
	return 0, nil, nil
}
func (p *recordingPusher) Close() error { return nil }

func TestPushServiceIncludesSenderForMultiDevice(t *testing.T) {
	pusher := &recordingPusher{}
	svc := &PushService{Pusher: pusher}
	svc.deliverP2POrSystem(context.Background(), &domain.Message{
		ID: 1, Topic: "p2p_1_2", SenderID: 1, Type: domain.MessageTypeText,
		Content: []byte("hi"), TopicSeq: 1, ClientMsgID: "c1",
	}, []int64{2, 1})
	if len(pusher.users) != 2 || pusher.users[0] != 2 || pusher.users[1] != 1 {
		t.Fatalf("sender device was not included: %v", pusher.users)
	}
}
