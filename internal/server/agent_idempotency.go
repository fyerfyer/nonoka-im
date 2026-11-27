package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"nonoka-im/internal/httpapi"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/middleware"
	jwtMiddleware "github.com/go-kratos/kratos/v2/middleware/auth/jwt"
	"github.com/go-kratos/kratos/v2/transport"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	jwt5 "github.com/golang-jwt/jwt/v5"
	"google.golang.org/protobuf/proto"
)

// errAgentResponseCommitted tells the Kratos error encoder that the
// idempotency middleware already flushed the response.
var errAgentResponseCommitted = &agentResponseCommittedError{}

type agentResponseCommittedError struct{}

func (*agentResponseCommittedError) Error() string { return "agent response already committed" }

func (*agentResponseCommittedError) ResponseCommitted() bool { return true }

const maxIdempotentResponse = 4 << 20

// agentIdempotencyMiddleware preserves the Idempotency-Key behavior that the
// former hand-written Agent handlers implemented before the Proto migration.
// It runs after JWT auth, fingerprints the bound proto request (the generated
// handler consumed the body before middleware runs), and replays the stored
// response without executing the usecase again.
func agentIdempotencyMiddleware(idem *httpapi.Idempotency) middleware.Middleware {
	if idem == nil || idem.Store == nil {
		return func(next middleware.Handler) middleware.Handler { return next }
	}
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			r, ok := khttp.RequestFromServerContext(ctx)
			if !ok || r == nil || !agentIdempotentRequest(r) {
				return next(ctx, req)
			}
			key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
			if key == "" {
				return next(ctx, req)
			}
			w := idempotencyResponseWriter(ctx)
			if w == nil {
				return next(ctx, req)
			}
			if len(key) > 255 {
				agentErrorEncoder(w, r, kerrors.BadRequest("INVALID_ARGUMENT", "Idempotency-Key must be at most 255 characters"))
				return nil, errAgentResponseCommitted
			}
			userID := httpUserID(ctx)
			if userID <= 0 {
				// Authentication middleware owns the 401 response.
				return next(ctx, req)
			}

			fingerprint := agentRequestFingerprint(r, req)
			if requestID := httpapi.RequestID(ctx); requestID != "" {
				// The generated request context does not carry the middleware
				// context; copy the ID so cached error envelopes keep it.
				r.Header.Set("X-Request-Id", requestID)
			}
			lockTTL := idem.LockTTL
			if lockTTL <= 0 {
				lockTTL = 30 * time.Second
			}
			record, acquired, err := idem.Store.Begin(ctx, userID, key, fingerprint, time.Now().Add(lockTTL))
			if err != nil {
				agentErrorEncoder(w, r, kerrors.InternalServer("INTERNAL", "internal error"))
				return nil, errAgentResponseCommitted
			}
			if !acquired {
				switch {
				case record.Fingerprint != "" && record.Fingerprint != fingerprint:
					agentErrorEncoder(w, r, kerrors.Conflict("IDEMPOTENCY_MISMATCH", httpapi.ErrIdempotencyMismatch.Error()))
				case record.Status == "completed":
					writeIdempotentReplay(w, record)
				default:
					agentErrorEncoder(w, r, kerrors.Conflict("IDEMPOTENCY_IN_PROGRESS", "a request with this idempotency key is still running"))
				}
				return nil, errAgentResponseCommitted
			}

			rec := newIdempotencyRecorder()
			reply, handlerErr := next(ctx, req)
			if handlerErr != nil {
				agentErrorEncoder(rec, r, handlerErr)
			} else if err := agentNumericResponseEncoder(rec, r, reply); err != nil {
				agentErrorEncoder(rec, r, kerrors.InternalServer("INTERNAL", "response encoding failed"))
			}

			code := rec.statusCode()
			body := rec.body.Bytes()
			if len(body) > maxIdempotentResponse {
				// Keep serving the original response, but do not persist a
				// multi-megabyte replay body.
				code = http.StatusRequestEntityTooLarge
				body = []byte(`{"code":"IDEMPOTENCY_RESPONSE_TOO_LARGE","message":"the original response is too large to replay"}`)
			}
			_ = idem.Store.Complete(ctx, userID, key, fingerprint, code, body)
			rec.flushTo(w)
			return nil, errAgentResponseCommitted
		}
	}
}

func agentIdempotentRequest(r *http.Request) bool {
	if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch {
		return false
	}
	return agentNumericAPIPath(r.URL.Path)
}

func idempotencyResponseWriter(ctx context.Context) http.ResponseWriter {
	if tr, ok := transport.FromServerContext(ctx); ok {
		if response, ok := tr.(khttp.ResponseTransporter); ok {
			return response.Response()
		}
	}
	return nil
}

func httpUserID(ctx context.Context) int64 {
	claims, ok := jwtMiddleware.FromContext(ctx)
	if !ok {
		return 0
	}
	mapped, ok := claims.(jwt5.MapClaims)
	if !ok {
		return 0
	}
	switch value := mapped["user_id"].(type) {
	case float64:
		return int64(value)
	case int64:
		return value
	case int:
		return int64(value)
	default:
		return 0
	}
}

// agentRequestFingerprint uses the canonical proto request because the
// generated JSON handler has already consumed the HTTP body by the time
// middleware runs. This is stricter and more stable than hashing raw JSON.
func agentRequestFingerprint(r *http.Request, req any) string {
	h := sha256.New()
	_, _ = h.Write([]byte(r.Method))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(r.URL.Path))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(r.URL.RawQuery))
	_, _ = h.Write([]byte{0})
	if message, ok := req.(proto.Message); ok {
		options := proto.MarshalOptions{Deterministic: true}
		if data, err := options.Marshal(message); err == nil {
			_, _ = h.Write(data)
		} else {
			_, _ = fmt.Fprintf(h, "%v", req)
		}
	} else {
		_, _ = fmt.Fprintf(h, "%v", req)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func writeIdempotentReplay(w http.ResponseWriter, record httpapi.IdempotencyRecord) {
	code := record.ResponseCode
	if code == 0 {
		code = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Idempotent-Replayed", "true")
	w.WriteHeader(code)
	_, _ = w.Write(record.ResponseBody)
}

// idempotencyRecorder captures a generated handler's encoded response so it
// can be persisted for replay.
type idempotencyRecorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newIdempotencyRecorder() *idempotencyRecorder {
	return &idempotencyRecorder{header: make(http.Header)}
}

func (r *idempotencyRecorder) Header() http.Header { return r.header }

func (r *idempotencyRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
}

func (r *idempotencyRecorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.body.Write(p)
}

func (r *idempotencyRecorder) statusCode() int {
	if r.status == 0 {
		return http.StatusOK
	}
	return r.status
}

func (r *idempotencyRecorder) flushTo(w http.ResponseWriter) {
	for key, values := range r.header {
		w.Header()[key] = append([]string(nil), values...)
	}
	w.WriteHeader(r.statusCode())
	_, _ = w.Write(r.body.Bytes())
}
