package app

import (
	"context"
	"sync"
	"time"

	"nonoka-im/internal/im/domain"
	"nonoka-im/internal/metrics"

	"github.com/go-kratos/kratos/v2/log"
)

// PushService owns online delivery, receipts, bot filtering and retry
// scheduling. It is deliberately independent of the gateway transport.
type PushService struct {
	Pusher       Pusher
	RetryQueue   RetryQueue
	GroupMembers GroupMemberReader
	Bots         BotDirectory
	Metrics      *metrics.Metrics
	Log          *log.Helper

	receiptBatchSize    int
	receiptBatchTimeout time.Duration
	batcher             *receiptBatcher
}

func (s *PushService) log() *log.Helper {
	if s.Log != nil {
		return s.Log
	}
	return log.NewHelper(log.DefaultLogger)
}

// ConfigureReceipts sets the receipt batch size and flush interval. Must be
// called before Start.
func (s *PushService) ConfigureReceipts(batchSize int, timeout time.Duration) {
	if batchSize > 0 {
		s.receiptBatchSize = batchSize
	}
	if timeout > 0 {
		s.receiptBatchTimeout = timeout
	}
}

func (s *PushService) Start(ctx context.Context) {
	if s.batcher != nil || s.Pusher == nil {
		return
	}
	size, timeout := s.receiptBatchSize, s.receiptBatchTimeout
	if size <= 0 {
		size = 32
	}
	if timeout <= 0 {
		timeout = 10 * time.Millisecond
	}
	s.batcher = newReceiptBatcher(s, size, timeout, s.log().Logger())
	s.batcher.Start(ctx)
}

func (s *PushService) Stop() {
	if s.batcher != nil {
		s.batcher.Stop()
		s.batcher = nil
	}
}

// Deliver pushes one persisted message to recipients and sends the sender a
// persistence receipt.
func (s *PushService) Deliver(ctx context.Context, result HandleResult) {
	start := time.Now()
	defer func() {
		if s.Metrics != nil {
			s.Metrics.ObserveGrpcPushLatency(time.Since(start).Seconds())
		}
	}()
	msg := result.Message
	if result.IsGroup {
		s.deliverGroup(ctx, &msg)
	} else {
		s.deliverP2POrSystem(ctx, &msg, result.RecipientIDs)
	}
}

func (s *PushService) deliverP2POrSystem(ctx context.Context, msg *domain.Message, recipientIDs []int64) {
	push := buildPushMessage(msg)
	if len(recipientIDs) > 0 && s.Pusher != nil {
		// Include the sender: the sender may have another device connected.
		// The originating client deduplicates its local echo by client_msg_id.
		recipients := uniqueUserIDs(recipientIDs)
		recipients = s.humanRecipients(ctx, recipients)
		if len(recipients) > 0 {
			if _, failed, err := s.Pusher.BatchPushToUsers(ctx, recipients, push); err != nil {
				s.log().Warnf("push to online users failed: %v", err)
				s.handleFailedPushes(ctx, recipients, push)
			} else {
				s.handleFailedPushes(ctx, failed, push)
			}
		}
	}
	s.pushSendReceipt(msg)
}

func (s *PushService) deliverGroup(ctx context.Context, msg *domain.Message) {
	push := buildPushMessage(msg)
	mentions := s.humanRecipients(ctx, msg.MentionedUserIDs)
	if len(mentions) > 0 && s.Pusher != nil {
		if _, failed, err := s.Pusher.BatchPushToUsers(ctx, mentions, push); err != nil {
			s.log().Warnf("push mentions to online users failed: %v", err)
			s.handleFailedPushes(ctx, mentions, push)
		} else {
			s.handleFailedPushes(ctx, failed, push)
		}
	}
	if s.GroupMembers != nil && s.Pusher != nil {
		groupID, err := domain.ExtractGroupID(msg.Topic)
		if err != nil {
			s.log().Warnf("extract group id failed: %v", err)
			return
		}
		memberIDs, err := s.GroupMembers.GetGroupMembers(ctx, groupID)
		if err != nil {
			s.log().Warnf("get group members failed: %v", err)
			return
		}
		mentioned := make(map[int64]struct{}, len(msg.MentionedUserIDs))
		for _, id := range msg.MentionedUserIDs {
			mentioned[id] = struct{}{}
		}
		recipients := make([]int64, 0, len(memberIDs))
		for _, id := range memberIDs {
			if _, ok := mentioned[id]; ok {
				continue
			}
			recipients = append(recipients, id)
		}
		// Keep the sender in the fan-out so their other devices see the
		// message; clients dedupe their own local echo by client_msg_id.
		recipients = uniqueUserIDs(recipients)
		recipients = s.humanRecipients(ctx, recipients)
		if len(recipients) > 0 {
			if _, failed, err := s.Pusher.BatchPushToUsers(ctx, recipients, push); err != nil {
				s.log().Warnf("push group message to online users failed: %v", err)
			} else {
				s.handleFailedPushes(ctx, failed, push)
			}
		}
	}
	s.pushSendReceipt(msg)
}

func (s *PushService) pushSendReceipt(msg *domain.Message) {
	receipt := &domain.SendReceipt{
		ClientMsgID: msg.ClientMsgID, MsgID: msg.ID, Topic: msg.Topic,
		TopicSeq: msg.TopicSeq, Timestamp: time.Now().Unix(),
	}
	if s.batcher != nil {
		s.batcher.Add(msg.SenderID, receipt)
		return
	}
	if s.Pusher == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := s.Pusher.PushReceiptToUser(ctx, msg.SenderID, receipt); err != nil {
		s.log().Warnf("push send receipt to sender %d failed: %v", msg.SenderID, err)
	}
}

func buildPushMessage(msg *domain.Message) *domain.PushMessage {
	return &domain.PushMessage{
		MsgID: msg.ID, Topic: msg.Topic, SenderID: msg.SenderID, MsgType: int32(msg.Type),
		Content: msg.Content, Timestamp: msg.Timestamp, TopicSeq: msg.TopicSeq,
		ClientMsgID: msg.ClientMsgID, MentionedUserIDs: msg.MentionedUserIDs, Recalled: msg.Recalled,
	}
}

func uniqueUserIDs(ids []int64) []int64 {
	out := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func (s *PushService) humanRecipients(ctx context.Context, ids []int64) []int64 {
	if s.Bots == nil || !s.Bots.Ready() || len(ids) == 0 {
		return ids
	}
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		bot, err := s.Bots.IsBot(ctx, id)
		if err != nil || !bot {
			out = append(out, id)
		}
	}
	return out
}

func (s *PushService) handleFailedPushes(ctx context.Context, failed []int64, msg *domain.PushMessage) {
	if len(failed) == 0 || s.RetryQueue == nil {
		return
	}
	for _, userID := range failed {
		if err := s.RetryQueue.ScheduleRetry(ctx, userID, msg); err != nil {
			s.log().Warnf("schedule push retry failed: user_id=%d err=%v", userID, err)
		}
	}
}

// receiptBatcher aggregates per-user send receipts to reduce gateway RPCs.
type receiptBatcher struct {
	push         *PushService
	batchSize    int
	batchTimeout time.Duration
	log          *log.Helper

	mu           sync.Mutex
	pending      map[int64][]*domain.SendReceipt
	pendingCount int
	stopCh       chan struct{}
	stopOnce     sync.Once
	wg           sync.WaitGroup
}

func newReceiptBatcher(push *PushService, batchSize int, batchTimeout time.Duration, logger log.Logger) *receiptBatcher {
	return &receiptBatcher{
		push: push, batchSize: batchSize, batchTimeout: batchTimeout,
		pending: make(map[int64][]*domain.SendReceipt),
		stopCh:  make(chan struct{}),
		log:     log.NewHelper(logger),
	}
}

func (b *receiptBatcher) Start(ctx context.Context) {
	b.wg.Add(1)
	go b.loop(ctx)
}

func (b *receiptBatcher) Stop() {
	b.stopOnce.Do(func() { close(b.stopCh) })
	b.wg.Wait()
	b.flush()
}

func (b *receiptBatcher) loop(ctx context.Context) {
	defer b.wg.Done()
	ticker := time.NewTicker(b.batchTimeout)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-b.stopCh:
			return
		case <-ticker.C:
			b.flush()
		}
	}
}

func (b *receiptBatcher) Add(userID int64, receipt *domain.SendReceipt) {
	if receipt == nil {
		return
	}
	b.mu.Lock()
	b.pending[userID] = append(b.pending[userID], receipt)
	b.pendingCount++
	count := b.pendingCount
	b.mu.Unlock()
	if count >= b.batchSize {
		b.flush()
	}
}

func (b *receiptBatcher) flush() {
	b.mu.Lock()
	if len(b.pending) == 0 {
		b.mu.Unlock()
		return
	}
	pending := b.pending
	b.pending = make(map[int64][]*domain.SendReceipt)
	b.pendingCount = 0
	b.mu.Unlock()

	items := make([]domain.ReceiptBatchItem, 0, len(pending))
	for uid, receipts := range pending {
		for _, receipt := range receipts {
			items = append(items, domain.ReceiptBatchItem{UserID: uid, Receipt: *receipt})
		}
	}
	if len(items) == 0 || b.push.Pusher == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, failed, err := b.push.Pusher.BatchPushReceiptsToUsers(ctx, items); err != nil {
		b.log.Warnf("batch push receipts failed: count=%d err=%v", len(items), err)
	} else if len(failed) > 0 {
		for _, item := range failed {
			receipt := item.Receipt
			if _, err := b.push.Pusher.PushReceiptToUser(ctx, item.UserID, &receipt); err != nil {
				b.log.Warnf("fallback push receipt to user %d failed: %v", item.UserID, err)
			}
		}
	}
}
