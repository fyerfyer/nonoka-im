package admin

import (
	"context"
	"testing"

	"nonoka-im/internal/agent/domain"
)

type catalogStub struct {
	service domain.ModelService
	model   domain.Model
	err     error
}

func (c catalogStub) Lookup(context.Context, int64, int64) (domain.ModelService, domain.Model, error) {
	return c.service, c.model, c.err
}

func TestValidateInputUsesModelCatalog(t *testing.T) {
	svc := &AgentService{Models: catalogStub{
		service: domain.ModelService{ID: 1, Name: "bifrost", Enabled: true},
		model:   domain.Model{ID: 2, ServiceID: 1, ModelKey: "deepseek/deepseek-chat", Enabled: true, MaxOutputTokens: 8192},
	}}
	in := &Input{Name: "catalog-agent", Instruction: "be helpful", ModelServiceID: 1, ModelID: 2, Tools: []string{"send_message", "get_group_members"}}
	if err := svc.validateInput(context.Background(), in); err != nil {
		t.Fatalf("catalog tools rejected: %v", err)
	}
	in.Tools = []string{"not_a_real_tool"}
	if err := svc.validateInput(context.Background(), in); err == nil {
		t.Fatal("unknown tool accepted; validation is still hardcoded")
	}
	in.Tools = nil
	in.ToolPolicies = map[string]string{"send_message": "sometimes"}
	if err := svc.validateInput(context.Background(), in); err == nil {
		t.Fatal("invalid tool policy accepted")
	}
}
