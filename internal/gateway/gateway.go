package gateway

import "github.com/google/wire"

// ProviderSet is gateway providers.
var ProviderSet = wire.NewSet(
	NewManager,
	NewSessionManager,
	NewKafkaProducer,
	wire.Bind(new(MessageProducer), new(*KafkaProducer)),
)

// WebSocketServerProviderSet is the provider set for WebSocket server with custom read timeout.
// Use provideWebSocketServer from wire.go instead of NewWebSocketServer directly.
var WebSocketServerProviderSet = wire.NewSet(
	NewWebSocketServer,
)
