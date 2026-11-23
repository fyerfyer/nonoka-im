package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (c *Client) ListProviderModels(ctx context.Context) ([]ProviderModel, error) {
	var out struct {
		Models []ProviderModel `json:"models"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/models", nil, &out); err != nil {
		return nil, err
	}
	return out.Models, nil
}

func (c *Client) CreateVirtualKey(ctx context.Context, spec VirtualKeySpec) (*VirtualKey, error) {
	body := virtualKeyPayload(spec)
	var out struct {
		VirtualKey struct {
			ID    string `json:"id"`
			Value string `json:"value"`
		} `json:"virtual_key"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/governance/virtual-keys", body, &out); err != nil {
		return nil, err
	}
	if out.VirtualKey.ID == "" || out.VirtualKey.Value == "" {
		return nil, fmt.Errorf("%w: virtual key response missing id/value", ErrGatewayRejected)
	}
	return &VirtualKey{ID: out.VirtualKey.ID, Value: out.VirtualKey.Value}, nil
}

func (c *Client) UpdateVirtualKey(ctx context.Context, id string, spec VirtualKeySpec, active bool) error {
	body := virtualKeyPayload(spec)
	body["is_active"] = active
	path := "/api/governance/virtual-keys/" + url.PathEscape(id)
	return c.do(ctx, http.MethodPut, path, body, nil)
}

func (c *Client) DeleteVirtualKey(ctx context.Context, id string) error {
	path := "/api/governance/virtual-keys/" + url.PathEscape(id)
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

func (c *Client) ListGatewayModels(ctx context.Context, virtualKey string) ([]GatewayModel, error) {
	if strings.TrimSpace(virtualKey) == "" {
		return nil, fmt.Errorf("%w: virtual key is required", ErrGatewayRejected)
	}
	var out struct {
		Data []GatewayModel `json:"data"`
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-bf-vk", virtualKey)
	if err := c.send(req, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

func virtualKeyPayload(spec VirtualKeySpec) map[string]any {
	providerConfigs := make([]map[string]any, 0, len(spec.ProviderModels))
	for provider, models := range spec.ProviderModels {
		if provider == "" || len(models) == 0 {
			continue
		}
		providerConfigs = append(providerConfigs, map[string]any{
			"provider":           provider,
			"allowed_models":     models,
			"blacklisted_models": []string{},
			"key_ids":            []string{"*"},
			"weight":             1,
		})
	}
	return map[string]any{
		"name":                spec.Name,
		"description":         spec.Description,
		"is_active":           true,
		"allow_all_providers": spec.AllowAll || len(providerConfigs) == 0,
		"provider_configs":    providerConfigs,
	}
}

// PricePerMTok converts the gateway per-token price string to USD per million
// tokens.
func PricePerMTok(raw string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || v < 0 {
		return 0
	}
	return v * 1_000_000
}
