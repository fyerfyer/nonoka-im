package authz

import (
	"context"
	"errors"
	"testing"
)

type membershipStub struct {
	members map[string]map[int64]bool
	err     error
}

func (s membershipStub) IsGroupMember(_ context.Context, groupID string, userID int64) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	return s.members[groupID][userID], nil
}

func TestTopicAuthorizer(t *testing.T) {
	group := membershipStub{members: map[string]map[int64]bool{"g": {2: true}}}
	tests := []struct {
		name    string
		repo    membershipStub
		uid     int64
		topic   string
		wantErr bool
	}{
		{name: "p2p participant", uid: 1, topic: "p2p_2_1"},
		{name: "p2p outsider", uid: 3, topic: "p2p_1_2", wantErr: true},
		{name: "group member", uid: 2, topic: "grp_g", repo: group},
		{name: "group outsider", uid: 3, topic: "grp_g", repo: group, wantErr: true},
		{name: "group lookup failure", uid: 2, topic: "grp_g", repo: membershipStub{err: errors.New("database unavailable")}, wantErr: true},
		{name: "system owner", uid: 4, topic: "sys_4"},
		{name: "malformed system topic", uid: 4, topic: "sys_4suffix", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewTopicAuthorizer(tt.repo).CanAccessTopic(context.Background(), tt.uid, tt.topic)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v, wantErr=%t", err, tt.wantErr)
			}
		})
	}
}

func TestTopicAuthorizerFileOwner(t *testing.T) {
	a := NewTopicAuthorizer(nil)
	if err := a.CanReadFile(context.Background(), 10, "10"); err != nil {
		t.Fatal(err)
	}
	if err := a.CanReadFile(context.Background(), 11, "10"); err == nil {
		t.Fatal("expected non-owner to be denied")
	}
}
