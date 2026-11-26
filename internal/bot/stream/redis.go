package stream

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"nonoka-im/internal/bot/app"
	"nonoka-im/internal/bot/domain"

	"github.com/redis/go-redis/v9"
)

const keyPrefix = "bot:stream:"

var createScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 1 then
  return 0
end
redis.call('HSET', KEYS[1],
  'bot_id', ARGV[1],
  'bot_user_id', ARGV[2],
  'topic', ARGV[3],
  'client_msg_id', ARGV[4],
  'seq', '0',
  'bytes', '0',
  'state', 'open',
  'content', '',
  'created_at', ARGV[5])
redis.call('EXPIRE', KEYS[1], ARGV[6])
return 1
`)

var appendScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 then
  return -1
end
if redis.call('HGET', KEYS[1], 'bot_id') ~= ARGV[1] then
  return -2
end
if redis.call('HGET', KEYS[1], 'state') ~= 'open' then
  return -3
end
local current = tonumber(redis.call('HGET', KEYS[1], 'seq') or '0')
local seq = tonumber(ARGV[2])
if seq <= current then
  return 0
end
local bytes = tonumber(redis.call('HGET', KEYS[1], 'bytes') or '0')
local delta = ARGV[3]
if bytes + string.len(delta) > tonumber(ARGV[4]) then
  return -4
end
local content = redis.call('HGET', KEYS[1], 'content') or ''
redis.call('HSET', KEYS[1], 'seq', seq, 'bytes', bytes + string.len(delta), 'content', content .. delta)
redis.call('EXPIRE', KEYS[1], ARGV[5])
return 1
`)

var finalScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 then
  return -1
end
if redis.call('HGET', KEYS[1], 'bot_id') ~= ARGV[1] then
  return -2
end
if redis.call('HGET', KEYS[1], 'state') == 'final' then
  redis.call('EXPIRE', KEYS[1], ARGV[2])
  return 0
end
redis.call('HSET', KEYS[1], 'state', 'final')
redis.call('EXPIRE', KEYS[1], ARGV[2])
return 1
`)

// RedisStore implements app.StreamStore with a TTL-bound Redis hash.
type RedisStore struct{ client redis.UniversalClient }

func NewRedisStore(client redis.UniversalClient) *RedisStore { return &RedisStore{client: client} }

func (s *RedisStore) Create(ctx context.Context, state app.StreamState, ttl time.Duration) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("stream redis client unavailable")
	}
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	created := state.CreatedAt
	if created.IsZero() {
		created = time.Now()
	}
	ok, err := createScript.Run(ctx, s.client, []string{key(state.ID)},
		state.BotID, state.BotUserID, state.Topic, state.ClientMsgID, created.Unix(), int(ttl.Seconds())).Int()
	if err != nil {
		return err
	}
	if ok == 0 {
		return domain.ErrConflict
	}
	return nil
}

func (s *RedisStore) Get(ctx context.Context, id string) (*app.StreamState, error) {
	values, err := s.client.HGetAll(ctx, key(id)).Result()
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, domain.ErrStreamExpired
	}
	return parseState(id, values)
}

func (s *RedisStore) Append(ctx context.Context, id string, botID int64, seq uint64, delta string, maxBytes int, ttl time.Duration) (*app.StreamState, error) {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	code, err := appendScript.Run(ctx, s.client, []string{key(id)},
		botID, seq, delta, maxBytes, int(ttl.Seconds())).Int()
	if err != nil {
		return nil, err
	}
	switch code {
	case -1, -3:
		return nil, domain.ErrStreamExpired
	case -2:
		return nil, domain.ErrForbidden
	case -4:
		return nil, fmt.Errorf("%w: stream content is too large", domain.ErrInvalid)
	case 0:
		if state, err := s.Get(ctx, id); err == nil {
			return state, nil
		}
		return nil, domain.ErrStreamExpired
	default:
		return s.Get(ctx, id)
	}
}

func (s *RedisStore) MarkFinal(ctx context.Context, id string, botID int64) (*app.StreamState, error) {
	code, err := finalScript.Run(ctx, s.client, []string{key(id)}, botID, int((10 * time.Minute).Seconds())).Int()
	if err != nil {
		return nil, err
	}
	switch code {
	case -1:
		return nil, domain.ErrStreamExpired
	case -2:
		return nil, domain.ErrForbidden
	default:
		return s.Get(ctx, id)
	}
}

func (s *RedisStore) Delete(ctx context.Context, id string, botID int64) (*app.StreamState, error) {
	state, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if state.BotID != botID {
		return nil, domain.ErrForbidden
	}
	if state.State == app.StreamStateFinal {
		return state, nil
	}
	if err := s.client.Del(ctx, key(id)).Err(); err != nil {
		return nil, err
	}
	return state, nil
}

func (s *RedisStore) AllowChunk(ctx context.Context, id string, limit int) (bool, error) {
	if limit <= 0 {
		limit = 20
	}
	key := "bot:stream:rl:" + id
	n, err := s.client.Incr(ctx, key).Result()
	if err != nil {
		return true, err
	}
	if n == 1 {
		_ = s.client.Expire(ctx, key, time.Second).Err()
	}
	return n <= int64(limit), nil
}

func key(id string) string { return keyPrefix + id }

func parseState(id string, values map[string]string) (*app.StreamState, error) {
	botID, err := strconv.ParseInt(values["bot_id"], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid stream bot id: %w", err)
	}
	botUserID, err := strconv.ParseInt(values["bot_user_id"], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid stream bot user id: %w", err)
	}
	seq, _ := strconv.ParseUint(values["seq"], 10, 64)
	bytes, _ := strconv.Atoi(values["bytes"])
	created, _ := strconv.ParseInt(values["created_at"], 10, 64)
	return &app.StreamState{
		ID: id, BotID: botID, BotUserID: botUserID, Topic: values["topic"],
		ClientMsgID: values["client_msg_id"], Seq: seq, Bytes: bytes,
		State: values["state"], Content: values["content"],
		CreatedAt: time.Unix(created, 0),
	}, nil
}

var _ app.StreamStore = (*RedisStore)(nil)

var ErrStoreUnavailable = errors.New("stream store unavailable")
