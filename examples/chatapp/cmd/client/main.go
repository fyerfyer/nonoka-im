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
)

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Config error: %v\n", err)
		os.Exit(1)
	}

	// Create chat app
	chatApp := app.NewChatApp(cfg)

	// Register and login
	fmt.Println("🔐 Registering/Logging in...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	token, userID, err := chatApp.RegisterAndLogin(ctx)
	cancel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Auth failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Logged in as user %d\n", userID)

	// Set up message handler
	msgHandler := handler.NewMessageHandler()
	chatApp.SetOnMessage(msgHandler.HandleMessage)

	// Connect to IM server
	fmt.Println("🔗 Connecting to IM server...")
	if err := chatApp.Connect(token); err != nil {
		fmt.Fprintf(os.Stderr, "Connect failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ Connected to IM server")

	// Demo: Send messages
	topic := fmt.Sprintf("p2p_%d_%d", userID, userID+1) // Demo topic
	fmt.Printf("💬 Demo topic: %s\n", topic)

	// Send a few messages
	for i := 1; i <= 3; i++ {
		msg := fmt.Sprintf("Hello message #%d", i)
		fmt.Printf("📤 Sending: %s\n", msg)
		if err := chatApp.SendMessage(topic, msg); err != nil {
			fmt.Fprintf(os.Stderr, "Send failed: %v\n", err)
			continue
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Try to load history
	fmt.Println("\n📜 Loading message history...")
	msgs, err := chatApp.LoadHistory(topic, 10)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Load history failed: %v\n", err)
	} else {
		fmt.Printf("📋 Loaded %d messages from history\n", len(msgs))
		for _, m := range msgs {
			fmt.Printf("  - Seq %d: %s\n", m.TopicSeq, string(m.Content))
		}
	}

	// Print conversation
	msgHandler.PrintConversation(topic, userID)

	// Show unread counts
	fmt.Printf("\n📊 Unread count for %s: %d\n", topic, msgHandler.GetUnreadCount(topic))

	// Keep running for a bit to receive any push messages
	fmt.Println("\n⏳ Waiting for push messages (5s)...")
	time.Sleep(5 * time.Second)

	// Print final state
	msgHandler.PrintConversation(topic, userID)

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("\nPress Ctrl+C to exit...")
	<-sigCh

	fmt.Println("\n👋 Shutting down...")
	chatApp.Close()
}
