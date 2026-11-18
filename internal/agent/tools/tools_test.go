package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type backendStub struct {
	actor  int64
	topic  string
	group  string
	filter SearchFilter
}

func (b *backendStub) SearchMessages(_ context.Context, rc RunContext, _ string, _ int, filter SearchFilter) ([]string, error) {
	b.actor, b.topic, b.filter = rc.ActorUserID, rc.Topic, filter
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
func (b *backendStub) ReadFile(_ context.Context, rc RunContext, _ string, _ int) (string, string, []byte, bool, error) {
	b.actor, b.topic = rc.ActorUserID, rc.Topic
	return "notes.txt", "text/plain", []byte("file body"), false, nil
}
func (b *backendStub) TopicInfo(_ context.Context, rc RunContext) (string, error) {
	b.actor, b.topic = rc.ActorUserID, rc.Topic
	return "type=group", nil
}
func (b *backendStub) MyInfo(_ context.Context, rc RunContext) (string, error) {
	b.actor, b.topic = rc.ActorUserID, rc.Topic
	return "user id=42", nil
}
func (b *backendStub) MyGroups(_ context.Context, rc RunContext) (string, error) {
	b.actor, b.topic = rc.ActorUserID, rc.Topic
	return "groups(1): name=\"g\" id=g1 members=2", nil
}
func (b *backendStub) GroupMembers(_ context.Context, rc RunContext, groupID string) (string, error) {
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
	tools := New(b, &guardStub{})
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

func TestReadFileBoundsAndForwardsIdentity(t *testing.T) {
	b := &backendStub{}
	ts := New(b, &guardStub{})
	// Order matches New: search, conversation, send, read_file, topic_info.
	readFile, topicInfo := ts[3], ts[4]

	args, _ := json.Marshal(ReadFileRequest{FileID: "abc-123"})
	ctx := WithRunContext(context.Background(), RunContext{ActorUserID: 7, Topic: "grp_1"})
	out, err := readFile.InvokableRun(ctx, string(args))
	if err != nil {
		t.Fatal(err)
	}
	var result ReadFileResult
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
	ctx := WithRunContext(context.Background(), RunContext{ActorUserID: 7, Topic: "p2p_1_7", AgentID: 3})
	for _, guard := range []Guard{&guardStub{denied: true}, nil} {
		ts := New(&backendStub{}, guard)
		if len(ts) != 8 {
			t.Fatalf("expected 8 tools, got %d", len(ts))
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
	ctx := WithRunContext(context.Background(), RunContext{ActorUserID: 7, Topic: "p2p_1_7"})
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
	ctx := WithRunContext(context.Background(), RunContext{ActorUserID: 7, Topic: "grp_g1"})

	args, _ := json.Marshal(SearchRequest{Query: "hello", Limit: 5, Since: 1697000000, SenderID: 42})
	if _, err := search.InvokableRun(ctx, string(args)); err != nil {
		t.Fatal(err)
	}
	if b.filter != (SearchFilter{Since: 1697000000, SenderID: 42}) {
		t.Fatalf("filters not forwarded: %+v", b.filter)
	}

	// Omitting the optional arguments must preserve the old behavior.
	args, _ = json.Marshal(SearchRequest{Query: "hello"})
	if _, err := search.InvokableRun(ctx, string(args)); err != nil {
		t.Fatal(err)
	}
	if b.filter != (SearchFilter{}) {
		t.Fatalf("empty request produced filters: %+v", b.filter)
	}
}

type deadlineProbeBackend struct {
	backendStub
	hasDeadline bool
	deadline    time.Time
}

func (b *deadlineProbeBackend) SendMessage(ctx context.Context, rc RunContext, _ string) error {
	b.deadline, b.hasDeadline = ctx.Deadline()
	return nil
}

// blockingBackend hangs until the tool context is canceled.
type blockingBackend struct{ backendStub }

func (b *blockingBackend) SendMessage(ctx context.Context, rc RunContext, _ string) error {
	<-ctx.Done()
	return ctx.Err()
}

// TestToolCallAppliesIndependentTimeout verifies every tool call gets its own
// 20s deadline even when the run context carries none.
func TestToolCallAppliesIndependentTimeout(t *testing.T) {
	b := &deadlineProbeBackend{}
	ts := New(b, &guardStub{})
	// Order matches New: search, conversation, send, read_file, topic_info.
	send := ts[2]
	ctx := WithRunContext(context.Background(), RunContext{ActorUserID: 1, Topic: "p2p_1_2"})
	args, _ := json.Marshal(SendRequest{Content: "hi"})
	if _, err := send.InvokableRun(ctx, string(args)); err != nil {
		t.Fatal(err)
	}
	if !b.hasDeadline {
		t.Fatal("tool call ran without a deadline")
	}
	if until := time.Until(b.deadline); until > toolCallTimeout || until < toolCallTimeout-time.Second {
		t.Fatalf("tool deadline is %v away, want ~%v", until, toolCallTimeout)
	}
}

// TestToolCallKeepsEarlierRunDeadline verifies the tool timeout composes with
// the run context: an earlier run-level deadline wins over the 20s cap, so a
// stuck backend call is interrupted by the run deadline, not the tool's.
func TestToolCallKeepsEarlierRunDeadline(t *testing.T) {
	b := &blockingBackend{}
	ts := New(b, &guardStub{})
	send := ts[2]
	ctx := WithRunContext(context.Background(), RunContext{ActorUserID: 1, Topic: "p2p_1_2"})
	ctx, cancel := context.WithTimeout(ctx, 60*time.Millisecond)
	defer cancel()
	args, _ := json.Marshal(SendRequest{Content: "hi"})
	start := time.Now()
	if _, err := send.InvokableRun(ctx, string(args)); err == nil {
		t.Fatal("expected the stuck backend call to be interrupted")
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("earlier run deadline was not honored, tool blocked for %v", elapsed)
	}
}
