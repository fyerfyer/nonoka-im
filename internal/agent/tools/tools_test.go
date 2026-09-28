package tools

import (
	"context"
	"encoding/json"
	"testing"
)

type backendStub struct {
	actor int64
	topic string
}

func (b *backendStub) SearchMessages(_ context.Context, rc RunContext, _ string, _ int) ([]string, error) {
	b.actor, b.topic = rc.ActorUserID, rc.Topic
	return []string{"ok"}, nil
}
func (b *backendStub) ConversationContext(_ context.Context, rc RunContext, _ int) ([]string, error) {
	b.actor, b.topic = rc.ActorUserID, rc.Topic
	return []string{"context"}, nil
}
func (b *backendStub) SendMessage(_ context.Context, rc RunContext, _ string) error {
	b.actor, b.topic = rc.ActorUserID, rc.Topic
	return nil
}

func TestSearchMessagesRequiresTrustedRunContext(t *testing.T) {
	b := &backendStub{}
	tools := New(b)
	args, _ := json.Marshal(SearchRequest{Query: "keyword"})
	if _, err := tools[0].InvokableRun(context.Background(), string(args)); err == nil {
		t.Fatal("expected missing trusted context to fail")
	}
	ctx := WithRunContext(context.Background(), RunContext{ActorUserID: 42, Topic: "p2p_1_42"})
	if _, err := tools[0].InvokableRun(ctx, string(args)); err != nil {
		t.Fatal(err)
	}
	if b.actor != 42 || b.topic != "p2p_1_42" {
		t.Fatalf("backend received untrusted identity: %+v", b)
	}
}
