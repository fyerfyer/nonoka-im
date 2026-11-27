package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"nonoka-im/pkg/sdk"
)

var firstSDKError atomic.Bool

type sdkUser struct {
	id      int64
	client  *sdk.Client
	events  <-chan sdk.RealtimeEvent
	cancel  func()
	sendSem chan struct{}
}

type latencyCollector struct {
	mu      sync.Mutex
	ack     []float64
	push    []float64
	receipt []float64
}

func (c *latencyCollector) addAck(sec float64) {
	c.mu.Lock()
	c.ack = append(c.ack, sec*1000)
	c.mu.Unlock()
}

func (c *latencyCollector) addPush(sec float64) {
	c.mu.Lock()
	c.push = append(c.push, sec*1000)
	c.mu.Unlock()
}

func (c *latencyCollector) addReceipt(sec float64) {
	c.mu.Lock()
	c.receipt = append(c.receipt, sec*1000)
	c.mu.Unlock()
}

func (c *latencyCollector) snapshot() (ack, push, receipt []float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]float64(nil), c.ack...), append([]float64(nil), c.push...), append([]float64(nil), c.receipt...)
}

func (c *latencyCollector) reset() {
	c.mu.Lock()
	c.ack = c.ack[:0]
	c.push = c.push[:0]
	c.receipt = c.receipt[:0]
	c.mu.Unlock()
}

// runSDKLoad drives the same P2P ring through the public SDK instead of the
// raw WebSocket protocol. It is intentionally a separate mode so the two
// clients can be compared without sharing implementation shortcuts.
func runSDKLoad(wsURLs []string) {
	fmt.Printf("SDK mode: BaseURL=%s, WS=%v, Users=%d, Duration=%s, MsgRate=%.2f/s, MsgSize=%d\n",
		*baseURL, wsURLs, *users, *duration, *msgRate, *msgSize)

	ctx := context.Background()
	auth, err := sdk.New(sdk.Config{BaseURL: *baseURL})
	if err != nil {
		fmt.Printf("create auth client: %v\n", err)
		os.Exit(1)
	}
	defer auth.Close()

	ring := make([]*sdkUser, *users)
	var authFailures, connectFailures int64
	var setupWg sync.WaitGroup
	setupSem := make(chan struct{}, 5)
	for i := 0; i < *users; i++ {
		setupWg.Add(1)
		go func(idx int) {
			defer setupWg.Done()
			setupSem <- struct{}{}
			defer func() { <-setupSem }()

			username := fmt.Sprintf("loaduser-%d-%d", idx, time.Now().UnixNano())
			password := "loadtest-password"
			if err := sdkRegisterWithRetry(ctx, auth, username, password); err != nil {
				fmt.Printf("sdk user %d register failed: %v\n", idx, err)
				atomic.AddInt64(&authFailures, 1)
				return
			}
			pair, err := sdkLoginWithRetry(ctx, auth, username, password)
			if err != nil {
				fmt.Printf("sdk user %d login failed: %v\n", idx, err)
				atomic.AddInt64(&authFailures, 1)
				return
			}
			client, err := sdk.New(sdk.Config{
				BaseURL:     *baseURL,
				TokenSource: sdk.StaticToken(pair.Token),
				Realtime: sdk.RealtimeConfig{
					GatewayURLs:    wsURLs,
					DeviceID:       "loadtest-sdk",
					DisableAutoAck: true,
				},
			})
			if err != nil {
				fmt.Printf("sdk user %d client: %v\n", idx, err)
				atomic.AddInt64(&connectFailures, 1)
				return
			}
			if err := client.Connect(ctx); err != nil {
				fmt.Printf("sdk user %d connect: %v\n", idx, err)
				_ = client.Close()
				atomic.AddInt64(&connectFailures, 1)
				return
			}
			events, cancel := client.Realtime.Subscribe()
			ring[idx] = &sdkUser{
				id: pair.UserID, client: client, events: events, cancel: cancel,
				sendSem: make(chan struct{}, 2),
			}
		}(i)
	}
	setupWg.Wait()

	active := 0
	for _, user := range ring {
		if user != nil {
			active++
		}
	}
	fmt.Printf("SDK active clients: %d/%d\n", active, *users)
	if active < 2 {
		fmt.Println("not enough active SDK clients")
		os.Exit(1)
	}

	collector := &latencyCollector{}
	starts := &sync.Map{}
	var totalSent, totalAcked, totalPushed, totalReceipts, sendFailures, reconnects int64
	groupTopic := ""
	if *chatMode == "group" {
		memberIDs := make([]int64, 0, active)
		for _, user := range ring {
			if user != nil {
				memberIDs = append(memberIDs, user.id)
			}
		}
		group, err := ring[firstActiveSDKUser(ring)].client.Groups.Create(ctx, "sdk-loadtest-group", memberIDs)
		if err != nil {
			fmt.Printf("create sdk loadtest group: %v\n", err)
			os.Exit(1)
		}
		groupTopic = group.Topic
		fmt.Printf("SDK group topic: %s (%d members)\n", groupTopic, len(memberIDs))
	}

	eventCtx, eventCancel := context.WithCancel(ctx)
	defer eventCancel()
	for _, user := range ring {
		if user == nil {
			continue
		}
		go user.eventLoop(eventCtx, collector, starts, &totalPushed, &totalReceipts)
	}
	if *reconnectEvery > 0 {
		go reconnectStorm(eventCtx, ring, &reconnects, &connectFailures)
	}

	fmt.Println("Warming up...")
	warmupCtx, warmupCancel := context.WithTimeout(eventCtx, *warmup)
	var warmupWg sync.WaitGroup
	var sendWG sync.WaitGroup
	for i, user := range ring {
		if user == nil {
			continue
		}
		warmupWg.Add(1)
		go func(idx int, item *sdkUser) {
			defer warmupWg.Done()
			item.sendLoop(warmupCtx, idx, ring, groupTopic, true, collector, starts, &totalSent, &totalAcked, &sendFailures, &sendWG)
		}(i, user)
	}
	warmupWg.Wait()
	warmupCancel()
	atomic.StoreInt64(&totalSent, 0)
	atomic.StoreInt64(&totalAcked, 0)
	atomic.StoreInt64(&totalPushed, 0)
	atomic.StoreInt64(&totalReceipts, 0)
	atomic.StoreInt64(&sendFailures, 0)
	collector.reset()
	starts.Range(func(key, _ any) bool { starts.Delete(key); return true })

	fmt.Println("SDK load test running...")
	testCtx, testCancel := context.WithTimeout(eventCtx, *duration)
	var testWg sync.WaitGroup
	for i, user := range ring {
		if user == nil {
			continue
		}
		testWg.Add(1)
		go func(idx int, item *sdkUser) {
			defer testWg.Done()
			item.sendLoop(testCtx, idx, ring, groupTopic, false, collector, starts, &totalSent, &totalAcked, &sendFailures, &sendWG)
		}(i, user)
	}
	testWg.Wait()
	sendWG.Wait()
	testCancel()
	time.Sleep(5 * time.Second)

	for _, user := range ring {
		if user == nil {
			continue
		}
		user.cancel()
		_ = user.client.Close()
	}
	eventCancel()

	ack, push, receipt := collector.snapshot()
	report := summary{
		Users: active, DurationSec: duration.Seconds(), MsgRatePerUser: *msgRate, MsgSize: *msgSize,
		TotalSent: atomic.LoadInt64(&totalSent), TotalAcked: atomic.LoadInt64(&totalAcked),
		TotalPushed: atomic.LoadInt64(&totalPushed), TotalReceipts: atomic.LoadInt64(&totalReceipts),
		Reconnects:   atomic.LoadInt64(&reconnects),
		SendFailures: atomic.LoadInt64(&sendFailures), AuthFailures: atomic.LoadInt64(&authFailures),
		ConnectFailures: atomic.LoadInt64(&connectFailures),
		AckLatency:      computeLatencies(ack), PushLatency: computeLatencies(push), ReceiptLatency: computeLatencies(receipt),
	}
	printReport(report)
	if *outFile != "" {
		data, _ := json.MarshalIndent(report, "", "  ")
		_ = os.WriteFile(*outFile, data, 0644)
		fmt.Printf("Report written to %s\n", *outFile)
	}
}

func (u *sdkUser) eventLoop(ctx context.Context, collector *latencyCollector, starts *sync.Map, totalPushed, totalReceipts *int64) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-u.events:
			if !ok {
				return
			}
			switch event.Type {
			case sdk.RealtimeMessage:
				if event.Message == nil {
					continue
				}
				atomic.AddInt64(totalPushed, 1)
				if start, ok := starts.Load(event.Message.ClientMessageID); ok {
					collector.addPush(time.Since(start.(time.Time)).Seconds())
				}
			case sdk.RealtimeSendReceipt:
				if event.SendReceipt == nil {
					continue
				}
				atomic.AddInt64(totalReceipts, 1)
				if start, ok := starts.Load(event.SendReceipt.ClientMessageID); ok {
					collector.addReceipt(time.Since(start.(time.Time)).Seconds())
					starts.Delete(event.SendReceipt.ClientMessageID)
				}
			}
		}
	}
}

func (u *sdkUser) sendLoop(ctx context.Context, idx int, ring []*sdkUser, groupTopic string, warmup bool, collector *latencyCollector, starts *sync.Map, totalSent, totalAcked, sendFailures *int64, sendWG *sync.WaitGroup) {
	interval := time.Duration(float64(time.Second) / *msgRate)
	if interval <= 0 {
		interval = time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	content := make([]byte, *msgSize)
	for i := range content {
		content[i] = byte('a' + i%26)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		receiver := pickSDKReceiver(idx, ring, u.id)
		if groupTopic == "" && receiver == nil {
			continue
		}
		clientID := fmt.Sprintf("sdkl-%d-%d-%d", idx, time.Now().UnixNano(), u.id)
		topic := groupTopic
		if topic == "" {
			topic = fmt.Sprintf("p2p_%d_%d", min64(u.id, receiver.id), max64(u.id, receiver.id))
		}
		start := time.Now()
		if !warmup {
			starts.Store(clientID, start)
		}
		if !warmup && *sdkPipeline {
			select {
			case u.sendSem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			atomic.AddInt64(totalSent, 1)
			sendWG.Add(1)
			go func() {
				defer sendWG.Done()
				defer func() { <-u.sendSem }()
				sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
				defer cancel()
				_, err := u.client.Messages.Send(sendCtx, sdk.SendRequest{
					Topic: topic, Type: sdk.MessageTypeText, Content: content, ClientMessageID: clientID,
				})
				if err != nil {
					if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
						starts.Delete(clientID)
						return
					}
					if firstSDKError.CompareAndSwap(false, true) {
						fmt.Printf("sdk send error: %v\n", err)
					}
					atomic.AddInt64(sendFailures, 1)
					starts.Delete(clientID)
					return
				}
				atomic.AddInt64(totalAcked, 1)
				collector.addAck(time.Since(start).Seconds())
			}()
			continue
		}
		_, err := u.client.Messages.Send(ctx, sdk.SendRequest{
			Topic: topic, Type: sdk.MessageTypeText, Content: content, ClientMessageID: clientID,
		})
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				starts.Delete(clientID)
				return
			}
			if firstSDKError.CompareAndSwap(false, true) {
				fmt.Printf("sdk send error: %v\n", err)
			}
			atomic.AddInt64(sendFailures, 1)
			starts.Delete(clientID)
			continue
		}
		if !warmup {
			atomic.AddInt64(totalSent, 1)
			atomic.AddInt64(totalAcked, 1)
			collector.addAck(time.Since(start).Seconds())
		}
	}
}

func pickSDKReceiver(idx int, ring []*sdkUser, self int64) *sdkUser {
	target := (idx + 1) % len(ring)
	for attempts := 0; attempts < len(ring); attempts++ {
		user := ring[target]
		if user != nil && user.id != self {
			return user
		}
		target = (target + 1) % len(ring)
	}
	return nil
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func firstActiveSDKUser(ring []*sdkUser) int {
	for i, user := range ring {
		if user != nil {
			return i
		}
	}
	return 0
}

func createGroupTopic(ctx context.Context, token string, memberIDs []int64) (string, error) {
	client, err := sdk.New(sdk.Config{BaseURL: *baseURL, TokenSource: sdk.StaticToken(token)})
	if err != nil {
		return "", err
	}
	defer client.Close()
	group, err := client.Groups.Create(ctx, "loadtest-group", memberIDs)
	if err != nil {
		return "", err
	}
	return group.Topic, nil
}

func reconnectStorm(ctx context.Context, ring []*sdkUser, reconnects, failures *int64) {
	ticker := time.NewTicker(*reconnectEvery)
	defer ticker.Stop()
	cursor := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		attempted := 0
		for i := 0; i < len(ring) && attempted < *reconnectUsers; i++ {
			user := ring[(cursor+i)%len(ring)]
			if user == nil {
				continue
			}
			attempted++
			_ = user.client.Realtime.Disconnect()
			if err := user.client.Connect(ctx); err != nil {
				atomic.AddInt64(failures, 1)
				continue
			}
			atomic.AddInt64(reconnects, 1)
		}
		cursor = (cursor + *reconnectUsers) % len(ring)
	}
}

func sdkRegisterWithRetry(ctx context.Context, client *sdk.Client, username, password string) error {
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		_, err := client.Auth.Register(ctx, username, password)
		if err == nil || sdk.IsConflict(err) {
			return nil
		}
		lastErr = err
		if !sdk.Retryable(err) && !sdk.IsRateLimited(err) {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 300 * time.Millisecond)
	}
	return lastErr
}

func sdkLoginWithRetry(ctx context.Context, client *sdk.Client, username, password string) (*sdk.TokenPair, error) {
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		pair, err := client.Auth.Login(ctx, username, password, "loadtest")
		if err == nil {
			return pair, nil
		}
		lastErr = err
		if !sdk.Retryable(err) && !sdk.IsRateLimited(err) {
			return nil, err
		}
		time.Sleep(time.Duration(attempt+1) * 300 * time.Millisecond)
	}
	return nil, lastErr
}
