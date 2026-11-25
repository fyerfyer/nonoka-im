package admin

import (
	"context"
	"testing"

	"nonoka-im/internal/agent/domain"
)

type catalogStub struct {
	model domain.Model
	err   error
}

func (c catalogStub) Lookup(context.Context, int64) (*domain.Model, error) {
	if c.err != nil {
		return nil, c.err
	}
	m := c.model
	return &m, nil
}

func TestValidateInputUsesModelCatalog(t *testing.T) {
	svc := &AgentService{Models: catalogStub{
		model: domain.Model{ID: 2, ModelKey: "deepseek/deepseek-chat", Enabled: true, Curated: true, MaxOutputTokens: 8192},
	}}
	in := &Input{Name: "catalog-agent", Instruction: "be helpful", ModelID: 2, Tools: []string{"send_message", "get_group_members"}}
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
