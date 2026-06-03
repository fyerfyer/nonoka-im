package gateway

import "github.com/google/wire"

// ProviderSet is gateway providers.
var ProviderSet = wire.NewSet(
	NewManager,
	NewSessionManager,
	NewHandler,
	NewWebSocketServer,
	NewKafkaProducer,
	wire.Bind(new(MessageProducer), new(*KafkaProducer)),
)
