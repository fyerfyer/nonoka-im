package botsdk

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	EventMessageMention = "message.mention"
	EventMessageDirect  = "message.direct"
	EventWebhookTest    = "webhook.test"
)

// Event is the webhook envelope. All 64-bit identifiers are strings.
type Event struct {
	SpecVersion string    `json:"spec_version"`
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	CreatedAt   string    `json:"created_at"`
	Bot         EventBot  `json:"bot"`
	Data        EventData `json:"data"`
}

type EventBot struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	Name   string `json:"name"`
}

type EventData struct {
	Message      *EventMessage `json:"message,omitempty"`
	Conversation *Conversation `json:"conversation,omitempty"`
}

type Conversation struct {
	Topic string `json:"topic"`
	Type  string `json:"type"`
}

type EventMessage struct {
	ID        string    `json:"id"`
	Topic     string    `json:"topic"`
	TopicSeq  string    `json:"topic_seq"`
	Type      string    `json:"type"`
	Content   string    `json:"content"`
	Sender    Sender    `json:"sender"`
	Mentions  []Mention `json:"mentions,omitempty"`
	CreatedAt string    `json:"created_at"`
}

func (m *EventMessage) TopicSequence() uint64 {
	if m == nil {
		return 0
	}
	value, _ := strconv.ParseUint(m.TopicSeq, 10, 64)
	return value
}

type Sender struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	IsBot    bool   `json:"is_bot"`
}

type Mention struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

// HandlerFunc handles one webhook event. Returning an error produces a 500 so
// the platform retries the delivery.
type HandlerFunc func(ctx context.Context, event *Event) error

// Deduper records processed webhook ids. Seen is checked before the handler
// runs; Mark is called only after it succeeds.
type Deduper interface {
	Seen(ctx context.Context, id string) (bool, error)
	Mark(ctx context.Context, id string) error
}

type WebhookConfig struct {
	Secret    string
	Deduper   Deduper
	Tolerance time.Duration
	MaxBody   int64
}

type Webhook struct {
	secret    []byte
	deduper   Deduper
	tolerance time.Duration
	maxBody   int64

	mu       sync.Mutex
	inFlight map[string]struct{}

	onMention HandlerFunc
	onDirect  HandlerFunc
	onTest    HandlerFunc
	onAny     HandlerFunc
}

func NewWebhook(cfg WebhookConfig) (*Webhook, error) {
	raw := strings.TrimPrefix(strings.TrimSpace(cfg.Secret), "whsec_")
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(key) == 0 {
		return nil, errors.New("botsdk: invalid webhook secret")
	}
	tolerance := cfg.Tolerance
	if tolerance <= 0 {
		tolerance = 5 * time.Minute
	}
	maxBody := cfg.MaxBody
	if maxBody <= 0 {
		maxBody = 1 << 20
	}
	return &Webhook{
		secret: key, deduper: cfg.Deduper, tolerance: tolerance, maxBody: maxBody,
		inFlight: make(map[string]struct{}),
	}, nil
}

func (w *Webhook) OnMention(fn HandlerFunc) *Webhook { w.onMention = fn; return w }
func (w *Webhook) OnDirect(fn HandlerFunc) *Webhook  { w.onDirect = fn; return w }
func (w *Webhook) OnTest(fn HandlerFunc) *Webhook    { w.onTest = fn; return w }
func (w *Webhook) OnAny(fn HandlerFunc) *Webhook     { w.onAny = fn; return w }

func (w *Webhook) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	eventID := strings.TrimSpace(r.Header.Get("webhook-id"))
	timestamp := strings.TrimSpace(r.Header.Get("webhook-timestamp"))
	signature := strings.TrimSpace(r.Header.Get("webhook-signature"))
	if eventID == "" || timestamp == "" || signature == "" {
		http.Error(rw, "missing webhook headers", http.StatusBadRequest)
		return
	}
	unix, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || absDuration(time.Since(time.Unix(unix, 0))) > w.tolerance {
		http.Error(rw, "stale webhook timestamp", http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, w.maxBody+1))
	if err != nil {
		http.Error(rw, "read body", http.StatusBadRequest)
		return
	}
	if int64(len(body)) > w.maxBody {
		http.Error(rw, "payload too large", http.StatusRequestEntityTooLarge)
		return
	}
	if !verifySignature(w.secret, eventID, timestamp, body, signature) {
		http.Error(rw, "invalid signature", http.StatusUnauthorized)
		return
	}
	// Claim the event locally before touching the deduper. A concurrent retry
	// for the same event must not run the handler twice while the first call
	// is still in progress.
	w.mu.Lock()
	if _, running := w.inFlight[eventID]; running {
		w.mu.Unlock()
		// Acknowledging a duplicate before the first handler finishes would
		// lose the event if that handler fails. Ask the sender to retry.
		http.Error(rw, "event is already being processed", http.StatusServiceUnavailable)
		return
	}
	w.inFlight[eventID] = struct{}{}
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		delete(w.inFlight, eventID)
		w.mu.Unlock()
	}()
	if w.deduper != nil {
		seen, err := w.deduper.Seen(r.Context(), eventID)
		if err != nil {
			http.Error(rw, "dedupe check failed", http.StatusInternalServerError)
			return
		}
		if seen {
			rw.WriteHeader(http.StatusOK)
			return
		}
	}
	var event Event
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(rw, "invalid payload", http.StatusBadRequest)
		return
	}
	if event.ID == "" {
		event.ID = eventID
	}
	handler := w.handlerFor(event.Type)
	if handler != nil {
		if err := handler(r.Context(), &event); err != nil {
			http.Error(rw, "handler failed", http.StatusInternalServerError)
			return
		}
	}
	if w.deduper != nil {
		if err := w.deduper.Mark(r.Context(), eventID); err != nil {
			http.Error(rw, "dedupe write failed", http.StatusInternalServerError)
			return
		}
	}
	rw.WriteHeader(http.StatusOK)
}

func (w *Webhook) handlerFor(eventType string) HandlerFunc {
	switch eventType {
	case EventMessageMention:
		return firstNonNil(w.onMention, w.onAny)
	case EventMessageDirect:
		return firstNonNil(w.onDirect, w.onAny)
	case EventWebhookTest:
		return firstNonNil(w.onTest, w.onAny)
	default:
		return w.onAny
	}
}

func firstNonNil(handlers ...HandlerFunc) HandlerFunc {
	for _, handler := range handlers {
		if handler != nil {
			return handler
		}
	}
	return nil
}

func verifySignature(secret []byte, eventID, timestamp string, body []byte, header string) bool {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(eventID + "." + timestamp + "."))
	_, _ = mac.Write(body)
	expected := mac.Sum(nil)
	for _, candidate := range strings.Fields(header) {
		version, raw, ok := strings.Cut(candidate, ",")
		if !ok || version != "v1" {
			continue
		}
		got, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			continue
		}
		if subtle.ConstantTimeCompare(expected, got) == 1 {
			return true
		}
	}
	return false
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

// MemoryDeduper is an in-process, TTL-bounded Deduper. Production bots should
// use a shared store if they run more than one instance.
type MemoryDeduper struct {
	ttl  time.Duration
	max  int
	mu   sync.Mutex
	seen map[string]time.Time
}

func NewMemoryDeduper(ttl time.Duration, max int) *MemoryDeduper {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	if max <= 0 {
		max = 10000
	}
	return &MemoryDeduper{ttl: ttl, max: max, seen: make(map[string]time.Time)}
}

func (d *MemoryDeduper) Seen(_ context.Context, id string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.prune()
	expires, ok := d.seen[id]
	return ok && time.Now().Before(expires), nil
}

func (d *MemoryDeduper) Mark(_ context.Context, id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.seen) >= d.max {
		d.prune()
		if len(d.seen) >= d.max {
			for key := range d.seen {
				delete(d.seen, key)
				break
			}
		}
	}
	d.seen[id] = time.Now().Add(d.ttl)
	return nil
}

func (d *MemoryDeduper) prune() {
	now := time.Now()
	for id, expires := range d.seen {
		if now.After(expires) {
			delete(d.seen, id)
		}
	}
}
