package sdk

import (
	"context"
	"fmt"
	"net/http"

	v1 "nonoka-im/api/im/v1"
)

type UsersResource struct {
	proto v1.UserServiceHTTPClient
	http  *apiTransport
}

func (r *UsersResource) Search(ctx context.Context, username string, limit int32) ([]User, error) {
	if limit <= 0 {
		limit = 20
	}
	reply, err := r.proto.SearchUsers(ctx, &v1.SearchUsersRequest{Username: username, Limit: limit})
	if err != nil {
		return nil, apiErrorFromKratos(err)
	}
	out := make([]User, 0, len(reply.GetUsers()))
	for _, user := range reply.GetUsers() {
		out = append(out, User{UserID: user.GetUserId(), Username: user.GetUsername()})
	}
	return out, nil
}

func (r *UsersResource) Presence(ctx context.Context, userID int64) (bool, error) {
	var reply v1.GetUserPresenceReply
	if err := r.http.doProto(ctx, http.MethodGet, fmt.Sprintf("/v1/users/%d/presence", userID), nil, nil, &reply); err != nil {
		return false, err
	}
	return reply.GetOnline(), nil
}
