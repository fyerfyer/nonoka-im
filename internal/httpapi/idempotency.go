package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"time"
)

// IdempotencyRecord is the persisted state of one key.
type IdempotencyRecord struct {
	Status       string // in_progress | completed
	Fingerprint  string
	ResponseCode int
	ResponseBody []byte
	LockedUntil  time.Time
}

// IdempotencyStore persists keys scoped to (userID, key).
type IdempotencyStore interface {
	// Begin inserts or takes over a key. acquired=false means another request
	// owns the key or a completed response is available in the record.
	Begin(ctx context.Context, userID int64, key, fingerprint string, lockUntil time.Time) (record IdempotencyRecord, acquired bool, err error)
	// Complete stores the response of the winning request.
	Complete(ctx context.Context, userID int64, key, fingerprint string, code int, body []byte) error
}

var ErrIdempotencyMismatch = errors.New("idempotency key reused with a different request")

// Idempotency executes an HTTP handler at most once per key.
type Idempotency struct {
	Store   IdempotencyStore
	LockTTL time.Duration
}

// Execute runs fn under the key (when present) and flushes exactly one
// response. A replayed response is marked with Idempotent-Replayed: true.
func (m *Idempotency) Execute(ctx context.Context, w http.ResponseWriter, r *http.Request, userID int64, fn func(http.ResponseWriter) error) {
	key := r.Header.Get("Idempotency-Key")
	mutating := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch
	if key != "" && len(key) > 255 {
		Write(w, ctx, BadRequest("INVALID_ARGUMENT", "Idempotency-Key must be at most 255 characters"))
		return
	}
	if key == "" || m == nil || m.Store == nil || !mutating {
		if err := fn(w); err != nil {
			Write(w, ctx, err)
		}
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		Write(w, ctx, BadRequest("INVALID_ARGUMENT", "invalid request body"))
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	fingerprint := requestFingerprint(r, body)

	lock := m.LockTTL
	if lock <= 0 {
		lock = 30 * time.Second
	}
	record, acquired, err := m.Store.Begin(ctx, userID, key, fingerprint, time.Now().Add(lock))
	if err != nil {
		Write(w, ctx, Internal(err))
		return
	}
	if !acquired {
		if record.Fingerprint != "" && record.Fingerprint != fingerprint {
			Write(w, ctx, Conflict("IDEMPOTENCY_MISMATCH", ErrIdempotencyMismatch.Error()))
			return
		}
		if record.Status == "completed" {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Idempotent-Replayed", "true")
			w.WriteHeader(record.ResponseCode)
			_, _ = w.Write(record.ResponseBody)
			return
		}
		Write(w, ctx, Conflict("IDEMPOTENCY_IN_PROGRESS", "a request with this idempotency key is still running"))
		return
	}

	rec := newRecorder(w)
	handlerErr := fn(rec)
	if handlerErr != nil {
		Write(rec, ctx, handlerErr)
	}
	code, body := rec.statusCode(), rec.body.Bytes()
	if len(body) > maxIdempotentResponse {
		// Persisting a multi-megabyte response is a retention and memory risk.
		// The original response is served normally; a replay gets an explicit
		// 413 instead of a silently truncated body.
		code = http.StatusRequestEntityTooLarge
		body = []byte(`{"code":"IDEMPOTENCY_RESPONSE_TOO_LARGE","message":"the original response is too large to replay"}`)
	}
	if completeErr := m.Store.Complete(ctx, userID, key, fingerprint, code, body); completeErr != nil {
		// The response is already computed; a failed completion only means a
		// retry may execute again. Keep serving the result.
		_ = completeErr
	}
	rec.flushTo(w)
}

// maxIdempotentResponse bounds the response body retained for replay.
const maxIdempotentResponse = 4 << 20

func requestFingerprint(r *http.Request, body []byte) string {
	h := sha256.New()
	_, _ = h.Write([]byte(r.Method))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(r.URL.Path))
	_, _ = h.Write([]byte{0})
	// The query string is part of the request identity: two writes with the
	// same body but different filters/targets must not replay each other.
	_, _ = h.Write([]byte(r.URL.RawQuery))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

// recorder captures a handler response so it can be persisted and replayed.
type recorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newRecorder(w http.ResponseWriter) *recorder {
	header := make(http.Header, len(w.Header()))
	for key, values := range w.Header() {
		header[key] = append([]string(nil), values...)
	}
	return &recorder{header: header}
}

func (r *recorder) Header() http.Header { return r.header }
func (r *recorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
}
func (r *recorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.body.Write(p)
}
func (r *recorder) statusCode() int {
	if r.status == 0 {
		return http.StatusOK
	}
	return r.status
}
func (r *recorder) flushTo(w http.ResponseWriter) {
	for key, values := range r.header {
		w.Header()[key] = append([]string(nil), values...)
	}
	w.WriteHeader(r.statusCode())
	_, _ = w.Write(r.body.Bytes())
}
