package mongostore

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// AllocateRange atomically reserves [first,last] from the durable
// topic sequence document. The document is the single source of truth for
// production range allocation; Redis is only a cache and cannot be allowed to
// hand the same range to two live workers after a restart.
func (s *Store) AllocateRange(ctx context.Context, topic string, size int, floor uint64) (uint64, uint64, error) {
	if size <= 0 {
		return 0, 0, fmt.Errorf("topic seq range size must be positive")
	}
	coll := s.collection(CollectionTopicSeqs)
	now := time.Now()
	if _, err := coll.UpdateOne(ctx, bson.M{"topic": topic}, bson.M{
		"$max":         bson.M{"max_seq": floor},
		"$setOnInsert": bson.M{"topic": topic, "updated_at": now},
	}, options.UpdateOne().SetUpsert(true)); err != nil && !IsDuplicateError(err) {
		// Concurrent first sends can both attempt the upsert; the loser gets a
		// duplicate-key error, which is harmless because the floor is equal.
		return 0, 0, fmt.Errorf("initialize topic seq floor: %w", err)
	}
	var row topicSeqBackupRow
	if err := coll.FindOneAndUpdate(ctx, bson.M{"topic": topic}, bson.M{
		"$inc": bson.M{"max_seq": int64(size)},
		"$set": bson.M{"updated_at": now},
	}, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&row); err != nil {
		return 0, 0, fmt.Errorf("allocate topic seq range: %w", err)
	}
	if row.MaxSeq < uint64(size) {
		return 0, 0, fmt.Errorf("topic seq allocator returned invalid max %d for size %d", row.MaxSeq, size)
	}
	last := row.MaxSeq
	first := last - uint64(size) + 1
	return first, last, nil
}
