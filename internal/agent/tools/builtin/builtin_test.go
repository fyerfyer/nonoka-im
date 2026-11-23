package builtin

import (
	"nonoka-im/internal/agent/tools"

	"context"
	"encoding/json"
	"errors"
	"testing"
)

type backendStub struct {
	actor  int64
	topic  string
	group  string
	filter tools.SearchFilter
}

func (b *backendStub) SearchMessages(_ context.Context, rc tools.RunContext, _ string, _ int, filter tools.SearchFilter) ([]string, error) {
	b.actor, b.topic, b.filter = rc.ActorUserID, rc.Topic, filter
	return []string{"ok"}, nil
}
func (b *backendStub) ConversationContext(_ context.Context, rc tools.RunContext, _ int) ([]string, error) {
	b.actor, b.topic = rc.ActorUserID, rc.Topic
	return []string{"context"}, nil
}
func (b *backendStub) SendMessage(_ context.Context, rc tools.RunContext, _ string) error {
	b.actor, b.topic = rc.ActorUserID, rc.Topic
	return nil
}
func (b *backendStub) ReadFile(_ context.Context, rc tools.RunContext, _ string, _ int) (string, string, []byte, bool, error) {
	b.actor, b.topic = rc.ActorUserID, rc.Topic
	return "notes.txt", "text/plain", []byte("file body"), false, nil
}
func (b *backendStub) TopicInfo(_ context.Context, rc tools.RunContext) (string, error) {
	b.actor, b.topic = rc.ActorUserID, rc.Topic
	return "type=group", nil
}
func (b *backendStub) MyInfo(_ context.Context, rc tools.RunContext) (string, error) {
	b.actor, b.topic = rc.ActorUserID, rc.Topic
	return "user id=42", nil
}
func (b *backendStub) MyGroups(_ context.Context, rc tools.RunContext) (string, error) {
	b.actor, b.topic = rc.ActorUserID, rc.Topic
	return "groups(1): name=\"g\" id=g1 members=2", nil
}
func (b *backendStub) GroupMembers(_ context.Context, rc tools.RunContext, groupID string) (string, error) {
	b.actor, b.topic, b.group = rc.ActorUserID, rc.Topic, groupID
	return "group members", nil
}

type guardStub struct{ denied bool }

func (g *guardStub) CanExecuteTool(context.Context, int64, int64, string, string, string, string) error {
	if g.denied {
		return errors.New("denied")
	}
	return nil
}

func TestSearchMessagesRequiresTrustedRunContext(t *testing.T) {
	b := &backendStub{}
	ts := New(b, &guardStub{})
	args, _ := json.Marshal(tools.SearchRequest{Query: "keyword"})
	if _, err := ts[0].InvokableRun(context.Background(), string(args)); err == nil {
		t.Fatal("expected missing trusted context to fail")
	}
	ctx := tools.WithRunContext(context.Background(), tools.RunContext{ActorUserID: 42, Topic: "p2p_1_42"})
	if _, err := ts[0].InvokableRun(ctx, string(args)); err != nil {
		t.Fatal(err)
	}
	if b.actor != 42 || b.topic != "p2p_1_42" {
		t.Fatalf("backend received untrusted identity: %+v", b)
	}
}

func TestReadFileBoundsAndForwardsIdentity(t *testing.T) {
	b := &backendStub{}
	ts := New(b, &guardStub{})
	// Order matches New: search, conversation, send, read_file, topic_info.
	readFile, topicInfo := ts[3], ts[4]

	args, _ := json.Marshal(tools.ReadFileRequest{FileID: "abc-123"})
	ctx := tools.WithRunContext(context.Background(), tools.RunContext{ActorUserID: 7, Topic: "grp_1"})
	out, err := readFile.InvokableRun(ctx, string(args))
	if err != nil {
		t.Fatal(err)
	}
	var result tools.ReadFileResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if result.Content != "file body" || result.Name != "notes.txt" || result.Truncated {
		t.Fatalf("unexpected read_file result: %+v", result)
	}
	if b.actor != 7 || b.topic != "grp_1" {
		t.Fatalf("read_file backend received untrusted identity: %+v", b)
	}

	if _, err := readFile.InvokableRun(ctx, `{"file_id":""}`); err == nil {
		t.Fatal("empty file_id accepted")
	}
	if _, err := readFile.InvokableRun(context.Background(), string(args)); err == nil {
		t.Fatal("read_file without trusted context accepted")
	}

	out, err = topicInfo.InvokableRun(ctx, "{}")
	if err != nil {
		t.Fatal(err)
	}
	if out != "type=group" {
		t.Fatalf("unexpected topic_info result %q", out)
	}
}

// TestNewToolsFailClosedWhenGuardDenies verifies the three identity/group
// tools refuse to run when the guard denies the call or is missing entirely.
func TestNewToolsFailClosedWhenGuardDenies(t *testing.T) {
	ctx := tools.WithRunContext(context.Background(), tools.RunContext{ActorUserID: 7, Topic: "p2p_1_7", AgentID: 3})
	for _, guard := range []tools.Guard{&guardStub{denied: true}, nil} {
		ts := New(&backendStub{}, guard)
		if len(ts) != 9 {
			t.Fatalf("expected 9 tools, got %d", len(ts))
		}
		// Order matches New: ..., get_my_info, list_my_groups, get_group_members.
		for i, args := range map[int]string{5: `{}`, 6: `{}`, 7: `{"group_id":"g1"}`} {
			if _, err := ts[i].InvokableRun(ctx, args); err == nil {
				t.Fatalf("tool %d ran with guard %+v", i, guard)
			}
		}
	}
}

func TestGroupMembersToolValidatesAndForwardsGroupID(t *testing.T) {
	b := &backendStub{}
	ts := New(b, &guardStub{})
	groupMembers := ts[7]
	ctx := tools.WithRunContext(context.Background(), tools.RunContext{ActorUserID: 7, Topic: "p2p_1_7"})
	if _, err := groupMembers.InvokableRun(ctx, `{"group_id":""}`); err == nil {
		t.Fatal("empty group_id accepted")
	}
	if _, err := groupMembers.InvokableRun(context.Background(), `{"group_id":"g1"}`); err == nil {
		t.Fatal("get_group_members without trusted context accepted")
	}
	out, err := groupMembers.InvokableRun(ctx, `{"group_id":"g1"}`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "group members" || b.group != "g1" || b.actor != 7 {
		t.Fatalf("group_id not forwarded: out=%q stub=%+v", out, b)
	}
}

func TestSearchMessagesParsesOptionalFilters(t *testing.T) {
	b := &backendStub{}
	ts := New(b, &guardStub{})
	search := ts[0]
	ctx := tools.WithRunContext(context.Background(), tools.RunContext{ActorUserID: 7, Topic: "grp_g1"})

	args, _ := json.Marshal(tools.SearchRequest{Query: "hello", Limit: 5, Since: 1697000000, SenderID: 42})
	if _, err := search.InvokableRun(ctx, string(args)); err != nil {
		t.Fatal(err)
	}
	if b.filter != (tools.SearchFilter{Since: 1697000000, SenderID: 42}) {
		t.Fatalf("filters not forwarded: %+v", b.filter)
	}

	// Omitting the optional arguments must preserve the old behavior.
	args, _ = json.Marshal(tools.SearchRequest{Query: "hello"})
	if _, err := search.InvokableRun(ctx, string(args)); err != nil {
		t.Fatal(err)
	}
	if b.filter != (tools.SearchFilter{}) {
		t.Fatalf("empty request produced filters: %+v", b.filter)
	}
}
