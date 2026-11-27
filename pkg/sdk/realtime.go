package sdk

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	v1 "nonoka-im/api/im/v1"
	rt "nonoka-im/pkg/sdk/internal/realtime"
)

type RealtimeClient struct {
	cfg  Config
	conn *rt.Client
	stop func()

	mu      sync.Mutex
	subs    map[uint64]chan RealtimeEvent
	nextSub uint64
	// receipts resolves one send in O(1) without creating a full event
	// subscription per message.
	receipts map[string]chan *SendReceipt
	closed   bool
}

// newRealtimeClient constructs the public event layer over the internal
// connection state machine.
func newRealtimeClient(cfg Config, httpTransport *apiTransport) *RealtimeClient {
	conn := rt.New(rt.Config{
		GatewayURLs:          cfg.Realtime.GatewayURLs,
		Origin:               cfg.Realtime.Origin,
		DeviceID:             cfg.Realtime.DeviceID,
		HeartbeatInterval:    cfg.Realtime.HeartbeatInterval,
		HeartbeatTimeout:     cfg.Realtime.HeartbeatTimeout,
		ReconnectInterval:    cfg.Realtime.ReconnectInterval,
		MaxReconnectDelay:    cfg.Realtime.MaxReconnectDelay,
		MaxReconnectAttempts: cfg.Realtime.MaxReconnectAttempts,
		DisableAutoReconnect: cfg.Realtime.DisableAutoReconnect,
	}, cfg.TokenSource, &dispatchAdapter{http: httpTransport})
	client := &RealtimeClient{
		cfg: cfg, conn: conn,
		subs: map[uint64]chan RealtimeEvent{}, receipts: map[string]chan *SendReceipt{},
	}
	events, stop := conn.Subscribe()
	client.stop = stop
	go client.pump(events)
	return client
}

type dispatchAdapter struct {
	http *apiTransport
}

func (d *dispatchAdapter) GatewayURLs(ctx context.Context, token string) ([]string, error) {
	userID := userIDFromToken(token)
	if userID <= 0 || d.http == nil {
		return nil, fmt.Errorf("gateway URLs are required")
	}
	query := url.Values{}
	query.Set("userId", strconv.FormatInt(userID, 10))
	var reply v1.GetGatewayReply
	if err := d.http.doProto(ctx, http.MethodGet, "/v1/dispatch/gateway", query, nil, &reply); err != nil {
		return nil, err
	}
	selected := reply.GetGatewayUrl()
	urls := reply.GetGatewayUrls()
	if selected == "" && len(urls) == 0 {
		return nil, fmt.Errorf("gateway URLs are required")
	}
	// Keep the selected gateway first but retain the advertised fallback
	// list; a single-node reply must not disable failover.
	out := make([]string, 0, len(urls)+1)
	if selected != "" {
		out = append(out, selected)
	}
	for _, url := range urls {
		if url == "" || url == selected {
			continue
		}
		out = append(out, url)
	}
	return out, nil
}

func (c *RealtimeClient) pump(events <-chan rt.Event) {
	for event := range events {
		switch event.Type {
		case rt.EventConnected:
			c.emit(RealtimeEvent{Type: RealtimeConnected})
		case rt.EventAuthed:
			c.emit(RealtimeEvent{Type: RealtimeAuthed})
		case rt.EventDisconnected:
			c.failSendReceipts()
			c.emit(RealtimeEvent{Type: RealtimeDisconnected, Err: event.Err})
		case rt.EventReconnecting:
			c.emit(RealtimeEvent{Type: RealtimeReconnecting})
		case rt.EventPacket:
			c.handlePacket(event.Packet)
		case rt.EventError:
			c.emit(RealtimeEvent{Type: RealtimeError, Err: event.Err})
		}
	}
}

// Events returns one ordered realtime event subscription. Multiple callers
// receive independent copies; each channel is closed after Close.
func (c *RealtimeClient) Events() <-chan RealtimeEvent {
	events, _ := c.Subscribe()
	return events
}

// Subscribe returns an independent event channel and a cancel function.
func (c *RealtimeClient) Subscribe() (<-chan RealtimeEvent, func()) {
	c.mu.Lock()
	if c.closed {
		ch := make(chan RealtimeEvent)
		close(ch)
		c.mu.Unlock()
		return ch, func() {}
	}
	c.nextSub++
	id := c.nextSub
	ch := make(chan RealtimeEvent, 256)
	c.subs[id] = ch
	c.mu.Unlock()
	cancel := func() {
		c.mu.Lock()
		if existing, ok := c.subs[id]; ok {
			delete(c.subs, id)
			close(existing)
		}
		c.mu.Unlock()
	}
	return ch, cancel
}

// waitSendReceipt registers a one-shot waiter for a send receipt. It must be
// registered before publishing so a fast server cannot race the waiter.
func (c *RealtimeClient) waitSendReceipt(clientMsgID string) (<-chan *SendReceipt, func()) {
	c.mu.Lock()
	if c.closed {
		ch := make(chan *SendReceipt)
		close(ch)
		c.mu.Unlock()
		return ch, func() {}
	}
	ch := make(chan *SendReceipt, 1)
	c.receipts[clientMsgID] = ch
	c.mu.Unlock()
	cancel := func() {
		c.mu.Lock()
		if existing, ok := c.receipts[clientMsgID]; ok && existing == ch {
			delete(c.receipts, clientMsgID)
			close(existing)
		}
		c.mu.Unlock()
	}
	return ch, cancel
}

func (c *RealtimeClient) resolveSendReceipt(receipt *SendReceipt) {
	if receipt == nil {
		return
	}
	c.mu.Lock()
	ch, ok := c.receipts[receipt.ClientMessageID]
	if ok {
		delete(c.receipts, receipt.ClientMessageID)
		ch <- receipt
		close(ch)
	}
	c.mu.Unlock()
}

func (c *RealtimeClient) failSendReceipts() {
	c.mu.Lock()
	for id, ch := range c.receipts {
		delete(c.receipts, id)
		close(ch)
	}
	c.mu.Unlock()
}

// IsConnected reports whether the WebSocket is currently open and authed.
func (c *RealtimeClient) IsConnected() bool { return c.conn.IsConnected() }

// State returns a human-readable connection state.
func (c *RealtimeClient) State() string { return c.conn.State() }

// Connect opens and authenticates the WebSocket. It is safe to call again
// after a disconnect, including when automatic reconnects are disabled.
func (c *RealtimeClient) Connect(ctx context.Context) error {
	return c.conn.Connect(ctx)
}

func (c *RealtimeClient) sendAck(messageID int64, topic string, seq uint64) {
	_ = c.conn.Send(&v1.Packet{
		Cmd: v1.Command_CMD_ACK, Seq: c.conn.NextSeq(),
		Payload: &v1.Packet_AckReq{AckReq: &v1.AckRequest{MsgId: messageID, Topic: topic, TopicSeq: seq}},
	})
}

// SendReadReceipt marks a conversation read through the live connection.
func (c *RealtimeClient) SendReadReceipt(topic string, upToSeq uint64) error {
	return c.conn.Send(&v1.Packet{
		Cmd: v1.Command_CMD_READ_RECEIPT, Seq: c.conn.NextSeq(),
		Payload: &v1.Packet_ReadReceipt{ReadReceipt: &v1.ReadReceipt{Topic: topic, UpToSeq: upToSeq}},
	})
}

// Recall requests a message recall.
func (c *RealtimeClient) Recall(topic string, topicSeq uint64, messageID int64) error {
	return c.conn.Send(&v1.Packet{
		Cmd: v1.Command_CMD_RECALL, Seq: c.conn.NextSeq(),
		Payload: &v1.Packet_RecallReq{RecallReq: &v1.RecallRequest{Topic: topic, TopicSeq: topicSeq, MsgId: messageID}},
	})
}

func (c *RealtimeClient) request(ctx context.Context, packet *v1.Packet, timeout time.Duration) (*v1.Packet, error) {
	return c.conn.Request(ctx, packet, timeout)
}

func (c *RealtimeClient) writePacket(packet *v1.Packet) error {
	return c.conn.Send(packet)
}

func (c *RealtimeClient) nextSeq() uint64 {
	return c.conn.NextSeq()
}

func (c *RealtimeClient) emit(event RealtimeEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	for _, ch := range c.subs {
		select {
		case ch <- event:
		default:
			// Slow consumers can recover via the run events endpoint.
		}
	}
}

func (c *RealtimeClient) closeEvents() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	for id, ch := range c.subs {
		delete(c.subs, id)
		close(ch)
	}
	for id, ch := range c.receipts {
		delete(c.receipts, id)
		close(ch)
	}
	c.mu.Unlock()
}

// Disconnect closes the current WebSocket but keeps subscriptions and the
// client reusable via Connect.
func (c *RealtimeClient) Disconnect() error {
	c.failSendReceipts()
	return c.conn.Disconnect()
}

// Close stops reconnect/heartbeat and closes the WebSocket.
func (c *RealtimeClient) Close() error {
	err := c.conn.Close()
	if c.stop != nil {
		c.stop()
	}
	c.closeEvents()
	return err
}

func (c *RealtimeClient) setTokenSource(source TokenSource) {
	c.cfg.TokenSource = source
	c.conn.SetTokenSource(source)
}
