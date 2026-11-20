package service

import (
	"github.com/google/wire"
)

// ProviderSet is service providers.
var ProviderSet = wire.NewSet(NewAuthService, NewDispatchService, NewPushService, NewMessageServiceWithAuthorizer, NewConversationService, NewGroupServiceWithAuthorizer, NewFileServiceWithAuthorizer, NewAgentHTTP, NewOutboxAdminHTTP)
