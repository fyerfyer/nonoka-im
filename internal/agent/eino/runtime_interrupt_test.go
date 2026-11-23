package eino

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"nonoka-im/internal/agent/port"
	"nonoka-im/internal/agent/tools"
	"nonoka-im/internal/agent/tools/builtin"
	"nonoka-im/internal/agent/tools/catalog"
)

type staticCatalog struct{ defs []catalog.Definition }

func (c staticCatalog) Definition(name string) (catalog.Definition, bool) {
	for _, def := range c.defs {
		if def.Name == name {
			return def, true
		}
	}
	return catalog.Definition{}, false
}
func (c staticCatalog) Definitions() []catalog.Definition { return c.defs }

// interruptBackend counts side effects so tests can assert the wrapped tool
// only executed after approval.
type interruptBackend struct {
	mu    sync.Mutex
	sends int
}

func (b *interruptBackend) SearchMessages(context.Context, tools.RunContext, string, int, tools.SearchFilter) ([]string, error) {
	return nil, nil
}
func (b *interruptBackend) ConversationContext(context.Context, tools.RunContext, int) ([]string, error) {
	return nil, nil
}
func (b *interruptBackend) SendMessage(_ context.Context, _ tools.RunContext, _ string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sends++
	return nil
}
func (b *interruptBackend) ReadFile(context.Context, tools.RunContext, string, int) (string, string, []byte, bool, error) {
	return "", "", nil, false, errors.New("no file")
}
func (b *interruptBackend) TopicInfo(context.Context, tools.RunContext) (string, error) {
	return "p2p", nil
}
func (b *interruptBackend) MyInfo(context.Context, tools.RunContext) (string, error) {
	return "", nil
}
func (b *interruptBackend) MyGroups(context.Context, tools.RunContext) (string, error) {
	return "", nil
}
func (b *interruptBackend) GroupMembers(context.Context, tools.RunContext, string) (string, error) {
	return "", nil
}

func (b *interruptBackend) sendCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sends
}

type allowGuard struct{}

func (allowGuard) CanExecuteTool(context.Context, int64, int64, string, string, string, string) error {
	return nil
}

// approvalModel returns a tool call on the first generation and a final answer
// afterwards.
type approvalModel struct{ calls int }

func (m *approvalModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	m.calls++
	if m.calls == 1 {
		return schema.AssistantMessage("", []schema.ToolCall{{ID: "call-1", Function: schema.FunctionCall{Name: "send_message", Arguments: `{"content":"hello"}`}}}), nil
	}
	return schema.AssistantMessage("final answer after approval", nil), nil
}

func (m *approvalModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	msg, err := m.Generate(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}

func (m *approvalModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

// memCheckPoints is an in-memory adk.CheckPointStore.
type memCheckPoints struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (m *memCheckPoints) Get(_ context.Context, id string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.data[id]
	return b, ok, nil
}
func (m *memCheckPoints) Set(_ context.Context, id string, b []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		m.data = make(map[string][]byte)
	}
	m.data[id] = b
	return nil
}

func approvalRuntime(backend *interruptBackend) *Engine {
	r := &Engine{newModel: fakeFactory(&approvalModel{}), checkpoints: &memCheckPoints{}, toolCatalog: staticCatalog{defs: catalog.BuiltinDefinitions()}}
	for _, t := range builtin.New(backend, allowGuard{}, 8192) {
		r.tools = append(r.tools, t)
	}
	return r
}

func approvalInput() port.Input {
	return port.Input{
		Text:          "send a greeting",
		Instruction:   "test",
		Model:         port.ModelEndpoint{BaseURL: "https://fixture.invalid/v1", APIKey: "fixture-key", Model: "fixture-model"},
		AgentID:       7,
		ActorUserID:   9,
		BotID:         8,
		Topic:         "p2p_8_9",
		EventID:       "evt-approval",
		CheckPointID:  "agent-run-77",
		AllowedTools:  []string{"send_message"},
		RestrictTools: true,
		ApprovalTools: []string{"send_message"},
	}
}

func TestRunInterruptsForApprovalAndResumeExecutesOnce(t *testing.T) {
	backend := &interruptBackend{}
	r := approvalRuntime(backend)

	res, err := r.Run(context.Background(), approvalInput())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != port.StatusInterrupted {
		t.Fatalf("expected interrupted run, got %+v", res)
	}
	if res.Interrupt == nil || res.Interrupt.Tool != "send_message" {
		t.Fatalf("expected send_message interrupt, got %+v", res.Interrupt)
	}
	if res.Interrupt.ToolCallID == "" {
		t.Fatal("interrupt ID missing")
	}
	if res.Interrupt.Summary == "" {
		t.Fatal("interrupt summary missing")
	}
	if strings.Contains(res.Interrupt.Summary, "hello") {
		t.Fatalf("approval summary leaked raw argument: %q", res.Interrupt.Summary)
	}
	if !strings.Contains(res.Interrupt.Summary, "send a message") {
		t.Fatalf("catalog summarizer was not used: %q", res.Interrupt.Summary)
	}
	if res.Interrupt.ArgsHash == "" {
		t.Fatal("interrupt args hash missing")
	}
	if backend.sendCount() != 0 {
		t.Fatalf("tool executed before approval: %d", backend.sendCount())
	}

	resume := port.ResumeInput{Input: approvalInput(), InterruptID: res.Interrupt.ToolCallID, Approved: true}
	res, err = r.Resume(context.Background(), resume)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != port.StatusCompleted || res.Text != "final answer after approval" {
		t.Fatalf("unexpected resume result: %+v", res)
	}
	if backend.sendCount() != 1 {
		t.Fatalf("expected exactly one execution after approval, got %d", backend.sendCount())
	}
}

func TestResumeRejectionSkipsToolExecution(t *testing.T) {
	backend := &interruptBackend{}
	r := approvalRuntime(backend)

	res, err := r.Run(context.Background(), approvalInput())
	if err != nil {
		t.Fatal(err)
	}
	resume := port.ResumeInput{Input: approvalInput(), InterruptID: res.Interrupt.ToolCallID, Approved: false}
	res, err = r.Resume(context.Background(), resume)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != port.StatusCompleted || res.Text == "" {
		t.Fatalf("unexpected resume result: %+v", res)
	}
	if backend.sendCount() != 0 {
		t.Fatalf("rejected tool must not execute, got %d", backend.sendCount())
	}
}

func TestRunWithoutApprovalToolsExecutesDirectly(t *testing.T) {
	backend := &interruptBackend{}
	r := approvalRuntime(backend)
	in := approvalInput()
	in.ApprovalTools = nil

	res, err := r.Run(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != port.StatusCompleted {
		t.Fatalf("expected completed run, got %+v", res)
	}
	if backend.sendCount() != 1 {
		t.Fatalf("tool without approval wrapper must execute, got %d", backend.sendCount())
	}
}
