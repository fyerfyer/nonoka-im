package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"nonoka-im/internal/im/domain"
)

type testAuthorizer struct{ deny bool }

func (a testAuthorizer) CanAccessTopic(context.Context, int64, string) error {
	if a.deny {
		return errors.New("denied")
	}
	return nil
}
func (a testAuthorizer) CanReadFile(context.Context, int64, string) error { return nil }
func (a testAuthorizer) CanExecuteTool(context.Context, int64, int64, string, string, string, string) error {
	return nil
}

type testProducer struct{ sent *domain.Message }

func (p *testProducer) Produce(_ context.Context, msg *domain.Message) error {
	p.sent = msg
	return nil
}

func TestMessageServiceSendValidation(t *testing.T) {
	producer := &testProducer{}
	svc := &MessageService{Producer: producer, Authorizer: testAuthorizer{}}
	if _, err := svc.Send(context.Background(), SendRequest{Topic: "p2p_1_2"}); !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("expected auth error, got %v", err)
	}
	if _, err := svc.Send(context.Background(), SendRequest{ActorID: 1}); !errors.Is(err, ErrClientMsgIDReq) {
		t.Fatalf("expected client id error, got %v", err)
	}
	if _, err := svc.Send(context.Background(), SendRequest{ActorID: 1, Topic: "bad", ClientMsgID: "c"}); !errors.Is(err, ErrInvalidTopic) {
		t.Fatalf("expected invalid topic, got %v", err)
	}
	tooLarge := SendRequest{ActorID: 1, Topic: "p2p_1_2", ClientMsgID: "c", Content: []byte(strings.Repeat("x", maxContentSize+1))}
	if _, err := svc.Send(context.Background(), tooLarge); !errors.Is(err, ErrMessageTooLarge) {
		t.Fatalf("expected message too large, got %v", err)
	}
	result, err := svc.Send(context.Background(), SendRequest{ActorID: 1, Topic: "p2p_2_1", ClientMsgID: "c1", Content: []byte("hi")})
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}
	if result.ClientMsgID != "c1" || producer.sent == nil || producer.sent.Topic != "p2p_1_2" {
		t.Fatalf("unexpected send result=%+v sent=%+v", result, producer.sent)
	}
}

func TestMessageServiceSendDenied(t *testing.T) {
	svc := &MessageService{Producer: &testProducer{}, Authorizer: testAuthorizer{deny: true}}
	_, err := svc.Send(context.Background(), SendRequest{ActorID: 1, Topic: "grp_123", ClientMsgID: "c"})
	if !errors.Is(err, ErrTopicDenied) {
		t.Fatalf("expected topic denied, got %v", err)
	}
}

func TestMessageServiceP2PUsesDeterministicACL(t *testing.T) {
	producer := &testProducer{}
	svc := &MessageService{Producer: producer, Authorizer: testAuthorizer{deny: true}}
	if _, err := svc.Send(context.Background(), SendRequest{ActorID: 1, Topic: "p2p_1_2", ClientMsgID: "c"}); err != nil {
		t.Fatalf("P2P send should not consult the group authorizer: %v", err)
	}
	if producer.sent == nil {
		t.Fatal("message was not produced")
	}
	// A user outside the deterministic topic must still be rejected.
	if _, err := svc.Send(context.Background(), SendRequest{ActorID: 3, Topic: "p2p_1_2", ClientMsgID: "c2"}); !errors.Is(err, ErrTopicDenied) {
		t.Fatalf("expected outsider denial, got %v", err)
	}
}
