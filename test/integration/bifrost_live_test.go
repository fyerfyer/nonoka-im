package integration

import (
	"context"
	"os"
	"testing"

	"nonoka-im/internal/llm"
)

// TestBifrostContract is a live, opt-in contract check. Run with
// NONOKA_BIFROST_LIVE=1 BIFROST_URL=... BIFROST_SETUP_TOKEN=... (or admin
// credentials) after upgrading the gateway image.
func TestBifrostContract(t *testing.T) {
	if os.Getenv("NONOKA_BIFROST_LIVE") != "1" {
		t.Skip("set NONOKA_BIFROST_LIVE=1 to run the Bifrost contract test")
	}
	baseURL := os.Getenv("BIFROST_URL")
	if baseURL == "" {
		t.Fatal("BIFROST_URL is required")
	}
	client := llm.NewClient(baseURL, llm.Auth{
		AdminToken: os.Getenv("BIFROST_ADMIN_TOKEN"),
		SetupToken: os.Getenv("BIFROST_SETUP_TOKEN"),
		Username:   os.Getenv("BIFROST_ADMIN_USERNAME"),
		Password:   os.Getenv("BIFROST_ADMIN_PASSWORD"),
	})
	ctx := context.Background()
	customer, err := client.EnsureCustomer(ctx, llm.CustomerSpec{Name: "nonoka-contract"})
	if err != nil || customer == "" {
		t.Fatalf("ensure customer: %q %v", customer, err)
	}
	key, err := client.CreateVirtualKey(ctx, llm.VirtualKeySpec{
		Name:           "nonoka-contract-agent",
		ProviderModels: map[string][]string{"deepseek": {"deepseek-flash"}},
		CustomerID:     customer,
		Budgets:        []llm.BudgetSpec{{MaxLimit: 1, ResetDuration: "1M"}},
		RateLimit: &llm.RateLimitSpec{
			TokenMaxLimit: 100_000, TokenResetDuration: "1m",
			RequestMaxLimit: 10, RequestResetDuration: "1m",
		},
	})
	if err != nil || key.Value == "" {
		t.Fatalf("create virtual key: %+v %v", key, err)
	}
	defer func() { _ = client.DeleteVirtualKey(ctx, key.ID) }()
	models, err := client.ListGatewayModels(ctx, key.Value)
	if err != nil || len(models) == 0 {
		t.Fatalf("list models: %d %v", len(models), err)
	}
	for _, m := range models {
		if m.ID == "" || m.OwnedBy == "" || m.Pricing.Prompt == "" {
			t.Fatalf("model contract changed: %+v", m)
		}
	}
}
