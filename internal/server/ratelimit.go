package server

import (
	"context"
	"net"
	"time"

	pb "nonoka-im/api/im/v1"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/redis/go-redis/v9"
)

// Fixed-window rate limits for the public auth endpoints. Keys live for
// one window; both limits of a login request must pass.
const (
	rateLimitWindow = time.Minute

	loginIPLimit   = 10 // im:rl:login:ip:{ip}
	loginUserLimit = 5  // im:rl:login:user:{username}

	registerIPLimit = 5 // im:rl:register:ip:{ip}
)

// AuthRateLimiter brute-force protection for /v1/auth/login and
// /v1/auth/register. It uses Redis fixed windows (INCR + EXPIRE) so limits
// are shared across all server instances. Redis failures fail open: an
// unavailable limiter must not take down login for everyone.
type AuthRateLimiter struct {
	redis redis.UniversalClient
	log   *log.Helper
}

// NewAuthRateLimiter creates the limiter. A nil redis client disables
// limiting (every request is allowed).
func NewAuthRateLimiter(r redis.UniversalClient, logger log.Logger) *AuthRateLimiter {
	return &AuthRateLimiter{redis: r, log: log.NewHelper(logger)}
}

// Middleware returns a kratos middleware that applies the auth endpoint
// limits. It only inspects the operation: mounting it via selector to the
// login/register routes keeps it off everything else.
func (l *AuthRateLimiter) Middleware() middleware.Middleware {
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			operation := ""
			if tr, ok := transport.FromServerContext(ctx); ok {
				operation = tr.Operation()
			}
			allowed, err := l.Allow(ctx, operation, clientIP(ctx), requestUsername(req))
			if err != nil {
				l.log.Warnf("auth rate limiter error (allowing request): %v", err)
			}
			if !allowed {
				return nil, errors.New(429, "RATE_LIMITED", "too many requests, please try again later")
			}
			return next(ctx, req)
		}
	}
}

// Allow reports whether a request may proceed. username is only used for
// the login-per-account window. Unknown/unmatched operations are always
// allowed.
func (l *AuthRateLimiter) Allow(ctx context.Context, operation, ip, username string) (bool, error) {
	if l == nil || l.redis == nil {
		return true, nil
	}
	if ip == "" {
		ip = "unknown"
	}

	switch operation {
	case "/api.im.v1.AuthService/Login":
		ok, err := l.allowN(ctx, "im:rl:login:ip:"+ip, loginIPLimit)
		if err != nil || !ok {
			return ok, err
		}
		if username != "" {
			return l.allowN(ctx, "im:rl:login:user:"+username, loginUserLimit)
		}
		return true, nil
	case "/api.im.v1.AuthService/Register":
		return l.allowN(ctx, "im:rl:register:ip:"+ip, registerIPLimit)
	default:
		return true, nil
	}
}

// allowN implements one fixed window: INCR the key, set the expiry on the
// first hit, deny once the count exceeds limit.
func (l *AuthRateLimiter) allowN(ctx context.Context, key string, limit int64) (bool, error) {
	n, err := l.redis.Incr(ctx, key).Result()
	if err != nil {
		// Fail open on Redis errors.
		return true, err
	}
	if n == 1 {
		if err := l.redis.Expire(ctx, key, rateLimitWindow).Err(); err != nil {
			return true, err
		}
	}
	return n <= limit, nil
}

// clientIP extracts the client IP from the HTTP request, stripping the port.
func clientIP(ctx context.Context) string {
	req, ok := khttp.RequestFromServerContext(ctx)
	if !ok || req == nil {
		return ""
	}
	return parseClientIP(req.RemoteAddr)
}

func parseClientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return host
	}
	return remoteAddr
}

// requestUsername pulls the login account from the decoded request so the
// per-account window can be applied.
func requestUsername(req interface{}) string {
	switch r := req.(type) {
	case *pb.LoginRequest:
		return r.GetUsername()
	default:
		return ""
	}
}
