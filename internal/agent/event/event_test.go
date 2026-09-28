package event

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMessageEventStableIDAndSchema(t *testing.T) {
	a := NewMessageEvent(12, "p2p_1_2", 1, 0, 99, []int64{2})
	b := NewMessageEvent(12, "p2p_1_2", 1, 0, 99, nil)
	if a.EventID != b.EventID {
		t.Fatalf("event id changed: %q != %q", a.EventID, b.EventID)
	}
	encoded, err := a.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	var decoded MessageEvent
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SchemaVersion != SchemaVersion || decoded.MsgID != 12 {
		t.Fatalf("unexpected decoded event: %+v", decoded)
	}
	if strings.Contains(string(encoded), "content") {
		t.Fatalf("event contains message content: %s", encoded)
	}
}
