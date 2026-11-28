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
	recipients := []int64{receiverID, msg.GetSenderId()}

	// The two inbox copies are one logical message. A retry after a partial
	// write must reuse the original msg_id/topic_seq instead of allocating a
	// new sequence and leaving the participants on different sequences.
	existing, err := s.findP2PExisting(ctx, msg.GetTopic(), msg.GetClientMsgId(), uid1, uid2)
	if err != nil {
		return nil, false, err
	}
	for _, row := range existing {
		msgID, topicSeq = row.MsgID, row.TopicSeq
		break
	}
	if len(existing) == len(recipients) {
		s.cacheMessageSender(ctx, msg.GetTopic(), topicSeq, msg.GetSenderId())
		s.log.Debugf("duplicate p2p message ignored: client_msg_id=%s, sender=%d",
			msg.GetClientMsgId(), msg.GetSenderId())
		return recipients, true, nil
	}
	msg.ID, msg.TopicSeq = msgID, topicSeq

	now := time.Now()
	docs := make([]interface{}, 0, len(recipients))
	for _, userID := range recipients {
		if _, done := existing[userID]; done {
			continue
		}
		docs = append(docs, &inboxMessageRow{
			UserID:      userID,
			MsgID:       msgID,
			Topic:       msg.GetTopic(),
			SenderID:    msg.GetSenderId(),
			MsgType:     msg.GetMsgType(),
			Content:     msg.GetContent(),
			ContentText: search.ContentText(msg.GetMsgType(), msg.GetContent()),
			Timestamp:   msg.GetTimestamp(),
			TopicSeq:    topicSeq,
			ClientMsgID: msg.GetClientMsgId(),
			Read:        userID == msg.GetSenderId(), // sender's own copy is considered read
			CreatedAt:   now,
		})
	}
	if len(docs) > 0 {
		if _, err = s.collection(CollectionInboxes).InsertMany(ctx, docs, options.InsertMany().SetOrdered(false)); err != nil {
			// A concurrent delivery can win the unique index between the
			// pre-check and insert. Let the caller retry; the next attempt
			// observes the existing row and repairs only the missing copy.
			return nil, false, fmt.Errorf("insert p2p inbox messages: %w", err)
		}
	}

	s.cacheMessageSender(ctx, msg.GetTopic(), topicSeq, msg.GetSenderId())
	s.log.Debugf("p2p message saved: msg_id=%d, topic=%s, receiver=%d, sender=%d", msgID, msg.GetTopic(), receiverID, msg.GetSenderId())
	return recipients, false, nil
}

// findP2PExisting returns the participant inbox rows that already exist for a
// client idempotency key. It is the first statement in the message transaction
// so a retry does not hit a duplicate-key abort inside MongoDB.
func (s *Store) findP2PExisting(ctx context.Context, topic, clientMsgID string, userIDs ...int64) (map[int64]*inboxMessageRow, error) {
	out := make(map[int64]*inboxMessageRow, len(userIDs))
	if clientMsgID == "" || len(userIDs) == 0 {
		return out, nil
	}
	cursor, err := s.collection(CollectionInboxes).Find(ctx, bson.M{
		"user_id": bson.M{"$in": userIDs}, "topic": topic, "client_msg_id": clientMsgID,
	})
	if err != nil {
		return nil, fmt.Errorf("find existing p2p inbox messages: %w", err)
	}
	defer cursor.Close(ctx)
	for cursor.Next(ctx) {
		var row inboxMessageRow
		if err := cursor.Decode(&row); err != nil {
			return nil, fmt.Errorf("decode existing p2p inbox message: %w", err)
		}
		out[row.UserID] = &row
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("iterate existing p2p inbox messages: %w", err)
	}
	return out, nil
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
			// A duplicate can be either the idempotent client_msg_id retry or
			// a colliding topic_seq from a lost Redis sequence. Only the
			// former is safe to swallow; otherwise the message was dropped and
			// must be surfaced/retried.
			exists, countErr := s.collection(CollectionMessages).CountDocuments(ctx, bson.M{
				"topic": msg.GetTopic(), "sender_id": msg.GetSenderId(), "client_msg_id": msg.GetClientMsgId(),
			})
			if countErr != nil {
				return false, fmt.Errorf("verify duplicate group message: %w", countErr)
			}
			if exists == 0 {
				return false, fmt.Errorf("duplicate topic seq without matching client_msg_id: topic=%s seq=%d client_msg_id=%s",
					msg.GetTopic(), msg.TopicSeq, msg.GetClientMsgId())
			}
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
	inserted, duplicate, err := s.saveGroupBatch(ctx, msgs)
	if err != nil || duplicate {
		return inserted, duplicate, err
	}
	for _, msg := range msgs {
		s.enqueueSearchBestEffort(ctx, msg)
	}
	return inserted, false, nil
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

	if _, err := s.collection(CollectionInboxes).InsertOne(ctx, inbox, options.InsertOne()); err != nil {
		if IsDuplicateError(err) {
			// A duplicate on (user_id, topic, topic_seq) can also be a seq
			// collision rather than a retry. Only treat it as idempotent when
			// the stored row is the same message.
			var existing inboxMessageRow
			check := s.collection(CollectionInboxes).FindOne(ctx, bson.M{
				"user_id": targetUID, "topic": msg.GetTopic(), "topic_seq": topicSeq,
			}).Decode(&existing)
			clientID := msg.GetClientMsgId()
			if check == nil && (existing.MsgID == msgID || (clientID != "" && existing.ClientMsgID == clientID)) {
				s.log.Debugf("duplicate system message ignored: client_msg_id=%s, sender=%d",
					clientID, msg.GetSenderId())
				return []int64{targetUID}, true, nil
			}
			if clientID != "" {
				var byClient inboxMessageRow
				if err := s.collection(CollectionInboxes).FindOne(ctx, bson.M{
					"user_id": targetUID, "client_msg_id": clientID,
				}).Decode(&byClient); err == nil && byClient.ClientMsgID == clientID {
					s.log.Debugf("duplicate system message ignored: client_msg_id=%s, sender=%d",
						clientID, msg.GetSenderId())
					return []int64{targetUID}, true, nil
				}
			}
			return nil, false, fmt.Errorf("insert system inbox message: conflicting duplicate topic=%s seq=%d msg_id=%d: %w",
				msg.GetTopic(), topicSeq, msgID, err)
		}
		return nil, false, fmt.Errorf("insert system inbox message: %w", err)
	}

	s.cacheMessageSender(ctx, msg.GetTopic(), topicSeq, msg.GetSenderId())
	s.enqueueSearchBestEffort(ctx, msg)
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
	unique := make([]int64, 0, len(mentionedUserIDs))
	seen := make(map[int64]struct{}, len(mentionedUserIDs))
	for _, uid := range mentionedUserIDs {
		if uid <= 0 {
			continue
		}
		if _, ok := seen[uid]; ok {
			continue
		}
		seen[uid] = struct{}{}
		unique = append(unique, uid)
	}
	if len(unique) == 0 {
		return nil
	}

	s.incMongoOp(CollectionMentionInboxes, "insert")
	now := time.Now()
	docs := make([]interface{}, 0, len(unique))
	for _, uid := range unique {
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
		// Every duplicate is only safe when all mentioned users already have the
		// same message; otherwise the missing rows must be repaired by retrying.
		if bulkErr, ok := err.(mongo.BulkWriteException); ok {
			allDuplicate := len(bulkErr.WriteErrors) > 0
			for _, we := range bulkErr.WriteErrors {
				if !IsDuplicateError(we) {
					allDuplicate = false
					break
				}
			}
			if allDuplicate {
				existing, countErr := s.collection(CollectionMentionInboxes).CountDocuments(ctx, bson.M{
					"user_id": bson.M{"$in": unique}, "topic": msg.GetTopic(), "client_msg_id": msg.GetClientMsgId(),
				})
				if countErr != nil {
					return fmt.Errorf("verify duplicate mention inbox: %w", countErr)
				}
				if existing != int64(len(unique)) {
					return fmt.Errorf("duplicate mention inbox is incomplete: topic=%s seq=%d expected=%d existing=%d",
						msg.GetTopic(), topicSeq, len(unique), existing)
				}
				s.log.Debugf("duplicate mention inbox ignored: client_msg_id=%s",
					msg.GetClientMsgId())
				return nil
			}
		}
		return fmt.Errorf("insert mention inbox: %w", err)
	}

	s.log.Debugf("mention inbox saved: msg_id=%d, topic=%s, users=%v", msgID, msg.GetTopic(), unique)
	return nil
}
