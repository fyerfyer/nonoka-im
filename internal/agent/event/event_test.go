package event

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMessageEventStableIDAndSchema(t *testing.T) {
	a := NewMessageEvent(12, "p2p_1_2", 1, 0, 99, []int64{2}, "")
	b := NewMessageEvent(12, "p2p_1_2", 1, 0, 99, nil, "")
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

func TestMessageEventIDTracksClientMessageID(t *testing.T) {
	// The IM worker regenerates msgID on redelivery, so the client message ID
	// must be the stable part of the event identity.
	first := NewMessageEvent(1, "grp_stable", 7, 0, 1, nil, "client-1")
	redelivered := NewMessageEvent(999, "grp_stable", 7, 0, 2, nil, "client-1")
	if first.EventID != redelivered.EventID {
		t.Fatalf("client message ID did not stabilize event ID: %q != %q", first.EventID, redelivered.EventID)
	}
	different := NewMessageEvent(1, "grp_stable", 7, 0, 1, nil, "client-2")
	if first.EventID == different.EventID {
		t.Fatal("different client message IDs must produce different event IDs")
	}
}
