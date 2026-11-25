package mongostore

import (
	"context"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"nonoka-im/internal/im/domain"
	"nonoka-im/internal/im/search"
	"strings"
	"time"
)

// SaveP2PMessage saves a P2P message using write扩散.
// Writes the message to the receiver's inbox.
// Returns isDuplicate=true if the message already exists (idempotent).
func (s *Store) saveP2P(ctx context.Context, msg *domain.Message) ([]int64, bool, error) {
	msgID, topicSeq := msg.ID, msg.TopicSeq
	s.incMongoOp(CollectionInboxes, "insert")
	uid1, uid2, err := domain.ExtractUserIDsFromP2PTopic(msg.GetTopic())
	if err != nil {
		return nil, false, err
	}

	var receiverID int64
	if msg.GetSenderId() == uid1 {
		receiverID = uid2
	} else {
		receiverID = uid1
	}

	now := time.Now()
	inboxes := []*inboxMessageRow{
		{
			UserID:      receiverID,
			MsgID:       msgID,
			Topic:       msg.GetTopic(),
			SenderID:    msg.GetSenderId(),
			MsgType:     msg.GetMsgType(),
			Content:     msg.GetContent(),
			ContentText: search.ContentText(msg.GetMsgType(), msg.GetContent()),
			Timestamp:   msg.GetTimestamp(),
			TopicSeq:    topicSeq,
			ClientMsgID: msg.GetClientMsgId(),
			Read:        false,
			CreatedAt:   now,
		},
		{
			// Also write to sender's inbox so multi-device sender can see own messages.
			UserID:      msg.GetSenderId(),
			MsgID:       msgID,
			Topic:       msg.GetTopic(),
			SenderID:    msg.GetSenderId(),
			MsgType:     msg.GetMsgType(),
			Content:     msg.GetContent(),
			ContentText: search.ContentText(msg.GetMsgType(), msg.GetContent()),
			Timestamp:   msg.GetTimestamp(),
			TopicSeq:    topicSeq,
			ClientMsgID: msg.GetClientMsgId(),
			Read:        true, // sender's own copy is considered read
			CreatedAt:   now,
		},
	}

	docs := make([]interface{}, len(inboxes))
	for i, inbox := range inboxes {
		docs[i] = inbox
	}

	_, err = s.collection(CollectionInboxes).InsertMany(ctx, docs, options.InsertMany().SetOrdered(false))
	if err != nil {
		// With unordered inserts, duplicate errors are returned as BulkWriteException.
		// If every write error is a duplicate key, the operation is idempotent.
		if bulkErr, ok := err.(mongo.BulkWriteException); ok {
			allDuplicate := len(bulkErr.WriteErrors) > 0
			for _, we := range bulkErr.WriteErrors {
				if !IsDuplicateError(we) {
					allDuplicate = false
					break
				}
			}
			if allDuplicate {
				s.log.Debugf("duplicate p2p message ignored: client_msg_id=%s, sender=%d",
					msg.GetClientMsgId(), msg.GetSenderId())
				s.cacheMessageSender(ctx, msg.GetTopic(), topicSeq, msg.GetSenderId())
				return []int64{receiverID, msg.GetSenderId()}, true, nil
			}
		}
		return nil, false, fmt.Errorf("insert p2p inbox messages: %w", err)
	}

	s.cacheMessageSender(ctx, msg.GetTopic(), topicSeq, msg.GetSenderId())
	s.log.Debugf("p2p message saved: msg_id=%d, topic=%s, receiver=%d, sender=%d", msgID, msg.GetTopic(), receiverID, msg.GetSenderId())
	return []int64{receiverID, msg.GetSenderId()}, false, nil
}

// SaveGroupMessage saves a group message using read扩散.
// Only stores one copy in the group messages collection.
// Returns isDuplicate=true if the message already exists (idempotent).
func (s *Store) saveGroup(ctx context.Context, msg *domain.Message) (bool, error) {
	msgID, topicSeq := msg.ID, msg.TopicSeq
	s.incMongoOp(CollectionMessages, "insert")
	now := time.Now()
	stored := &storedMessageRow{
		MsgID:       msgID,
		Topic:       msg.GetTopic(),
		SenderID:    msg.GetSenderId(),
		MsgType:     msg.GetMsgType(),
		Content:     msg.GetContent(),
		ContentText: search.ContentText(msg.GetMsgType(), msg.GetContent()),
		Timestamp:   msg.GetTimestamp(),
		TopicSeq:    topicSeq,
		ClientMsgID: msg.GetClientMsgId(),
		CreatedAt:   now,
	}

	_, err := s.collection(CollectionMessages).InsertOne(ctx, stored, options.InsertOne())
	if err != nil {
		if IsDuplicateError(err) {
			s.log.Debugf("duplicate group message ignored: client_msg_id=%s, sender=%d",
				msg.GetClientMsgId(), msg.GetSenderId())
			return true, nil
		}
		return false, fmt.Errorf("insert group message: %w", err)
	}

	s.cacheMessageSender(ctx, msg.GetTopic(), topicSeq, msg.GetSenderId())
	s.log.Debugf("group message saved: msg_id=%d, topic=%s", msgID, msg.GetTopic())
	return false, nil
}

// SaveGroupMessagesBatch inserts multiple group messages in one unordered
// InsertMany call. Duplicate key errors are ignored. This is used by the
// group message batcher to amortize MongoDB round-trip cost.
func (s *Store) SaveGroupBatch(ctx context.Context, msgs []*domain.Message) (int, bool, error) {
	if !s.searchEnabled || len(msgs) == 0 {
		return s.saveGroupBatch(ctx, msgs)
	}
	sess, err := s.db.Client().StartSession()
	if err != nil {
		return 0, false, fmt.Errorf("start mongo transaction session: %w", err)
	}
	defer sess.EndSession(ctx)
	var inserted int
	var duplicate bool
	_, err = sess.WithTransaction(ctx, func(txCtx context.Context) (any, error) {
		var err error
		inserted, duplicate, err = s.saveGroupBatch(txCtx, msgs)
		if err != nil || duplicate {
			return nil, err
		}
		for _, msg := range msgs {
			if err := s.enqueueSearchTask(txCtx, msg); err != nil {
				return nil, err
			}
		}
		return nil, nil
	})
	if err != nil {
		return 0, false, fmt.Errorf("persist group message batch and search index: %w", err)
	}
	return inserted, duplicate, nil
}

func (s *Store) saveGroupBatch(ctx context.Context, msgs []*domain.Message) (int, bool, error) {
	if len(msgs) == 0 {
		return 0, false, nil
	}

	s.incMongoOp(CollectionMessages, "insertMany")
	now := time.Now()
	docs := make([]interface{}, len(msgs))
	for i, msg := range msgs {
		docs[i] = &storedMessageRow{
			MsgID:       msg.ID,
			Topic:       msg.GetTopic(),
			SenderID:    msg.GetSenderId(),
			MsgType:     msg.GetMsgType(),
			Content:     msg.GetContent(),
			ContentText: search.ContentText(msg.GetMsgType(), msg.GetContent()),
			Timestamp:   msg.GetTimestamp(),
			TopicSeq:    msg.TopicSeq,
			ClientMsgID: msg.GetClientMsgId(),
			CreatedAt:   now,
		}
	}

	res, err := s.collection(CollectionMessages).InsertMany(ctx, docs, options.InsertMany().SetOrdered(false))
	if err != nil {
		if bulkErr, ok := err.(mongo.BulkWriteException); ok {
			allDuplicate := len(bulkErr.WriteErrors) > 0
			for _, we := range bulkErr.WriteErrors {
				if !IsDuplicateError(we) {
					allDuplicate = false
					break
				}
			}
			if allDuplicate {
				s.log.Debugf("duplicate group message batch ignored: count=%d", len(msgs))
				return 0, true, nil
			}
			// Partial success: some documents were inserted. Return inserted count
			// and mark the operation as not fully duplicate so the caller can
			// continue with per-message fallback if needed.
			inserted := len(res.InsertedIDs)
			s.log.Debugf("partial duplicate group message batch: count=%d inserted=%d", len(msgs), inserted)
			return inserted, false, nil
		}
		return 0, false, fmt.Errorf("insert group message batch: %w", err)
	}

	for _, msg := range msgs {
		s.cacheMessageSender(ctx, msg.GetTopic(), msg.TopicSeq, msg.GetSenderId())
	}
	s.log.Debugf("group message batch saved: count=%d", len(msgs))
	return len(msgs), false, nil
}

// SaveSystemMessage saves a system notification using write扩散.
// Returns isDuplicate=true if the message already exists (idempotent).
func (s *Store) SaveSystem(ctx context.Context, msg *domain.Message) ([]int64, bool, error) {
	msgID, topicSeq := msg.ID, msg.TopicSeq
	s.incMongoOp(CollectionInboxes, "insert")
	parts := strings.Split(msg.GetTopic(), "_")
	if len(parts) != 2 {
		return nil, false, fmt.Errorf("invalid system topic format: %s", msg.GetTopic())
	}
	var targetUID int64
	if _, err := fmt.Sscanf(parts[1], "%d", &targetUID); err != nil {
		return nil, false, fmt.Errorf("parse system topic user ID: %w", err)
	}

	now := time.Now()
	inbox := &inboxMessageRow{
		UserID:      targetUID,
		MsgID:       msgID,
		Topic:       msg.GetTopic(),
		SenderID:    msg.GetSenderId(),
		MsgType:     msg.GetMsgType(),
		Content:     msg.GetContent(),
		ContentText: search.ContentText(msg.GetMsgType(), msg.GetContent()),
		Timestamp:   msg.GetTimestamp(),
		TopicSeq:    topicSeq,
		ClientMsgID: msg.GetClientMsgId(),
		Read:        false,
		CreatedAt:   now,
	}

	insert := func(ctx context.Context) error {
		if _, err := s.collection(CollectionInboxes).InsertOne(ctx, inbox, options.InsertOne()); err != nil {
			return err
		}
		if s.searchEnabled {
			return s.enqueueSearchTask(ctx, msg)
		}
		return nil
	}
	var err error
	if s.searchEnabled {
		sess, serr := s.db.Client().StartSession()
		if serr != nil {
			return nil, false, fmt.Errorf("start mongo transaction session: %w", serr)
		}
		defer sess.EndSession(ctx)
		_, err = sess.WithTransaction(ctx, func(txCtx context.Context) (any, error) {
			return nil, insert(txCtx)
		})
	} else {
		err = insert(ctx)
	}
	if err != nil {
		if IsDuplicateError(err) {
			s.log.Debugf("duplicate system message ignored: client_msg_id=%s, sender=%d",
				msg.GetClientMsgId(), msg.GetSenderId())
			return []int64{targetUID}, true, nil
		}
		return nil, false, fmt.Errorf("insert system inbox message: %w", err)
	}

	s.cacheMessageSender(ctx, msg.GetTopic(), topicSeq, msg.GetSenderId())
	return []int64{targetUID}, false, nil
}

// BackupTopicSeq saves the current max seq for a topic to MongoDB as a fallback.
// Uses $max for atomic concurrency-safe updates.
func (s *Store) BackupTopicSeq(ctx context.Context, topic string, seq uint64) error {
	s.incMongoOp(CollectionTopicSeqs, "update")
	filter := bson.M{"topic": topic}
	update := bson.M{
		"$max": bson.M{
			"max_seq": seq,
		},
		"$setOnInsert": bson.M{
			"topic":      topic,
			"updated_at": time.Now(),
		},
	}
	opts := options.UpdateOne().SetUpsert(true)

	_, err := s.collection(CollectionTopicSeqs).UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("backup topic seq: %w", err)
	}
	return nil
}

// SaveMentionInbox saves @mention notifications for specified users.
// This implements the special write扩散 for @mentions in large groups (read扩散).
// Returns ErrDuplicateKey if a mention already exists (idempotent).
func (s *Store) SaveMentionInbox(ctx context.Context, msg *domain.Message, mentionedUserIDs []int64) error {
	msgID, topicSeq := msg.ID, msg.TopicSeq
	if len(mentionedUserIDs) == 0 {
		return nil
	}

	s.incMongoOp(CollectionMentionInboxes, "insert")
	now := time.Now()
	var docs []interface{}
	for _, uid := range mentionedUserIDs {
		docs = append(docs, &mentionMessageRow{
			UserID:      uid,
			MsgID:       msgID,
			Topic:       msg.GetTopic(),
			SenderID:    msg.GetSenderId(),
			MsgType:     msg.GetMsgType(),
			Content:     msg.GetContent(),
			Timestamp:   msg.GetTimestamp(),
			TopicSeq:    topicSeq,
			ClientMsgID: msg.GetClientMsgId(),
			Read:        false,
			CreatedAt:   now,
		})
	}

	_, err := s.collection(CollectionMentionInboxes).InsertMany(ctx, docs, options.InsertMany().SetOrdered(false))
	if err != nil {
		// With unordered inserts, duplicate errors are returned as BulkWriteException.
		// If every write error is a duplicate key, the operation is idempotent.
		if bulkErr, ok := err.(mongo.BulkWriteException); ok {
			allDuplicate := len(bulkErr.WriteErrors) > 0
			for _, we := range bulkErr.WriteErrors {
				if !IsDuplicateError(we) {
					allDuplicate = false
					break
				}
			}
			if allDuplicate {
				s.log.Debugf("duplicate mention inbox ignored: client_msg_id=%s",
					msg.GetClientMsgId())
				return nil
			}
		}
		return fmt.Errorf("insert mention inbox: %w", err)
	}

	s.log.Debugf("mention inbox saved: msg_id=%d, topic=%s, users=%v", msgID, msg.GetTopic(), mentionedUserIDs)
	return nil
}
