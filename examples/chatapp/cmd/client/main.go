package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nonoka-im/examples/chatapp/internal/app"
	"nonoka-im/examples/chatapp/internal/config"
	"nonoka-im/examples/chatapp/internal/handler"
	"nonoka-im/pkg/sdk"
)

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Config error: %v\n", err)
		os.Exit(1)
	}

	// Create chat app
	chatApp := app.NewChatApp(cfg)

	// Register and login using SDK AuthService (no custom HTTP client needed)
	fmt.Println("🔐 Registering/Logging in...")
	ctx, cancel := context.WithTimeout(context.Background(), cfg.AuthTimeout)
	token, userID, err := chatApp.RegisterAndLogin(ctx)
	cancel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Auth failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Logged in as user %d\n", userID)

	// Set up message handler with status tracking
	msgHandler := handler.NewMessageHandler()

	// Bridge send receipts from the SDK to the local message handler so the
	// server-assigned msg_id and topic_seq are reflected in the UI.
	chatApp.OnSendReceipt = func(clientMsgID string, msgID int64, topic string, topicSeq uint64) {
		msgHandler.ConfirmReceipt(clientMsgID, msgID, topic, topicSeq)
	}

	// Bridge delivery receipts so the UI can show when the recipient receives
	// a message.
	chatApp.OnDeliveryReceipt = func(topic string, topicSeq uint64, msgID int64) {
		msgHandler.MarkDelivered(topic, topicSeq)
	}

	// Connect to IM server
	fmt.Println("🔗 Connecting to IM server...")
	if err := chatApp.Connect(token); err != nil {
		fmt.Fprintf(os.Stderr, "Connect failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Connected to IM server (state=%s)\n", chatApp.ConnectionState())

	// Demo: Setup a conversation and send messages
	// With the refactored SDK, we use Conversation API for message management
	peerID := cfg.PeerUserID
	if peerID == 0 {
		peerID = userID + 1
	}
	var topic string
	if userID < peerID {
		topic = fmt.Sprintf("p2p_%d_%d", userID, peerID)
	} else {
		topic = fmt.Sprintf("p2p_%d_%d", peerID, userID)
	}
	fmt.Printf("💬 Demo topic: %s (userID=%d, peerID=%d)\n", topic, userID, peerID)

	conv := chatApp.GetConversation(topic)
	if conv == nil {
		fmt.Fprintf(os.Stderr, "Failed to get conversation\n")
		os.Exit(1)
	}

	// Set up conversation-level message handler to observe real-time messages
	conv.OnMessage = func(msg *sdk.Message) {
		msgHandler.HandleMessage(msg)
	}

	// Set up read receipt handler to observe status transitions
	conv.OnReadReceipt = func(upToSeq uint64) {
		fmt.Printf("[📖] Read receipt received: messages up to seq %d are read\n", upToSeq)
	}

	// Send a few messages with status tracking
	fmt.Println("\n📤 Sending messages...")
	for i := 1; i <= 3; i++ {
		text := fmt.Sprintf("Hello message #%d", i)
		fmt.Printf("  Sending: %s\n", text)

		// Track the message before sending (Sending state)
		idx := msgHandler.AddSendingMessage(topic, userID, text)

		result, err := chatApp.SendMessage(topic, text)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Send failed: %v\n", err)
			msgHandler.MarkFailed(idx)
			continue
		}

		// Update with server-assigned metadata (Sent state)
		msgHandler.ConfirmSent(idx, result)
		fmt.Printf("  ✅ Sent (clientMsgID=%s, msgID=%d, topicSeq=%d)\n",
			result.ClientMsgID, result.MsgID, result.TopicSeq)

		time.Sleep(500 * time.Millisecond)
	}

	// Load history using Conversation.LoadHistory()
	// The refactored SDK merges history with local pending messages
	fmt.Println("\n📜 Loading message history...")
	msgs, err := chatApp.LoadHistory(topic, 10)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Load history failed: %v\n", err)
	} else {
		fmt.Printf("📋 Loaded %d messages from history\n", len(msgs))
		for _, m := range msgs {
			fmt.Printf("  - Seq %d: %s [%s]\n", m.TopicSeq, string(m.Content), m.Status.String())
		}
	}

	// Print conversation with status indicators
	msgHandler.PrintConversation(topic, userID)

	// Show unread counts
	fmt.Printf("\n📊 Unread count for %s: %d\n", topic, msgHandler.GetUnreadCount(topic))

	// Print status summary
	msgHandler.PrintStatusSummary()

	// Keep running for a bit to receive any push messages
	fmt.Printf("\n⏳ Waiting for push messages (%s)...\n", cfg.PushWaitDuration)
	time.Sleep(cfg.PushWaitDuration)

	// Print final state
	fmt.Printf("\n📊 Final connection state: %s\n", chatApp.ConnectionState())
	msgHandler.PrintConversation(topic, userID)
	msgHandler.PrintStatusSummary()

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Auto-exit mode: set CHAT_AUTO_EXIT=duration (e.g., "10s") for non-interactive testing
	if cfg.AutoExitDuration > 0 {
		fmt.Printf("\n⏳ Auto-exit in %s...\n", cfg.AutoExitDuration)
		select {
		case <-sigCh:
		case <-time.After(cfg.AutoExitDuration):
		}
	} else {
		fmt.Println("\nPress Ctrl+C to exit...")
		<-sigCh
	}

	fmt.Println("\n👋 Shutting down...")
	chatApp.Close()
}
