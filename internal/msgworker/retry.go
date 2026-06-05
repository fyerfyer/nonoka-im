package msgworker

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	pb "nonoka-im/api/im/v1"
	"github.com/redis/go-redis/v9"
)

const (
	// Redis key for the push retry sorted set (score = retry timestamp).
	pushRetryKey = "im:push_retry"
	// Default retry delay.
	defaultRetryDelay = 5 * time.Second
	// Max retry attempts before giving up.
	maxRetryAttempts = 3
	// Retry loop scan interval.
	retryScanInterval = 5 * time.Second
)

// PushRetryQueue manages delayed retries for failed push deliveries using Redis Sorted Set.
type PushRetryQueue struct {
	redis      redis.UniversalClient
	pusher     *GatewayPusher
	log        *log.Helper
	stopCh     chan struct{}
	stopOnce   sync.Once
}

// RetryItem represents a single push retry entry.
type RetryItem struct {
	UserID      int64  `json:"user_id"`
	MsgID       int64  `json:"msg_id"`
	Topic       string `json:"topic"`
	SenderID    int64  `json:"sender_id"`
	MsgType     int32  `json:"msg_type"`
	Content     []byte `json:"content"`
	Timestamp   int64  `json:"timestamp"`
	TopicSeq    uint64 `json:"topic_seq"`
	RetryCount  int    `json:"retry_count"`
	RetryAt     int64  `json:"retry_at"`
}

// NewPushRetryQueue creates a new push retry queue.
func NewPushRetryQueue(redis redis.UniversalClient, pusher *GatewayPusher, logger log.Logger) *PushRetryQueue {
	return &PushRetryQueue{
		redis:  redis,
		pusher: pusher,
		log:    log.NewHelper(logger),
		stopCh: make(chan struct{}),
	}
}

// ScheduleRetry adds a failed push to the retry queue.
func (q *PushRetryQueue) ScheduleRetry(ctx context.Context, userID int64, msg *pb.MessagePush) error {
	if q.redis == nil {
		return nil
	}

	item := RetryItem{
		UserID:    userID,
		MsgID:     msg.MsgId,
		Topic:     msg.Topic,
		SenderID:  msg.SenderId,
		MsgType:   msg.MsgType,
		Content:   msg.Content,
		Timestamp: msg.Timestamp,
		TopicSeq:  msg.TopicSeq,
		RetryCount: 0,
		RetryAt:   time.Now().Add(defaultRetryDelay).Unix(),
	}

	data, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("marshal retry item: %w", err)
	}

	score := float64(item.RetryAt)
	if err := q.redis.ZAdd(ctx, pushRetryKey, redis.Z{Score: score, Member: string(data)}).Err(); err != nil {
		return fmt.Errorf("zadd retry item: %w", err)
	}

	q.log.Debugf("scheduled push retry: user_id=%d, msg_id=%d, retry_at=%d", userID, msg.MsgId, item.RetryAt)
	return nil
}

// Start runs the retry loop in a background goroutine.
func (q *PushRetryQueue) Start(ctx context.Context) {
	ticker := time.NewTicker(retryScanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-q.stopCh:
			return
		case <-ticker.C:
			q.processRetries(ctx)
		}
	}
}

// Stop signals the retry loop to stop.
func (q *PushRetryQueue) Stop() {
	q.stopOnce.Do(func() {
		close(q.stopCh)
	})
}

// processRetries scans and processes overdue retry items.
func (q *PushRetryQueue) processRetries(ctx context.Context) {
	now := time.Now().Unix()

	// Fetch items with retry_at <= now
	items, err := q.redis.ZRangeByScore(ctx, pushRetryKey, &redis.ZRangeBy{
		Min: "0",
		Max: fmt.Sprintf("%d", now),
	}).Result()
	if err != nil {
		q.log.Warnf("fetch retry items failed: %v", err)
		return
	}

	if len(items) == 0 {
		return
	}

	for _, itemData := range items {
		var item RetryItem
		if err := json.Unmarshal([]byte(itemData), &item); err != nil {
			q.log.Warnf("unmarshal retry item failed: %v", err)
			q.removeRetryItem(ctx, itemData)
			continue
		}

		// Remove from queue before processing (avoid double processing)
		q.removeRetryItem(ctx, itemData)

		// Check retry limit
		if item.RetryCount >= maxRetryAttempts {
			q.log.Debugf("push retry exhausted: user_id=%d, msg_id=%d", item.UserID, item.MsgID)
			continue
		}

		// Attempt push
		msg := &pb.MessagePush{
			MsgId:     item.MsgID,
			Topic:     item.Topic,
			SenderId:  item.SenderID,
			MsgType:   item.MsgType,
			Content:   item.Content,
			Timestamp: item.Timestamp,
			TopicSeq:  item.TopicSeq,
		}

		_, failedIDs, err := q.pusher.BatchPushToUsers(ctx, []int64{item.UserID}, msg)
		if err != nil {
			q.log.Warnf("retry push failed: user_id=%d, msg_id=%d, err=%v", item.UserID, item.MsgID, err)
			q.requeueIfNeeded(ctx, item)
			continue
		}

		if len(failedIDs) > 0 {
			// Still offline, requeue with incremented retry count
			q.requeueIfNeeded(ctx, item)
		} else {
			q.log.Debugf("retry push succeeded: user_id=%d, msg_id=%d", item.UserID, item.MsgID)
		}
	}
}

// requeueIfNeeded re-adds a failed retry item with incremented count and delayed time.
func (q *PushRetryQueue) requeueIfNeeded(ctx context.Context, item RetryItem) {
	if item.RetryCount >= maxRetryAttempts-1 {
		q.log.Debugf("push retry limit reached, dropping: user_id=%d, msg_id=%d", item.UserID, item.MsgID)
		return
	}

	item.RetryCount++
	item.RetryAt = time.Now().Add(defaultRetryDelay).Unix()

	data, err := json.Marshal(item)
	if err != nil {
		q.log.Warnf("marshal requeue item failed: %v", err)
		return
	}

	score := float64(item.RetryAt)
	if err := q.redis.ZAdd(ctx, pushRetryKey, redis.Z{Score: score, Member: string(data)}).Err(); err != nil {
		q.log.Warnf("requeue retry item failed: %v", err)
	}
}

// removeRetryItem removes a specific item from the retry queue.
func (q *PushRetryQueue) removeRetryItem(ctx context.Context, itemData string) {
	if err := q.redis.ZRem(ctx, pushRetryKey, itemData).Err(); err != nil {
		q.log.Warnf("remove retry item failed: %v", err)
	}
}
