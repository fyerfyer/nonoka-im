package search

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Backfill rebuilds the search index from the existing IM collections. It is
// idempotent: document IDs are msg_ids, so rerunning only refreshes documents.
// P2P messages exist in both participants' inboxes; the duplicate upsert is
// harmless and keeps the rebuild free of cross-collection joins.
func Backfill(ctx context.Context, db *mongo.Database, client *Client, batchSize int) (int, error) {
	if db == nil || client == nil || !client.Enabled() {
		return 0, fmt.Errorf("search backfill requires mongodb and a configured search backend")
	}
	if batchSize <= 0 || batchSize > 1000 {
		batchSize = 500
	}
	total := 0
	for _, collection := range []string{"messages", "inboxes", "mention_inboxes"} {
		cursor, err := db.Collection(collection).Find(ctx, bson.M{"recalled": bson.M{"$ne": true}},
			options.Find().SetBatchSize(int32(batchSize)))
		if err != nil {
			return total, fmt.Errorf("scan %s: %w", collection, err)
		}
		batch := make([]Document, 0, batchSize)
		for cursor.Next(ctx) {
			var row struct {
				MsgID       int64  `bson:"msg_id"`
				Topic       string `bson:"topic"`
				SenderID    int64  `bson:"sender_id"`
				MsgType     int32  `bson:"msg_type"`
				Content     []byte `bson:"content"`
				ContentText string `bson:"content_text"`
				Timestamp   int64  `bson:"timestamp"`
				TopicSeq    uint64 `bson:"topic_seq"`
			}
			if err := cursor.Decode(&row); err != nil {
				_ = cursor.Close(ctx)
				return total, fmt.Errorf("decode %s: %w", collection, err)
			}
			if row.MsgID == 0 || row.Topic == "" {
				continue
			}
			text := row.ContentText
			if text == "" {
				text = ContentText(row.MsgType, row.Content)
			}
			doc := Document{
				ID: docID(row.MsgID), Topic: row.Topic, MsgID: row.MsgID, TopicSeq: row.TopicSeq,
				SenderID: row.SenderID, Timestamp: row.Timestamp, MsgType: row.MsgType,
				HasFile: HasFile(row.MsgType), Content: text,
			}
			if doc.Content == "" && !doc.HasFile {
				continue
			}
			batch = append(batch, doc)
			if len(batch) >= batchSize {
				if err := client.IndexDocuments(ctx, batch); err != nil {
					_ = cursor.Close(ctx)
					return total, fmt.Errorf("index %s batch: %w", collection, err)
				}
				total += len(batch)
				batch = batch[:0]
			}
		}
		if err := cursor.Err(); err != nil {
			_ = cursor.Close(ctx)
			return total, fmt.Errorf("scan %s: %w", collection, err)
		}
		_ = cursor.Close(ctx)
		if len(batch) > 0 {
			if err := client.IndexDocuments(ctx, batch); err != nil {
				return total, fmt.Errorf("index %s final batch: %w", collection, err)
			}
			total += len(batch)
		}
	}
	return total, nil
}
