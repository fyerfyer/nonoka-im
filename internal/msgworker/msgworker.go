package msgworker

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	pb "nonoka-im/api/im/v1"
	"nonoka-im/internal/metrics"
	"google.golang.org/protobuf/proto"
)

// Pusher pushes messages and receipts to online users via Gateway.
// It is implemented by GatewayPusher and can be mocked in tests.
type Pusher interface {
	PushToUser(ctx context.Context, userID int64, msg *pb.MessagePush) (int32, error)
	BatchPushToUsers(ctx context.Context, userIDs []int64, msg *pb.MessagePush) (int32, []int64, error)
	PushReceiptToUser(ctx context.Context, userID int64, receipt *pb.SendReceipt) (int32, error)
	BatchPushReceiptToUsers(ctx context.Context, userIDs []int64, receipt *pb.SendReceipt) (int32, []int64, error)
	BatchPushReceiptsToUsers(ctx context.Context, items []*pb.ReceiptBatchItem) (int32, []*pb.ReceiptBatchItem, error)
	Close() error
}

// receiptBatcher aggregates send receipts per user and flushes them in batches
// to reduce gRPC round-trips between MsgWorker and Gateway.
type receiptBatcher struct {
	pusher       Pusher
	batchSize    int
	batchTimeout time.Duration
	log          *log.Helper

	mu      sync.Mutex
	pending map[int64][]*pb.SendReceipt
	stopCh  chan struct{}
	stopOnce sync.Once
	wg      sync.WaitGroup
}

// newReceiptBatcher creates a new receipt batcher.
func newReceiptBatcher(pusher Pusher, batchSize int, batchTimeout time.Duration, logger log.Logger) *receiptBatcher {
	if batchSize <= 0 {
		batchSize = 32
	}
	if batchTimeout <= 0 {
		batchTimeout = 10 * time.Millisecond
	}
	return &receiptBatcher{
		pusher:       pusher,
		batchSize:    batchSize,
		batchTimeout: batchTimeout,
		pending:      make(map[int64][]*pb.SendReceipt),
		stopCh:       make(chan struct{}),
		log:          log.NewHelper(logger),
	}
}

// Start begins the background flush loop. The receipt batcher can also operate
// without Start: Add() will trigger immediate flushes when batchSize is reached.
func (b *receiptBatcher) Start(ctx context.Context) {
	b.wg.Add(1)
	go b.loop(ctx)
}

// Stop gracefully stops the receipt batcher and flushes remaining receipts.
func (b *receiptBatcher) Stop() {
	b.stopOnce.Do(func() {
		close(b.stopCh)
	})
	b.wg.Wait()
	b.flush()
}

// loop periodically flushes pending receipts on a ticker.
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

// Add submits a receipt for a user. If the batch threshold is reached, it flushes immediately.
func (b *receiptBatcher) Add(userID int64, receipt *pb.SendReceipt) {
	if receipt == nil {
		return
	}

	b.mu.Lock()
	b.pending[userID] = append(b.pending[userID], receipt)
	count := 0
	for _, receipts := range b.pending {
		count += len(receipts)
	}
	b.mu.Unlock()

	if count >= b.batchSize {
		b.flush()
	}
}

// flush sends all pending receipts via BatchPushReceiptsToUsers.
func (b *receiptBatcher) flush() {
	b.mu.Lock()
	if len(b.pending) == 0 {
		b.mu.Unlock()
		return
	}

	pending := b.pending
	b.pending = make(map[int64][]*pb.SendReceipt)
	b.mu.Unlock()

	items := make([]*pb.ReceiptBatchItem, 0, len(pending))
	for uid, receipts := range pending {
		for _, receipt := range receipts {
			items = append(items, &pb.ReceiptBatchItem{
				UserId:  uid,
				Receipt: receipt,
			})
		}
	}
	if len(items) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, failed, err := b.pusher.BatchPushReceiptsToUsers(ctx, items); err != nil {
		b.log.Warnf("batch push receipts failed: count=%d err=%v", len(items), err)
	} else if len(failed) > 0 {
		b.log.Debugf("batch push receipts partial failure: count=%d failed=%d", len(items), len(failed))
		// Fallback: route failed receipts individually. PushReceiptToUser will
		// broadcast to all gateways if dynamic routing cannot locate the session,
		// ensuring receipts are not lost due to stale cache entries.
		for _, fi := range failed {
			if _, err := b.pusher.PushReceiptToUser(ctx, fi.GetUserId(), fi.GetReceipt()); err != nil {
				b.log.Warnf("fallback push receipt to user %d failed: %v", fi.GetUserId(), err)
			}
		}
	} else {
		b.log.Debugf("batch push receipts success: count=%d", len(items))
	}
}

// groupBatchItem holds a single group message waiting to be flushed in batch.
type groupBatchItem struct {
	ctx      context.Context
	msg      *pb.UpstreamMessage
	msgID    int64
	topicSeq uint64
	result   chan groupBatchResult
}

type groupBatchResult struct {
	isDuplicate bool
	err         error
}

// groupMessageBatcher aggregates group messages and inserts them into MongoDB
// in batches. Unlike the receipt batcher, it is synchronous: Add blocks until
// the message has been persisted (or batch-flushed), because the Kafka handler
// must guarantee durability before committing offsets.
type groupMessageBatcher struct {
	storage      *MessageStorage
	batchSize    int
	batchTimeout time.Duration
	log          *log.Helper

	mu      sync.Mutex
	pending []*groupBatchItem
	stopCh  chan struct{}
	stopOnce sync.Once
	wg      sync.WaitGroup
}

// newGroupMessageBatcher creates a new group message batcher.
func newGroupMessageBatcher(storage *MessageStorage, batchSize int, batchTimeout time.Duration, logger log.Logger) *groupMessageBatcher {
	if batchSize <= 0 {
		batchSize = 16
	}
	if batchTimeout <= 0 {
		batchTimeout = 5 * time.Millisecond
	}
	return &groupMessageBatcher{
		storage:      storage,
		batchSize:    batchSize,
		batchTimeout: batchTimeout,
		pending:      make([]*groupBatchItem, 0, batchSize),
		stopCh:       make(chan struct{}),
		log:          log.NewHelper(logger),
	}
}

// Start begins the background flush loop.
func (b *groupMessageBatcher) Start(ctx context.Context) {
	b.wg.Add(1)
	go b.loop(ctx)
}

// Stop gracefully stops the batcher and flushes remaining messages.
func (b *groupMessageBatcher) Stop() {
	b.stopOnce.Do(func() {
		close(b.stopCh)
	})
	b.wg.Wait()
	b.flush()
}

// loop periodically flushes pending group messages on a ticker.
func (b *groupMessageBatcher) loop(ctx context.Context) {
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

// Add submits a group message to the batcher and blocks until it is persisted.
func (b *groupMessageBatcher) Add(ctx context.Context, msg *pb.UpstreamMessage, msgID int64, topicSeq uint64) (bool, error) {
	if b.storage == nil {
		return false, fmt.Errorf("group message batcher: no storage configured")
	}

	resCh := make(chan groupBatchResult, 1)
	item := &groupBatchItem{
		ctx:      ctx,
		msg:      msg,
		msgID:    msgID,
		topicSeq: topicSeq,
		result:   resCh,
	}

	b.mu.Lock()
	b.pending = append(b.pending, item)
	count := len(b.pending)
	b.mu.Unlock()

	if count >= b.batchSize {
		b.flush()
	}

	select {
	case res := <-resCh:
		return res.isDuplicate, res.err
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

// flush persists all pending group messages. If the batch contains any
// duplicate key errors, it falls back to per-message SaveGroupMessage so that
// callers receive accurate isDuplicate results.
func (b *groupMessageBatcher) flush() {
	b.mu.Lock()
	if len(b.pending) == 0 {
		b.mu.Unlock()
		return
	}

	items := b.pending
	b.pending = make([]*groupBatchItem, 0, b.batchSize)
	b.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msgs := make([]*pb.UpstreamMessage, len(items))
	msgIDs := make([]int64, len(items))
	topicSeqs := make([]uint64, len(items))
	for i, item := range items {
		msgs[i] = item.msg
		msgIDs[i] = item.msgID
		topicSeqs[i] = item.topicSeq
	}

	inserted, allDuplicate, err := b.storage.SaveGroupMessagesBatch(ctx, msgs, msgIDs, topicSeqs)
	if err != nil {
		b.log.Warnf("save group message batch failed: count=%d err=%v", len(items), err)
		// Fallback: save each message individually so callers know the exact outcome.
		for _, item := range items {
			itemCtx, itemCancel := context.WithTimeout(context.Background(), 3*time.Second)
			dup, saveErr := b.storage.SaveGroupMessage(itemCtx, item.msg, item.msgID, item.topicSeq)
			itemCancel()
			item.result <- groupBatchResult{isDuplicate: dup, err: saveErr}
		}
		return
	}

	if allDuplicate {
		for _, item := range items {
			item.result <- groupBatchResult{isDuplicate: true}
		}
		return
	}

	// Partial success: some were inserted, some were duplicates. We cannot map
	// MongoDB inserted IDs back to original messages without an explicit _id,
	// so we conservatively save each remaining message individually.
	if inserted < len(items) {
		for _, item := range items {
			itemCtx, itemCancel := context.WithTimeout(context.Background(), 3*time.Second)
			dup, saveErr := b.storage.SaveGroupMessage(itemCtx, item.msg, item.msgID, item.topicSeq)
			itemCancel()
			item.result <- groupBatchResult{isDuplicate: dup, err: saveErr}
		}
		return
	}

	for _, item := range items {
		item.result <- groupBatchResult{}
	}
}

// deliveryTask holds everything needed to push a persisted message.
// It is submitted to the async push pool after the message has been
// durably persisted, so the Kafka handler can return and commit offsets.
type deliveryTask struct {
	upstream     *pb.UpstreamMessage
	msgID        int64
	topicSeq     uint64
	recipientIDs []int64
	isGroup      bool
}

// MsgWorker consumes Kafka upstream messages, persists them to MongoDB,
// generates sequence numbers, and pushes messages to online users via Gateway.
type MsgWorker struct {
	consumer        *KafkaConsumer
	seqGen          *SeqGenerator
	snowflake       *IDGenerator
	storage         *MessageStorage
	pusher          Pusher
	groupMemberSvc  GroupMemberService
	retryQueue       *PushRetryQueue
	receiptBatcher   *receiptBatcher
	groupBatcher     *groupMessageBatcher
	metrics          *metrics.Metrics
	log              *log.Helper

	// Async delivery pipeline
	pushPool     chan deliveryTask
	pushWorkers  int
	pushWg       sync.WaitGroup
	pushStopCh   chan struct{}
	pushStopOnce sync.Once

	// Receipt batching configuration
	receiptBatchSize    int
	receiptBatchTimeout time.Duration

	// Group message batching configuration
	groupBatchSize    int
	groupBatchTimeout time.Duration
}

// Storage returns the underlying MessageStorage for testing purposes.
func (w *MsgWorker) Storage() *MessageStorage {
	return w.storage
}

// MsgWorkerConfig holds all dependencies for MsgWorker.
type MsgWorkerConfig struct {
	ConsumerCfg KafkaConsumerConfig
	Logger      log.Logger
}

// defaultPushWorkers returns a reasonable push worker count.
func defaultPushWorkers() int {
	n := runtime.NumCPU() * 2
	if n < 4 {
		return 4
	}
	if n > 64 {
		return 64
	}
	return n
}

// NewMsgWorker creates a new MsgWorker.
func NewMsgWorker(
	consumer *KafkaConsumer,
	seqGen *SeqGenerator,
	snowflake *IDGenerator,
	storage *MessageStorage,
	pusher Pusher,
	logger log.Logger,
	m ...*metrics.Metrics,
) *MsgWorker {
	w := &MsgWorker{
		consumer:            consumer,
		seqGen:              seqGen,
		snowflake:           snowflake,
		storage:             storage,
		pusher:              pusher,
		pushPool:            make(chan deliveryTask, 1024),
		pushWorkers:         defaultPushWorkers(),
		pushStopCh:          make(chan struct{}),
		receiptBatchSize:    32,
		receiptBatchTimeout: 10 * time.Millisecond,
		groupBatchSize:      16,
		groupBatchTimeout:   5 * time.Millisecond,
		log:                 log.NewHelper(logger),
	}
	if len(m) > 0 {
		w.metrics = m[0]
	}
	return w
}

// SetReceiptBatcherConfig configures receipt batching. Must be called before Start.
func (w *MsgWorker) SetReceiptBatcherConfig(batchSize int, batchTimeout time.Duration) {
	if batchSize > 0 {
		w.receiptBatchSize = batchSize
	}
	if batchTimeout > 0 {
		w.receiptBatchTimeout = batchTimeout
	}
}

// SetGroupBatcherConfig configures group message batching. Must be called before Start.
func (w *MsgWorker) SetGroupBatcherConfig(batchSize int, batchTimeout time.Duration) {
	if batchSize > 0 {
		w.groupBatchSize = batchSize
	}
	if batchTimeout > 0 {
		w.groupBatchTimeout = batchTimeout
	}
}

// SetGroupMemberService configures the group member service for group message
// push and membership queries.
func (w *MsgWorker) SetGroupMemberService(svc GroupMemberService) {
	w.groupMemberSvc = svc
}

// SetPusher configures the gateway pusher. Useful for tests and for late
// initialization when the pusher address is discovered at runtime.
func (w *MsgWorker) SetPusher(pusher Pusher) {
	w.pusher = pusher
}

// SetPushWorkers configures the number of async push workers. Must be called
// before Start.
func (w *MsgWorker) SetPushWorkers(n int) {
	if n > 0 {
		w.pushWorkers = n
	}
}

// Start begins consuming and processing messages.
func (w *MsgWorker) Start(ctx context.Context) error {
	w.log.Info("msgworker started")

	// Initialize receipt batcher if pusher supports batch receipts.
	if w.pusher != nil && w.receiptBatcher == nil {
		w.receiptBatcher = newReceiptBatcher(w.pusher, w.receiptBatchSize, w.receiptBatchTimeout, w.log.Logger())
		w.receiptBatcher.Start(ctx)
	}

	// Initialize group message batcher if storage is configured.
	if w.storage != nil && w.groupBatcher == nil {
		w.groupBatcher = newGroupMessageBatcher(w.storage, w.groupBatchSize, w.groupBatchTimeout, w.log.Logger())
		w.groupBatcher.Start(ctx)
	}

	// Start push retry loop if retry queue is configured.
	if w.retryQueue != nil {
		go w.retryQueue.Start(ctx)
	}

	// Start async delivery workers.
	for i := 0; i < w.pushWorkers; i++ {
		w.pushWg.Add(1)
		go w.pushWorkerLoop()
	}

	return w.consumer.Start(ctx)
}

// Stop gracefully stops the worker.
func (w *MsgWorker) Stop() error {
	w.log.Info("msgworker stopping")
	if err := w.consumer.Stop(); err != nil {
		w.log.Warnf("stop consumer: %v", err)
	}

	w.pushStopOnce.Do(func() {
		close(w.pushStopCh)
		// Close the pool after the consumer has stopped so no new tasks arrive.
		// Workers drain remaining tasks before exiting.
		close(w.pushPool)
	})

	if w.retryQueue != nil {
		w.retryQueue.Stop()
	}

	w.pushWg.Wait()

	if w.receiptBatcher != nil {
		w.receiptBatcher.Stop()
	}

	if w.groupBatcher != nil {
		w.groupBatcher.Stop()
	}

	if w.pusher != nil {
		if err := w.pusher.Close(); err != nil {
			w.log.Warnf("close pusher: %v", err)
		}
	}
	return nil
}

// SetRetryQueue configures the push retry queue for failed deliveries.
func (w *MsgWorker) SetRetryQueue(q *PushRetryQueue) {
	w.retryQueue = q
}

// HandleMessage is the Kafka message handler entry point.
// It persists the message synchronously and then submits the delivery
// (push + receipt) to an async worker pool. This keeps the Kafka consumer
// path hot and allows offsets to be committed as soon as durability is
// guaranteed. Failed deliveries are handled by the retry queue.
func (w *MsgWorker) HandleMessage(ctx context.Context, key, value []byte, headers map[string]string) (err error) {
	start := time.Now()
	defer func() {
		if r := recover(); r != nil {
			w.log.Errorf("HandleMessage panic recovered: %v", r)
			err = fmt.Errorf("panic recovered: %v", r)
		}
	}()

	w.metrics.IncMessagesConsumed()

	var upstream pb.UpstreamMessage
	if err = proto.Unmarshal(value, &upstream); err != nil {
		return fmt.Errorf("unmarshal upstream message: %w", err)
	}

	w.log.Debugf("handling message: topic=%s sender=%d client_msg_id=%s",
		upstream.GetTopic(), upstream.GetSenderId(), upstream.GetClientMsgId())

	// Generate global message ID and topic seq
	msgID := w.snowflake.NextID()
	topicSeq, err := w.seqGen.NextSeq(ctx, upstream.GetTopic())
	if err != nil {
		return fmt.Errorf("generate topic seq: %w", err)
	}

	// Persist based on topic type
	topicType := ParseTopicType(upstream.GetTopic())
	var recipientIDs []int64
	var isDuplicate bool

	switch topicType {
	case TopicTypeP2P:
		recipientIDs, isDuplicate, err = w.storage.SaveP2PMessage(ctx, &upstream, msgID, topicSeq)
		if err != nil {
			return fmt.Errorf("save p2p message: %w", err)
		}

	case TopicTypeGroup:
		if w.groupBatcher != nil {
			isDuplicate, err = w.groupBatcher.Add(ctx, &upstream, msgID, topicSeq)
		} else {
			isDuplicate, err = w.storage.SaveGroupMessage(ctx, &upstream, msgID, topicSeq)
		}
		if err != nil {
			return fmt.Errorf("save group message: %w", err)
		}
		// Handle @mentions for large groups: write扩散 to mention_inbox.
		// SaveMentionInbox is idempotent thanks to MongoDB unique indexes, so it
		// is safe to call even when the group message itself is a duplicate.
		if len(upstream.GetMentionedUserIds()) > 0 {
			if err := w.storage.SaveMentionInbox(ctx, &upstream, msgID, topicSeq, upstream.GetMentionedUserIds()); err != nil {
				w.log.Warnf("save mention inbox failed: %v", err)
			}
		}
		w.log.Debugf("group message persisted: topic=%s", upstream.GetTopic())

	case TopicTypeSystem:
		recipientIDs, isDuplicate, err = w.storage.SaveSystemMessage(ctx, &upstream, msgID, topicSeq)
		if err != nil {
			return fmt.Errorf("save system message: %w", err)
		}

	default:
		return fmt.Errorf("unknown topic type: %s", upstream.GetTopic())
	}

	// If duplicate, skip seq backup and push to avoid duplicate notifications.
	if isDuplicate {
		w.metrics.IncMessagesDuplicate()
		w.metrics.ObserveProcessingLatency(time.Since(start).Seconds())
		w.log.Debugf("duplicate message handled idempotently: client_msg_id=%s", upstream.GetClientMsgId())
		return nil
	}

	// Backup topic seq to MongoDB as a fallback for fresh messages only.
	if err := w.storage.BackupTopicSeq(ctx, upstream.GetTopic(), topicSeq); err != nil {
		w.log.Warnf("backup topic seq failed: %v", err)
	}

	w.metrics.IncMessagesProcessed()
	w.metrics.ObserveProcessingLatency(time.Since(start).Seconds())

	// Submit delivery (push + receipt) to async workers so the Kafka consumer
	// can keep polling. A background context with timeout is used because the
	// original handler context may be cancelled after we return.
	select {
	case w.pushPool <- deliveryTask{
		upstream:     &upstream,
		msgID:        msgID,
		topicSeq:     topicSeq,
		recipientIDs: recipientIDs,
		isGroup:      topicType == TopicTypeGroup,
	}:
	case <-w.pushStopCh:
		w.log.Warnf("push pool closed, delivery dropped: msg_id=%d", msgID)
	}

	w.log.Debugf("message persisted and delivery queued: msg_id=%d topic=%s seq=%d", msgID, upstream.GetTopic(), topicSeq)
	return nil
}

// pushWorkerLoop processes delivery tasks asynchronously.
func (w *MsgWorker) pushWorkerLoop() {
	defer w.pushWg.Done()
	for {
		select {
		case <-w.pushStopCh:
			return
		case task, ok := <-w.pushPool:
			if !ok {
				return
			}
			w.deliverMessage(task)
		}
	}
}

// deliverMessage pushes the message to online recipients and sends the
// sender receipt. It runs in the async push worker pool.
func (w *MsgWorker) deliverMessage(task deliveryTask) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	upstream := task.upstream
	msgID := task.msgID
	topicSeq := task.topicSeq

	if task.isGroup {
		w.deliverGroupMessage(ctx, upstream, msgID, topicSeq)
	} else {
		w.deliverP2POrSystemMessage(ctx, upstream, msgID, topicSeq, task.recipientIDs)
	}

	w.metrics.ObserveGrpcPushLatency(time.Since(start).Seconds())
}

// deliverP2POrSystemMessage pushes messages to recipients and sends a receipt.
func (w *MsgWorker) deliverP2POrSystemMessage(ctx context.Context, upstream *pb.UpstreamMessage, msgID int64, topicSeq uint64, recipientIDs []int64) {
	pushMsg := w.buildMessagePush(msgID, topicSeq, upstream)

	// Push to online recipients (exclude sender).
	if len(recipientIDs) > 0 && w.pusher != nil {
		recipients := make([]int64, 0, len(recipientIDs))
		for _, id := range recipientIDs {
			if id != upstream.GetSenderId() {
				recipients = append(recipients, id)
			}
		}
		if len(recipients) > 0 {
			_, failedIDs, err := w.pusher.BatchPushToUsers(ctx, recipients, pushMsg)
			if err != nil {
				w.log.Warnf("push to online users failed: %v", err)
			} else {
				w.handleFailedPushes(ctx, failedIDs, pushMsg)
			}
		}
	}

	// Push send receipt to the sender (batched).
	w.pushSendReceipt(upstream.GetSenderId(), msgID, upstream.GetTopic(), topicSeq, upstream.GetClientMsgId())
}

// deliverGroupMessage handles group message delivery including @mentions
// and broadcast to group members.
func (w *MsgWorker) deliverGroupMessage(ctx context.Context, upstream *pb.UpstreamMessage, msgID int64, topicSeq uint64) {
	pushMsg := w.buildMessagePush(msgID, topicSeq, upstream)

	// Push @mention notifications to online users.
	if len(upstream.GetMentionedUserIds()) > 0 && w.pusher != nil {
		_, failedIDs, err := w.pusher.BatchPushToUsers(ctx, upstream.GetMentionedUserIds(), pushMsg)
		if err != nil {
			w.log.Warnf("push mentions to online users failed: %v", err)
		} else {
			w.handleFailedPushes(ctx, failedIDs, pushMsg)
		}
	}

	// Push group message to all online group members (excluding sender and already-mentioned users).
	if w.groupMemberSvc != nil && w.pusher != nil {
		groupID, err := ExtractGroupID(upstream.GetTopic())
		if err != nil {
			w.log.Warnf("extract group id failed: %v", err)
			return
		}
		memberIDs, err := w.groupMemberSvc.GetGroupMembers(ctx, groupID)
		if err != nil {
			w.log.Warnf("get group members failed: %v", err)
			return
		}
		if len(memberIDs) == 0 {
			return
		}

		mentionedSet := make(map[int64]struct{}, len(upstream.GetMentionedUserIds()))
		for _, id := range upstream.GetMentionedUserIds() {
			mentionedSet[id] = struct{}{}
		}

		recipients := make([]int64, 0, len(memberIDs))
		for _, id := range memberIDs {
			if id == upstream.GetSenderId() {
				continue
			}
			if _, ok := mentionedSet[id]; ok {
				continue
			}
			recipients = append(recipients, id)
		}
		if len(recipients) > 0 {
			_, failedIDs, err := w.pusher.BatchPushToUsers(ctx, recipients, pushMsg)
			if err != nil {
				w.log.Warnf("push group message to online users failed: %v", err)
			} else {
				w.handleFailedPushes(ctx, failedIDs, pushMsg)
			}
		}
	}

	// Push send receipt to the sender (batched).
	w.pushSendReceipt(upstream.GetSenderId(), msgID, upstream.GetTopic(), topicSeq, upstream.GetClientMsgId())
}

// pushSendReceipt submits a send receipt to the batcher if available, otherwise
// sends it immediately via the pusher.
func (w *MsgWorker) pushSendReceipt(userID int64, msgID int64, topic string, topicSeq uint64, clientMsgID string) {
	receipt := &pb.SendReceipt{
		ClientMsgId: clientMsgID,
		MsgId:       msgID,
		Topic:       topic,
		TopicSeq:    topicSeq,
		Timestamp:   time.Now().Unix(),
	}

	if w.receiptBatcher != nil {
		w.receiptBatcher.Add(userID, receipt)
		return
	}

	if w.pusher != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, err := w.pusher.PushReceiptToUser(ctx, userID, receipt); err != nil {
			w.log.Warnf("push send receipt to sender %d failed: %v", userID, err)
		}
	}
}

// buildMessagePush constructs a MessagePush from upstream message data.
func (w *MsgWorker) buildMessagePush(msgID int64, topicSeq uint64, upstream *pb.UpstreamMessage) *pb.MessagePush {
	return &pb.MessagePush{
		MsgId:       msgID,
		Topic:       upstream.GetTopic(),
		SenderId:    upstream.GetSenderId(),
		MsgType:     upstream.GetMsgType(),
		Content:     upstream.GetContent(),
		Timestamp:   upstream.GetTimestamp(),
		TopicSeq:    topicSeq,
		ClientMsgId: upstream.GetClientMsgId(),
	}
}

// handleFailedPushes records failed push deliveries for retry.
func (w *MsgWorker) handleFailedPushes(ctx context.Context, failedUserIDs []int64, msg *pb.MessagePush) {
	if len(failedUserIDs) == 0 || w.retryQueue == nil {
		return
	}
	for _, userID := range failedUserIDs {
		if err := w.retryQueue.ScheduleRetry(ctx, userID, msg); err != nil {
			w.log.Warnf("schedule push retry failed: user_id=%d, err=%v", userID, err)
		}
	}
}
