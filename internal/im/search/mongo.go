package search

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"nonoka-im/internal/im/domain"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// MongoIndex is the zero-dependency search fallback. It uses the content_text
// projection already written with each message and a topic-scoped regex. It is
// slower than Meilisearch on large corpora but keeps small self-hosted
// deployments fully functional without another service.
type MongoIndex struct {
	db *mongo.Database
}

func NewMongoIndex(db *mongo.Database) *MongoIndex { return &MongoIndex{db: db} }

func (m *MongoIndex) Healthy(ctx context.Context) error {
	if m == nil || m.db == nil {
		return fmt.Errorf("mongodb search is not configured")
	}
	return m.db.Client().Ping(ctx, nil)
}

func (m *MongoIndex) Search(ctx context.Context, topic, query string, filter domain.SearchFilter, limit, offset int) ([]domain.SearchHit, bool, error) {
	if m == nil || m.db == nil {
		return nil, false, fmt.Errorf("mongodb search is not configured")
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	collection := "messages"
	topicType := domain.ParseTopicType(topic)
	if topicType == domain.TopicTypeP2P || topicType == domain.TopicTypeSystem {
		collection = "inboxes"
	}
	query = strings.TrimSpace(query)
	match := bson.M{"topic": topic, "recalled": bson.M{"$ne": true}}
	if query != "" {
		match["content_text"] = bson.M{"$regex": regexp.QuoteMeta(query), "$options": "i"}
	}
	if filter.SenderID > 0 {
		match["sender_id"] = filter.SenderID
	}
	if filter.Since > 0 || filter.Until > 0 {
		bounds := bson.M{}
		if filter.Since > 0 {
			bounds["$gte"] = filter.Since
		}
		if filter.Until > 0 {
			bounds["$lte"] = filter.Until
		}
		match["timestamp"] = bounds
	}
	if filter.HasFile {
		match["msg_type"] = bson.M{"$in": []int32{2, 3}}
	}

	// Inboxes contain one copy per participant; fetch extra rows so duplicate
	// msg_ids cannot starve the requested page.
	fetchLimit := limit + 1
	if collection == "inboxes" {
		fetchLimit = (limit + 1) * 4
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "timestamp", Value: -1}, {Key: "msg_id", Value: -1}}).
		SetSkip(int64(offset)).
		SetLimit(int64(fetchLimit)).
		SetProjection(bson.M{
			"msg_id": 1, "topic": 1, "topic_seq": 1, "sender_id": 1,
			"timestamp": 1, "msg_type": 1, "content_text": 1,
		})
	cursor, err := m.db.Collection(collection).Find(ctx, match, opts)
	if err != nil {
		return nil, false, err
	}
	defer cursor.Close(ctx)

	var rows []struct {
		MsgID       int64  `bson:"msg_id"`
		Topic       string `bson:"topic"`
		TopicSeq    uint64 `bson:"topic_seq"`
		SenderID    int64  `bson:"sender_id"`
		Timestamp   int64  `bson:"timestamp"`
		MsgType     int32  `bson:"msg_type"`
		ContentText string `bson:"content_text"`
	}
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, false, err
	}
	seen := make(map[int64]struct{}, len(rows))
	hits := make([]domain.SearchHit, 0, limit)
	for _, row := range rows {
		if _, duplicate := seen[row.MsgID]; duplicate {
			continue
		}
		seen[row.MsgID] = struct{}{}
		hits = append(hits, domain.SearchHit{
			MsgID: row.MsgID, Topic: row.Topic, TopicSeq: row.TopicSeq, SenderID: row.SenderID,
			Timestamp: row.Timestamp, MsgType: row.MsgType,
			HasFile: HasFile(row.MsgType), Snippet: mongoSnippet(row.ContentText, query),
		})
		if len(hits) == limit+1 {
			break
		}
	}
	hasMore := len(hits) > limit
	if hasMore {
		hits = hits[:limit]
	}
	return hits, hasMore, nil
}

// BackfillContentText derives content_text for messages written before the
// search projection existed. It is idempotent and only fills empty fields.
func BackfillContentText(ctx context.Context, db *mongo.Database, batchSize int) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("mongodb is required")
	}
	if batchSize <= 0 || batchSize > 1000 {
		batchSize = 500
	}
	total := 0
	for _, collection := range []string{"messages", "inboxes", "mention_inboxes"} {
		cursor, err := db.Collection(collection).Find(ctx, bson.M{
			"$or": []bson.M{
				{"content_text": bson.M{"$exists": false}},
				{"content_text": ""},
			},
		})
		if err != nil {
			return total, err
		}
		models := make([]mongo.WriteModel, 0, batchSize)
		for cursor.Next(ctx) {
			var row struct {
				ID      bson.ObjectID `bson:"_id"`
				MsgType int32         `bson:"msg_type"`
				Content []byte        `bson:"content"`
			}
			if err := cursor.Decode(&row); err != nil {
				_ = cursor.Close(ctx)
				return total, err
			}
			text := ContentText(row.MsgType, row.Content)
			if text == "" {
				continue
			}
			models = append(models, mongo.NewUpdateOneModel().
				SetFilter(bson.M{"_id": row.ID}).
				SetUpdate(bson.M{"$set": bson.M{"content_text": text}}))
			if len(models) >= batchSize {
				if _, err := db.Collection(collection).BulkWrite(ctx, models); err != nil {
					_ = cursor.Close(ctx)
					return total, err
				}
				total += len(models)
				models = models[:0]
			}
		}
		if err := cursor.Err(); err != nil {
			_ = cursor.Close(ctx)
			return total, err
		}
		_ = cursor.Close(ctx)
		if len(models) > 0 {
			if _, err := db.Collection(collection).BulkWrite(ctx, models); err != nil {
				return total, err
			}
			total += len(models)
		}
	}
	return total, nil
}

func mongoSnippet(text, query string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	const max = 240
	if len(text) <= max {
		return text
	}
	start := 0
	if query != "" {
		if idx := strings.Index(strings.ToLower(text), strings.ToLower(query)); idx > 40 {
			start = idx - 40
		}
	}
	text = text[start:]
	if len(text) > max {
		text = text[:max]
	}
	for len(text) > 0 && !utf8.ValidString(text) {
		text = text[:len(text)-1]
	}
	if start > 0 {
		return "…" + text
	}
	return text + "…"
}
