package service

import (
	"context"
	"errors"
	"testing"

	pb "nonoka-im/api/im/v1"
)

type fakePresenceChecker struct {
	online bool
	err    error
}

func (f fakePresenceChecker) IsUserOnline(context.Context, int64) (bool, error) {
	return f.online, f.err
}

func TestUserService_GetUserPresence(t *testing.T) {
	svc := NewUserService(nil)
	svc.SetPresenceChecker(fakePresenceChecker{online: true})

	reply, err := svc.GetUserPresence(context.Background(), &pb.GetUserPresenceRequest{UserId: 42})
	if err != nil {
		t.Fatal(err)
	}
	if reply.GetUserId() != 42 || !reply.GetOnline() {
		t.Fatalf("unexpected presence reply: %+v", reply)
	}
}

func TestUserService_GetUserPresenceValidationAndFallback(t *testing.T) {
	svc := NewUserService(nil)
	if _, err := svc.GetUserPresence(context.Background(), &pb.GetUserPresenceRequest{}); err == nil {
		t.Fatal("expected invalid user id error")
	}

	reply, err := svc.GetUserPresence(context.Background(), &pb.GetUserPresenceRequest{UserId: 7})
	if err != nil || reply.GetOnline() {
		t.Fatalf("expected offline fallback without checker, reply=%+v err=%v", reply, err)
	}

	wantErr := errors.New("redis unavailable")
	svc.SetPresenceChecker(fakePresenceChecker{err: wantErr})
	if _, err := svc.GetUserPresence(context.Background(), &pb.GetUserPresenceRequest{UserId: 7}); !errors.Is(err, wantErr) {
		t.Fatalf("expected checker error, got %v", err)
	}
}
