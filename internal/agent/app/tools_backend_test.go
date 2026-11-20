package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"nonoka-im/internal/agent/tools"
)

type idRecordingSender struct{ ids []string }

func (r *idRecordingSender) Send(_ context.Context, _, _ string, clientID string) error {
	r.ids = append(r.ids, clientID)
	return nil
}

func (r *idRecordingSender) SendWithReceipt(ctx context.Context, topic, content, clientID string) (int64, error) {
	return 0, r.Send(ctx, topic, content, clientID)
}

func (r *idRecordingSender) SendCard(context.Context, string, string, string) (int64, error) {
	return 0, nil
}

// TestSendMessageDistinctIDsForIdenticalContent verifies that the model
// posting the same text twice in one run produces two different client_msg_id
// values, so the IM dedup layer (network-retry only) cannot drop one.
func TestSendMessageDistinctIDsForIdenticalContent(t *testing.T) {
	rec := &idRecordingSender{}
	b := &MessageTools{Authorizer: &authorizerStub{}, Sender: rec}
	rc := tools.RunContext{EventID: "evt-dup", Topic: "p2p_1_2", ActorUserID: 1, BotID: 2}
	ctx := context.Background()

	rc.ToolCallID = "call-1"
	if err := b.SendMessage(ctx, rc, "same content"); err != nil {
		t.Fatal(err)
	}
	rc.ToolCallID = "call-2"
	if err := b.SendMessage(ctx, rc, "same content"); err != nil {
		t.Fatal(err)
	}
	rc.ToolCallID = "call-3"
	if err := b.SendMessage(ctx, rc, "different content"); err != nil {
		t.Fatal(err)
	}
	if len(rec.ids) != 3 {
		t.Fatalf("expected 3 recorded client_msg_ids, got %v", rec.ids)
	}
	if rec.ids[0] == rec.ids[1] {
		t.Fatalf("identical content produced identical client_msg_id %q", rec.ids[0])
	}
	if rec.ids[2] == rec.ids[0] {
		t.Fatalf("content change did not affect client_msg_id: %q", rec.ids[2])
	}
}

// groupToolDirectory is a Directory fake for the get_my_info / list_my_groups /
// get_group_members backend tests.
type groupToolDirectory struct {
	profileID    int64
	profiles     map[int64]UserProfile
	groups       []GroupRef
	groupNames   map[string]string
	memberIDs    map[string][]int64
	usernames    map[int64]string
	memberProbes int
}

func (d *groupToolDirectory) Username(_ context.Context, id int64) (string, error) {
	if name, ok := d.usernames[id]; ok {
		return name, nil
	}
	return fmt.Sprintf("user-%d", id), nil
}
func (d *groupToolDirectory) GroupName(_ context.Context, groupID string) (string, error) {
	name, ok := d.groupNames[groupID]
	if !ok {
		return "", errors.New("group not found")
	}
	return name, nil
}
func (d *groupToolDirectory) GroupMemberIDs(_ context.Context, groupID string) ([]int64, error) {
	d.memberProbes++
	ids, ok := d.memberIDs[groupID]
	if !ok {
		return nil, errors.New("group not found")
	}
	return ids, nil
}
func (d *groupToolDirectory) UserProfile(_ context.Context, id int64) (UserProfile, error) {
	d.profileID = id
	p, ok := d.profiles[id]
	if !ok {
		return UserProfile{}, errors.New("user not found")
	}
	return p, nil
}
func (d *groupToolDirectory) MyGroups(_ context.Context, userID int64) ([]GroupRef, error) {
	d.profileID = userID
	return d.groups, nil
}

// groupToolAuthorizer denies topic access per (user, topic) pair so tests can
// distinguish members from non-members.
type groupToolAuthorizer struct {
	deny func(uid int64, topic string) bool
}

func (a groupToolAuthorizer) CanAccessTopic(_ context.Context, uid int64, t string) error {
	if a.deny != nil && a.deny(uid, t) {
		return errors.New("denied")
	}
	return nil
}
func (groupToolAuthorizer) CanReadFile(context.Context, int64, string) error { return nil }
func (groupToolAuthorizer) CanExecuteTool(context.Context, int64, int64, string, string, string, string) error {
	return nil
}

func TestMyInfoOnlyLooksUpActor(t *testing.T) {
	dir := &groupToolDirectory{profiles: map[int64]UserProfile{
		7: {ID: 7, Username: "alice", CreatedAt: 1697000000},
		8: {ID: 8, Username: "mallory", CreatedAt: 1697000001},
	}}
	b := &MessageTools{Directory: dir}
	out, err := b.MyInfo(context.Background(), tools.RunContext{ActorUserID: 7, Topic: "p2p_7_8"})
	if err != nil {
		t.Fatal(err)
	}
	if dir.profileID != 7 {
		t.Fatalf("directory queried user %d, want actor 7", dir.profileID)
	}
	if out != `user id=7 name="alice" created_at=1697000000` {
		t.Fatalf("unexpected my_info result %q", out)
	}
}

func TestMyGroupsCapsAtFifty(t *testing.T) {
	many := make([]GroupRef, 0, 60)
	for i := 0; i < 60; i++ {
		many = append(many, GroupRef{ID: fmt.Sprintf("g%d", i), Name: fmt.Sprintf("group-%d", i), MemberCount: i})
	}
	b := &MessageTools{Directory: &groupToolDirectory{groups: many}}
	out, err := b.MyGroups(context.Background(), tools.RunContext{ActorUserID: 7})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "groups(50):") || !strings.HasSuffix(out, ", ...") {
		t.Fatalf("expected capped group list with truncation marker, got %q", out)
	}

	few := []GroupRef{{ID: "g1", Name: "core", MemberCount: 3}, {ID: "g2", Name: "ops", MemberCount: 5}}
	b = &MessageTools{Directory: &groupToolDirectory{groups: few}}
	out, err = b.MyGroups(context.Background(), tools.RunContext{ActorUserID: 7})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `name="core" id=g1 members=3`) || !strings.Contains(out, `name="ops" id=g2 members=5`) || strings.HasSuffix(out, ", ...") {
		t.Fatalf("unexpected short group list %q", out)
	}
}

func TestGroupMembersDeniesNonMemberWithAmbiguousError(t *testing.T) {
	dir := &groupToolDirectory{
		groupNames: map[string]string{"g1": "core"},
		memberIDs:  map[string][]int64{"g1": {1, 2, 7}},
		usernames:  map[int64]string{1: "alice", 2: "bob", 7: "carol"},
	}
	// Only members of grp_g1 pass; every other (user, topic) is denied, which
	// covers both "group does not exist" and "actor is not a member".
	az := groupToolAuthorizer{deny: func(uid int64, t string) bool {
		return !(uid == 7 && t == "grp_g1")
	}}
	b := &MessageTools{Authorizer: az, Directory: dir}
	ctx := context.Background()

	_, nonMemberErr := b.GroupMembers(ctx, tools.RunContext{ActorUserID: 7}, "g2")
	_, missingErr := b.GroupMembers(ctx, tools.RunContext{ActorUserID: 7}, "does-not-exist")
	if nonMemberErr == nil || missingErr == nil {
		t.Fatal("expected denials for non-member and missing group")
	}
	if nonMemberErr.Error() != missingErr.Error() {
		t.Fatalf("errors differ, leaking existence: %q vs %q", nonMemberErr, missingErr)
	}
	if dir.memberProbes != 0 {
		t.Fatalf("directory was probed %d times despite denial", dir.memberProbes)
	}

	out, err := b.GroupMembers(ctx, tools.RunContext{ActorUserID: 7}, "g1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `group name="core" id=g1 members(3)`) ||
		!strings.Contains(out, "alice(id=1)") || !strings.Contains(out, "carol(id=7)") {
		t.Fatalf("unexpected member list %q", out)
	}
}

func TestGroupMembersCapsAtTwoHundred(t *testing.T) {
	ids := make([]int64, 0, 250)
	for i := 1; i <= 250; i++ {
		ids = append(ids, int64(i))
	}
	dir := &groupToolDirectory{
		groupNames: map[string]string{"big": "big-group"},
		memberIDs:  map[string][]int64{"big": ids},
	}
	b := &MessageTools{Authorizer: groupToolAuthorizer{}, Directory: dir}
	out, err := b.GroupMembers(context.Background(), tools.RunContext{ActorUserID: 1}, "big")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "members(200):") || !strings.HasSuffix(out, ", ...") {
		t.Fatalf("expected member list capped at 200 with marker, got %q", out)
	}
}

func TestSearchInboxRowsFiltersSenderAndTime(t *testing.T) {
	rows := []Message{
		{SenderID: 1, Timestamp: 100, Content: []byte("hello world")},
		{SenderID: 2, Timestamp: 200, Content: []byte("hello again")},
		{SenderID: 1, Timestamp: 300, Content: []byte("goodbye")},
		{SenderID: 1, Timestamp: 400, Content: []byte("HELLO there")},
	}
	base := searchInboxRows(rows, "hello", 10, tools.SearchFilter{})
	if len(base) != 3 || !strings.Contains(base[0], "time=400") {
		t.Fatalf("baseline search changed: %v", base)
	}
	bySender := searchInboxRows(rows, "hello", 10, tools.SearchFilter{SenderID: 2})
	if len(bySender) != 1 || !strings.Contains(bySender[0], "sender=2") {
		t.Fatalf("sender filter not applied: %v", bySender)
	}
	byTime := searchInboxRows(rows, "hello", 10, tools.SearchFilter{Since: 200})
	if len(byTime) != 2 || !strings.Contains(byTime[0], "time=400") || !strings.Contains(byTime[1], "time=200") {
		t.Fatalf("since filter not applied: %v", byTime)
	}
	both := searchInboxRows(rows, "hello", 10, tools.SearchFilter{SenderID: 1, Since: 150})
	if len(both) != 1 || !strings.Contains(both[0], "time=400") {
		t.Fatalf("combined filters not applied: %v", both)
	}
}
