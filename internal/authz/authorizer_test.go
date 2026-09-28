package authz

import (
	"context"
	"errors"
	"testing"

	pb "nonoka-im/api/im/v1"
	"nonoka-im/internal/data"
)

type groupRepoStub struct {
	members []*pb.GroupMember
	err     error
}

func (s groupRepoStub) CreateGroup(context.Context, string, int64, []int64) (*data.Group, error) {
	return nil, nil
}
func (s groupRepoStub) ListMyGroups(context.Context, int64) ([]*data.Group, error) { return nil, nil }
func (s groupRepoStub) GetGroup(context.Context, string) (*data.Group, error)      { return nil, nil }
func (s groupRepoStub) AddMember(context.Context, string, int64) error             { return nil }
func (s groupRepoStub) RemoveMember(context.Context, string, int64) error          { return nil }
func (s groupRepoStub) ListMembers(context.Context, string) ([]*pb.GroupMember, error) {
	return s.members, s.err
}

func TestTopicAuthorizer(t *testing.T) {
	tests := []struct {
		name    string
		repo    groupRepoStub
		uid     int64
		topic   string
		wantErr bool
	}{
		{name: "p2p participant", uid: 1, topic: "p2p_2_1"},
		{name: "p2p outsider", uid: 3, topic: "p2p_1_2", wantErr: true},
		{name: "group member", uid: 2, topic: "grp_g", repo: groupRepoStub{members: []*pb.GroupMember{{UserId: 2}}}},
		{name: "group outsider", uid: 3, topic: "grp_g", repo: groupRepoStub{members: []*pb.GroupMember{{UserId: 2}}}, wantErr: true},
		{name: "group lookup failure", uid: 2, topic: "grp_g", repo: groupRepoStub{err: errors.New("database unavailable")}, wantErr: true},
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
	if err := a.CanReadFile(context.Background(), 10, 10); err != nil {
		t.Fatal(err)
	}
	if err := a.CanReadFile(context.Background(), 11, 10); err == nil {
		t.Fatal("expected non-owner to be denied")
	}
}
