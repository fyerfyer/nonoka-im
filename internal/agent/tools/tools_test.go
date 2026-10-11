package tools

import (
	"context"
	"encoding/json"
	"errors"
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
func (b *backendStub) ReadFile(_ context.Context, rc RunContext, _ string, _ int) (string, string, []byte, bool, error) {
	b.actor, b.topic = rc.ActorUserID, rc.Topic
	return "notes.txt", "text/plain", []byte("file body"), false, nil
}
func (b *backendStub) TopicInfo(_ context.Context, rc RunContext) (string, error) {
	b.actor, b.topic = rc.ActorUserID, rc.Topic
	return "type=group", nil
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
