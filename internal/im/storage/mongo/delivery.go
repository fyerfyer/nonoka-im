package mongostore

import (
	"context"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"sync"
	"time"
)

// UpdateDeliveryStatus marks a message as delivered for a user.
// It updates inbox and mention_inbox in parallel (a message may exist in both
// for group @mentions that are also delivered via write扩散). If neither
// collection contains the message, it falls back to delivery_status for read扩散
// group messages. This keeps the common case to a single parallel round-trip.
func (s *Store) UpdateDeliveryStatus(ctx context.Context, userID int64, topic string, topicSeq uint64) error {
	s.incMongoOp(CollectionDeliveryStatus, "update")
	now := time.Now()
	filter := bson.M{"user_id": userID, "topic": topic, "topic_seq": topicSeq}
	update := bson.M{"$set": bson.M{"delivered_at": now}}

	type updateResult struct {
		matched int64
		err     error
	}
	results := make(chan updateResult, 2)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		res, err := s.collection(CollectionInboxes).UpdateOne(ctx, filter, update, options.UpdateOne())
		matched := int64(0)
		if err == nil && res != nil {
			matched = res.MatchedCount
		}
		results <- updateResult{matched: matched, err: err}
	}()
	go func() {
		defer wg.Done()
		res, err := s.collection(CollectionMentionInboxes).UpdateOne(ctx, filter, update, options.UpdateOne())
		matched := int64(0)
		if err == nil && res != nil {
			matched = res.MatchedCount
		}
		results <- updateResult{matched: matched, err: err}
	}()
	wg.Wait()
	close(results)

	anyMatched := false
	for r := range results {
		if r.err != nil {
			return fmt.Errorf("update delivery status: %w", r.err)
		}
		if r.matched > 0 {
			anyMatched = true
		}
	}

	if anyMatched {
		return nil
	}

	// Fall back to delivery_status for read扩散 group messages.
	ds := &deliveryStatusRow{
		UserID:      userID,
		Topic:       topic,
		TopicSeq:    topicSeq,
		DeliveredAt: now,
	}
	_, err := s.collection(CollectionDeliveryStatus).InsertOne(ctx, ds, options.InsertOne())
	if err != nil {
		if IsDuplicateError(err) {
			return nil // already recorded, ignore
		}
		return fmt.Errorf("insert delivery status: %w", err)
	}

	return nil
}

// UpdateReadStatus marks messages as read up to a certain seq for a user.
// It updates both inbox and mention_inbox collections.
func (s *Store) UpdateReadStatus(ctx context.Context, userID int64, topic string, upToSeq uint64) error {
	s.incMongoOp(CollectionInboxes, "update")
	now := time.Now()
	filter := bson.M{
		"user_id":   userID,
		"topic":     topic,
		"topic_seq": bson.M{"$lte": upToSeq},
		"read":      bson.M{"$ne": true},
	}
	update := bson.M{"$set": bson.M{"read": true, "read_at": now}}

	// Update inbox
	inboxRes, err := s.collection(CollectionInboxes).UpdateMany(ctx, filter, update, options.UpdateMany())
	if err != nil {
		return fmt.Errorf("update inbox read status: %w", err)
	}

	// Update mention inbox
	mentionRes, err := s.collection(CollectionMentionInboxes).UpdateMany(ctx, filter, update, options.UpdateMany())
	if err != nil {
		return fmt.Errorf("update mention inbox read status: %w", err)
	}

	s.log.Debugf("read status updated: user_id=%d, topic=%s, up_to_seq=%d, inbox=%d, mention=%d",
		userID, topic, upToSeq, inboxRes.ModifiedCount, mentionRes.ModifiedCount)
	return nil
}

// RecallMessage marks a message as recalled in every storage representation.
// The caller must authorize the sender before invoking this method.
func (s *Store) RecallMessage(ctx context.Context, topic string, topicSeq uint64, msgID int64, senderID int64, window time.Duration) error {
	filter := bson.M{"topic": topic, "topic_seq": topicSeq, "sender_id": senderID, "recalled": bson.M{"$ne": true}}
	if msgID > 0 {
		filter["msg_id"] = msgID
	}
	if window > 0 {
		filter["created_at"] = bson.M{"$gte": time.Now().Add(-window)}
	}
	update := bson.M{"$set": bson.M{"recalled": true, "content": []byte(""), "recalled_at": time.Now()}}
	matched := int64(0)
	for _, name := range []string{CollectionMessages, CollectionInboxes, CollectionMentionInboxes} {
		result, err := s.collection(name).UpdateMany(ctx, filter, update, options.UpdateMany())
		if err != nil {
			return fmt.Errorf("recall message in %s: %w", name, err)
		}
		matched += result.MatchedCount
	}
	if matched == 0 {
		return ErrRecallNotFound
	}
	// Search is a derived index; a failed enqueue is logged by the caller's
	// metrics/relay and never blocks the user-visible recall.
	_ = s.enqueueSearchDelete(ctx, msgID, topic)
	return nil
}

// GetMessageSender returns the sender_id of a message by topic and topic_seq.
// It first checks Redis cache, then searches inbox, mention_inbox, and messages
// collections in parallel (#25).
func (s *Store) GetMessageSender(ctx context.Context, topic string, topicSeq uint64) (int64, error) {
	if senderID, ok := s.getCachedMessageSender(ctx, topic, topicSeq); ok {
		return senderID, nil
	}

	type result struct {
		senderID int64
		err      error
	}

	ch := make(chan result, 3)

	// Query all three collections concurrently
	go func() {
		var doc struct {
			SenderID int64 `bson:"sender_id"`
		}
		err := s.collection(CollectionInboxes).FindOne(ctx, bson.M{
			"topic":     topic,
			"topic_seq": topicSeq,
		}).Decode(&doc)
		if err == nil {
			ch <- result{senderID: doc.SenderID}
			return
		}
		ch <- result{err: err}
	}()

	go func() {
		var doc struct {
			SenderID int64 `bson:"sender_id"`
		}
		err := s.collection(CollectionMentionInboxes).FindOne(ctx, bson.M{
			"topic":     topic,
			"topic_seq": topicSeq,
		}).Decode(&doc)
		if err == nil {
			ch <- result{senderID: doc.SenderID}
			return
		}
		ch <- result{err: err}
	}()

	go func() {
		var doc struct {
			SenderID int64 `bson:"sender_id"`
		}
		err := s.collection(CollectionMessages).FindOne(ctx, bson.M{
			"topic":     topic,
			"topic_seq": topicSeq,
		}).Decode(&doc)
		if err == nil {
			ch <- result{senderID: doc.SenderID}
			return
		}
		ch <- result{err: err}
	}()

	var lastErr error
	for i := 0; i < 3; i++ {
		r := <-ch
		if r.err == nil {
			s.cacheMessageSender(ctx, topic, topicSeq, r.senderID)
			return r.senderID, nil
		}
		lastErr = r.err
	}

	if lastErr == mongo.ErrNoDocuments {
		return 0, fmt.Errorf("message not found: topic=%s, seq=%d", topic, topicSeq)
	}
	return 0, fmt.Errorf("find message sender: %w", lastErr)
}
