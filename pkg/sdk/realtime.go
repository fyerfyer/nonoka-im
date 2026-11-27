package sdk

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	v1 "nonoka-im/api/im/v1"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

type RealtimeClient struct {
	cfg      Config
	dispatch v1.DispatchServiceHTTPClient

	connectMu sync.Mutex
	mu        sync.Mutex
	ws        *websocket.Conn
	writeMu   sync.Mutex
	seq       uint64
	pending   map[uint64]chan *v1.Packet
	subs      map[uint64]chan RealtimeEvent
	nextSub   uint64
	done      chan struct{}
	closed    bool
	authed    bool

	reconnectAttempt int
	timer            *time.Timer

	heartbeatTimer *time.Timer
	heartbeatStop  chan struct{}
	closeOnce      sync.Once
}

func newRealtimeClient(cfg Config, dispatch v1.DispatchServiceHTTPClient) *RealtimeClient {
	return &RealtimeClient{
		cfg: cfg, dispatch: dispatch, pending: map[uint64]chan *v1.Packet{},
		subs: map[uint64]chan RealtimeEvent{}, done: make(chan struct{}),
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

// IsConnected reports whether the WebSocket is currently open and authed.
func (c *RealtimeClient) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ws != nil && c.authed
}

// State returns a human-readable connection state.
func (c *RealtimeClient) State() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case c.closed:
		return "closed"
	case c.ws == nil:
		return "disconnected"
	case c.authed:
		return "authed"
	default:
		return "connected"
	}
}

// Connect opens and authenticates the WebSocket. It is safe to call again
// after a disconnect, including when automatic reconnects are disabled.
func (c *RealtimeClient) Connect(ctx context.Context) error {
	c.connectMu.Lock()
	defer c.connectMu.Unlock()

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrClosed
	}
	if c.ws != nil && c.authed {
		c.mu.Unlock()
		return ErrAlreadyConnected
	}
	c.mu.Unlock()
	return c.connect(ctx)
}

func (c *RealtimeClient) connect(ctx context.Context) error {
	urls, err := c.gatewayURLs(ctx)
	if err != nil {
		return err
	}
	token, err := c.token(ctx)
	if err != nil {
		return err
	}
	var lastErr error
	for _, gatewayURL := range urls {
		if gatewayURL == "" {
			continue
		}
		conn, _, dialErr := websocket.DefaultDialer.DialContext(ctx, gatewayURL, nil)
		if dialErr != nil {
			lastErr = dialErr
			continue
		}
		c.mu.Lock()
		c.ws = conn
		c.authed = false
		c.reconnectAttempt = 0
		c.mu.Unlock()
		c.emit(RealtimeEvent{Type: RealtimeConnected})
		go c.readLoop(conn)
		if authErr := c.authenticate(ctx, token); authErr != nil {
			_ = conn.Close()
			c.mu.Lock()
			if c.ws == conn {
				c.ws = nil
				c.authed = false
			}
			c.mu.Unlock()
			if refreshed, refreshErr := c.refresh(ctx); refreshErr == nil && refreshed != token {
				token = refreshed
				if retryErr := c.connectOnce(ctx, gatewayURL, token); retryErr == nil {
					return nil
				}
			}
			lastErr = authErr
			continue
		}
		c.markConnected()
		c.startHeartbeat()
		return nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no gateway URL available")
	}
	return lastErr
}

func (c *RealtimeClient) connectOnce(ctx context.Context, gatewayURL, token string) error {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, gatewayURL, nil)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.ws = conn
	c.authed = false
	c.mu.Unlock()
	c.emit(RealtimeEvent{Type: RealtimeConnected})
	go c.readLoop(conn)
	if err := c.authenticate(ctx, token); err != nil {
		_ = conn.Close()
		c.mu.Lock()
		if c.ws == conn {
			c.ws = nil
			c.authed = false
		}
		c.mu.Unlock()
		return err
	}
	c.markConnected()
	c.startHeartbeat()
	return nil
}

func (c *RealtimeClient) markConnected() {
	c.mu.Lock()
	c.reconnectAttempt = 0
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	c.mu.Unlock()
}

func (c *RealtimeClient) gatewayURLs(ctx context.Context) ([]string, error) {
	if len(c.cfg.Realtime.GatewayURLs) > 0 {
		return c.cfg.Realtime.GatewayURLs, nil
	}
	token, err := c.token(ctx)
	if err != nil {
		return nil, err
	}
	userID := userIDFromToken(token)
	if userID <= 0 || c.dispatch == nil {
		return nil, fmt.Errorf("gateway URLs are required")
	}
	reply, err := c.dispatch.Gateway(ctx, &v1.GetGatewayRequest{UserId: userID})
	if err != nil {
		return nil, apiErrorFromKratos(err)
	}
	if reply.GetGatewayUrl() != "" {
		return []string{reply.GetGatewayUrl()}, nil
	}
	return reply.GetGatewayUrls(), nil
}

func (c *RealtimeClient) token(ctx context.Context) (string, error) {
	if c.cfg.TokenSource == nil {
		return "", fmt.Errorf("%w: token source is required", ErrInvalidConfiguration)
	}
	return c.cfg.TokenSource.Token(ctx)
}

func (c *RealtimeClient) refresh(ctx context.Context) (string, error) {
	if refreshable, ok := c.cfg.TokenSource.(RefreshableTokenSource); ok {
		return refreshable.Refresh(ctx)
	}
	return "", errors.New("token source is not refreshable")
}

func (c *RealtimeClient) authenticate(ctx context.Context, token string) error {
	packet := &v1.Packet{
		Cmd:     v1.Command_CMD_AUTH,
		Seq:     c.nextSeq(),
		Payload: &v1.Packet_AuthReq{AuthReq: &v1.AuthRequest{Token: token, DeviceId: c.cfg.Realtime.DeviceID}},
	}
	reply, err := c.request(ctx, packet, 10*time.Second)
	if err != nil {
		return err
	}
	if reply.GetAuthResp() == nil || !reply.GetAuthResp().GetSuccess() {
		return fmt.Errorf("%w: auth rejected", ErrAuthFailed)
	}
	c.mu.Lock()
	c.authed = true
	c.mu.Unlock()
	c.emit(RealtimeEvent{Type: RealtimeAuthed})
	return nil
}

func (c *RealtimeClient) readLoop(conn *websocket.Conn) {
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			c.handleDisconnect(conn, err)
			return
		}
		var packet v1.Packet
		if err := proto.Unmarshal(data, &packet); err != nil {
			c.emit(RealtimeEvent{Type: RealtimeError, Err: err})
			continue
		}
		c.handlePacket(&packet)
	}
}

func (c *RealtimeClient) handleDisconnect(conn *websocket.Conn, reason error) {
	c.mu.Lock()
	if c.ws != conn {
		c.mu.Unlock()
		return
	}
	c.ws = nil
	c.authed = false
	closed := c.closed
	c.mu.Unlock()
	c.stopHeartbeat()
	c.emit(RealtimeEvent{Type: RealtimeDisconnected, Err: reason})
	if closed || c.cfg.Realtime.DisableAutoReconnect {
		// Keep subscriptions open: callers may call Connect again explicitly.
		// Only Close tears down the event streams.
		return
	}
	c.scheduleReconnect()
}

func (c *RealtimeClient) scheduleReconnect() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	attempt := c.reconnectAttempt
	c.reconnectAttempt++
	maxAttempts := c.cfg.Realtime.MaxReconnectAttempts
	c.mu.Unlock()
	if maxAttempts > 0 && attempt >= maxAttempts {
		c.emit(RealtimeEvent{Type: RealtimeError, Err: fmt.Errorf("sdk: max reconnect attempts reached")})
		// Leave subscriptions open; the caller can call Connect again.
		return
	}
	delay := c.cfg.Realtime.ReconnectInterval * time.Duration(1<<minInt(attempt, 5))
	if delay > c.cfg.Realtime.MaxReconnectDelay {
		delay = c.cfg.Realtime.MaxReconnectDelay
	}
	c.emit(RealtimeEvent{Type: RealtimeReconnecting})
	c.mu.Lock()
	if c.timer != nil {
		c.timer.Stop()
	}
	c.timer = time.AfterFunc(delay, func() {
		c.connectMu.Lock()
		err := c.connect(context.Background())
		c.connectMu.Unlock()
		if err != nil {
			c.emit(RealtimeEvent{Type: RealtimeError, Err: err})
			c.scheduleReconnect()
		}
	})
	c.mu.Unlock()
}

func (c *RealtimeClient) startHeartbeat() {
	c.stopHeartbeat()
	stop := make(chan struct{})
	c.mu.Lock()
	c.heartbeatStop = stop
	c.mu.Unlock()
	interval := c.cfg.Realtime.HeartbeatInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	c.mu.Lock()
	c.heartbeatTimer = time.AfterFunc(interval, func() { c.heartbeatLoop(stop, interval) })
	c.mu.Unlock()
}

func (c *RealtimeClient) heartbeatLoop(stop chan struct{}, interval time.Duration) {
	select {
	case <-stop:
		return
	case <-c.done:
		return
	default:
	}
	packet := &v1.Packet{Cmd: v1.Command_CMD_HEARTBEAT, Seq: c.nextSeq()}
	_ = c.writePacket(packet)
	c.mu.Lock()
	if c.heartbeatStop == stop {
		c.heartbeatTimer = time.AfterFunc(interval, func() { c.heartbeatLoop(stop, interval) })
	}
	c.mu.Unlock()
}

func (c *RealtimeClient) stopHeartbeat() {
	c.mu.Lock()
	if c.heartbeatStop != nil {
		close(c.heartbeatStop)
		c.heartbeatStop = nil
	}
	if c.heartbeatTimer != nil {
		c.heartbeatTimer.Stop()
		c.heartbeatTimer = nil
	}
	c.mu.Unlock()
}

func (c *RealtimeClient) sendAck(messageID int64, topic string, seq uint64) {
	packet := &v1.Packet{
		Cmd: v1.Command_CMD_ACK, Seq: c.nextSeq(),
		Payload: &v1.Packet_AckReq{AckReq: &v1.AckRequest{MsgId: messageID, Topic: topic, TopicSeq: seq}},
	}
	_ = c.writePacket(packet)
}

func (c *RealtimeClient) SendReadReceipt(topic string, upToSeq uint64) error {
	packet := &v1.Packet{
		Cmd: v1.Command_CMD_READ_RECEIPT, Seq: c.nextSeq(),
		Payload: &v1.Packet_ReadReceipt{ReadReceipt: &v1.ReadReceipt{Topic: topic, UpToSeq: upToSeq}},
	}
	return c.writePacket(packet)
}

func (c *RealtimeClient) Recall(topic string, topicSeq uint64, messageID int64) error {
	packet := &v1.Packet{
		Cmd: v1.Command_CMD_RECALL, Seq: c.nextSeq(),
		Payload: &v1.Packet_RecallReq{RecallReq: &v1.RecallRequest{Topic: topic, TopicSeq: topicSeq, MsgId: messageID}},
	}
	return c.writePacket(packet)
}

func (c *RealtimeClient) request(ctx context.Context, packet *v1.Packet, timeout time.Duration) (*v1.Packet, error) {
	ch := make(chan *v1.Packet, 1)
	c.mu.Lock()
	c.pending[packet.GetSeq()] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, packet.GetSeq())
		c.mu.Unlock()
	}()
	if err := c.writePacket(packet); err != nil {
		return nil, err
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, fmt.Errorf("%w: request timeout", ErrRequestTimeout)
	case reply := <-ch:
		if reply.GetError() != nil {
			return nil, &APIError{Code: fmt.Sprintf("%d", reply.GetError().GetCode()), Message: reply.GetError().GetMessage(), Retryable: reply.GetError().GetRetryable()}
		}
		return reply, nil
	}
}

func (c *RealtimeClient) writePacket(packet *v1.Packet) error {
	data, err := proto.Marshal(packet)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.mu.Lock()
	conn := c.ws
	c.mu.Unlock()
	if conn == nil {
		return ErrNotConnected
	}
	return conn.WriteMessage(websocket.BinaryMessage, data)
}

func (c *RealtimeClient) pendingFor(seq uint64) (chan *v1.Packet, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch, ok := c.pending[seq]
	return ch, ok
}

func (c *RealtimeClient) nextSeq() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seq++
	return c.seq
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
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closed = true
		for id, ch := range c.subs {
			delete(c.subs, id)
			close(ch)
		}
		close(c.done)
		c.mu.Unlock()
	})
}

// Close stops reconnect/heartbeat and closes the WebSocket.
func (c *RealtimeClient) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	conn := c.ws
	c.ws = nil
	if c.timer != nil {
		c.timer.Stop()
	}
	c.mu.Unlock()
	c.stopHeartbeat()
	c.closeEvents()
	if conn != nil {
		return conn.Close()
	}
	return nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
