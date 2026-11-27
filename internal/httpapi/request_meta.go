package httpapi

import (
	"context"
	"net"
	"net/http"
	"os"
	"strings"

	"nonoka-im/internal/audit"

	"github.com/go-kratos/kratos/v2/middleware"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"go.opentelemetry.io/otel/trace"
)

// AuditRequestMetaMiddleware attaches request correlation metadata to every
// HTTP request context. Raw and generated handlers can then record audit
// events without duplicating IP/request/trace extraction.
func AuditRequestMetaMiddleware() middleware.Middleware {
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			if r, ok := khttp.RequestFromServerContext(ctx); ok && r != nil {
				ctx = audit.WithRequestMeta(ctx, requestMeta(ctx, r))
			}
			return next(ctx, req)
		}
	}
}

func requestMeta(ctx context.Context, r *http.Request) audit.RequestMeta {
	ip := r.RemoteAddr
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("TRUST_PROXY_HEADERS")), "true") {
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			ip = strings.TrimSpace(strings.Split(forwarded, ",")[0])
		}
	}
	traceID := ""
	if sc := trace.SpanContextFromContext(ctx); sc.HasTraceID() {
		traceID = sc.TraceID().String()
	}
	return audit.RequestMeta{
		RequestID: RequestID(ctx), TraceID: traceID, IP: ip,
		UserAgent: r.UserAgent(),
	}
}
