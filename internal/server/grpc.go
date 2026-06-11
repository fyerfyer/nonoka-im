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
	"github.com/go-kratos/kratos/v2/transport/grpc"
	jwt5 "github.com/golang-jwt/jwt/v5"
)

// NewGRPCServer new a gRPC server.
func NewGRPCServer(c *conf.Server, auth *service.AuthService, dispatch *service.DispatchService, push *service.PushService, authConf *conf.Auth, logger log.Logger) *grpc.Server {
	var opts = []grpc.ServerOption{
		grpc.Middleware(
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
					"/api.im.v1.DispatchService/Gateway",
					"/api.im.v1.PushService/PushToUser",
					"/api.im.v1.PushService/BatchPushToUsers":
					return false
				}
				return true
			}).Build(),
		),
	}
	if c.Grpc.Network != "" {
		opts = append(opts, grpc.Network(c.Grpc.Network))
	}
	if c.Grpc.Addr != "" {
		opts = append(opts, grpc.Address(c.Grpc.Addr))
	}
	if c.Grpc.Timeout != nil {
		opts = append(opts, grpc.Timeout(c.Grpc.Timeout.AsDuration()))
	}
	srv := grpc.NewServer(opts...)
	v1.RegisterAuthServiceServer(srv, auth)
	v1.RegisterDispatchServiceServer(srv, dispatch)
	v1.RegisterPushServiceServer(srv, push)
	return srv
}