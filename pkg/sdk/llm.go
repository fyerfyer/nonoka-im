package sdk

import (
	"context"
	"fmt"
	"net/http"
)

type LLMServiceInput struct {
	Name           string              `json:"name"`
	DisplayName    string              `json:"displayName,omitempty"`
	GatewayType    string              `json:"gatewayType,omitempty"`
	BaseURL        string              `json:"baseUrl,omitempty"`
	APIKey         string              `json:"apiKey,omitempty"`
	UpdateAPIKey   bool                `json:"updateApiKey,omitempty"`
	Enabled        *bool               `json:"enabled,omitempty"`
	AllowAll       *bool               `json:"allowAll,omitempty"`
	ProviderModels map[string][]string `json:"providerModels,omitempty"`
}

type LLMModelInput struct {
	ModelKey           string   `json:"modelKey"`
	DisplayName        string   `json:"displayName,omitempty"`
	ContextWindow      int      `json:"contextWindow,omitempty"`
	MaxOutputTokens    int      `json:"maxOutputTokens,omitempty"`
	SupportsTools      *bool    `json:"supportsTools,omitempty"`
	SupportsVision     *bool    `json:"supportsVision,omitempty"`
	SupportsReasoning  *bool    `json:"supportsReasoning,omitempty"`
	InputPricePerMtok  *float64 `json:"inputPricePerMtok,omitempty"`
	OutputPricePerMtok *float64 `json:"outputPricePerMtok,omitempty"`
	Enabled            *bool    `json:"enabled,omitempty"`
}

type LLMResource struct {
	http *apiTransport
}

func (r *LLMResource) Services(ctx context.Context) ([]LLMService, bool, error) {
	var reply struct {
		Services  []LLMService `json:"services"`
		CanManage bool         `json:"canManage"`
	}
	if err := r.http.doJSON(ctx, http.MethodGet, "/v1/model-services", nil, nil, &reply, true); err != nil {
		return nil, false, err
	}
	return reply.Services, reply.CanManage, nil
}

func (r *LLMResource) CreateService(ctx context.Context, input LLMServiceInput) (*LLMService, error) {
	var out LLMService
	if err := r.http.doJSON(ctx, http.MethodPost, "/v1/model-services", nil, input, &out, false); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *LLMResource) UpdateService(ctx context.Context, id int64, input LLMServiceInput) (*LLMService, error) {
	var out LLMService
	if err := r.http.doJSON(ctx, http.MethodPatch, fmt.Sprintf("/v1/model-services/%d", id), nil, input, &out, false); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *LLMResource) DeleteService(ctx context.Context, id int64) error {
	return r.http.doJSON(ctx, http.MethodDelete, fmt.Sprintf("/v1/model-services/%d", id), nil, nil, nil, false)
}

func (r *LLMResource) SyncService(ctx context.Context, id int64) ([]LLMModel, error) {
	var reply struct {
		Models []LLMModel `json:"models"`
	}
	if err := r.http.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/model-services/%d/sync", id), nil, nil, &reply, false); err != nil {
		return nil, err
	}
	return reply.Models, nil
}

func (r *LLMResource) CreateModel(ctx context.Context, serviceID int64, input LLMModelInput) (*LLMModel, error) {
	var out LLMModel
	if err := r.http.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/model-services/%d/models", serviceID), nil, input, &out, false); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *LLMResource) UpdateModel(ctx context.Context, id int64, input LLMModelInput) (*LLMModel, error) {
	var out LLMModel
	if err := r.http.doJSON(ctx, http.MethodPatch, fmt.Sprintf("/v1/model-models/%d", id), nil, input, &out, false); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *LLMResource) DeleteModel(ctx context.Context, id int64) error {
	return r.http.doJSON(ctx, http.MethodDelete, fmt.Sprintf("/v1/model-models/%d", id), nil, nil, nil, false)
}

func (r *LLMResource) GatewayModels(ctx context.Context) ([]struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
}, error) {
	var reply struct {
		Models []struct {
			Name     string `json:"name"`
			Provider string `json:"provider"`
		} `json:"models"`
	}
	if err := r.http.doJSON(ctx, http.MethodGet, "/v1/model-gateway/models", nil, nil, &reply, true); err != nil {
		return nil, err
	}
	return reply.Models, nil
}
