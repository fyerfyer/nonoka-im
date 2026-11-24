package main

import (
	"context"
	"fmt"
	"time"

	"nonoka-im/pkg/sdk"
)

func main() {
	baseURL := "http://127.0.0.1:18000"
	realtime := sdk.DefaultRealtimeConfig()
	realtime.GatewayURLs = []string{"ws://127.0.0.1:18000/ws"}
	realtime.DeviceID = "test-device"

	client, err := sdk.New(sdk.Config{
		BaseURL: baseURL, Timeout: 10 * time.Second, Realtime: realtime,
	})
	if err != nil {
		panic(err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	_, _ = client.Auth.Register(ctx, "testuser123", "123456")
	pair, err := client.Auth.Login(ctx, "testuser123", "123456", "test-device")
	cancel()
	if err != nil {
		fmt.Printf("Login failed: %v\n", err)
		return
	}
	fmt.Printf("Login success: userID=%d token=%s...\n", pair.UserID, pair.Token[:min(20, len(pair.Token))])

	client.UpdateToken(pair.Token)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
	err = client.Connect(ctx2)
	cancel2()
	if err != nil {
		fmt.Printf("Connect failed: %v\n", err)
		return
	}
	fmt.Printf("Connected! IsConnected=%v IsAuthed=%v UserID=%d State=%s\n",
		client.IsConnected(), client.IsAuthed(), client.UserID(), client.ConnectionState())

	ctx3, cancel3 := context.WithTimeout(context.Background(), 10*time.Second)
	result, err := client.Messages.SendText(ctx3, fmt.Sprintf("p2p_%d_%d", pair.UserID, pair.UserID+1), "Hello from test")
	cancel3()
	if err != nil {
		fmt.Printf("Send failed: %v\n", err)
	} else {
		fmt.Printf("Send success: msgID=%d, topicSeq=%d\n", result.MessageID, result.TopicSeq)
	}

	fmt.Println("Waiting 35s to observe heartbeat behavior...")
	time.Sleep(35 * time.Second)
	fmt.Printf("After 35s: IsConnected=%v State=%s\n", client.IsConnected(), client.ConnectionState())
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
