package sdk

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	v1 "nonoka-im/api/im/v1"
)

type UsersResource struct {
	http *apiTransport
}

func (r *UsersResource) Search(ctx context.Context, username string, limit int32) ([]User, error) {
	if limit <= 0 {
		limit = 20
	}
	query := url.Values{}
	query.Set("username", username)
	query.Set("limit", strconv.Itoa(int(limit)))
	var reply v1.SearchUsersReply
	if err := r.http.doProto(ctx, http.MethodGet, "/v1/users/search", query, nil, &reply); err != nil {
		return nil, err
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
