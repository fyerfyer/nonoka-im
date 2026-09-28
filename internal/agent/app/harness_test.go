package app

import (
	"context"
	"errors"
	"testing"

	agevent "nonoka-im/internal/agent/event"
)

type authorizerStub struct {
	denied int64
	calls  []int64
}

func (a *authorizerStub) CanAccessTopic(_ context.Context, uid int64, _ string) error {
	a.calls = append(a.calls, uid)
	if uid == a.denied {
		return errors.New("denied")
	}
	return nil
}
func (*authorizerStub) CanReadFile(context.Context, int64, int64) error { return nil }

func TestHarnessTriggerAndBotAuthorization(t *testing.T) {
	t.Run("unmentioned group ignored", func(t *testing.T) {
		az := &authorizerStub{}
		h := &Harness{BotID: 99, Authorizer: az}
		err := h.Handle(context.Background(), agevent.MessageEvent{SenderID: 1, Topic: "grp_group"})
		if err != nil || len(az.calls) != 0 {
			t.Fatalf("err=%v auth calls=%v", err, az.calls)
		}
	})
	t.Run("bot message ignored", func(t *testing.T) {
		az := &authorizerStub{}
		h := &Harness{BotID: 99, Authorizer: az}
		err := h.Handle(context.Background(), agevent.MessageEvent{SenderID: 99, Topic: "p2p_1_99"})
		if err != nil || len(az.calls) != 0 {
			t.Fatalf("err=%v auth calls=%v", err, az.calls)
		}
	})
	t.Run("mentioned group requires actor and bot access", func(t *testing.T) {
		az := &authorizerStub{denied: 99}
		h := &Harness{BotID: 99, Authorizer: az}
		err := h.Handle(context.Background(), agevent.MessageEvent{SenderID: 1, Topic: "grp_group", MentionedAgentIDs: []int64{99}})
		if err == nil {
			t.Fatal("expected bot membership denial")
		}
		if len(az.calls) != 2 || az.calls[0] != 1 || az.calls[1] != 99 {
			t.Fatalf("unexpected auth calls: %v", az.calls)
		}
	})
}

func TestTruncateRunTextCountsRunes(t *testing.T) {
	got := truncateRunText("你好abc", 3)
	if got != "你好a" {
		t.Fatalf("got %q", got)
	}
}
