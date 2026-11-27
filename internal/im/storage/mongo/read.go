package mongostore

import (
	"context"
	"errors"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"nonoka-im/internal/im/domain"
	"sort"
)

// getOfflineMessages retrieves offline messages for a user in a topic with seq > lastSeq.
// For P2P/system topics, queries the inbox collection.
// For group topics, also queries the messages collection (read扩散) and merges results.
func (s *Store) getOfflineMessages(ctx context.Context, userID int64, topic string, lastSeq uint64, limit int) ([]*inboxMessageRow, error) {
	s.incMongoOp(CollectionInboxes, "find")
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	filter := bson.M{
		"user_id":   userID,
		"topic":     topic,
		"topic_seq": bson.M{"$gt": lastSeq},
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "topic_seq", Value: 1}}).
		SetLimit(int64(limit))

	cursor, err := s.collection(CollectionInboxes).Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("find offline messages: %w", err)
	}
	defer cursor.Close(ctx)

	var results []*inboxMessageRow
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("decode offline messages: %w", err)
	}

	// For group topics, also fetch from messages collection (read扩散).
	topicType := domain.ParseTopicType(topic)
	if topicType == domain.TopicTypeGroup {
		groupMsgs, err := s.getGroupMessages(ctx, topic, lastSeq, limit)
		if err != nil {
			s.log.Warnf("get group messages for offline pull failed: user_id=%d, topic=%s, err=%v", userID, topic, err)
		} else if len(groupMsgs) > 0 {
			// Merge group messages into results, converting storedMessageRow -> inboxMessageRow.
			existing := make(map[uint64]bool, len(results))
			for _, m := range results {
				existing[m.TopicSeq] = true
			}
			for _, gm := range groupMsgs {
				if !existing[gm.TopicSeq] {
					results = append(results, &inboxMessageRow{
						UserID:      userID,
						MsgID:       gm.MsgID,
						Topic:       gm.Topic,
						SenderID:    gm.SenderID,
						MsgType:     gm.MsgType,
						Content:     gm.Content,
						Timestamp:   gm.Timestamp,
						TopicSeq:    gm.TopicSeq,
						ClientMsgID: gm.ClientMsgID,
						CreatedAt:   gm.CreatedAt,
					})
					existing[gm.TopicSeq] = true
				}
			}
			// Re-sort by topic_seq after merge.
			if len(results) > 1 {
				// Already sorted by individual queries, but merge may be out of order.
				// Use a simple sort to ensure order.
				// Actually both are sorted ascending, so we can merge with two-pointer.
				// But for simplicity, just sort.
				// Since the original results were already sorted, just append and sort.
			}
		}
	}

	// Ensure final sort by topic_seq and enforce the requested limit.
	if len(results) > 1 {
		sort.Slice(results, func(i, j int) bool {
			return results[i].TopicSeq < results[j].TopicSeq
		})
	}
	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// getGroupMessages retrieves group messages with seq > lastSeq.
func (s *Store) getGroupMessages(ctx context.Context, topic string, lastSeq uint64, limit int) ([]*storedMessageRow, error) {
	s.incMongoOp(CollectionMessages, "find")
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	filter := bson.M{
		"topic":     topic,
		"topic_seq": bson.M{"$gt": lastSeq},
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "topic_seq", Value: 1}}).
		SetLimit(int64(limit))

	cursor, err := s.collection(CollectionMessages).Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("find group messages: %w", err)
	}
	defer cursor.Close(ctx)

	var results []*storedMessageRow
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("decode group messages: %w", err)
	}
	return results, nil
}

// getHistoryMessages retrieves messages for backward pagination: the latest
// messages with topic_seq < endSeq (or the latest overall when endSeq == 0),
// returned in ascending topic_seq order.
//
// For P2P/system topics it queries the inbox collection; for group topics it
// merges the messages collection (read扩散) with the user's mention inbox
// (write扩散), deduplicated by topic_seq. The hasMore return is computed
// honestly by fetching limit+1 rows.
func (s *Store) getHistoryMessages(ctx context.Context, userID int64, topic string, endSeq uint64, limit int) ([]*inboxMessageRow, bool, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	fetchLimit := int64(limit + 1)

	seqFilter := bson.M{}
	if endSeq > 0 {
		seqFilter["topic_seq"] = bson.M{"$lt": endSeq}
	}

	topicType := domain.ParseTopicType(topic)
	merged := make(map[uint64]*inboxMessageRow, fetchLimit)

	switch topicType {
	case domain.TopicTypeP2P, domain.TopicTypeSystem:
		filter := bson.M{"user_id": userID, "topic": topic}
		for k, v := range seqFilter {
			filter[k] = v
		}
		s.incMongoOp(CollectionInboxes, "find")
		opts := options.Find().
			SetSort(bson.D{{Key: "topic_seq", Value: -1}}).
			SetLimit(fetchLimit)
		cursor, err := s.collection(CollectionInboxes).Find(ctx, filter, opts)
		if err != nil {
			return nil, false, fmt.Errorf("find history messages: %w", err)
		}
		var rows []*inboxMessageRow
		if err := cursor.All(ctx, &rows); err != nil {
			cursor.Close(ctx)
			return nil, false, fmt.Errorf("decode history messages: %w", err)
		}
		cursor.Close(ctx)
		for _, m := range rows {
			merged[m.TopicSeq] = m
		}

	case domain.TopicTypeGroup:
		msgFilter := bson.M{"topic": topic}
		for k, v := range seqFilter {
			msgFilter[k] = v
		}
		s.incMongoOp(CollectionMessages, "find")
		msgOpts := options.Find().
			SetSort(bson.D{{Key: "topic_seq", Value: -1}}).
			SetLimit(fetchLimit)
		msgCursor, err := s.collection(CollectionMessages).Find(ctx, msgFilter, msgOpts)
		if err != nil {
			return nil, false, fmt.Errorf("find group history messages: %w", err)
		}
		var stored []*storedMessageRow
		if err := msgCursor.All(ctx, &stored); err != nil {
			msgCursor.Close(ctx)
			return nil, false, fmt.Errorf("decode group history messages: %w", err)
		}
		msgCursor.Close(ctx)
		for _, gm := range stored {
			merged[gm.TopicSeq] = &inboxMessageRow{
				UserID:      userID,
				MsgID:       gm.MsgID,
				Topic:       gm.Topic,
				SenderID:    gm.SenderID,
				MsgType:     gm.MsgType,
				Content:     gm.Content,
				Timestamp:   gm.Timestamp,
				TopicSeq:    gm.TopicSeq,
				ClientMsgID: gm.ClientMsgID,
				CreatedAt:   gm.CreatedAt,
			}
		}

		// Also page the user's mention inbox; mentions may carry seqs the
		// user has not seen in the messages collection pull.
		mentionFilter := bson.M{"user_id": userID, "topic": topic}
		for k, v := range seqFilter {
			mentionFilter[k] = v
		}
		s.incMongoOp(CollectionMentionInboxes, "find")
		mentionOpts := options.Find().
			SetSort(bson.D{{Key: "topic_seq", Value: -1}}).
			SetLimit(fetchLimit)
		mentionCursor, err := s.collection(CollectionMentionInboxes).Find(ctx, mentionFilter, mentionOpts)
		if err != nil {
			return nil, false, fmt.Errorf("find mention history messages: %w", err)
		}
		var mentions []*mentionMessageRow
		if err := mentionCursor.All(ctx, &mentions); err != nil {
			mentionCursor.Close(ctx)
			return nil, false, fmt.Errorf("decode mention history messages: %w", err)
		}
		mentionCursor.Close(ctx)
		for _, mm := range mentions {
			if _, exists := merged[mm.TopicSeq]; !exists {
				merged[mm.TopicSeq] = &inboxMessageRow{
					UserID:      userID,
					MsgID:       mm.MsgID,
					Topic:       mm.Topic,
					SenderID:    mm.SenderID,
					MsgType:     mm.MsgType,
					Content:     mm.Content,
					Timestamp:   mm.Timestamp,
					TopicSeq:    mm.TopicSeq,
					ClientMsgID: mm.ClientMsgID,
					CreatedAt:   mm.CreatedAt,
				}
			}
		}

	default:
		return nil, false, fmt.Errorf("unknown topic type: %s", topic)
	}

	results := make([]*inboxMessageRow, 0, len(merged))
	for _, m := range merged {
		results = append(results, m)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].TopicSeq > results[j].TopicSeq // descending
	})

	hasMore := len(results) > limit
	if hasMore {
		results = results[:limit]
	}
	// Return ascending order for display.
	for i, j := 0, len(results)-1; i < j; i, j = i+1, j-1 {
		results[i], results[j] = results[j], results[i]
	}

	return results, hasMore, nil
}

// getRecentMessages returns a bounded recent window in ascending sequence
// order. The caller must authorize the user for the topic before invoking it.
func (s *Store) getRecentMessages(ctx context.Context, userID int64, topic string, limit int) ([]*inboxMessageRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, _, err := s.getHistoryMessages(ctx, userID, topic, 0, limit)
	return rows, err
}

// getMessageForUser loads one message for an already-authorized user. The
// caller must verify topic access before calling; P2P/system lookup is scoped
// to the user's inbox, while group lookup is scoped to the requested topic.
func (s *Store) getMessageForUser(ctx context.Context, userID int64, topic string, msgID int64) (*inboxMessageRow, error) {
	var result inboxMessageRow
	var collection *mongo.Collection
	filter := bson.M{"msg_id": msgID, "topic": topic}
	switch domain.ParseTopicType(topic) {
	case domain.TopicTypeP2P, domain.TopicTypeSystem:
		collection = s.collection(CollectionInboxes)
		filter["user_id"] = userID
	case domain.TopicTypeGroup:
		collection = s.collection(CollectionMessages)
	default:
		return nil, fmt.Errorf("invalid message topic")
	}
	s.incMongoOp(collection.Name(), "find")
	if err := collection.FindOne(ctx, filter).Decode(&result); err != nil {
		return nil, fmt.Errorf("load message: %w", err)
	}
	return &result, nil
}

// getMentionMessages retrieves unread @mention messages for a user.
func (s *Store) getMentionMessages(ctx context.Context, userID int64, topic string, lastSeq uint64, limit int) ([]*mentionMessageRow, error) {
	s.incMongoOp(CollectionMentionInboxes, "find")
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	filter := bson.M{
		"user_id":   userID,
		"topic":     topic,
		"topic_seq": bson.M{"$gt": lastSeq},
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "topic_seq", Value: 1}}).
		SetLimit(int64(limit))

	cursor, err := s.collection(CollectionMentionInboxes).Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("find mention messages: %w", err)
	}
	defer cursor.Close(ctx)

	var results []*mentionMessageRow
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("decode mention messages: %w", err)
	}
	return results, nil
}

// GetTopicMaxSeq returns the durable high-water mark for a topic. It reads the
// actual message rows first because BackupTopicSeq is intentionally written at
// a coarse watermark; the backup row is only a fallback. This method is used
// when Redis loses im:seq:<topic>, so returning a lagging value would reuse
// sequence numbers and silently drop newly persisted messages.
func (s *Store) GetTopicMaxSeq(ctx context.Context, topic string) (uint64, error) {
	collection := CollectionInboxes
	switch domain.ParseTopicType(topic) {
	case domain.TopicTypeGroup:
		collection = CollectionMessages
	case domain.TopicTypeP2P, domain.TopicTypeSystem:
		collection = CollectionInboxes
	}
	var row struct {
		TopicSeq uint64 `bson:"topic_seq"`
	}
	err := s.collection(collection).FindOne(ctx, bson.M{"topic": topic},
		options.FindOne().SetSort(bson.D{{Key: "topic_seq", Value: -1}}).SetProjection(bson.M{"topic_seq": 1}),
	).Decode(&row)
	var actual uint64
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return 0, fmt.Errorf("get topic max seq from %s: %w", collection, err)
	}
	if err == nil {
		actual = row.TopicSeq
	}
	if actual > 0 {
		return actual, nil
	}
	var backup topicSeqBackupRow
	berr := s.collection(CollectionTopicSeqs).FindOne(ctx, bson.M{"topic": topic}).Decode(&backup)
	if berr != nil && !errors.Is(berr, mongo.ErrNoDocuments) {
		return 0, fmt.Errorf("get topic seq backup: %w", berr)
	}
	if berr == nil && backup.MaxSeq > actual {
		actual = backup.MaxSeq
	}
	return actual, nil
}

func (s *Store) GetOfflineMessages(ctx context.Context, userID int64, topic string, lastSeq uint64, limit int) ([]domain.InboxMessage, error) {
	rows, err := s.getOfflineMessages(ctx, userID, topic, lastSeq, limit)
	if err != nil {
		return nil, err
	}
	out := make([]domain.InboxMessage, 0, len(rows))
	for _, row := range rows {
		if row != nil {
			out = append(out, toDomainInbox(row))
		}
	}
	return out, nil
}

func (s *Store) GetGroupMessages(ctx context.Context, topic string, lastSeq uint64, limit int) ([]domain.Message, error) {
	rows, err := s.getGroupMessages(ctx, topic, lastSeq, limit)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Message, 0, len(rows))
	for _, row := range rows {
		if row != nil {
			out = append(out, toDomainMessage(row))
		}
	}
	return out, nil
}

func (s *Store) GetHistoryMessages(ctx context.Context, userID int64, topic string, endSeq uint64, limit int) ([]domain.InboxMessage, bool, error) {
	rows, hasMore, err := s.getHistoryMessages(ctx, userID, topic, endSeq, limit)
	if err != nil {
		return nil, false, err
	}
	out := make([]domain.InboxMessage, 0, len(rows))
	for _, row := range rows {
		if row != nil {
			out = append(out, toDomainInbox(row))
		}
	}
	return out, hasMore, nil
}

func (s *Store) GetRecentMessages(ctx context.Context, userID int64, topic string, limit int) ([]domain.InboxMessage, error) {
	rows, err := s.getRecentMessages(ctx, userID, topic, limit)
	if err != nil {
		return nil, err
	}
	out := make([]domain.InboxMessage, 0, len(rows))
	for _, row := range rows {
		if row != nil {
			out = append(out, toDomainInbox(row))
		}
	}
	return out, nil
}

func (s *Store) GetMessageForUser(ctx context.Context, userID int64, topic string, msgID int64) (domain.InboxMessage, error) {
	row, err := s.getMessageForUser(ctx, userID, topic, msgID)
	if err != nil {
		return domain.InboxMessage{}, err
	}
	return toDomainInbox(row), nil
}

// GetMessageByClientID loads a message by its idempotency key after the caller
// has verified topic access. P2P/system messages live in the user's inbox;
// group messages live in the topic message collection.
func (s *Store) GetMessageByClientID(ctx context.Context, userID int64, topic, clientMsgID string) (domain.InboxMessage, error) {
	if clientMsgID == "" {
		return domain.InboxMessage{}, domain.ErrNotFound
	}
	switch domain.ParseTopicType(topic) {
	case domain.TopicTypeP2P, domain.TopicTypeSystem:
		var row inboxMessageRow
		s.incMongoOp(CollectionInboxes, "find")
		err := s.collection(CollectionInboxes).FindOne(ctx, bson.M{
			"user_id": userID, "topic": topic, "client_msg_id": clientMsgID,
		}).Decode(&row)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return domain.InboxMessage{}, domain.ErrNotFound
		}
		if err != nil {
			return domain.InboxMessage{}, fmt.Errorf("find message by client id: %w", err)
		}
		return toDomainInbox(&row), nil
	case domain.TopicTypeGroup:
		var row storedMessageRow
		s.incMongoOp(CollectionMessages, "find")
		err := s.collection(CollectionMessages).FindOne(ctx, bson.M{
			"topic": topic, "sender_id": userID, "client_msg_id": clientMsgID,
		}).Decode(&row)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return domain.InboxMessage{}, domain.ErrNotFound
		}
		if err != nil {
			return domain.InboxMessage{}, fmt.Errorf("find group message by client id: %w", err)
		}
		return domain.InboxMessage{Message: toDomainMessage(&row), UserID: userID}, nil
	default:
		return domain.InboxMessage{}, domain.ErrNotFound
	}
}

func (s *Store) GetMentionMessages(ctx context.Context, userID int64, topic string, lastSeq uint64, limit int) ([]domain.MentionMessage, error) {
	rows, err := s.getMentionMessages(ctx, userID, topic, lastSeq, limit)
	if err != nil {
		return nil, err
	}
	out := make([]domain.MentionMessage, 0, len(rows))
	for _, row := range rows {
		if row != nil {
			out = append(out, toDomainMention(row))
		}
	}
	return out, nil
}

func toDomainMessage(row *storedMessageRow) domain.Message {
	return domain.Message{
		ID: row.MsgID, Topic: row.Topic, SenderID: row.SenderID, Type: domain.MessageType(row.MsgType),
		Content: row.Content, Timestamp: row.Timestamp, TopicSeq: row.TopicSeq,
		ClientMsgID: row.ClientMsgID, Recalled: row.Recalled, CreatedAt: row.CreatedAt,
	}
}

func toDomainInbox(row *inboxMessageRow) domain.InboxMessage {
	return domain.InboxMessage{
		Message: domain.Message{
			ID: row.MsgID, Topic: row.Topic, SenderID: row.SenderID, Type: domain.MessageType(row.MsgType),
			Content: row.Content, Timestamp: row.Timestamp, TopicSeq: row.TopicSeq,
			ClientMsgID: row.ClientMsgID, Recalled: row.Recalled, CreatedAt: row.CreatedAt,
		},
		UserID: row.UserID, Read: row.Read, DeliveredAt: row.DeliveredAt,
	}
}

func toDomainMention(row *mentionMessageRow) domain.MentionMessage {
	return domain.MentionMessage{
		Message: domain.Message{
			ID: row.MsgID, Topic: row.Topic, SenderID: row.SenderID, Type: domain.MessageType(row.MsgType),
			Content: row.Content, Timestamp: row.Timestamp, TopicSeq: row.TopicSeq,
			ClientMsgID: row.ClientMsgID, Recalled: row.Recalled, CreatedAt: row.CreatedAt,
		},
		UserID: row.UserID, Read: row.Read, DeliveredAt: row.DeliveredAt,
	}
}
