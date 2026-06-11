package main

import (
	"context"
	"fmt"
	"time"

	"nonoka-im/pkg/sdk"
)

func main() {
	// Register
	client := sdk.NewClient(sdk.Options{
		BaseURL:        "http://127.0.0.1:18000",
		DeviceID:       "test-device",
		RequestTimeout: 10 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	_, _ = client.Auth.Register(ctx, "testuser123", "123456")
	loginRes, err := client.Auth.Login(ctx, "testuser123", "123456")
	cancel()
	if err != nil {
		fmt.Printf("Login failed: %v\n", err)
		return
	}
	fmt.Printf("Login success: userID=%d, token=%s...\n", loginRes.UserID, loginRes.Token[:20])

	// Now create a new client with the token and connect
	client2 := sdk.NewClient(sdk.Options{
		BaseURL:        "http://127.0.0.1:18000",
		GatewayURL:     "ws://127.0.0.1:18000/ws",
		Token:          loginRes.Token,
		DeviceID:       "test-device",
		RequestTimeout: 10 * time.Second,
		OnConnect: func() {
			fmt.Println("Connected!")
		},
		OnDisconnect: func(reason error) {
			fmt.Printf("Disconnected: %v\n", reason)
		},
	})

	ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
	err = client2.Connect(ctx2)
	cancel2()
	if err != nil {
		fmt.Printf("Connect failed: %v\n", err)
		return
	}

	fmt.Printf("Connected! IsConnected=%v IsAuthed=%v UserID=%d State=%s\n",
		client2.IsConnected(), client2.IsAuthed(), client2.UserID(), client2.Realtime.State())

	// Try sending
	ctx3, cancel3 := context.WithTimeout(context.Background(), 10*time.Second)
	result, err := client2.SendText(ctx3, fmt.Sprintf("p2p_%d_%d", loginRes.UserID, loginRes.UserID+1), "Hello from test")
	cancel3()
	if err != nil {
		fmt.Printf("Send failed: %v\n", err)
	} else {
		fmt.Printf("Send success: msgID=%d, topicSeq=%d\n", result.MsgID, result.TopicSeq)
	}

	fmt.Println("Waiting 35s to observe heartbeat behavior...")
	time.Sleep(35 * time.Second)
	fmt.Printf("After 35s: IsConnected=%v State=%s\n", client2.IsConnected(), client2.Realtime.State())
	client2.Close()
}
