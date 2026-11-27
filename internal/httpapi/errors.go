// Package httpapi contains the HTTP contract shared by the gateway REST
// handlers: the error envelope, request IDs, cursor pagination and
// Idempotency-Key handling.
package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport"
)

type requestIDKey struct{}

// ResponseCommittedError is implemented by handlers that already wrote the
// HTTP response. Middleware must pass it through without re-encoding it.
type ResponseCommittedError interface {
	error
	ResponseCommitted() bool
}

// RequestID returns the request-scoped correlation ID.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// WithRequestID stores a request ID in the context (used by middleware and
// tests).
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestIDMiddleware accepts an incoming X-Request-Id or creates one, echoes
// it on the response and makes it available to error responses.
func RequestIDMiddleware() middleware.Middleware {
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			id := RequestIDFromContext(ctx)
			if id == "" {
				id = newRequestID()
			}
			ctx = WithRequestID(ctx, id)
			if tr, ok := transport.FromServerContext(ctx); ok {
				tr.ReplyHeader().Set("X-Request-Id", id)
			}
			resp, err := next(ctx, req)
			if err != nil {
				var committed ResponseCommittedError
				if errors.As(err, &committed) && committed.ResponseCommitted() {
					return nil, err
				}
				// Kratos error responses cannot carry our custom body here, but
				// attaching the request ID to the error metadata keeps traces
				// correlated for generated proto handlers too.
				if se := kerrors.FromError(err); se != nil {
					return nil, withRequestID(se, id)
				}
			}
			return resp, err
		}
	}
}

// withRequestID returns a copy of a Kratos error with the request ID attached.
// Kratos errors returned by package-level helpers can be shared across
// requests; mutating their Metadata map in place races under concurrent load.
func withRequestID(se *kerrors.Error, id string) *kerrors.Error {
	if se == nil {
		return nil
	}
	clone := kerrors.New(int(se.Code), se.Reason, se.Message)
	clone.Metadata = make(map[string]string, len(se.Metadata)+1)
	for key, value := range se.Metadata {
		clone.Metadata[key] = value
	}
	clone.Metadata["requestId"] = id
	return clone
}

// Error is the normalized error body:
//
//	{code, message, retryable, retryAfterMs, requestId}
type Error struct {
	Status     int           `json:"-"`
	Code       string        `json:"code"`
	Message    string        `json:"message"`
	Retryable  bool          `json:"retryable,omitempty"`
	RetryAfter time.Duration `json:"-"`
	RequestID  string        `json:"requestId,omitempty"`
	Internal   error         `json:"-"`
}

func (e *Error) Error() string {
	if e.Internal != nil {
		return e.Internal.Error()
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Internal }

// BadRequest / Forbidden / NotFound / Conflict / TooMany / Internal build the
// common envelope values.
func BadRequest(code, message string) *Error {
	return &Error{Status: http.StatusBadRequest, Code: code, Message: message}
}
func Forbidden(message string) *Error {
	return &Error{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: message}
}
func NotFound(message string) *Error {
	return &Error{Status: http.StatusNotFound, Code: "NOT_FOUND", Message: message}
}
func Conflict(code, message string) *Error {
	return &Error{Status: http.StatusConflict, Code: code, Message: message}
}
func RequestEntityTooLarge(code, message string) *Error {
	return &Error{Status: http.StatusRequestEntityTooLarge, Code: code, Message: message}
}
func Internal(err error) *Error {
	return &Error{Status: http.StatusInternalServerError, Code: "INTERNAL", Message: "internal error", Internal: err}
}

type errorBody struct {
	Code         string `json:"code"`
	Message      string `json:"message"`
	Retryable    bool   `json:"retryable,omitempty"`
	RetryAfterMs int64  `json:"retryAfterMs,omitempty"`
	RequestID    string `json:"requestId,omitempty"`
}

// AsError normalizes any error into the envelope, defaulting to a 500 whose
// message does not leak internals.
func AsError(ctx context.Context, err error) *Error {
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		if e.RequestID == "" {
			clone := *e
			clone.RequestID = RequestID(ctx)
			return &clone
		}
		return e
	}
	se := kerrors.FromError(err)
	if se != nil && se.Code >= 400 && se.Code < 500 {
		return &Error{Status: int(se.Code), Code: se.Reason, Message: se.Message, RequestID: RequestID(ctx), Internal: err}
	}
	return &Error{Status: http.StatusInternalServerError, Code: "INTERNAL", Message: "internal error", RequestID: RequestID(ctx), Internal: err}
}

// Write writes the error envelope. Server-side 5xx errors are also logged with
// the request ID so an incident can be correlated with the client's report
// without leaking internals into the response.
func Write(w http.ResponseWriter, ctx context.Context, err error) {
	e := AsError(ctx, err)
	if e.Status >= 500 && e.Internal != nil {
		msg := e.Internal.Error()
		if len(msg) > 500 {
			msg = msg[:500] + "…"
		}
		log.NewHelper(log.DefaultLogger).Errorf("http %d request_id=%s code=%s err=%s", e.Status, e.RequestID, e.Code, msg)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.Status)
	writeJSON(w, errorBody{
		Code: e.Code, Message: e.Message, Retryable: e.Retryable,
		RetryAfterMs: e.RetryAfter.Milliseconds(), RequestID: e.RequestID,
	})
}
