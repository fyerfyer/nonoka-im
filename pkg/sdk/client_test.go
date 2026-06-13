package sdk

import (
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
