package app

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	agentcontext "nonoka-im/internal/agent/context"
	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/agent/port"
	"nonoka-im/internal/messaging"
)

type loadSender struct{ next atomic.Int64 }

func (*loadSender) Send(context.Context, string, string, string) error { return nil }
func (s *loadSender) SendWithReceipt(context.Context, string, string, string) (int64, error) {
	return s.next.Add(1), nil
}
func (*loadSender) SendCard(context.Context, string, string, string) (int64, error) { return 1, nil }

// TestHarness_FakeRuntimeLoad runs claimed runs through the full harness with
// a stub model runtime. AGENT_LOAD_RUNS controls the run count (default 100;
// set 500 for the Phase 4 queue matrix).
func TestHarness_FakeRuntimeLoad(t *testing.T) {
	store := setupHarnessStore(t)
	total := 100
	if raw := os.Getenv("AGENT_LOAD_RUNS"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			total = parsed
		}
	}
	ctx := context.Background()
	harness := &Harness{
		Store:       store,
		Runtime:     stubRuntime{result: port.Result{Text: "ok"}},
		Authorizer:  &authorizerStub{},
		Sender:      &loadSender{},
		Builder:     agentcontext.Builder{},
		MaxAttempts: 1,
	}

	type loadRun struct {
		run      *domain.AgentRun
		session  *domain.AgentSession
		snapshot *domain.AgentContextSnapshot
		turns    []domain.AgentTurn
		agent    ResolvedAgent
		ev       messaging.MessageEvent
	}
	runs := make([]loadRun, 0, total)
	for i := 0; i < total; i++ {
		agentID := int64(100000 + i)
		botID := int64(200000 + i)
		actorID := int64(300000 + i)
		topic := fmt.Sprintf("p2p_%d_%d", actorID, botID)
		run, session, snapshot, turns := claimRun(t, store, &domain.AgentRun{
			EventID: fmt.Sprintf("evt-fake-load-%d-%d", time.Now().UnixNano(), i),
			AgentID: agentID, AgentVersion: 1, ActorUserID: actorID,
			Topic: topic, MsgID: int64(i + 1), TopicSeq: 1, CreatedAt: time.Now(),
		})
		runs = append(runs, loadRun{
			run: run, session: session, snapshot: snapshot, turns: turns,
			agent: ResolvedAgent{
				ID: agentID, BotID: botID, Version: 1,
				Instruction: "be helpful",
				Model:       port.ModelEndpoint{Model: "fake"},
			},
			ev: messaging.MessageEvent{
				SchemaVersion: messaging.SchemaVersion,
				EventID:       run.EventID, MsgID: run.MsgID, TopicSeq: run.TopicSeq,
				Topic: topic, SenderID: actorID, Content: "hello",
			},
		})
	}

	sem := make(chan struct{}, 16)
	var wg sync.WaitGroup
	var failures atomic.Int64
	for _, item := range runs {
		wg.Add(1)
		go func(item loadRun) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := harness.HandleResolved(ctx, item.ev, item.agent, item.run, item.session, item.snapshot, item.turns); err != nil {
				t.Errorf("handle run %d: %v", item.run.ID, err)
				failures.Add(1)
			}
		}(item)
	}
	wg.Wait()
	if failures.Load() != 0 {
		t.Fatalf("%d fake-runtime runs failed", failures.Load())
	}
}
