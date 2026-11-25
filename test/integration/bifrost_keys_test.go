package integration

import (
	"strings"
	"testing"

	"nonoka-im/internal/agent/domain"
)

func TestBifrost_PerAgentKeyLifecycleAndResolution(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	model, err := ts.modelSvc.Lookup(t.Context(), ts.modelID)
	if err != nil {
		t.Fatalf("lookup model: %v", err)
	}
	if err := ts.modelSvc.EnsureAgentKey(t.Context(), 880001, []domain.Model{*model}); err != nil {
		t.Fatalf("ensure agent key: %v", err)
	}
	status, err := ts.modelSvc.AgentKeyStatus(t.Context(), 880001)
	if err != nil || status != domain.GatewayKeyActive {
		t.Fatalf("key status: %q %v", status, err)
	}
	endpoint, err := ts.modelSvc.ResolveForRun(t.Context(), 880001, ts.modelID)
	if err != nil {
		t.Fatalf("resolve agent run: %v", err)
	}
	if endpoint.VirtualKey != "sk-bf-test" || endpoint.Model != model.ModelKey || endpoint.ModelID != model.ID {
		t.Fatalf("endpoint: %+v", endpoint)
	}
	if strings.Contains(endpoint.APIKey, "sk-bf") {
		t.Fatalf("virtual key leaked into APIKey: %q", endpoint.APIKey)
	}

	// The stored value is sealed, not plaintext.
	key, err := ts.modelSvc.Store.GetAgentGatewayKey(t.Context(), 880001)
	if err != nil {
		t.Fatalf("read key: %v", err)
	}
	if key.ValueEnc == "" || strings.Contains(key.ValueEnc, "sk-bf-test") {
		t.Fatalf("key value not sealed: %+v", key)
	}

	if err := ts.modelSvc.RevokeAgentKey(t.Context(), 880001); err != nil {
		t.Fatalf("revoke key: %v", err)
	}
	if _, err := ts.modelSvc.ResolveForRun(t.Context(), 880001, ts.modelID); err == nil {
		t.Fatal("resolved after revoke")
	}
}
