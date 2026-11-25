package domain

import "time"

// SearchFilter is the structured filter for a topic-scoped message search.
// Zero values disable the corresponding condition.
type SearchFilter struct {
	SenderID int64
	Since    int64
	Until    int64
	HasFile  bool
}

// SearchHit is one authorized message search result. Snippet is a short,
// plain-text excerpt and never HTML.
type SearchHit struct {
	MsgID     int64
	Topic     string
	TopicSeq  uint64
	SenderID  int64
	Timestamp int64
	MsgType   int32
	HasFile   bool
	Snippet   string
}

// SearchIndexTask is one durable, best-effort index operation. It is written
// in the same MongoDB transaction as the message so a Meilisearch outage
// cannot silently lose an index update.
type SearchIndexTask struct {
	ID            string
	Op            string
	DocID         string
	Payload       []byte
	Status        string
	Attempts      int
	NextAttemptAt time.Time
	LockedUntil   time.Time
	LastError     string
	CreatedAt     time.Time
	PublishedAt   time.Time
}

const (
	SearchIndexUpsert = "upsert"
	SearchIndexDelete = "delete"

	SearchIndexPending    = "pending"
	SearchIndexProcessing = "processing"
	SearchIndexPublished  = "published"
	SearchIndexFailed     = "failed"
)
