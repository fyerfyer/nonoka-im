package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type memoryIdem struct {
	mu      sync.Mutex
	records map[string]IdempotencyRecord
}

func (m *memoryIdem) Begin(_ context.Context, userID int64, key, fingerprint string, lockUntil time.Time) (IdempotencyRecord, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := key
	rec, ok := m.records[id]
	if !ok {
		m.records[id] = IdempotencyRecord{Status: "in_progress", Fingerprint: fingerprint, LockedUntil: lockUntil}
		return IdempotencyRecord{}, true, nil
	}
	return rec, false, nil
}

func (m *memoryIdem) Complete(_ context.Context, userID int64, key, fingerprint string, code int, body []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records[key] = IdempotencyRecord{Status: "completed", Fingerprint: fingerprint, ResponseCode: code, ResponseBody: body}
	return nil
}

func (m *memoryIdem) Refresh(_ context.Context, userID int64, key, fingerprint string, lockUntil time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.records[key]
	if !ok || rec.Status != "in_progress" || rec.Fingerprint != fingerprint {
		return nil
	}
	rec.LockedUntil = lockUntil
	m.records[key] = rec
	return nil
}

func TestIdempotencyReplaysCompletedResponse(t *testing.T) {
	store := &memoryIdem{records: map[string]IdempotencyRecord{}}
	idem := &Idempotency{Store: store, LockTTL: time.Minute}
	ctx := context.Background()
	calls := 0

	do := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/agents", strings.NewReader(`{"name":"a"}`))
		req.Header.Set("Idempotency-Key", "key-1")
		rec := httptest.NewRecorder()
		idem.Execute(ctx, rec, req, 7, func(w http.ResponseWriter) error {
			calls++
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":1}`))
			return nil
		})
		return rec
	}
	first := do()
	if first.Code != http.StatusCreated || calls != 1 {
		t.Fatalf("first: code=%d calls=%d", first.Code, calls)
	}
	second := do()
	if second.Code != http.StatusCreated || calls != 1 {
		t.Fatalf("replay must not execute: code=%d calls=%d", second.Code, calls)
	}
	if second.Header().Get("Idempotent-Replayed") != "true" || second.Body.String() != `{"id":1}` {
		t.Fatalf("replay body/header mismatch: %q %q", second.Body.String(), second.Header().Get("Idempotent-Replayed"))
	}
}

func TestIdempotencyRejectsDifferentFingerprint(t *testing.T) {
	store := &memoryIdem{records: map[string]IdempotencyRecord{}}
	idem := &Idempotency{Store: store, LockTTL: time.Minute}
	ctx := context.Background()
	run := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/agents", strings.NewReader(body))
		req.Header.Set("Idempotency-Key", "key-2")
		rec := httptest.NewRecorder()
		idem.Execute(ctx, rec, req, 7, func(w http.ResponseWriter) error { return nil })
		return rec
	}
	if code := run(`{"name":"a"}`).Code; code != http.StatusOK {
		t.Fatalf("first request: %d", code)
	}
	rec := run(`{"name":"b"}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "IDEMPOTENCY_MISMATCH") {
		t.Fatalf("mismatch: code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestIdempotencyFingerprintIncludesQuery(t *testing.T) {
	store := &memoryIdem{records: map[string]IdempotencyRecord{}}
	idem := &Idempotency{Store: store, LockTTL: time.Minute}
	ctx := context.Background()
	run := func(query string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/agents?"+query, strings.NewReader(`{"name":"a"}`))
		req.Header.Set("Idempotency-Key", "key-query")
		rec := httptest.NewRecorder()
		idem.Execute(ctx, rec, req, 7, func(w http.ResponseWriter) error { return nil })
		return rec
	}
	if code := run("target=one").Code; code != http.StatusOK {
		t.Fatalf("first request: %d", code)
	}
	rec := run("target=two")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "IDEMPOTENCY_MISMATCH") {
		t.Fatalf("query mismatch must not replay: code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestIdempotencyInProgressConflict(t *testing.T) {
	store := &memoryIdem{records: map[string]IdempotencyRecord{
		"key-3": {Status: "in_progress", Fingerprint: "", LockedUntil: time.Now().Add(time.Minute)},
	}}
	idem := &Idempotency{Store: store, LockTTL: time.Minute}
	req := httptest.NewRequest(http.MethodPost, "/v1/agents", strings.NewReader(`{}`))
	req.Header.Set("Idempotency-Key", "key-3")
	rec := httptest.NewRecorder()
	idem.Execute(context.Background(), rec, req, 7, func(w http.ResponseWriter) error { return nil })
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "IDEMPOTENCY_IN_PROGRESS") {
		t.Fatalf("in-progress key: code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCursorRoundTrip(t *testing.T) {
	offset, err := Offset(EncodeCursor(42))
	if err != nil || offset != 42 {
		t.Fatalf("cursor: offset=%d err=%v", offset, err)
	}
	if _, err := Offset("not-base64!"); err == nil {
		t.Fatal("invalid cursor must fail")
	}
	page := NewPage([]string{"a"}, 0, 1, true)
	if !page.HasMore || page.NextCursor == "" {
		t.Fatalf("unexpected page: %+v", page)
	}
}

func TestAsErrorHidesInternal(t *testing.T) {
	e := AsError(context.Background(), errors.New("secret database detail"))
	if e.Status != http.StatusInternalServerError || e.Message != "internal error" {
		t.Fatalf("unexpected error: %+v", e)
	}
}
