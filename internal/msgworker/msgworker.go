package msgworker

import (
	"context"
	"fmt"

	"github.com/go-kratos/kratos/v2/log"
	pb "nonoka-im/api/im/v1"
	"google.golang.org/protobuf/proto"
)

// MsgWorker consumes Kafka upstream messages, persists them to MongoDB,
// generates sequence numbers, and pushes messages to online users via Gateway.
type MsgWorker struct {
	consumer   *KafkaConsumer
	seqGen     *SeqGenerator
	snowflake  *Snowflake
	storage    *MessageStorage
	pusher     *GatewayPusher
	retryQueue *PushRetryQueue
	log        *log.Helper
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

// NewMsgWorker creates a new MsgWorker.
func NewMsgWorker(
	consumer *KafkaConsumer,
	seqGen *SeqGenerator,
	snowflake *Snowflake,
	storage *MessageStorage,
	pusher *GatewayPusher,
	logger log.Logger,
) *MsgWorker {
	return &MsgWorker{
		consumer:  consumer,
		seqGen:    seqGen,
		snowflake: snowflake,
		storage:   storage,
		pusher:    pusher,
		log:       log.NewHelper(logger),
	}
}

// Start begins consuming and processing messages.
func (w *MsgWorker) Start(ctx context.Context) error {
	w.log.Info("msgworker started")

	// Start push retry loop if retry queue is configured.
	if w.retryQueue != nil {
		go w.retryQueue.Start(ctx)
	}

	return w.consumer.Start(ctx)
}

// Stop gracefully stops the worker.
func (w *MsgWorker) Stop() error {
	w.log.Info("msgworker stopping")
	if err := w.consumer.Stop(); err != nil {
		w.log.Warnf("stop consumer: %v", err)
	}
	if w.retryQueue != nil {
		w.retryQueue.Stop()
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
func (w *MsgWorker) HandleMessage(ctx context.Context, key, value []byte, headers map[string]string) error {
	var upstream pb.UpstreamMessage
	if err := proto.Unmarshal(value, &upstream); err != nil {
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

	// Persist and dispatch based on topic type
	topicType := ParseTopicType(upstream.GetTopic())
	var recipientIDs []int64
	var pushMsg *pb.MessagePush
	var isDuplicate bool

	switch topicType {
	case TopicTypeP2P:
		recipientIDs, err = w.storage.SaveP2PMessage(ctx, &upstream, msgID, topicSeq)
		if err != nil {
			return fmt.Errorf("save p2p message: %w", err)
		}
		// If duplicate, recipientIDs is still returned (idempotent).
		if IsDuplicateError(err) {
			isDuplicate = true
		}

	case TopicTypeGroup:
		err = w.storage.SaveGroupMessage(ctx, &upstream, msgID, topicSeq)
		if err != nil {
			return fmt.Errorf("save group message: %w", err)
		}
		if IsDuplicateError(err) {
			isDuplicate = true
		}

		// Handle @mentions for large groups: write扩散 to mention_inbox.
		// Also push @mentions to online users immediately.
		if len(upstream.GetMentionedUserIds()) > 0 {
			if err := w.storage.SaveMentionInbox(ctx, &upstream, msgID, topicSeq, upstream.GetMentionedUserIds()); err != nil {
				w.log.Warnf("save mention inbox failed: %v", err)
			}
			// Push @mention notifications to online users
			if w.pusher != nil {
				pushMsg = w.buildMessagePush(msgID, topicSeq, &upstream)
				_, failedIDs, err := w.pusher.BatchPushToUsers(ctx, upstream.GetMentionedUserIds(), pushMsg)
				if err != nil {
					w.log.Warnf("push mentions to online users failed: %v", err)
				} else {
					w.handleFailedPushes(ctx, failedIDs, pushMsg)
				}
			}
		}
		w.log.Debugf("group message persisted: topic=%s", upstream.GetTopic())

	case TopicTypeSystem:
		recipientIDs, err = w.storage.SaveSystemMessage(ctx, &upstream, msgID, topicSeq)
		if err != nil {
			return fmt.Errorf("save system message: %w", err)
		}
		if IsDuplicateError(err) {
			isDuplicate = true
		}

	default:
		return fmt.Errorf("unknown topic type: %s", upstream.GetTopic())
	}

	// If duplicate, still backup seq (idempotent) but skip push to avoid duplicate notifications.
	if isDuplicate {
		w.log.Debugf("duplicate message handled idempotently: client_msg_id=%s", upstream.GetClientMsgId())
		if err := w.storage.BackupTopicSeq(ctx, upstream.GetTopic(), topicSeq); err != nil {
			w.log.Warnf("backup topic seq failed: %v", err)
		}
		return nil
	}

	// Backup topic seq to MongoDB as a fallback
	if err := w.storage.BackupTopicSeq(ctx, upstream.GetTopic(), topicSeq); err != nil {
		w.log.Warnf("backup topic seq failed: %v", err)
	}

	// Push to online recipients (only for P2P and system messages in this version)
	if len(recipientIDs) > 0 && w.pusher != nil {
		pushMsg = w.buildMessagePush(msgID, topicSeq, &upstream)
		_, failedIDs, err := w.pusher.BatchPushToUsers(ctx, recipientIDs, pushMsg)
		if err != nil {
			w.log.Warnf("push to online users failed: %v", err)
		} else {
			w.handleFailedPushes(ctx, failedIDs, pushMsg)
		}
	}

	w.log.Debugf("message processed: msg_id=%d topic=%s seq=%d", msgID, upstream.GetTopic(), topicSeq)
	return nil
}

// buildMessagePush constructs a MessagePush from upstream message data.
func (w *MsgWorker) buildMessagePush(msgID int64, topicSeq uint64, upstream *pb.UpstreamMessage) *pb.MessagePush {
	return &pb.MessagePush{
		MsgId:     msgID,
		Topic:     upstream.GetTopic(),
		SenderId:  upstream.GetSenderId(),
		MsgType:   upstream.GetMsgType(),
		Content:   upstream.GetContent(),
		Timestamp: upstream.GetTimestamp(),
		TopicSeq:  topicSeq,
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
