package app

import (
	"context"
	"testing"
)

type audienceStub struct{ members []int64 }

func (a audienceStub) GroupMemberIDs(context.Context, string) ([]int64, error) {
	return a.members, nil
}

func TestGroupRecipientsFanOutButKeepActorFirst(t *testing.T) {
	h := &Harness{Audience: audienceStub{members: []int64{1, 2, 3}}}
	recipients := h.groupRecipients(context.Background(), "grp_g1", 1)
	if len(recipients) != 3 || recipients[0] != 1 {
		t.Fatalf("unexpected group recipients: %v", recipients)
	}
	if p2p := h.groupRecipients(context.Background(), "p2p_1_2", 1); len(p2p) != 1 || p2p[0] != 1 {
		t.Fatalf("p2p should only target the actor: %v", p2p)
	}
}
