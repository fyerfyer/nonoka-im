package server

import (
	"encoding/json"
	"testing"

	agentv1 "nonoka-im/api/agent/v1"
)

func TestConvertNumericStringsKeepsAgentIDsAsNumbers(t *testing.T) {
	raw := map[string]any{
		"id":            "123",
		"agentId":       "7",
		"topicSeq":      "9",
		"eventId":       "123",
		"clientMsgId":   "456",
		"steps":         []any{map[string]any{"id": "55", "seq": "3"}},
		"nestedUnknown": map[string]any{"id": "88", "name": "x"},
	}
	converted := convertNumericStrings(raw).(map[string]any)

	if _, ok := converted["id"].(json.Number); !ok {
		t.Fatalf("id was not converted: %#v", converted["id"])
	}
	if converted["eventId"] != "123" || converted["clientMsgId"] != "456" {
		t.Fatalf("string fields were converted: %#v", converted)
	}
	step := converted["steps"].([]any)[0].(map[string]any)
	if _, ok := step["id"].(json.Number); !ok {
		t.Fatalf("nested id was not converted: %#v", step)
	}

	// The encoder must accept a real proto message and emit a numeric id.
	data, err := json.Marshal(agentv1.AgentRun{Id: 42, AgentId: 7, Status: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["id"] != float64(42) {
		t.Fatalf("unexpected standard JSON id: %#v", decoded["id"])
	}
}
