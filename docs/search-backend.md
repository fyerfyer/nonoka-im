# Search backends

Nonoka's search API is stable regardless of the storage used behind it:

```text
GET /v1/message/search?topic=...&query=...
```

The backend is selected on the server. SDK clients always call this API and do
not know whether the deployment uses MongoDB, Meilisearch or a custom adapter.

## Configuration

```yaml
search:
  enabled: true
  backend: mongo        # mongo | meilisearch
  url: ""               # required for meilisearch
  index: messages
  timeout: 5s
  relay_interval: 1s
  relay_batch_size: 32
```

Environment overrides:

```text
SEARCH_BACKEND=mongo|meilisearch
MEILISEARCH_URL=http://meilisearch:7700
MEILISEARCH_API_KEY=...
MEILISEARCH_INDEX=messages
```

`backend` empty is inferred: a non-empty Meilisearch URL selects
`meilisearch`, otherwise `mongo`. Search is disabled when
`search.enabled=false`; the API then returns `SEARCH_UNAVAILABLE` and the Agent
does not advertise `search_messages`.

## MongoDB (default, zero dependency)

`internal/im/search/mongo.go` queries the existing `messages` / `inboxes`
collections:

- filters by `topic`, `sender_id`, timestamp range and `msg_type` (has_file)
- matches case-insensitive substrings in `content_text`
- de-duplicates P2P inbox copies by `msg_id`

This is intended for small/medium self-hosted deployments. For large corpora,
use Meilisearch. Existing messages can be backfilled with:

```bash
make search-reindex
```

With the Mongo backend this fills `content_text`; with Meilisearch it rebuilds
the remote index.

## Meilisearch

Set `backend: meilisearch` and `MEILISEARCH_URL`. Message writes enqueue a
durable `search_index_outbox` task in the same MongoDB transaction; the
`msgworker` relay applies upserts/deletes with retry. The API authorizes the
topic before querying Meilisearch and always applies an exact `topic` filter.

## Custom adapter

The server extension point is `internal/im/app.SearchIndex`:

```go
type SearchIndex interface {
    Search(ctx context.Context, topic, query string, filter domain.SearchFilter, limit, offset int) ([]domain.SearchHit, bool, error)
    Healthy(ctx context.Context) error
}
```

A custom implementation can live in your own package:

```go
type MySearch struct{ /* client, credentials */ }

func (s *MySearch) Search(ctx context.Context, topic, query string, filter domain.SearchFilter, limit, offset int) ([]domain.SearchHit, bool, error) {
    // 1. enforce the exact topic; never widen it
    // 2. apply sender/time/has_file filters before limiting
    // 3. return msg_id, topic, topic_seq, sender_id, timestamp, msg_type, has_file, snippet
}

func (s *MySearch) Healthy(context.Context) error { return nil }
```

Then replace the provider in `cmd/nonoka-im/wire.go` and
`cmd/agent-worker/main.go`:

```go
func provideSearchIndex(...) (imapp.SearchIndex, error) {
    return NewMySearch(...), nil
}
```

For non-Go backends, a later P2 option is a small HTTP search protocol. At that
point a `HTTPSearchIndex` adapter can implement the same interface; the public
Search API and SDK stay unchanged.

### Indexing custom backends

MongoDB uses the `content_text` projection written with every message.
Meilisearch uses the transactional `search_index_outbox`. A custom backend that
needs its own remote index must either:

1. consume the same outbox (recommended), or
2. provide its own durable indexer and a rebuild command.

Do not index outside the authorization boundary: the search API authorizes
`topic` first, and every backend must keep the exact topic filter.

## SDK

```go
result, err := client.Messages.Search(ctx, sdk.SearchRequest{
    Topic: "p2p_1_2",
    Query: "incident",
    SenderID: 0,
    Since:    0,
    Until:    0,
    HasFile:  false,
    Limit:    20,
})
```

`ErrSearchUnavailable` is returned when the deployment has no search backend.
