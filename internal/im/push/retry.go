package push

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"nonoka-im/internal/im/app"
	"nonoka-im/internal/im/domain"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/redis/go-redis/v9"
)

const (
	pushRetryKey      = "im:push_retry"
	defaultRetryDelay = 5 * time.Second
	maxRetryDelay     = 5 * time.Minute
	maxRetryAttempts  = 3
	retryScanInterval = 5 * time.Second
	retryWorkerCount  = 8
	maxRetryBatchSize = 100
)

func nextRetryAt(retryCount int) int64 {
	backoff := defaultRetryDelay * time.Duration(1<<retryCount)
	if backoff > maxRetryDelay {
		backoff = maxRetryDelay
	}
	return time.Now().Add(backoff).Unix()
}

// RetryQueue manages delayed push retries in a Redis sorted set.
type RetryQueue struct {
	redis        redis.UniversalClient
	pusher       app.Pusher
	log          *log.Helper
	stopCh       chan struct{}
	stopOnce     sync.Once
	doneCh       chan struct{}
	processingMu sync.Mutex
}

type RetryItem struct {
	UserID      int64   `json:"user_id"`
	MsgID       int64   `json:"msg_id"`
	Topic       string  `json:"topic"`
	SenderID    int64   `json:"sender_id"`
	MsgType     int32   `json:"msg_type"`
	Content     []byte  `json:"content"`
	Timestamp   int64   `json:"timestamp"`
	TopicSeq    uint64  `json:"topic_seq"`
	ClientMsgID string  `json:"client_msg_id"`
	Mentioned   []int64 `json:"mentioned_user_ids,omitempty"`
	RetryCount  int     `json:"retry_count"`
	RetryAt     int64   `json:"retry_at"`
}

func NewRetryQueue(redis redis.UniversalClient, pusher app.Pusher, logger log.Logger) *RetryQueue {
	return &RetryQueue{
		redis: redis, pusher: pusher, log: log.NewHelper(logger),
		stopCh: make(chan struct{}), doneCh: make(chan struct{}),
	}
}

func (q *RetryQueue) ScheduleRetry(ctx context.Context, userID int64, msg *domain.PushMessage) error {
	if q.redis == nil || msg == nil {
		return nil
	}
	item := RetryItem{
		UserID: userID, MsgID: msg.MsgID, Topic: msg.Topic, SenderID: msg.SenderID,
		MsgType: msg.MsgType, Content: msg.Content, Timestamp: msg.Timestamp,
		TopicSeq: msg.TopicSeq, ClientMsgID: msg.ClientMsgID, Mentioned: msg.MentionedUserIDs,
		RetryAt: nextRetryAt(0),
	}
	return q.enqueue(ctx, item)
}

func (q *RetryQueue) enqueue(ctx context.Context, item RetryItem) error {
	data, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("marshal retry item: %w", err)
	}
	member := fmt.Sprintf("%d:%d:%d|%s", item.UserID, item.MsgID, item.RetryCount, data)
	return q.redis.ZAdd(ctx, pushRetryKey, redis.Z{Score: float64(item.RetryAt), Member: member}).Err()
}

func (q *RetryQueue) Start(ctx context.Context) {
	ticker := time.NewTicker(retryScanInterval)
	defer ticker.Stop()
	defer close(q.doneCh)
	for {
		select {
		case <-ctx.Done():
			return
		case <-q.stopCh:
			return
		case <-ticker.C:
			q.process(ctx)
		}
	}
}

func (q *RetryQueue) Stop() {
	q.stopOnce.Do(func() { close(q.stopCh) })
	<-q.doneCh
}

type retryTask struct {
	member string
	data   string
}

func (q *RetryQueue) process(ctx context.Context) {
	if !q.processingMu.TryLock() {
		return
	}
	defer q.processingMu.Unlock()
	now := time.Now().Unix()
	items, err := q.redis.ZRangeByScore(ctx, pushRetryKey, &redis.ZRangeBy{Min: "0", Max: fmt.Sprintf("%d", now), Count: maxRetryBatchSize}).Result()
	if err != nil {
		q.log.Warnf("fetch retry items failed: %v", err)
		return
	}
	tasks := make([]retryTask, 0, len(items))
	for _, member := range items {
		parts := strings.SplitN(member, "|", 2)
		if len(parts) != 2 {
			q.remove(ctx, member)
			continue
		}
		tasks = append(tasks, retryTask{member: member, data: parts[1]})
	}
	if len(tasks) == 0 {
		return
	}
	workers := min(retryWorkerCount, len(tasks))
	ch := make(chan retryTask)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range ch {
				q.processTask(ctx, task)
			}
		}()
	}
sendLoop:
	for _, task := range tasks {
		select {
		case ch <- task:
		case <-ctx.Done():
			break sendLoop
		case <-q.stopCh:
			break sendLoop
		}
	}
	close(ch)
	wg.Wait()
}

func (q *RetryQueue) processTask(ctx context.Context, task retryTask) {
	var item RetryItem
	if err := json.Unmarshal([]byte(task.data), &item); err != nil {
		q.remove(ctx, task.member)
		return
	}
	if !q.remove(ctx, task.member) {
		return
	}
	if item.RetryCount >= maxRetryAttempts {
		return
	}
	msg := &domain.PushMessage{
		MsgID: item.MsgID, Topic: item.Topic, SenderID: item.SenderID, MsgType: item.MsgType,
		Content: item.Content, Timestamp: item.Timestamp, TopicSeq: item.TopicSeq,
		ClientMsgID: item.ClientMsgID, MentionedUserIDs: item.Mentioned,
	}
	_, failed, err := q.pusher.BatchPushToUsers(ctx, []int64{item.UserID}, msg)
	if err != nil || len(failed) > 0 {
		q.requeue(ctx, item)
	}
}

func (q *RetryQueue) requeue(ctx context.Context, item RetryItem) {
	if item.RetryCount >= maxRetryAttempts-1 {
		return
	}
	item.RetryCount++
	item.RetryAt = nextRetryAt(item.RetryCount)
	if err := q.enqueue(ctx, item); err != nil {
		q.log.Warnf("requeue retry item failed: %v", err)
	}
}

func (q *RetryQueue) remove(ctx context.Context, member string) bool {
	n, err := q.redis.ZRem(ctx, pushRetryKey, member).Result()
	return err == nil && n > 0
}

var _ app.RetryQueue = (*RetryQueue)(nil)
