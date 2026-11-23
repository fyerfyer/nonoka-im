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
		case r.Method == http.MethodGet && r.URL.Path == "/api/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]string{{"name": "deepseek-chat", "provider": "deepseek"}},
			})
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
					"pricing": map[string]string{"prompt": "0.0000003", "completion": "0.0000012"},
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, Auth{SetupToken: "setup"})
	models, err := client.ListProviderModels(context.Background())
	if err != nil || len(models) != 1 || models[0].Provider != "deepseek" {
		t.Fatalf("provider models: %+v, %v", models, err)
	}
	vk, err := client.CreateVirtualKey(context.Background(), VirtualKeySpec{
		Name: "svc", ProviderModels: map[string][]string{"deepseek": {"deepseek-chat"}},
	})
	if err != nil || vk.ID != "vk-1" || vk.Value != "sk-bf-test" {
		t.Fatalf("create vk: %+v, %v", vk, err)
	}
	if created["name"] != "svc" {
		t.Fatalf("request name: %+v", created)
	}
	gatewayModels, err := client.ListGatewayModels(context.Background(), vk.Value)
	if err != nil || len(gatewayModels) != 1 {
		t.Fatalf("gateway models: %+v, %v", gatewayModels, err)
	}
	m := gatewayModels[0]
	if m.ID != "deepseek/deepseek-chat" || m.MaxOutputTokens != 8192 {
		t.Fatalf("gateway model: %+v", m)
	}
	if PricePerMTok(m.Pricing.Prompt) != 0.3 || PricePerMTok(m.Pricing.Completion) != 1.2 {
		t.Fatalf("price conversion: %+v", m.Pricing)
	}
}

func TestBifrostClientReturnsTypedGatewayErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":{"message":"upstream down"}}`))
	}))
	defer srv.Close()
	client := NewClient(srv.URL, Auth{})
	_, err := client.ListProviderModels(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrGatewayRejected) {
		t.Fatalf("expected ErrGatewayRejected, got %v", err)
	}
}
