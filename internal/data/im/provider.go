package imdata

import "github.com/google/wire"

// ProviderSet wires the IM PostgreSQL repositories. Each constructor takes the
// shared *gorm.DB and Redis client from internal/data.
var ProviderSet = wire.NewSet(
	NewAuthRepo,
	NewGroupRepo,
	NewGroupMemberRepo,
	NewConversationRepo,
)
