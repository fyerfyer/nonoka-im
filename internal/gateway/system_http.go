package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"nonoka-im/internal/authz"
	"nonoka-im/internal/conf"

	jwtMiddleware "github.com/go-kratos/kratos/v2/middleware/auth/jwt"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	jwt5 "github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

const (
	defaultGatewayAdminStrategy = "consistent_hash"
	defaultGatewayAdminNodeTTL  = 30 * time.Second
)

// SystemHTTP exposes deployment-level operational state that is not part of
// the IM user API. It is intentionally small: the reference console needs a
// stable read-only view of the live gateway registry.
type SystemHTTP struct {
	redis      redis.UniversalClient
	strategy   string
	nodeTTL    time.Duration
	authorizer authz.PlatformAuthorizer
}

func NewSystemHTTP(redisClient redis.UniversalClient, dispatch *conf.Dispatch, authorizer authz.PlatformAuthorizer) *SystemHTTP {
	strategy := defaultGatewayAdminStrategy
	nodeTTL := defaultGatewayAdminNodeTTL
	if dispatch != nil {
		if dispatch.GetStrategy() != "" {
			strategy = dispatch.GetStrategy()
		}
		if dispatch.GetNodeTtl() != nil {
			nodeTTL = dispatch.GetNodeTtl().AsDuration()
		}
	}
	if nodeTTL <= 0 {
		nodeTTL = defaultGatewayAdminNodeTTL
	}
	return &SystemHTTP{
		redis:      redisClient,
		strategy:   strategy,
		nodeTTL:    nodeTTL,
		authorizer: authorizer,
	}
}

type gatewayNodeView struct {
	NodeID      string    `json:"nodeId"`
	URL         string    `json:"url"`
	GrpcAddr    string    `json:"grpcAddr"`
	Connections int       `json:"connections"`
	HeartbeatAt time.Time `json:"heartbeatAt"`
	Alive       bool      `json:"alive"`
}

type gatewayListReply struct {
	Strategy       string            `json:"strategy"`
	NodeTTLSeconds int64             `json:"nodeTtlSeconds"`
	Degraded       bool              `json:"degraded"`
	Warning        string            `json:"warning,omitempty"`
	Gateways       []gatewayNodeView `json:"gateways"`
}

// ListGateways returns the live gateway nodes discovered from Redis
// heartbeats. A Redis outage is reported as a degraded 200 response so the
// operator console can distinguish "no gateways" from "registry unavailable".
func (s *SystemHTTP) ListGateways(c khttp.Context) error {
	h := c.Middleware(func(ctx context.Context, _ interface{}) (interface{}, error) {
		uid := extractUserIDFromContext(ctx)
		if uid <= 0 {
			writeAgentJSON(c.Response(), http.StatusUnauthorized, map[string]string{
				"code": "UNAUTHORIZED", "message": "authentication required",
			})
			return nil, nil
		}
		if s.authorizer == nil || s.authorizer.CanManagePlatform(ctx, uid) != nil {
			writeAgentJSON(c.Response(), http.StatusForbidden, map[string]string{
				"code": "FORBIDDEN", "message": "platform admin required",
			})
			return nil, nil
		}

		reply := gatewayListReply{
			Strategy:       s.strategy,
			NodeTTLSeconds: int64(s.nodeTTL / time.Second),
			Gateways:       []gatewayNodeView{},
		}
		if s.redis == nil {
			reply.Degraded = true
			reply.Warning = "gateway registry is not configured"
			writeAgentJSON(c.Response(), http.StatusOK, reply)
			return nil, nil
		}

		nodes, err := GetAliveNodes(ctx, s.redis, s.nodeTTL)
		if err != nil {
			reply.Degraded = true
			reply.Warning = "gateway registry unavailable: " + err.Error()
			writeAgentJSON(c.Response(), http.StatusOK, reply)
			return nil, nil
		}

		sort.Slice(nodes, func(i, j int) bool { return nodes[i].NodeID < nodes[j].NodeID })
		for _, node := range nodes {
			reply.Gateways = append(reply.Gateways, gatewayNodeView{
				NodeID:      node.NodeID,
				URL:         node.URL,
				GrpcAddr:    node.GrpcAddr,
				Connections: node.ConnCount,
				HeartbeatAt: node.UpdatedAt,
				Alive:       true,
			})
		}
		writeAgentJSON(c.Response(), http.StatusOK, reply)
		return nil, nil
	})
	_, err := h(c, nil)
	return err
}

func extractUserIDFromContext(ctx context.Context) int64 {
	claims, ok := jwtMiddleware.FromContext(ctx)
	if !ok {
		return 0
	}
	if mapClaims, ok := claims.(jwt5.MapClaims); ok {
		if uid, ok := mapClaims["user_id"].(float64); ok {
			return int64(uid)
		}
	}
	return 0
}

func writeAgentJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
