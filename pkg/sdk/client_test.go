package sdk

import (
	"strings"
	"testing"
)

// TestHandleSendReceipt_UntracksMessage verifies that a server-pushed send
// receipt removes the message from the in-flight sendingMsgs map, preventing
// a memory leak.
func TestHandleSendReceipt_UntracksMessage(t *testing.T) {
	client := NewClient(Options{})
	defer client.Close()

	clientMsgID := "test-client-msg-001"
	msg := &Message{
		ClientMsgID: clientMsgID,
		Topic:       "p2p_1_2",
		Status:      MessageStatusSending,
	}

	client.trackSendingMsg(msg)

	if _, ok := client.getSendingMsg(clientMsgID); !ok {
		t.Fatal("message should be in sendingMsgs before receipt")
	}

	client.handleSendReceipt(clientMsgID, 12345, "p2p_1_2", 1)

	if _, ok := client.getSendingMsg(clientMsgID); ok {
		t.Fatal("message should be removed from sendingMsgs after receipt")
	}

	if msg.Status != MessageStatusSent {
		t.Fatalf("expected message status Sent, got %v", msg.Status)
	}
	if msg.MsgID != 12345 {
		t.Fatalf("expected msg_id=12345, got %d", msg.MsgID)
	}
	if msg.TopicSeq != 1 {
		t.Fatalf("expected topic_seq=1, got %d", msg.TopicSeq)
	}
}

// TestHandleSendReceipt_Idempotent verifies that receiving a receipt for an
// already-untracked message does not panic or leak state.
func TestHandleSendReceipt_Idempotent(t *testing.T) {
	client := NewClient(Options{})
	defer client.Close()

	client.handleSendReceipt("missing-client-msg", 1, "p2p_1_2", 1)
	if len(client.sendingMsgs) != 0 {
		t.Fatalf("expected empty sendingMsgs, got %d", len(client.sendingMsgs))
	}
}

// getSendingMsg is a test helper to safely read from sendingMsgs.
func (c *Client) getSendingMsg(clientMsgID string) (*Message, bool) {
	c.sendingMu.RLock()
	defer c.sendingMu.RUnlock()
	msg, ok := c.sendingMsgs[clientMsgID]
	return msg, ok
}

// TestGatewaySelector_RoundRobin verifies round-robin cycling across URLs.
func TestGatewaySelector_RoundRobin(t *testing.T) {
	urls := []string{"ws://g1/ws", "ws://g2/ws", "ws://g3/ws"}
	seen := make(map[string]bool)
	for i := 0; i < len(urls)*2; i++ {
		url, _ := selectGatewayURL(urls, GatewaySelectorRoundRobin, 0, i)
		seen[url] = true
	}
	for _, u := range urls {
		if !seen[u] {
			t.Fatalf("expected URL %s to be selected", u)
		}
	}
}

// TestGatewaySelector_HashUserID verifies deterministic selection by user ID.
func TestGatewaySelector_HashUserID(t *testing.T) {
	urls := []string{"ws://g1/ws", "ws://g2/ws", "ws://g3/ws"}
	url1, _ := selectGatewayURL(urls, GatewaySelectorHashUserID, 42, 0)
	url2, _ := selectGatewayURL(urls, GatewaySelectorHashUserID, 42, 0)
	if url1 != url2 {
		t.Fatalf("expected deterministic selection for same user, got %s and %s", url1, url2)
	}

	url3, _ := selectGatewayURL(urls, GatewaySelectorHashUserID, 43, 0)
	if url1 == url3 {
		t.Fatalf("expected different selection for different users")
	}
}

// TestRealtimeClient_GatewayURLList verifies that the realtime client picks a
// URL from the configured list and rotates to the next one on failure.
func TestRealtimeClient_GatewayURLList(t *testing.T) {
	rt := NewRealtimeClient(RealtimeOptions{
		GatewayURLs:     []string{"ws://g1/ws", "ws://g2/ws"},
		GatewaySelector: GatewaySelectorRoundRobin,
		UserID:          1,
	})

	url1 := rt.nextGatewayURL()
	if !strings.Contains(url1, "g1") {
		t.Fatalf("expected first URL, got %s", url1)
	}

	rt.rotateGateway()
	url2 := rt.nextGatewayURL()
	if !strings.Contains(url2, "g2") {
		t.Fatalf("expected second URL after rotation, got %s", url2)
	}
}

// TestGatewayList_Priority verifies GatewayURL > GatewayURLs > discovered.
func TestGatewayList_Priority(t *testing.T) {
	if got := gatewayList("ws://explicit/ws", []string{"ws://list/ws"}, []string{"ws://discovered/ws"}); got[0] != "ws://explicit/ws" {
		t.Fatalf("expected explicit URL priority, got %v", got)
	}
	if got := gatewayList("", []string{"ws://list/ws"}, []string{"ws://discovered/ws"}); got[0] != "ws://list/ws" {
		t.Fatalf("expected list URL priority, got %v", got)
	}
	if got := gatewayList("", nil, []string{"ws://discovered/ws"}); got[0] != "ws://discovered/ws" {
		t.Fatalf("expected discovered URL fallback, got %v", got)
	}
}
