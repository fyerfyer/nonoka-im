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

func (s *groupRepoStub) GetGroup(context.Context, string) (*domain.Group, error) {
	if s.group == nil {
		return nil, errors.New("not found")
	}
	return s.group, nil
}

func (s *groupRepoStub) AddMember(context.Context, string, int64) error    { return nil }
func (s *groupRepoStub) RemoveMember(context.Context, string, int64) error { return nil }

type invalidatorStub struct {
	called  bool
	groupID string
}

func (s *invalidatorStub) InvalidateGroupMembers(_ context.Context, groupID string) error {
	s.called = true
	s.groupID = groupID
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

func (s *batchAuthzStub) CanManageGroup(context.Context, int64, string) error {
	return s.err
}

func (s *batchAuthzStub) SetGroupMember(context.Context, string, int64, bool) error {
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

func TestGroupAddMemberRequiresManagerAndInvalidatesCache(t *testing.T) {
	repo := &groupRepoStub{group: &domain.Group{GroupID: "g1", Topic: "grp_g1", Name: "team", OwnerID: 7}}
	denied := &GroupService{Repo: repo, Authorizer: &batchAuthzStub{err: errors.New("denied")}}
	if err := denied.AddMember(context.Background(), 9, "g1", 8); !errors.Is(err, ErrGroupForbidden) {
		t.Fatalf("non-owner add = %v, want ErrGroupForbidden", err)
	}

	invalidator := &invalidatorStub{}
	svc := &GroupService{Repo: repo, Authorizer: &batchAuthzStub{}, Invalidator: invalidator}
	if err := svc.AddMember(context.Background(), 7, "g1", 8); err != nil {
		t.Fatalf("owner add: %v", err)
	}
	if !invalidator.called || invalidator.groupID != "g1" {
		t.Fatalf("cache invalidation not called: %+v", invalidator)
	}
}

func TestGroupRemoveMemberInvalidatesCache(t *testing.T) {
	repo := &groupRepoStub{group: &domain.Group{GroupID: "g1", Topic: "grp_g1", Name: "team", OwnerID: 7}}
	invalidator := &invalidatorStub{}
	svc := &GroupService{Repo: repo, Authorizer: &batchAuthzStub{}, Invalidator: invalidator}
	if err := svc.RemoveMember(context.Background(), 7, "g1", 8); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if !invalidator.called || invalidator.groupID != "g1" {
		t.Fatalf("cache invalidation not called: %+v", invalidator)
	}
}
