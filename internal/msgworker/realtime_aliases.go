package msgworker

import "nonoka-im/internal/realtime"

// Router, GatewayPusher and their node snapshots are implemented in
// internal/realtime; aliases keep the worker assembly and integration
// fixtures source-compatible while the implementation lives in the shared
// package.
type (
	Router          = realtime.Router
	GatewayNodeInfo = realtime.GatewayNodeInfo
	GatewayPusher   = realtime.GatewayPusher
)

var (
	NewGatewayPusher = realtime.NewGatewayPusher
	NewGatewayRouter = realtime.NewGatewayRouter
)
