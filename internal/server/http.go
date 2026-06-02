package server

import (
	"context"

	v1 "nonoka-im/api/im/v1"
	"nonoka-im/internal/conf"
	"nonoka-im/internal/service"

	"github.com/go-kratos/kratos/v2/log"
	jwtMiddleware "github.com/go-kratos/kratos/v2/middleware/auth/jwt"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/middleware/selector"
	"github.com/go-kratos/kratos/v2/transport/http"
	jwt5 "github.com/golang-jwt/jwt/v5"
)

// NewHTTPServer new an HTTP server.
func NewHTTPServer(c *conf.Server, auth *service.AuthService, dispatch *service.DispatchService, authConf *conf.Auth, logger log.Logger) *http.Server {
	var opts = []http.ServerOption{
		http.Middleware(
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
		opts = append(opts, http.Network(c.Http.Network))
	}
	if c.Http.Addr != "" {
		opts = append(opts, http.Address(c.Http.Addr))
	}
	if c.Http.Timeout != nil {
		opts = append(opts, http.Timeout(c.Http.Timeout.AsDuration()))
	}
	srv := http.NewServer(opts...)
	v1.RegisterAuthServiceHTTPServer(srv, auth)
	v1.RegisterDispatchServiceHTTPServer(srv, dispatch)
	return srv
}