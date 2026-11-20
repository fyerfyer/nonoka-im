package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-kratos/kratos/v2/transport"
	"github.com/google/uuid"
)

func newRequestID() string { return uuid.NewString() }

// RequestIDFromContext reads an incoming X-Request-Id from the Kratos server
// transport, if present.
func RequestIDFromContext(ctx context.Context) string {
	if tr, ok := transport.FromServerContext(ctx); ok {
		return tr.RequestHeader().Get("X-Request-Id")
	}
	return ""
}

func writeJSON(w http.ResponseWriter, value any) {
	_ = json.NewEncoder(w).Encode(value)
}
