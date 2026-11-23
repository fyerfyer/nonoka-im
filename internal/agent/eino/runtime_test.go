package eino

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"nonoka-im/internal/agent/port"
)

type fakeModel struct {
	calls       [][]*schema.Message
	failSummary bool
}

func (f *fakeModel) Generate(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	f.calls = append(f.calls, input)
	if strings.Contains(input[len(input)-1].Content, "CURRENT_INPUT_MARKER") {
		return schema.AssistantMessage("final answer", nil), nil
	}
	if f.failSummary {
		return nil, errors.New("summary unavailable")
	}
	return schema.AssistantMessage("stable previous fact", nil), nil
}

func (f *fakeModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	msg, err := f.Generate(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}

func (f *fakeModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return f, nil
}

func TestSummarizationExcludesCurrentInputAndLoadsAcrossRuntime(t *testing.T) {
	fake := &fakeModel{}
	r := &Engine{newModel: fakeFactory(fake)}
	in := port.Input{
		Instruction: "Answer carefully", Text: "CURRENT_INPUT_MARKER",
		Model:                port.ModelEndpoint{ContextWindow: 1},
		History:              []port.Turn{{ID: 7, Input: "old question", Output: "old answer"}},
		SummaryThroughTurnID: 7,
	}
	result, err := r.Run(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "final answer" || result.SummaryThroughTurnID != 7 || !strings.Contains(result.Summary, "stable previous fact") {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(fake.calls) != 2 {
		t.Fatalf("expected summary and answer model calls, got %d", len(fake.calls))
	}
	for _, msg := range fake.calls[0] {
		if strings.Contains(msg.Content, "CURRENT_INPUT_MARKER") {
			t.Fatal("current input leaked into summary request")
		}
	}
	fresh := &fakeModel{}
	_, err = (&Engine{newModel: fakeFactory(fresh)}).Run(context.Background(), port.Input{
		Instruction: in.Instruction, Summary: result.Summary, Text: "CURRENT_INPUT_MARKER",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fresh.calls) != 1 || !strings.Contains(fresh.calls[0][1].Content, "stable previous fact") {
		t.Fatal("new runtime did not load persisted summary")
	}
}

func TestSummarizationFailureAllowsRecentTurnFallback(t *testing.T) {
	fake := &fakeModel{failSummary: true}
	r := &Engine{newModel: fakeFactory(fake)}
	in := port.Input{Instruction: "Answer carefully", Text: "CURRENT_INPUT_MARKER", Model: port.ModelEndpoint{ContextWindow: 1}, History: []port.Turn{{ID: 1, Input: "old", Output: "reply"}}}
	if _, err := r.Run(context.Background(), in); !errors.Is(err, port.ErrSummary) {
		t.Fatalf("expected summary error, got %v", err)
	}
	in.DisableSummary = true
	result, err := r.Run(context.Background(), in)
	if err != nil || result.Text != "final answer" {
		t.Fatalf("fallback lost current input: result=%+v err=%v", result, err)
	}
}

// TestSummaryTrackerWrapsGenerationErrors pins the error contract: the tracker
// must tag summary model failures by wrapping port.ErrSummary (keeping errors.Is
// working) so no global flag is needed to recognize them later.
func TestSummaryTrackerWrapsGenerationErrors(t *testing.T) {
	tr := summaryTracker{model: &fakeModel{failSummary: true}}
	msgs := []*schema.Message{schema.UserMessage("summarize this")}
	if _, err := tr.Generate(context.Background(), msgs); !errors.Is(err, port.ErrSummary) {
		t.Fatalf("Generate error must wrap port.ErrSummary, got %v", err)
	}
	if _, err := tr.Stream(context.Background(), msgs); !errors.Is(err, port.ErrSummary) {
		t.Fatalf("Stream error must wrap port.ErrSummary, got %v", err)
	}
}

// failFinalModel summarizes successfully but blows up on the actual answer.
type failFinalModel struct{ calls int }

func (f *failFinalModel) Generate(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	f.calls++
	if strings.Contains(input[len(input)-1].Content, "CURRENT_INPUT_MARKER") {
		return nil, errors.New("answer generation exploded")
	}
	return schema.AssistantMessage("stable previous fact", nil), nil
}

func (f *failFinalModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	msg, err := f.Generate(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}

func (f *failFinalModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return f, nil
}

// TestRunErrorAfterSuccessfulSummaryIsNotErrSummary guards the fragile
// heuristic this replaced: only errors raised by the summary model itself may
// classify as ErrSummary; a later, unrelated model failure must surface as-is.
func TestRunErrorAfterSuccessfulSummaryIsNotErrSummary(t *testing.T) {
	r := &Engine{newModel: fakeFactory(&failFinalModel{})}
	_, err := r.Run(context.Background(), port.Input{
		Instruction: "Answer carefully", Text: "CURRENT_INPUT_MARKER",
		Model:   port.ModelEndpoint{ContextWindow: 1},
		History: []port.Turn{{ID: 1, Input: "old", Output: "reply"}},
	})
	if err == nil || errors.Is(err, port.ErrSummary) {
		t.Fatalf("answer failure must not be reported as ErrSummary: %v", err)
	}
}

// TestRunRoutesToAgentOwnedEndpoint verifies that an agent's own base URL,
// API key and model name override the runtime defaults, so one worker can run
// agents backed by different OpenAI-compatible providers.
func TestRunRoutesToAgentOwnedEndpoint(t *testing.T) {
	var gotModel, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Model string `json:"model"`
		}
		_ = json.Unmarshal(body, &req)
		gotModel = req.Model
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"fixture-model","choices":[{"index":0,"message":{"role":"assistant","content":"reply from fixture"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`))
	}))
	defer srv.Close()

	rt, err := New(context.Background(), Deps{})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	res, err := rt.Run(context.Background(), port.Input{
		Instruction: "hi", Text: "hello",
		Model: port.ModelEndpoint{BaseURL: srv.URL + "/v1", APIKey: "agent-own-key", Model: "fixture-model", MaxTokens: 512},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.Text != "reply from fixture" {
		t.Fatalf("unexpected reply %q", res.Text)
	}
	if gotModel != "fixture-model" {
		t.Fatalf("fixture saw model %q", gotModel)
	}
	if gotAuth != "Bearer agent-own-key" {
		t.Fatalf("fixture saw auth %q", gotAuth)
	}
	if res.Usage.TotalTokens != 3 {
		t.Fatalf("usage not recorded: %+v", res.Usage)
	}
}

func fakeFactory(m model.ToolCallingChatModel) ChatModelFactory {
	return func(context.Context, port.ModelEndpoint, int) (model.ToolCallingChatModel, error) {
		return m, nil
	}
}
