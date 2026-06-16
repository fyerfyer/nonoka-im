package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	v1 "nonoka-im/api/im/v1"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

var (
	baseURL   = flag.String("base-url", "http://127.0.0.1:8000", "HTTP API base URL")
	wsURL     = flag.String("ws-url", "ws://127.0.0.1:8000/ws", "WebSocket gateway URL")
	users     = flag.Int("users", 100, "Number of concurrent users")
	duration  = flag.Duration("duration", 30*time.Second, "Load test duration")
	msgRate   = flag.Float64("msg-rate", 1.0, "Messages per second per user")
	msgSize   = flag.Int("msg-size", 128, "Message content size in bytes")
	pairs     = flag.Int("pairs", 0, "Number of P2P pairs; 0 means users send to user+1")
	timeout   = flag.Duration("timeout", 10*time.Second, "Request timeout")
	warmup    = flag.Duration("warmup", 2*time.Second, "Warmup duration before measuring")
	outFile   = flag.String("out", "", "Optional JSON report output file")
	keepAlive = flag.Bool("keep-alive", true, "Keep connections alive after warmup")
)

type client struct {
	userID int64
	token  string
	conn   *websocket.Conn
	seq    uint64
	writeMu sync.Mutex

	// publishStarted tracks when each message was sent.
	// Using sync.Map removes lock contention between sendLoop and processLoop.
	publishStarted *sync.Map

	// packetCh decouples raw WebSocket reads from protobuf parsing/statistics.
	packetCh chan []byte

	// Latency slices are only touched by processLoop and main after shutdown.
	ackLatencies     []float64
	pushLatencies    []float64
	receiptLatencies []float64

	// processLoop completion signal.
	doneCh chan struct{}
}

type summary struct {
	Users           int       `json:"users"`
	DurationSec     float64   `json:"duration_sec"`
	MsgRatePerUser  float64   `json:"msg_rate_per_user"`
	MsgSize         int       `json:"msg_size"`
	TotalSent       int64     `json:"total_sent"`
	TotalAcked      int64     `json:"total_acked"`
	TotalPushed     int64     `json:"total_pushed"`
	TotalReceipts   int64     `json:"total_receipts"`
	SendFailures    int64     `json:"send_failures"`
	AuthFailures    int64     `json:"auth_failures"`
	ConnectFailures int64     `json:"connect_failures"`
	AckLatency      latencies `json:"ack_latency_ms"`
	PushLatency     latencies `json:"push_latency_ms"`
	ReceiptLatency  latencies `json:"receipt_latency_ms"`
}

type latencies struct {
	Count int     `json:"count"`
	Min   float64 `json:"min_ms"`
	Max   float64 `json:"max_ms"`
	Mean  float64 `json:"mean_ms"`
	P50   float64 `json:"p50_ms"`
	P95   float64 `json:"p95_ms"`
	P99   float64 `json:"p99_ms"`
}

func main() {
	flag.Parse()
	fmt.Printf("Nonoka IM Load Test\n")
	fmt.Printf("BaseURL: %s, WS: %s, Users: %d, Duration: %s, MsgRate: %.2f/s, MsgSize: %d\n",
		*baseURL, *wsURL, *users, *duration, *msgRate, *msgSize)

	if *users < 2 {
		fmt.Println("need at least 2 users")
		os.Exit(1)
	}

	var (
		totalSent       int64
		totalAcked      int64
		totalPushed     int64
		totalReceipts   int64
		sendFailures    int64
		authFailures    int64
		connectFailures int64
	)

	clients := make([]*client, *users)
	httpClient := &http.Client{Timeout: *timeout}

	fmt.Println("Registering and logging in users...")
	// Limit concurrent register/login to avoid overwhelming the auth DB pool.
	setupSem := make(chan struct{}, 50)
	var setupWg sync.WaitGroup
	for i := 0; i < *users; i++ {
		setupWg.Add(1)
		go func(idx int) {
			defer setupWg.Done()
			setupSem <- struct{}{}
			defer func() { <-setupSem }()

			username := fmt.Sprintf("loaduser-%d-%d", idx, time.Now().UnixNano())
			password := "loadtest-password"
			c := &client{}
			if err := c.registerAndLogin(httpClient, username, password); err != nil {
				fmt.Printf("user %d register/login failed: %v\n", idx, err)
				atomic.AddInt64(&authFailures, 1)
				return
			}
			c.publishStarted = new(sync.Map)
			c.packetCh = make(chan []byte, 4096)
			c.doneCh = make(chan struct{})
			if err := c.connectAndAuth(*wsURL); err != nil {
				fmt.Printf("user %d connect/auth failed: %v\n", idx, err)
				atomic.AddInt64(&connectFailures, 1)
				return
			}
			clients[idx] = c
		}(i)
	}
	setupWg.Wait()

	active := 0
	for _, c := range clients {
		if c != nil {
			active++
		}
	}
	fmt.Printf("Active clients: %d/%d\n", active, *users)
	if active < 2 {
		fmt.Println("not enough active clients")
		os.Exit(1)
	}

	// Start readers and processors for all clients.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var processWg sync.WaitGroup
	for _, c := range clients {
		if c != nil {
			go c.readLoop(ctx)
			processWg.Add(1)
			go c.processLoop(ctx, &processWg, &totalAcked, &totalPushed, &totalReceipts)
		}
	}

	// Warmup: send messages without measuring.
	fmt.Println("Warming up...")
	warmupCtx, warmupCancel := context.WithTimeout(ctx, *warmup)
	var warmupWg sync.WaitGroup
	for i, c := range clients {
		if c == nil {
			continue
		}
		warmupWg.Add(1)
		go func(idx int, cl *client) {
			defer warmupWg.Done()
			cl.sendLoop(warmupCtx, idx, clients, true, &totalSent, &sendFailures)
		}(i, c)
	}
	warmupWg.Wait()
	warmupCancel()

	// Reset latencies after warmup.
	for _, c := range clients {
		if c != nil {
			c.resetLatencies()
		}
	}

	fmt.Println("Load test running...")
	testCtx, testCancel := context.WithTimeout(ctx, *duration)
	var testWg sync.WaitGroup
	for i, c := range clients {
		if c == nil {
			continue
		}
		testWg.Add(1)
		go func(idx int, cl *client) {
			defer testWg.Done()
			cl.sendLoop(testCtx, idx, clients, false, &totalSent, &sendFailures)
		}(i, c)
	}
	testWg.Wait()
	testCancel()

	// Allow in-flight packets to arrive.
	time.Sleep(5 * time.Second)

	// Close connections to unblock readLoop so processLoop can drain quickly.
	for _, c := range clients {
		if c != nil {
			c.conn.Close()
		}
	}
	cancel()

	// Wait for process loops to drain pending packets before collecting stats.
	processWg.Wait()

	// Collect latencies.
	var ack, push, receipt []float64
	for _, c := range clients {
		if c == nil {
			continue
		}
		ack = append(ack, c.ackLatencies...)
		push = append(push, c.pushLatencies...)
		receipt = append(receipt, c.receiptLatencies...)
	}

	report := summary{
		Users:           active,
		DurationSec:     duration.Seconds(),
		MsgRatePerUser:  *msgRate,
		MsgSize:         *msgSize,
		TotalSent:       atomic.LoadInt64(&totalSent),
		TotalAcked:      atomic.LoadInt64(&totalAcked),
		TotalPushed:     atomic.LoadInt64(&totalPushed),
		TotalReceipts:   atomic.LoadInt64(&totalReceipts),
		SendFailures:    atomic.LoadInt64(&sendFailures),
		AuthFailures:    atomic.LoadInt64(&authFailures),
		ConnectFailures: atomic.LoadInt64(&connectFailures),
		AckLatency:      computeLatencies(ack),
		PushLatency:     computeLatencies(push),
		ReceiptLatency:  computeLatencies(receipt),
	}

	printReport(report)

	if *outFile != "" {
		data, _ := json.MarshalIndent(report, "", "  ")
		_ = os.WriteFile(*outFile, data, 0644)
		fmt.Printf("Report written to %s\n", *outFile)
	}
}

func (c *client) registerAndLogin(hc *http.Client, username, password string) error {
	// Register (ignore conflict if already exists).
	regBody, _ := json.Marshal(map[string]string{"username": username, "password": password})
	_, _ = hc.Post(*baseURL+"/v1/auth/register", "application/json", bytes.NewReader(regBody))

	// Login.
	loginBody, _ := json.Marshal(map[string]string{"username": username, "password": password, "deviceId": "loadtest"})
	resp, err := hc.Post(*baseURL+"/v1/auth/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("login status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var reply v1.LoginReply
	if err := protojson.Unmarshal(body, &reply); err != nil {
		return err
	}
	c.userID = reply.UserId
	c.token = reply.Token
	return nil
}

func (c *client) connectAndAuth(wsURL string) error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		ReadBufferSize:   8192,
		WriteBufferSize:  8192,
	}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return err
	}
	c.conn = conn

	seq := c.nextSeq()
	authReq := &v1.Packet{
		Cmd: v1.Command_CMD_AUTH,
		Seq: seq,
		Payload: &v1.Packet_AuthReq{
			AuthReq: &v1.AuthRequest{
				Token:    c.token,
				DeviceId: "loadtest",
			},
		},
	}
	if err := c.writePacket(authReq); err != nil {
		return err
	}

	for {
		_ = c.conn.SetReadDeadline(time.Now().Add(*timeout))
		_, data, err := c.conn.ReadMessage()
		_ = c.conn.SetReadDeadline(time.Time{})
		if err != nil {
			return err
		}
		var pkt v1.Packet
		if err := proto.Unmarshal(data, &pkt); err != nil {
			continue
		}
		if pkt.Cmd == v1.Command_CMD_AUTH && pkt.Seq == seq {
			resp := pkt.GetAuthResp()
			if resp == nil || !resp.Success {
				return fmt.Errorf("auth failed")
			}
			return nil
		}
	}
}

func (c *client) sendLoop(ctx context.Context, idx int, clients []*client, warmup bool, totalSent, sendFailures *int64) {
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

		receiver := c.pickReceiver(idx, clients)
		if receiver == nil {
			continue
		}

		clientMsgID := fmt.Sprintf("load-%d-%d-%d", idx, time.Now().UnixNano(), c.nextSeq())
		seq := c.nextSeq()
		topic := fmt.Sprintf("p2p_%d_%d", min(c.userID, receiver.userID), max(c.userID, receiver.userID))

		pkt := &v1.Packet{
			Cmd: v1.Command_CMD_PUBLISH,
			Seq: seq,
			Payload: &v1.Packet_SendReq{
				SendReq: &v1.SendMessageRequest{
					Topic:       topic,
					MsgType:     v1.MsgType_MSG_TYPE_TEXT,
					Content:     content,
					ClientMsgId: clientMsgID,
				},
			},
		}

		if !warmup {
			c.publishStarted.Store(clientMsgID, time.Now())
		}
		if err := c.writePacket(pkt); err != nil {
			atomic.AddInt64(sendFailures, 1)
			c.publishStarted.Delete(clientMsgID)
			continue
		}
		atomic.AddInt64(totalSent, 1)
	}
}

// readLoop only reads raw WebSocket frames and forwards them to packetCh.
// This keeps the hot read path as light as possible.
func (c *client) readLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			close(c.packetCh)
			return
		default:
		}

		_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		_, data, err := c.conn.ReadMessage()
		_ = c.conn.SetReadDeadline(time.Time{})
		if err != nil {
			close(c.packetCh)
			return
		}

		buf := make([]byte, len(data))
		copy(buf, data)
		select {
		case c.packetCh <- buf:
		case <-ctx.Done():
			close(c.packetCh)
			return
		}
	}
}

// processLoop parses packets and records statistics. It runs independently of
// readLoop to avoid blocking the WebSocket read path.
func (c *client) processLoop(ctx context.Context, wg *sync.WaitGroup, totalAcked, totalPushed, totalReceipts *int64) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-c.packetCh:
			if !ok {
				return
			}
			c.handlePacket(data, totalAcked, totalPushed, totalReceipts)
		}
	}
}

func (c *client) handlePacket(data []byte, totalAcked, totalPushed, totalReceipts *int64) {
	var pkt v1.Packet
	if err := proto.Unmarshal(data, &pkt); err != nil {
		return
	}

	switch pkt.Cmd {
	case v1.Command_CMD_PUBLISH:
		reply := pkt.GetSendReply()
		if reply != nil {
			if start, ok := c.publishStarted.Load(reply.ClientMsgId); ok {
				c.recordAck(time.Since(start.(time.Time)).Seconds())
				atomic.AddInt64(totalAcked, 1)
			}
		}
	case v1.Command_CMD_NOTIFY:
		if pkt.GetNotify() != nil {
			atomic.AddInt64(totalPushed, 1)
		}
	case v1.Command_CMD_SEND_RECEIPT:
		receipt := pkt.GetSendReceipt()
		if receipt != nil {
			if start, ok := c.publishStarted.Load(receipt.ClientMsgId); ok {
				c.publishStarted.Delete(receipt.ClientMsgId)
				c.recordReceipt(time.Since(start.(time.Time)).Seconds())
				atomic.AddInt64(totalReceipts, 1)
			}
		}
	}
}

func (c *client) writePacket(pkt *v1.Packet) error {
	data, err := proto.Marshal(pkt)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteMessage(websocket.BinaryMessage, data)
}

func (c *client) nextSeq() uint64 {
	c.seq++
	return c.seq
}

func (c *client) pickReceiver(idx int, clients []*client) *client {
	// Send to the next user in the ring.
	target := (idx + 1) % len(clients)
	for attempts := 0; attempts < len(clients); attempts++ {
		if clients[target] != nil && clients[target].userID != c.userID {
			return clients[target]
		}
		target = (target + 1) % len(clients)
	}
	return nil
}

func (c *client) recordAck(sec float64) {
	c.ackLatencies = append(c.ackLatencies, sec*1000)
}

func (c *client) recordPush(sec float64) {
	if sec <= 0 {
		return
	}
	c.pushLatencies = append(c.pushLatencies, sec*1000)
}

func (c *client) recordReceipt(sec float64) {
	c.receiptLatencies = append(c.receiptLatencies, sec*1000)
}

func (c *client) resetLatencies() {
	c.ackLatencies = c.ackLatencies[:0]
	c.pushLatencies = c.pushLatencies[:0]
	c.receiptLatencies = c.receiptLatencies[:0]
}

func computeLatencies(values []float64) latencies {
	if len(values) == 0 {
		return latencies{}
	}
	sort.Float64s(values)
	sum := 0.0
	min := values[0]
	max := values[0]
	for _, v := range values {
		sum += v
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return latencies{
		Count: len(values),
		Min:   round(min),
		Max:   round(max),
		Mean:  round(sum / float64(len(values))),
		P50:   round(percentile(values, 0.5)),
		P95:   round(percentile(values, 0.95)),
		P99:   round(percentile(values, 0.99)),
	}
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(p*float64(len(sorted)-1)))
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func round(v float64) float64 {
	return math.Round(v*1000) / 1000
}

func printReport(r summary) {
	fmt.Println("\n========== Load Test Report ==========")
	fmt.Printf("Active users:        %d\n", r.Users)
	fmt.Printf("Duration:            %.1fs\n", r.DurationSec)
	fmt.Printf("Messages sent:       %d\n", r.TotalSent)
	fmt.Printf("Messages ACKed:      %d (%.2f%%)\n", r.TotalAcked, percent(r.TotalAcked, r.TotalSent))
	fmt.Printf("Messages pushed:     %d\n", r.TotalPushed)
	fmt.Printf("Send receipts:       %d\n", r.TotalReceipts)
	fmt.Printf("Send failures:       %d\n", r.SendFailures)
	fmt.Printf("Connect failures:    %d\n", r.ConnectFailures)
	fmt.Printf("Auth failures:       %d\n", r.AuthFailures)
	fmt.Printf("Effective send rate: %.2f msg/s\n", float64(r.TotalSent)/r.DurationSec)
	fmt.Println("\nLatency (ms):")
	fmt.Printf("  ACK:      count=%d  min=%.3f  mean=%.3f  p50=%.3f  p95=%.3f  p99=%.3f  max=%.3f\n",
		r.AckLatency.Count, r.AckLatency.Min, r.AckLatency.Mean, r.AckLatency.P50, r.AckLatency.P95, r.AckLatency.P99, r.AckLatency.Max)
	fmt.Printf("  Push:     count=%d  min=%.3f  mean=%.3f  p50=%.3f  p95=%.3f  p99=%.3f  max=%.3f\n",
		r.PushLatency.Count, r.PushLatency.Min, r.PushLatency.Mean, r.PushLatency.P50, r.PushLatency.P95, r.PushLatency.P99, r.PushLatency.Max)
	fmt.Printf("  Receipt:  count=%d  min=%.3f  mean=%.3f  p50=%.3f  p95=%.3f  p99=%.3f  max=%.3f\n",
		r.ReceiptLatency.Count, r.ReceiptLatency.Min, r.ReceiptLatency.Mean, r.ReceiptLatency.P50, r.ReceiptLatency.P95, r.ReceiptLatency.P99, r.ReceiptLatency.Max)
}

func percent(part, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(part) / float64(total) * 100
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
