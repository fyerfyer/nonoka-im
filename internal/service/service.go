package service

import "github.com/google/wire"

// ProviderSet is the Kratos transport adapter provider set. Constructors in
// this package depend only on application usecases and shared contracts.
var ProviderSet = wire.NewSet(
	NewAuthService,
	NewMessageService,
	NewConversationService,
	NewGroupService,
	NewFileService,
	NewAgentHTTP,
	NewOutboxAdminHTTP,
)
