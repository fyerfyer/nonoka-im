package server

import (
	"context"
	"net/http"
	"net/http/pprof"
	"net/url"
	"os"
	"strings"

	v1 "nonoka-im/api/im/v1"
	"nonoka-im/internal/conf"
	"nonoka-im/internal/gateway"
	"nonoka-im/internal/httpapi"
	"nonoka-im/internal/metrics"
	"nonoka-im/internal/service"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware"
	jwtMiddleware "github.com/go-kratos/kratos/v2/middleware/auth/jwt"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/middleware/selector"
	"github.com/go-kratos/kratos/v2/transport"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	jwt5 "github.com/golang-jwt/jwt/v5"
	"github.com/rs/cors"
)

// NewHTTPServer new an HTTP server.
func NewHTTPServer(c *conf.Server, auth *service.AuthService, dispatch *service.DispatchService, message *service.MessageService, user *service.UserService, conversation *service.ConversationService, group *service.GroupService, file *service.FileService, agents *service.AgentHTTP, outbox *service.OutboxAdminHTTP, ws *gateway.WebSocketServer, authConf *conf.Auth, gatewayConf *conf.GatewayConfig, rateLimiter *AuthRateLimiter, logger log.Logger, m *metrics.Metrics) *khttp.Server {
	middlewares := []middleware.Middleware{
		recovery.Recovery(),
		httpapi.RequestIDMiddleware(),
	}
	if rateLimiter != nil {
		middlewares = append(middlewares, selector.Server(
			rateLimiter.Middleware(),
		).Match(func(ctx context.Context, operation string) bool {
			switch operation {
			case "/api.im.v1.AuthService/Login",
				"/api.im.v1.AuthService/Register":
				return true
			}
			return false
		}).Build())
	}
	middlewares = append(middlewares, selector.Server(
		jwtMiddleware.Server(func(token *jwt5.Token) (interface{}, error) {
			if token.Method.Alg() != jwt5.SigningMethodHS256.Alg() {
				return nil, kerrors.Unauthorized("INVALID_TOKEN", "invalid token algorithm")
			}
			if token.Header["typ"] == "agent-bot" {
				secret := os.Getenv("AGENT_WORKER_JWT_SECRET")
				if len(secret) < 32 {
					return nil, kerrors.Unauthorized("BOT_TOKEN_DISABLED", "bot credentials unavailable")
				}
				return []byte(secret), nil
			}
			return []byte(authConf.JwtSecret), nil
		}),
	).Match(func(ctx context.Context, operation string) bool {
		// Skip JWT validation for public endpoints
		switch operation {
		case "/api.im.v1.AuthService/Register",
			"/api.im.v1.AuthService/Login",
			"/api.im.v1.AuthService/RefreshToken",
			"/api.im.v1.DispatchService/Gateway":
			return false
		}
		return true
	}).Build())
	middlewares = append(middlewares, func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			claims, ok := jwtMiddleware.FromContext(ctx)
			if !ok {
				return next(ctx, req)
			}
			mapped, ok := claims.(jwt5.MapClaims)
			if !ok || mapped["scope"] == nil {
				return next(ctx, req)
			}
			tr, ok := transport.FromServerContext(ctx)
			if !ok || tr.Operation() != "/api.im.v1.MessageService/SendMessage" || agents == nil {
				return nil, kerrors.Forbidden("BOT_SCOPE", "bot credential can only send messages")
			}
			messageReq, ok := req.(*v1.SendMessageRequest)
			if !ok || messageReq.Topic != mapped["topic"] || !(strings.HasPrefix(messageReq.ClientMsgId, "agent-") || strings.HasPrefix(messageReq.ClientMsgId, "agent-tool-")) {
				return nil, kerrors.Forbidden("BOT_SCOPE", "bot credential scope mismatch")
			}
			if err := agents.VerifyBotSend(ctx, mapped); err != nil {
				return nil, kerrors.Forbidden("BOT_SCOPE", "bot credential not authorized")
			}
			return next(ctx, req)
		}
	})

	var opts = []khttp.ServerOption{
		khttp.Middleware(middlewares...),
	}
	if c.Http.Network != "" {
		opts = append(opts, khttp.Network(c.Http.Network))
	}
	if c.Http.Addr != "" {
		opts = append(opts, khttp.Address(c.Http.Addr))
	}
	if c.Http.Timeout != nil {
		opts = append(opts, khttp.Timeout(c.Http.Timeout.AsDuration()))
	}

	// Configure CORS for local frontend development if origins are provided.
	if gatewayConf != nil && len(gatewayConf.CorsOrigins) > 0 {
		corsOpts := cors.Options{
			AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Authorization", "Content-Type", "Accept", "Idempotency-Key", "X-Request-Id"},
			AllowCredentials: true,
			MaxAge:           86400,
		}
		// If "*" is present, allow any localhost origin and configured origins.
		if hasWildcardOrigin(gatewayConf.CorsOrigins) {
			corsOpts.AllowOriginFunc = func(origin string) bool {
				return isLoopbackOrigin(origin) ||
					isConfiguredOrigin(origin, gatewayConf.CorsOrigins)
			}
		} else {
			corsOpts.AllowedOrigins = gatewayConf.CorsOrigins
		}
		corsHandler := cors.New(corsOpts)
		opts = append(opts, khttp.Filter(func(h http.Handler) http.Handler {
			return corsHandler.Handler(h)
		}))
	}

	srv := khttp.NewServer(opts...)
	v1.RegisterAuthServiceHTTPServer(srv, auth)
	v1.RegisterDispatchServiceHTTPServer(srv, dispatch)
	if message != nil {
		v1.RegisterMessageServiceHTTPServer(srv, message)
	}
	if user != nil {
		v1.RegisterUserServiceHTTPServer(srv, user)
	}
	if conversation != nil {
		v1.RegisterConversationServiceHTTPServer(srv, conversation)
	}
	if group != nil {
		v1.RegisterGroupServiceHTTPServer(srv, group)
	}
	// File upload/download are stream-oriented, so they are registered as
	// raw routes on the kratos router instead of generated JSON handlers.
	if file != nil {
		r := srv.Route("/")
		r.POST("/v1/files", file.UploadHTTP)
		r.GET("/v1/files/{file_id}", file.DownloadHTTP)
		r.POST("/v1/files/{file_id}/share", file.ShareHTTP)
	}
	if agents != nil {
		r := srv.Route("/")
		r.GET("/v1/agents", agents.List)
		r.POST("/v1/agents", agents.Create)
		r.GET("/v1/agents/{id}", agents.Detail)
		r.DELETE("/v1/agents/{id}", agents.Delete)
		r.PUT("/v1/agents/{id}/draft", agents.Draft)
		r.POST("/v1/agents/{id}/publish", agents.Publish)
		r.POST("/v1/agents/{id}/status", agents.Status)
		r.PUT("/v1/agents/{id}/shares", agents.Share)
		r.GET("/v1/agents/{id}/shares", agents.Shares)
		r.GET("/v1/agents/{id}/runs", agents.Runs)
		r.GET("/v1/agent-runs/{id}", agents.RunDetail)
		r.POST("/v1/agent-runs/{id}/retry", func(ctx khttp.Context) error { return agents.RunAction(ctx, "retry") })
		r.POST("/v1/agent-runs/{id}/skip", func(ctx khttp.Context) error { return agents.RunAction(ctx, "skip") })
		r.POST("/v1/agent-approvals/{id}/approve", func(ctx khttp.Context) error { return agents.ApprovalAction(ctx, "approve") })
		r.POST("/v1/agent-approvals/{id}/reject", func(ctx khttp.Context) error { return agents.ApprovalAction(ctx, "reject") })
		r.GET("/v1/agent-approvals", agents.ListApprovals)
		r.GET("/v1/agent-approvals/{id}", agents.ApprovalDetail)
		r.POST("/v1/mcp/servers", agents.RegisterMCPServer)
		r.GET("/v1/mcp/servers", agents.ListMCPServers)
		r.POST("/v1/mcp/servers/{id}/status", agents.SetMCPServerStatus)
		r.POST("/v1/skills", agents.PublishSkill)
		r.GET("/v1/skills", agents.ListSkills)
	}
	if outbox != nil {
		r := srv.Route("/")
		r.GET("/v1/admin/agent-outbox/failed", outbox.ListFailed)
		r.POST("/v1/admin/agent-outbox/{id}/replay", outbox.Replay)
	}

	// Register WebSocket handler
	if ws != nil {
		srv.Handle("/ws", ws)
	}

	// Register Prometheus metrics and pprof endpoints for observability.
	if m != nil {
		srv.Handle("/metrics", m.Handler())
	}
	srv.HandleFunc("/debug/pprof/", pprof.Index)
	srv.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	srv.HandleFunc("/debug/pprof/profile", pprof.Profile)
	srv.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	srv.HandleFunc("/debug/pprof/trace", pprof.Trace)

	return srv
}

func isLoopbackOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func hasWildcardOrigin(origins []string) bool {
	for _, o := range origins {
		if o == "*" {
			return true
		}
	}
	return false
}

func isConfiguredOrigin(origin string, origins []string) bool {
	for _, o := range origins {
		if o == origin {
			return true
		}
	}
	return false
}
