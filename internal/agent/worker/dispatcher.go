package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"nonoka-im/internal/agent/app"
	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/authz"
	"nonoka-im/internal/messaging"
	"nonoka-im/internal/topic"

	"github.com/go-kratos/kratos/v2/log"
)

// Dispatcher resolves a trigger event to active agents and records queued
// runs. It performs only short reads so the Kafka consumer can commit offsets
// before any model work starts.
type Dispatcher struct {
	Store      DispatcherStore
	Authorizer DispatcherAuthorizer
	Reader     app.MessageReader
	Quota      *app.QuotaService
	Notifier   *app.QuotaNotifier
	Log        *log.Helper
}

func (d *Dispatcher) Dispatch(ctx context.Context, ev messaging.MessageEvent) error {
	if ev.SenderID <= 0 || ev.Topic == "" {
		return fmt.Errorf("invalid agent event: sender and topic required")
	}
	if err := d.Authorizer.CanAccessTopic(ctx, ev.SenderID, ev.Topic); err != nil {
		if isDenied(err) {
			return nil
		}
		return fmt.Errorf("event actor topic authorization failed: %w", err)
	}
	// Verify the trigger message is visible to the actor before creating work.
	if _, err := d.Reader.MessageForUser(ctx, ev.SenderID, ev.Topic, ev.MsgID); err != nil {
		return fmt.Errorf("load trigger message: %w", err)
	}
	var botIDs []int64
	switch topic.ParseType(ev.Topic) {
	case topic.TypeP2P:
		x, y, err := topic.ExtractUserIDsFromP2PTopic(ev.Topic)
		if err != nil {
			return err
		}
		if x == ev.SenderID {
			botIDs = []int64{y}
		} else if y == ev.SenderID {
			botIDs = []int64{x}
		}
	case topic.TypeGroup:
		botIDs = ev.MentionedAgentIDs
	}
	agents, err := d.Store.ActiveAgentsByBot(ctx, botIDs)
	if err != nil {
		return err
	}
	for _, agent := range agents {
		if ev.SenderID == agent.BotUserID {
			continue
		}
		candidate := &domain.AgentRun{
			EventID: ev.EventID, AgentID: agent.ID, AgentVersion: agent.CurrentVersion,
			ActorUserID: ev.SenderID, Topic: ev.Topic, MsgID: ev.MsgID, TopicSeq: ev.TopicSeq,
			Status: domain.RunQueued, Kind: domain.RunKindIM, CreatedAt: time.Now(),
		}
		if d.Quota != nil {
			created, quotaErr, err := d.Quota.AdmitEnqueue(ctx, candidate)
			if err != nil {
				return fmt.Errorf("quota admit for agent %d: %w", agent.ID, err)
			}
			if quotaErr != nil {
				if quotaErr.Metric == "queued_runs" {
					if derr := d.Quota.Defer(ctx, candidate, quotaErr.RetryAt); derr != nil {
						return fmt.Errorf("defer run for agent %d: %w", agent.ID, derr)
					}
					d.notify(ctx, agent, candidate, ev, quotaErr.Message)
					continue
				}
				if rerr := d.Quota.Reject(ctx, candidate, "quota_exceeded", "quota", quotaErr, &quotaErr.RetryAt); rerr != nil {
					return fmt.Errorf("record quota rejection for agent %d: %w", agent.ID, rerr)
				}
				d.notify(ctx, agent, candidate, ev, quotaErr.Message)
				continue
			}
			if !created {
				continue
			}
			continue
		}
		if _, err := d.Store.EnqueueRun(ctx, candidate); err != nil {
			return fmt.Errorf("enqueue run for agent %d: %w", agent.ID, err)
		}
	}
	return nil
}

func (d *Dispatcher) notify(ctx context.Context, agent domain.Agent, run *domain.AgentRun, ev messaging.MessageEvent, message string) {
	if d.Notifier == nil {
		return
	}
	d.Notifier.Notify(ctx, agent.ID, agent.BotUserID, ev.SenderID, ev.Topic, ev.EventID, "quota_exceeded", message)
}

func isDenied(err error) bool {
	return errors.Is(err, authz.ErrDenied)
}
