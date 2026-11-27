package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBifrostClientControlPlaneAndModelCatalog(t *testing.T) {
	var created map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Bifrost-Setup-Token") != "setup" {
			http.Error(w, `{"error":{"message":"missing setup token"}}`, http.StatusForbidden)
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/governance/customers":
			_ = json.NewEncoder(w).Encode(map[string]any{"customer": map[string]any{"id": "cust-1"}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/governance/virtual-keys":
			if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
				t.Errorf("decode create: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"virtual_key": map[string]any{"id": "vk-1", "value": "sk-bf-test"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
			if r.Header.Get("x-bf-vk") != "sk-bf-test" {
				http.Error(w, "missing vk", http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{
					"id": "deepseek/deepseek-chat", "normalized_name": "DeepSeek Chat",
					"context_length": 65536, "max_output_tokens": 8192,
					"owned_by": "deepseek",
					"pricing":  map[string]string{"prompt": "0.0000003", "completion": "0.0000012"},
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, Auth{SetupToken: "setup"})
	customer, err := client.EnsureCustomer(context.Background(), CustomerSpec{Name: "nonoka"})
	if err != nil || customer != "cust-1" {
		t.Fatalf("ensure customer: %q, %v", customer, err)
	}
	vk, err := client.CreateVirtualKey(context.Background(), VirtualKeySpec{
		Name: "svc", ProviderModels: map[string][]string{"deepseek": {"deepseek-chat"}},
		CustomerID: customer,
		Budgets:    []BudgetSpec{{MaxLimit: 20, ResetDuration: "1M"}},
		RateLimit: &RateLimitSpec{
			TokenMaxLimit: 100_000, TokenResetDuration: "1m",
			RequestMaxLimit: 60, RequestResetDuration: "1m",
		},
	})
	if err != nil || vk.ID != "vk-1" || vk.Value != "sk-bf-test" {
		t.Fatalf("create vk: %+v, %v", vk, err)
	}
	if created["name"] != "svc" || created["customer_id"] != "cust-1" {
		t.Fatalf("request body: %+v", created)
	}
	gatewayModels, err := client.ListGatewayModels(context.Background(), vk.Value)
	if err != nil || len(gatewayModels) != 1 {
		t.Fatalf("gateway models: %+v, %v", gatewayModels, err)
	}
	m := gatewayModels[0]
	if m.ID != "deepseek/deepseek-chat" || m.MaxOutputTokens != 8192 || m.OwnedBy != "deepseek" {
		t.Fatalf("gateway model: %+v", m)
	}
	if PricePerMTok(m.Pricing.Prompt) != 0.3 || PricePerMTok(m.Pricing.Completion) != 1.2 {
		t.Fatalf("price conversion: %+v", m.Pricing)
	}
}

func TestBifrostClientReturnsTypedGatewayErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"type":"provider_error","error":{"message":"upstream down"}}`))
	}))
	defer srv.Close()
	client := NewClient(srv.URL, Auth{})
	_, err := client.CreateVirtualKey(context.Background(), VirtualKeySpec{Name: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrGatewayRejected) {
		t.Fatalf("expected ErrGatewayRejected, got %v", err)
	}
}
