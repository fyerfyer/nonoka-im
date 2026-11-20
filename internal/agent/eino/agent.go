package eino

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"sync/atomic"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/patchtoolcalls"
	"github.com/cloudwego/eino/adk/middlewares/skill"
	"github.com/cloudwego/eino/adk/middlewares/summarization"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	agentcontext "nonoka-im/internal/agent/context"
	"nonoka-im/internal/agent/port"
	"nonoka-im/internal/agent/tools"
	"nonoka-im/internal/agent/tools/catalog"
)

type approvalInterruptInfo struct {
	Tool     string `json:"tool"`
	Summary  string `json:"summary"`
	ArgsHash string `json:"args_hash"`
}

// approvalDecision is handed back to the interrupted tool on resume.
type approvalDecision struct {
	Approved bool `json:"approved"`
}

// approvalTool wraps a tool whose calls require human confirmation. The first
// execution interrupts with the call summary; the resume delivers the user's
// decision, after which the wrapped tool either runs or reports a rejection.
type approvalTool struct {
	wrapped   tool.InvokableTool
	summarize func(toolName, args string) (string, string)
}

func (t *approvalTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return t.wrapped.Info(ctx)
}

func (t *approvalTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	wasInterrupted, _, _ := tool.GetInterruptState[any](ctx)
	if !wasInterrupted {
		name := "tool"
		if info, err := t.wrapped.Info(ctx); err == nil && info != nil {
			name = info.Name
		}
		summary, hash := "", ""
		if t.summarize != nil {
			summary, hash = t.summarize(name, args)
		}
		return "", tool.Interrupt(ctx, approvalInterruptInfo{Tool: name, Summary: summary, ArgsHash: hash})
	}
	isTarget, _, decision := tool.GetResumeContext[approvalDecision](ctx)
	if !isTarget {
		// A different interrupt point is being resumed; preserve our state.
		return "", tool.Interrupt(ctx, nil)
	}
	if !decision.Approved {
		return "The user rejected this tool call.", nil
	}
	return t.wrapped.InvokableRun(ctx, args, opts...)
}

// summarizeArgs builds the redacted approval summary from the catalog. Raw
// argument values never leave the checkpoint; a tool without a custom
// summarizer falls back to listing only argument keys.
func (r *Engine) summarizeArgs(toolName, args string) (summary, hash string) {
	sum := sha256.Sum256([]byte(args))
	hash = hex.EncodeToString(sum[:])
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(args), &fields); err != nil {
		return fmt.Sprintf("call %s (arguments unavailable)", toolName), hash
	}
	if r.toolCatalog != nil {
		if def, ok := r.toolCatalog.Definition(toolName); ok && def.Summarize != nil {
			return def.Summarize(fields), hash
		}
	}
	return catalog.GenericSummary(toolName, fields), hash
}

// runState collects cross-phase outputs of one execution.
type runState struct {
	summary    string
	usage      port.Usage
	modelError atomic.Bool
}

// buildAgent assembles the ChatModelAgent for a run or resume. withSummary
// enables the summarization middleware (fresh runs only; a resumed execution
// restores its compacted state from the checkpoint).
func (r *Engine) buildAgent(ctx context.Context, in port.Input, withSummary bool) (adk.Agent, *runState, error) {
	chatModel, err := r.chatModel(ctx, in)
	if err != nil {
		return nil, nil, err
	}
	toolCtx := tools.WithRunContext(ctx, tools.RunContext{ActorUserID: in.ActorUserID, AgentID: in.AgentID, BotID: in.BotID, RunID: in.RunID, AttemptID: in.AttemptID, EventID: in.EventID, Topic: in.Topic})
	state := &runState{}
	var handlers []adk.ChatModelAgentMiddleware
	// Patch dangling tool calls left in a checkpoint by a crashed run before
	// the model sees the history. On normal fresh runs this is a no-op.
	if patch, err := patchtoolcalls.New(ctx, nil); err == nil {
		handlers = append(handlers, patch)
	} else {
		return nil, nil, fmt.Errorf("create patch tool calls middleware: %w", err)
	}
	if withSummary && in.NeedsSummary && !in.DisableSummary && len(in.History) > 0 {
		trigger := in.TriggerTokens
		if trigger <= 0 {
			trigger = 38000
		}
		mw, err := summarization.New(toolCtx, &summarization.Config{
			Model:                summaryTracker{model: chatModel},
			PreserveUserMessages: &summarization.PreserveUserMessages{Enabled: false},
			Trigger:              &summarization.TriggerCondition{ContextTokens: trigger},
			TokenCounter: func(_ context.Context, input *summarization.TokenCounterInput) (int, error) {
				if state.summary != "" {
					return 0, nil
				}
				// Count the actually assembled messages (including the system
				// instruction) so the middleware triggers on real content size,
				// matching the builder's coarse budget estimate.
				total := 0
				for _, msg := range input.Messages {
					total += agentcontext.EstimateTokens(messageText(msg))
				}
				return total, nil
			},
			GenModelInput: func(_ context.Context, system, user adk.Message, original []adk.Message) ([]adk.Message, error) {
				// The final message is the current input and must not enter the snapshot.
				request := append([]adk.Message{system}, original[:len(original)-1]...)
				return append(request, user), nil
			},
			Finalize: func(_ context.Context, original []adk.Message, compacted adk.Message) ([]adk.Message, error) {
				result := make([]adk.Message, 0, 3)
				for _, msg := range original {
					if msg.Role == schema.System {
						result = append(result, msg)
					}
				}
				return append(result, compacted, original[len(original)-1]), nil
			},
			Callback: func(_ context.Context, _, after adk.ChatModelAgentState) error {
				for _, msg := range after.Messages {
					if len(msg.UserInputMultiContent) == 0 {
						continue
					}
					state.summary = msg.UserInputMultiContent[0].Text
					break
				}
				return nil
			},
		})
		if err != nil {
			return nil, nil, fmt.Errorf("create summarization middleware: %w", err)
		}
		handlers = append(handlers, mw)
	}
	instruction := in.Instruction
	if instruction == "" {
		instruction = "You are the Nonoka IM assistant. Use concise, helpful answers. Use tools only when needed. Never claim a tool action succeeded unless it returned success."
	}
	if len(in.Skills) > 0 && r.skillBackend != nil {
		backend, err := r.skillBackend(in.Skills)
		if err != nil {
			return nil, nil, fmt.Errorf("load skills: %w", err)
		}
		mw, err := skill.NewMiddleware(toolCtx, &skill.Config{Backend: backend})
		if err != nil {
			return nil, nil, fmt.Errorf("create skill middleware: %w", err)
		}
		handlers = append(handlers, mw)
	}
	agentTools, err := r.resolveTools(toolCtx, in)
	if err != nil {
		return nil, nil, err
	}
	agent, err := adk.NewChatModelAgent(toolCtx, &adk.ChatModelAgentConfig{
		Name:             "nonoka-assistant",
		Description:      "Nonoka IM conversation assistant",
		Instruction:      instruction,
		Model:            chatModel,
		Handlers:         handlers,
		MaxIterations:    5,
		ModelRetryConfig: &adk.ModelRetryConfig{MaxRetries: modelRetryMax, IsRetryAble: retryableModelError},
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools:               agentTools,
				ToolCallMiddlewares: []compose.ToolMiddleware{tools.TimeoutMiddleware(tools.ToolCallTimeout)},
			},
		},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("create Eino agent: %w", err)
	}
	return agent, state, nil
}

// resolveTools builds the tool list for a run: authorized local tools, the
// run's MCP bindings, and approval wrappers around every tool that requires
// human confirmation.
func (r *Engine) resolveTools(toolCtx context.Context, in port.Input) ([]tool.BaseTool, error) {
	agentTools := allowedTools(toolCtx, r.tools, in.AllowedTools, in.RestrictTools)
	if len(in.MCPTools) > 0 {
		if r.mcpTools == nil {
			return nil, fmt.Errorf("mcp tools not available")
		}
		extra, err := r.mcpTools(toolCtx, in.MCPTools)
		if err != nil {
			return nil, fmt.Errorf("resolve mcp tools: %w", err)
		}
		agentTools = append(agentTools, extra...)
	}
	// Wrap side-effecting tools before the approval wrapper so a resumed
	// approval executes through the durable ledger. The catalog decides which
	// built-ins are writes; approval-gated MCP tools are writes by policy.
	sideEffects := map[string]bool{}
	if r.toolCatalog != nil {
		for _, def := range r.toolCatalog.Definitions() {
			if def.SideEffect {
				sideEffects[def.Name] = true
			}
		}
	}
	for _, name := range in.ApprovalTools {
		sideEffects[name] = true
	}
	agentTools = r.withLedger(toolCtx, agentTools, sideEffects)
	if len(in.ApprovalTools) == 0 {
		return agentTools, nil
	}
	approval := make(map[string]bool, len(in.ApprovalTools))
	for _, name := range in.ApprovalTools {
		approval[name] = true
	}
	wrapped := make([]tool.BaseTool, 0, len(agentTools))
	for _, t := range agentTools {
		info, err := t.Info(toolCtx)
		if err == nil && info != nil && approval[info.Name] {
			if inv, ok := t.(tool.InvokableTool); ok {
				wrapped = append(wrapped, &approvalTool{wrapped: inv, summarize: r.summarizeArgs})
				continue
			}
		}
		wrapped = append(wrapped, t)
	}
	return wrapped, nil
}

// withLedger wraps the named tools with the durable side-effect ledger. The
// tool name is read from Info so renamed MCP tools are covered too.
func (r *Engine) withLedger(ctx context.Context, ts []tool.BaseTool, names map[string]bool) []tool.BaseTool {
	if r.ledger == nil || len(names) == 0 {
		return ts
	}
	out := make([]tool.BaseTool, 0, len(ts))
	for _, t := range ts {
		inv, ok := t.(tool.InvokableTool)
		if !ok {
			out = append(out, t)
			continue
		}
		info, err := t.Info(ctx)
		if err == nil && info != nil && names[info.Name] {
			out = append(out, tools.NewLedgerTool(inv, r.ledger, r.markSideEffect))
			continue
		}
		out = append(out, t)
	}
	return out
}

// runLoop drains the runner event stream. An approval interrupt returns with
// StatusInterrupted and the root-cause interrupt details; model and tool

type summaryTracker struct {
	model model.BaseChatModel
}

func (t summaryTracker) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	msg, err := t.model.Generate(ctx, input, opts...)
	if err != nil {
		return msg, fmt.Errorf("%w: %v", port.ErrSummary, err)
	}
	return msg, nil
}

func (t summaryTracker) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	reader, err := t.model.Stream(ctx, input, opts...)
	if err != nil {
		return reader, fmt.Errorf("%w: %v", port.ErrSummary, err)
	}
	return reader, nil
}

func allowedTools(ctx context.Context, ts []tool.BaseTool, names []string, restrict bool) []tool.BaseTool {
	if !restrict {
		return ts
	}
	result := make([]tool.BaseTool, 0, len(ts))
	for _, t := range ts {
		info, err := t.Info(ctx)
		if err == nil && info != nil && slices.Contains(names, info.Name) {
			result = append(result, t)
		}
	}
	return result
}

// retryableModelError classifies provider failures for Eino's in-process
// ChatModel retry. Auth/validation/not-found errors fail fast; 429, 408, 5xx
