package app

import (
	"context"
	"errors"
	"testing"

	"nonoka-im/internal/im/domain"
)

type groupRepoStub struct {
	GroupRepository
	group   *domain.Group
	deleted string
}

func (s *groupRepoStub) CreateGroup(context.Context, string, int64, []int64) (*domain.Group, error) {
	if s.group == nil {
		s.group = &domain.Group{GroupID: "g1", Topic: "grp_g1", Name: "team", OwnerID: 7}
	}
	return s.group, nil
}

func (s *groupRepoStub) DeleteGroup(_ context.Context, groupID string) error {
	s.deleted = groupID
	return nil
}

type batchAuthzStub struct {
	GroupAuthorizer
	err     error
	called  bool
	members []int64
	groupID string
	topic   string
	ownerID int64
}

func (s *batchAuthzStub) SetGroupAccess(_ context.Context, groupID, topic string, ownerID int64, memberIDs []int64) error {
	s.called = true
	s.groupID, s.topic, s.ownerID, s.members = groupID, topic, ownerID, append([]int64(nil), memberIDs...)
	return s.err
}

func TestGroupCreateUsesBatchAuthorization(t *testing.T) {
	repo := &groupRepoStub{}
	authz := &batchAuthzStub{}
	svc := &GroupService{Repo: repo, Authorizer: authz}
	group, err := svc.Create(context.Background(), 7, "team", []int64{8, 9})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if group == nil || !authz.called {
		t.Fatalf("batch authorization was not used: %+v", authz)
	}
	if authz.groupID != "g1" || authz.topic != "grp_g1" || authz.ownerID != 7 || len(authz.members) != 2 {
		t.Fatalf("unexpected batch write: %+v", authz)
	}
}

func TestGroupCreateRollsBackWhenAuthorizationFails(t *testing.T) {
	repo := &groupRepoStub{}
	authz := &batchAuthzStub{err: errors.New("openfga unavailable")}
	svc := &GroupService{Repo: repo, Authorizer: authz}
	if _, err := svc.Create(context.Background(), 7, "team", []int64{8}); err == nil {
		t.Fatal("expected create failure")
	}
	if repo.deleted != "g1" {
		t.Fatalf("group was not rolled back: deleted=%q", repo.deleted)
	}
}
