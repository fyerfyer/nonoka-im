package domain

import "testing"

func TestAuthorizeTopicDirect(t *testing.T) {
	tests := []struct {
		name    string
		userID  int64
		topic   string
		allowed bool
		handled bool
	}{
		{name: "p2p participant", userID: 1, topic: "p2p_1_2", allowed: true, handled: true},
		{name: "p2p peer", userID: 2, topic: "p2p_2_1", allowed: true, handled: true},
		{name: "p2p outsider", userID: 3, topic: "p2p_1_2", allowed: false, handled: true},
		{name: "system owner", userID: 7, topic: "sys_7", allowed: true, handled: true},
		{name: "system outsider", userID: 8, topic: "sys_7", allowed: false, handled: true},
		{name: "group delegated to authz", userID: 7, topic: "grp_abc", handled: false},
		{name: "invalid topic", userID: 7, topic: "bad", handled: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, handled := AuthorizeTopicDirect(tt.userID, tt.topic)
			if allowed != tt.allowed || handled != tt.handled {
				t.Fatalf("got allowed=%v handled=%v, want allowed=%v handled=%v", allowed, handled, tt.allowed, tt.handled)
			}
		})
	}
}
