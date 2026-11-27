package sdk

import (
	"context"
	"fmt"
	"net/http"

	v1 "nonoka-im/api/im/v1"

	"google.golang.org/protobuf/types/known/emptypb"
)

type GroupsResource struct {
	http *apiTransport
}

func (r *GroupsResource) Create(ctx context.Context, name string, memberIDs []int64) (*Group, error) {
	var reply v1.CreateGroupReply
	if err := r.http.doProto(ctx, http.MethodPost, "/v1/groups", nil, &v1.CreateGroupRequest{Name: name, MemberIds: memberIDs}, &reply); err != nil {
		return nil, err
	}
	return &Group{GroupID: reply.GetGroupId(), Topic: reply.GetTopic(), Name: name}, nil
}

func (r *GroupsResource) ListMine(ctx context.Context) ([]Group, error) {
	var reply v1.ListGroupsReply
	if err := r.http.doProto(ctx, http.MethodGet, "/v1/groups", nil, nil, &reply); err != nil {
		return nil, err
	}
	out := make([]Group, 0, len(reply.GetGroups()))
	for _, group := range reply.GetGroups() {
		out = append(out, mapGroup(group))
	}
	return out, nil
}

func (r *GroupsResource) Get(ctx context.Context, groupID string) (*Group, error) {
	var reply v1.Group
	if err := r.http.doProto(ctx, http.MethodGet, fmt.Sprintf("/v1/groups/%s", groupID), nil, nil, &reply); err != nil {
		return nil, err
	}
	group := mapGroup(&reply)
	return &group, nil
}

func (r *GroupsResource) AddMember(ctx context.Context, groupID string, userID int64) error {
	body := &v1.AddGroupMemberRequest{GroupId: groupID, UserId: userID}
	return r.http.doProto(ctx, http.MethodPost, fmt.Sprintf("/v1/groups/%s/members", groupID), nil, body, &emptypb.Empty{})
}

func (r *GroupsResource) RemoveMember(ctx context.Context, groupID string, userID int64) error {
	return r.http.doProto(ctx, http.MethodDelete, fmt.Sprintf("/v1/groups/%s/members/%d", groupID, userID), nil, nil, &emptypb.Empty{})
}

func (r *GroupsResource) ListMembers(ctx context.Context, groupID string) ([]GroupMember, error) {
	var reply v1.ListGroupMembersReply
	if err := r.http.doProto(ctx, http.MethodGet, fmt.Sprintf("/v1/groups/%s/members", groupID), nil, nil, &reply); err != nil {
		return nil, err
	}
	out := make([]GroupMember, 0, len(reply.GetMembers()))
	for _, member := range reply.GetMembers() {
		out = append(out, GroupMember{UserID: member.GetUserId(), Username: member.GetUsername()})
	}
	return out, nil
}

func mapGroup(group *v1.Group) Group {
	return Group{GroupID: group.GetGroupId(), Topic: group.GetTopic(), Name: group.GetName(), OwnerID: group.GetOwnerId(), CreatedAt: group.GetCreatedAt()}
}
