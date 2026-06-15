package server

import (
	"context"
	"net/http/pprof"

	v1 "nonoka-im/api/im/v1"
	"nonoka-im/internal/conf"
	"nonoka-im/internal/gateway"
	"nonoka-im/internal/metrics"
	"nonoka-im/internal/service"

	"github.com/go-kratos/kratos/v2/log"
	jwtMiddleware "github.com/go-kratos/kratos/v2/middleware/auth/jwt"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/middleware/selector"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	jwt5 "github.com/golang-jwt/jwt/v5"
)

// NewHTTPServer new an HTTP server.
func NewHTTPServer(c *conf.Server, auth *service.AuthService, dispatch *service.DispatchService, message *service.MessageService, ws *gateway.WebSocketServer, authConf *conf.Auth, logger log.Logger, m *metrics.Metrics) *khttp.Server {
	var opts = []khttp.ServerOption{
		khttp.Middleware(
			recovery.Recovery(),
			selector.Server(
				jwtMiddleware.Server(func(token *jwt5.Token) (interface{}, error) {
					return []byte(authConf.JwtSecret), nil
				}),
			).Match(func(ctx context.Context, operation string) bool {
				// Skip JWT validation for public endpoints
				switch operation {
				case "/api.im.v1.AuthService/Register",
					"/api.im.v1.AuthService/Login",
					"/api.im.v1.DispatchService/Gateway":
					return false
				}
				return true
			}).Build(),
		),
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
	srv := khttp.NewServer(opts...)
	v1.RegisterAuthServiceHTTPServer(srv, auth)
	v1.RegisterDispatchServiceHTTPServer(srv, dispatch)
	if message != nil {
		v1.RegisterMessageServiceHTTPServer(srv, message)
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
