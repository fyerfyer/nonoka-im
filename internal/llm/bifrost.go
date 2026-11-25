package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

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

// EnsureCustomer creates or updates the deployment-level Bifrost customer and
// applies its monthly budget. A VK can only attach to one customer, so the
// customer is the global safety ceiling for all agent keys.
func (c *Client) EnsureCustomer(ctx context.Context, spec CustomerSpec) (string, error) {
	var list struct {
		Customers []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"customers"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/governance/customers?limit=100", nil, &list); err == nil {
		for _, item := range list.Customers {
			if item.Name == spec.Name && item.ID != "" {
				if err := c.updateCustomer(ctx, item.ID, spec); err != nil {
					return "", err
				}
				return item.ID, nil
			}
		}
	}
	body := map[string]any{"name": spec.Name}
	if len(spec.Budgets) > 0 {
		body["budgets"] = budgetPayloads(spec.Budgets)
	} else {
		body["budgets"] = []map[string]any{}
	}
	if len(spec.Metadata) > 0 {
		body["metadata"] = spec.Metadata
	}
	var out struct {
		Customer struct {
			ID string `json:"id"`
		} `json:"customer"`
	}
	createErr := c.do(ctx, http.MethodPost, "/api/governance/customers", body, &out)
	if createErr != nil {
		// Another instance may have created the customer between the list and
		// create calls; re-list before surfacing the error.
		var retry struct {
			Customers []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"customers"`
		}
		if lerr := c.do(ctx, http.MethodGet, "/api/governance/customers?limit=100", nil, &retry); lerr == nil {
			for _, item := range retry.Customers {
				if item.Name == spec.Name && item.ID != "" {
					if uerr := c.updateCustomer(ctx, item.ID, spec); uerr != nil {
						return "", uerr
					}
					return item.ID, nil
				}
			}
		}
		return "", createErr
	}
	if out.Customer.ID == "" {
		return "", fmt.Errorf("%w: customer response missing id", ErrGatewayRejected)
	}
	return out.Customer.ID, nil
}

func (c *Client) updateCustomer(ctx context.Context, id string, spec CustomerSpec) error {
	body := map[string]any{"name": spec.Name}
	if len(spec.Budgets) > 0 {
		body["budgets"] = budgetPayloads(spec.Budgets)
	} else {
		body["budgets"] = []map[string]any{}
	}
	if len(spec.Metadata) > 0 {
		body["metadata"] = spec.Metadata
	}
	return c.do(ctx, http.MethodPut, "/api/governance/customers/"+url.PathEscape(id), body, nil)
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
	body := map[string]any{
		"name":                spec.Name,
		"description":         spec.Description,
		"is_active":           true,
		"allow_all_providers": !spec.DenyAll && (spec.AllowAll || len(providerConfigs) == 0),
		"provider_configs":    providerConfigs,
	}
	if spec.CustomerID != "" {
		body["customer_id"] = spec.CustomerID
	}
	if len(spec.Metadata) > 0 {
		body["metadata"] = spec.Metadata
	}
	if len(spec.Budgets) > 0 {
		body["budgets"] = budgetPayloads(spec.Budgets)
	} else if spec.ClearBudgets {
		body["budgets"] = []map[string]any{}
	}
	if spec.RateLimit != nil {
		body["rate_limit"] = map[string]any{
			"token_max_limit":        spec.RateLimit.TokenMaxLimit,
			"token_reset_duration":   spec.RateLimit.TokenResetDuration,
			"request_max_limit":      spec.RateLimit.RequestMaxLimit,
			"request_reset_duration": spec.RateLimit.RequestResetDuration,
		}
	} else if spec.ClearRateLimit {
		body["rate_limit"] = nil
	}
	return body
}

func budgetPayloads(budgets []BudgetSpec) []map[string]any {
	out := make([]map[string]any, 0, len(budgets))
	for _, b := range budgets {
		if b.MaxLimit <= 0 {
			continue
		}
		reset := strings.TrimSpace(b.ResetDuration)
		if reset == "" {
			reset = "1M"
		}
		out = append(out, map[string]any{"max_limit": b.MaxLimit, "reset_duration": reset})
	}
	return out
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
