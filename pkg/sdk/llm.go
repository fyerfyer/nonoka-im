package sdk

import (
	"context"
	"fmt"
	"net/http"
)

// ModelsResource is the read-only Bifrost model catalog plus platform curation.
// Provider credentials and gateway routing are never exposed here.
type ModelsResource struct {
	http *apiTransport
}

// List returns curated models. canManage is true for platform admins, who also
// see disabled/uncurated rows.
func (r *ModelsResource) List(ctx context.Context) ([]Model, bool, error) {
	var reply struct {
		Models    []Model `json:"models"`
		CanManage bool    `json:"canManage"`
	}
	if err := r.http.doJSON(ctx, http.MethodGet, "/v1/models", nil, nil, &reply, true); err != nil {
		return nil, false, err
	}
	return reply.Models, reply.CanManage, nil
}

// Sync refreshes Bifrost-owned fields from /v1/models. Local curation is
// preserved; new models start disabled.
func (r *ModelsResource) Sync(ctx context.Context) (int, error) {
	var reply struct {
		Synced int `json:"synced"`
	}
	if err := r.http.doJSON(ctx, http.MethodPost, "/v1/models/sync", nil, nil, &reply, false); err != nil {
		return 0, err
	}
	return reply.Synced, nil
}

// Update edits the platform-owned curation fields of one model.
func (r *ModelsResource) Update(ctx context.Context, id int64, input ModelCuration) (*Model, error) {
	var out Model
	if err := r.http.doJSON(ctx, http.MethodPatch, fmt.Sprintf("/v1/models/%d", id), nil, input, &out, false); err != nil {
		return nil, err
	}
	return &out, nil
}
