package server

import (
	"context"
	"crypto/subtle"
	"os"
	"strings"

	agentv1 "nonoka-im/api/agent/v1"
	botv1 "nonoka-im/api/bot/v1"
	v1 "nonoka-im/api/im/v1"
	"nonoka-im/internal/conf"
	"nonoka-im/internal/gateway"
	"nonoka-im/internal/realtime"
	"nonoka-im/internal/service"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware"
	jwtMiddleware "github.com/go-kratos/kratos/v2/middleware/auth/jwt"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/middleware/selector"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	jwt5 "github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc/metadata"
)

// requireInternalPushToken protects Agent Worker run-event pushes. The endpoint
// fails closed when the token is absent: an unauthenticated push RPC can inject
// arbitrary messages/events into any user's live connection.
func requireInternalPushToken(next middleware.Handler) middleware.Handler {
	return func(ctx context.Context, req interface{}) (interface{}, error) {
		expected := os.Getenv("INTERNAL_PUSH_TOKEN")
		if expected == "" {
			return nil, kerrors.InternalServer("INTERNAL_PUSH_TOKEN_MISSING", "internal push token is not configured")
		}
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, kerrors.Unauthorized("INTERNAL_PUSH_UNAUTHORIZED", "missing internal push token")
		}
		values := md.Get(realtime.InternalPushTokenHeader)
		if len(values) == 0 || subtle.ConstantTimeCompare([]byte(values[0]), []byte(expected)) != 1 {
			return nil, kerrors.Unauthorized("INTERNAL_PUSH_UNAUTHORIZED", "invalid internal push token")
		}
		return next(ctx, req)
	}
}

// userTokenScope is the only scope accepted on the gRPC API. Bot credentials
// carry scope "agent:send" and are valid only for the HTTP message-send path.
const userTokenScope = "user"

// rejectNonUserScope refuses tokens that carry an explicit non-user scope
// (e.g. agent bot credentials). User access tokens are issued without a scope
// claim (see internal/biz), so a missing or non-string scope keeps the
// previous behavior and is allowed through.
func rejectNonUserScope(next middleware.Handler) middleware.Handler {
	return func(ctx context.Context, req interface{}) (interface{}, error) {
		claims, ok := jwtMiddleware.FromContext(ctx)
		if !ok {
			return next(ctx, req)
		}
		mapped, ok := claims.(jwt5.MapClaims)
		if !ok {
			return next(ctx, req)
		}
		raw, exists := mapped["scope"]
		if !exists {
			return next(ctx, req)
		}
		scope, ok := raw.(string)
		if !ok || scope == userTokenScope {
			// Unparseable scope keeps existing behavior; "user" is accepted.
			return next(ctx, req)
		}
		return nil, kerrors.Forbidden("TOKEN_SCOPE", "token scope not permitted on gRPC")
	}
}

// NewGRPCServer new a gRPC server.
func NewGRPCServer(c *conf.Server, auth *service.AuthService, dispatch *gateway.DispatchService, message *service.MessageService, user *service.UserService, conversation *service.ConversationService, push *gateway.PushService, bots *service.BotHTTP, agents *service.AgentHTTP, outbox *service.OutboxAdminHTTP, authConf *conf.Auth, logger log.Logger) *grpc.Server {
	if strings.TrimSpace(os.Getenv("INTERNAL_PUSH_TOKEN")) == "" {
		log.NewHelper(logger).Error("INTERNAL_PUSH_TOKEN is empty; internal PushService RPCs will reject all calls until it is configured")
	}
	var opts = []grpc.ServerOption{
		grpc.Middleware(
			recovery.Recovery(),
			selector.Server(requireInternalPushToken).Match(func(ctx context.Context, operation string) bool {
				// Every PushService RPC is an internal control-plane call from
				// MsgWorker/AgentWorker/BotWorker. None of them may be exposed
				// without the shared internal token.
				return strings.HasPrefix(operation, "/api.im.v1.PushService/")
			}).Build(),
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
					"/api.im.v1.PushService/BatchPushToUsers",
					"/api.im.v1.PushService/PushReceiptToUser",
					"/api.im.v1.PushService/BatchPushReceiptToUsers",
					"/api.im.v1.PushService/BatchPushReceiptsToUsers",
					"/api.im.v1.PushService/PushRunEventToUser",
					"/api.im.v1.PushService/PushBotStreamEventToUser":
					return false
				}
				return true
			}).Build(),
			rejectNonUserScope,
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
	if message != nil {
		v1.RegisterMessageServiceServer(srv, message)
	}
	if user != nil {
		v1.RegisterUserServiceServer(srv, user)
	}
	if conversation != nil {
		v1.RegisterConversationServiceServer(srv, conversation)
	}
	v1.RegisterPushServiceServer(srv, push)
	if bots != nil {
		botv1.RegisterBotServiceServer(srv, bots)
	}
	if agents != nil {
		agentv1.RegisterAgentRunServiceServer(srv, agents)
		agentv1.RegisterAgentServiceServer(srv, agents)
		agentv1.RegisterAgentRegistryServiceServer(srv, agents)
		agentv1.RegisterAgentQuotaServiceServer(srv, agents)
		agentv1.RegisterAgentAuditServiceServer(srv, agents)
	}
	if outbox != nil {
		agentv1.RegisterAgentOutboxAdminServiceServer(srv, outbox)
	}
	return srv
}
