package domain

import "nonoka-im/internal/topic"

// AuthorizeTopicDirect applies the deterministic ACLs for topics whose
// membership is encoded in the topic name itself. It is the hot path for P2P
// and system messages; group topics deliberately return handled=false so the
// caller can consult OpenFGA (the single source of truth for group membership).
//
// handled=false also means the topic could not be parsed; callers should treat
// that as an invalid topic rather than an authorization failure.
func AuthorizeTopicDirect(userID int64, raw string) (allowed bool, handled bool) {
	return topic.AuthorizeDirect(userID, raw)
}
