package service

import (
	"context"
	"net/http"
	"sort"
	"time"

	"nonoka-im/internal/authz"
	"nonoka-im/internal/conf"
	"nonoka-im/internal/gateway"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
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

		nodes, err := gateway.GetAliveNodes(ctx, s.redis, s.nodeTTL)
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
