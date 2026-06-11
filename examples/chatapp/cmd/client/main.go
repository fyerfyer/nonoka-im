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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	token, userID, err := chatApp.RegisterAndLogin(ctx)
	cancel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Auth failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Logged in as user %d\n", userID)

	// Set up message handler with status tracking
	msgHandler := handler.NewMessageHandler()

	// Connect to IM server
	fmt.Println("🔗 Connecting to IM server...")
	if err := chatApp.Connect(token); err != nil {
		fmt.Fprintf(os.Stderr, "Connect failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Connected to IM server (state=%s)\n", chatApp.ConnectionState())

	// Demo: Setup a conversation and send messages
	// With the refactored SDK, we use Conversation API for message management
	topic := fmt.Sprintf("p2p_%d_%d", userID, userID+1)
	fmt.Printf("💬 Demo topic: %s\n", topic)

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

		msg := &sdk.Message{
			Topic:     topic,
			SenderID:  userID,
			MsgType:   sdk.MsgTypeText,
			Content:   []byte(text),
			Timestamp: time.Now().Unix(),
			Status:    sdk.MessageStatusSending,
		}
		msgHandler.TrackSentMessage(msg)

		result, err := chatApp.SendMessage(topic, text)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Send failed: %v\n", err)
			msgHandler.UpdateSentStatus(msg.ClientMsgID, sdk.MessageStatusFailed)
			continue
		}

		// Update with server-assigned metadata
		msg.MsgID = result.MsgID
		msg.TopicSeq = result.TopicSeq
		msg.Status = sdk.MessageStatusSent
		msgHandler.UpdateSentStatus(msg.ClientMsgID, sdk.MessageStatusSent)
		fmt.Printf("  ✅ Sent (msgID=%d, topicSeq=%d)\n", result.MsgID, result.TopicSeq)

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
	fmt.Println("\n⏳ Waiting for push messages (5s)...")
	time.Sleep(5 * time.Second)

	// Print final state
	fmt.Printf("\n📊 Final connection state: %s\n", chatApp.ConnectionState())
	msgHandler.PrintConversation(topic, userID)
	msgHandler.PrintStatusSummary()

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("\nPress Ctrl+C to exit...")
	<-sigCh

	fmt.Println("\n👋 Shutting down...")
	chatApp.Close()
}
