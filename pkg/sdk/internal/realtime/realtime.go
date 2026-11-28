// Package realtime owns the WebSocket connection state machine used by the
// public SDK: dial, auth, heartbeat, reconnect, request/reply correlation and
// raw packet delivery. Public SDK event mapping stays in pkg/sdk.
package realtime

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	v1 "nonoka-im/api/im/v1"
	"nonoka-im/pkg/sdk/internal/transport"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

var (
	ErrNotConnected     = errors.New("sdk: not connected")
	ErrAlreadyConnected = errors.New("sdk: already connected")
	ErrClosed           = errors.New("sdk: client closed")
	ErrAuthFailed       = errors.New("sdk: authentication failed")
	ErrRequestTimeout   = errors.New("sdk: request timeout")
)

// Config controls the connection state machine.
type Config struct {
	GatewayURLs          []string
	Origin               string
	DeviceID             string
	HeartbeatInterval    time.Duration
	HeartbeatTimeout     time.Duration
	ReconnectInterval    time.Duration
	MaxReconnectDelay    time.Duration
	MaxReconnectAttempts int
	DisableAutoReconnect bool
}

// Dispatcher resolves gateway URLs when Config.GatewayURLs is empty.
type Dispatcher interface {
	GatewayURLs(ctx context.Context, token string) ([]string, error)
}

type EventType uint8

const (
	EventConnected EventType = iota
	EventAuthed
	EventDisconnected
	EventReconnecting
	EventPacket
	EventError
)

// Event is one lifecycle or packet notification. Exactly one of Packet/Err is
// set for EventPacket/EventError.
type Event struct {
	Type   EventType
	Packet *v1.Packet
	Err    error
}

// Client is a single WebSocket connection with reconnect and request/reply
// correlation. It does not interpret IM or agent packet payloads.
type Client struct {
	cfg      Config
	tokens   transport.TokenSource
	dispatch Dispatcher

	connectMu sync.Mutex
	mu        sync.Mutex
	ws        *websocket.Conn
	writeMu   sync.Mutex
	seq       uint64
	pending   map[uint64]chan *v1.Packet
	subs      map[uint64]chan Event
	nextSub   uint64
	done      chan struct{}
	closed    bool
	authed    bool

	reconnectAttempt int
	timer            *time.Timer

	heartbeatTimer *time.Timer
	heartbeatStop  chan struct{}
	lastAlive      atomic.Int64
	closeOnce      sync.Once
}

func New(cfg Config, tokens transport.TokenSource, dispatch Dispatcher) *Client {
	return &Client{
		cfg: cfg, tokens: tokens, dispatch: dispatch,
		pending: map[uint64]chan *v1.Packet{}, subs: map[uint64]chan Event{},
		done: make(chan struct{}),
	}
}

// SetTokenSource swaps the token source used for the next connect/auth.
func (c *Client) SetTokenSource(source transport.TokenSource) {
	c.mu.Lock()
	c.tokens = source
	c.mu.Unlock()
}

// Subscribe returns an independent event channel and a cancel function.
func (c *Client) Subscribe() (<-chan Event, func()) {
	c.mu.Lock()
	if c.closed {
		ch := make(chan Event)
		close(ch)
		c.mu.Unlock()
		return ch, func() {}
	}
	c.nextSub++
	id := c.nextSub
	ch := make(chan Event, 256)
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

func (c *Client) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ws != nil && c.authed
}

func (c *Client) State() string {
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
func (c *Client) Connect(ctx context.Context) error {
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

func (c *Client) dial(ctx context.Context, gatewayURL string) (*websocket.Conn, *http.Response, error) {
	header := http.Header{}
	if c.cfg.Origin != "" {
		header.Set("Origin", c.cfg.Origin)
	}
	return websocket.DefaultDialer.DialContext(ctx, gatewayURL, header)
}

func (c *Client) connect(ctx context.Context) error {
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return ErrClosed
	}
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
		conn, _, dialErr := c.dial(ctx, gatewayURL)
		if dialErr != nil {
			lastErr = dialErr
			continue
		}
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			_ = conn.Close()
			return ErrClosed
		}
		c.ws = conn
		c.authed = false
		c.reconnectAttempt = 0
		c.mu.Unlock()
		c.emit(Event{Type: EventConnected})
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

func (c *Client) connectOnce(ctx context.Context, gatewayURL, token string) error {
	conn, _, err := c.dial(ctx, gatewayURL)
	if err != nil {
		return err
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		_ = conn.Close()
		return ErrClosed
	}
	c.ws = conn
	c.authed = false
	c.mu.Unlock()
	c.emit(Event{Type: EventConnected})
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

func (c *Client) markConnected() {
	c.lastAlive.Store(time.Now().UnixNano())
	c.mu.Lock()
	c.reconnectAttempt = 0
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	c.mu.Unlock()
}

func (c *Client) gatewayURLs(ctx context.Context) ([]string, error) {
	if len(c.cfg.GatewayURLs) > 0 {
		return c.cfg.GatewayURLs, nil
	}
	token, err := c.token(ctx)
	if err != nil {
		return nil, err
	}
	if c.dispatch == nil {
		return nil, fmt.Errorf("gateway URLs are required")
	}
	return c.dispatch.GatewayURLs(ctx, token)
}

func (c *Client) token(ctx context.Context) (string, error) {
	c.mu.Lock()
	source := c.tokens
	c.mu.Unlock()
	if source == nil {
		return "", fmt.Errorf("sdk: invalid configuration: token source is required")
	}
	return source.Token(ctx)
}

func (c *Client) refresh(ctx context.Context) (string, error) {
	c.mu.Lock()
	source := c.tokens
	c.mu.Unlock()
	if refreshable, ok := source.(transport.RefreshableTokenSource); ok {
		return refreshable.Refresh(ctx)
	}
	return "", errors.New("token source is not refreshable")
}

func (c *Client) authenticate(ctx context.Context, token string) error {
	packet := &v1.Packet{
		Cmd:     v1.Command_CMD_AUTH,
		Seq:     c.NextSeq(),
		Payload: &v1.Packet_AuthReq{AuthReq: &v1.AuthRequest{Token: token, DeviceId: c.cfg.DeviceID}},
	}
	reply, err := c.Request(ctx, packet, 10*time.Second)
	if err != nil {
		return err
	}
	if reply.GetAuthResp() == nil || !reply.GetAuthResp().GetSuccess() {
		return fmt.Errorf("%w: auth rejected", ErrAuthFailed)
	}
	c.mu.Lock()
	c.authed = true
	c.mu.Unlock()
	c.emit(Event{Type: EventAuthed})
	return nil
}

func (c *Client) readLoop(conn *websocket.Conn) {
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			c.handleDisconnect(conn, err)
			return
		}
		c.lastAlive.Store(time.Now().UnixNano())
		var packet v1.Packet
		if err := proto.Unmarshal(data, &packet); err != nil {
			c.emit(Event{Type: EventError, Err: err})
			continue
		}
		c.handlePacket(&packet)
	}
}

func (c *Client) handlePacket(packet *v1.Packet) {
	if packet.GetCmd() == v1.Command_CMD_HEARTBEAT {
		return
	}
	if ch, ok := c.pendingFor(packet.GetSeq()); ok {
		select {
		case ch <- packet:
		default:
		}
		return
	}
	c.emit(Event{Type: EventPacket, Packet: packet})
}

func (c *Client) handleDisconnect(conn *websocket.Conn, reason error) {
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
	c.emit(Event{Type: EventDisconnected, Err: reason})
	if closed || c.cfg.DisableAutoReconnect {
		return
	}
	c.scheduleReconnect()
}

func (c *Client) scheduleReconnect() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	attempt := c.reconnectAttempt
	c.reconnectAttempt++
	maxAttempts := c.cfg.MaxReconnectAttempts
	c.mu.Unlock()
	if maxAttempts > 0 && attempt >= maxAttempts {
		c.emit(Event{Type: EventError, Err: fmt.Errorf("sdk: max reconnect attempts reached")})
		return
	}
	delay := c.cfg.ReconnectInterval * time.Duration(1<<minInt(attempt, 5))
	if delay > c.cfg.MaxReconnectDelay {
		delay = c.cfg.MaxReconnectDelay
	}
	delay = jitterDelay(delay)
	c.emit(Event{Type: EventReconnecting})
	c.mu.Lock()
	if c.timer != nil {
		c.timer.Stop()
	}
	c.timer = time.AfterFunc(delay, func() {
		c.connectMu.Lock()
		err := c.connect(context.Background())
		c.connectMu.Unlock()
		if err != nil {
			c.emit(Event{Type: EventError, Err: err})
			c.scheduleReconnect()
		}
	})
	c.mu.Unlock()
}

func (c *Client) startHeartbeat() {
	c.stopHeartbeat()
	stop := make(chan struct{})
	c.mu.Lock()
	c.heartbeatStop = stop
	c.mu.Unlock()
	interval := c.cfg.HeartbeatInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	c.mu.Lock()
	c.heartbeatTimer = time.AfterFunc(interval, func() { c.heartbeatLoop(stop, interval) })
	c.mu.Unlock()
}

func (c *Client) heartbeatLoop(stop chan struct{}, interval time.Duration) {
	select {
	case <-stop:
		return
	case <-c.done:
		return
	default:
	}
	if timeout := c.cfg.HeartbeatTimeout; timeout > 0 {
		if last := c.lastAlive.Load(); last > 0 && time.Since(time.Unix(0, last)) > timeout {
			c.mu.Lock()
			conn := c.ws
			c.mu.Unlock()
			if conn != nil {
				c.emit(Event{Type: EventError, Err: fmt.Errorf("sdk: heartbeat timeout")})
				_ = conn.Close()
			}
			return
		}
	}
	_ = c.Send(&v1.Packet{Cmd: v1.Command_CMD_HEARTBEAT, Seq: c.NextSeq()})
	c.mu.Lock()
	if c.heartbeatStop == stop {
		c.heartbeatTimer = time.AfterFunc(interval, func() { c.heartbeatLoop(stop, interval) })
	}
	c.mu.Unlock()
}

func (c *Client) stopHeartbeat() {
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

// Request sends a packet and waits for the reply with the same sequence.
func (c *Client) Request(ctx context.Context, packet *v1.Packet, timeout time.Duration) (*v1.Packet, error) {
	ch := make(chan *v1.Packet, 1)
	c.mu.Lock()
	c.pending[packet.GetSeq()] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, packet.GetSeq())
		c.mu.Unlock()
	}()
	if err := c.Send(packet); err != nil {
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
			return nil, &transport.APIError{
				Code:    fmt.Sprintf("%d", reply.GetError().GetCode()),
				Message: reply.GetError().GetMessage(), Retryable: reply.GetError().GetRetryable(),
			}
		}
		return reply, nil
	}
}

// Send writes one packet to the current WebSocket.
func (c *Client) Send(packet *v1.Packet) error {
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

func (c *Client) pendingFor(seq uint64) (chan *v1.Packet, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch, ok := c.pending[seq]
	return ch, ok
}

func (c *Client) NextSeq() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seq++
	return c.seq
}

func (c *Client) emit(event Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	for _, ch := range c.subs {
		select {
		case ch <- event:
		default:
		}
	}
}

func (c *Client) closeEvents() {
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

// Disconnect closes only the current WebSocket while keeping subscriptions
// and the client open. It is used by reconnect-storm tests and callers that
// want to force a fresh connection.
func (c *Client) Disconnect() error {
	c.mu.Lock()
	conn := c.ws
	c.ws = nil
	c.authed = false
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	c.mu.Unlock()
	c.stopHeartbeat()
	if conn != nil {
		return conn.Close()
	}
	return nil
}

// Close stops reconnect/heartbeat and closes the WebSocket.
func (c *Client) Close() error {
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

// jitterDelay applies full jitter so a gateway restart does not make every
// client reconnect in the same tick. The upper bound remains the configured
// backoff.
func jitterDelay(delay time.Duration) time.Duration {
	if delay <= 0 {
		return 0
	}
	// Equal jitter: keep at least half the backoff so a persistent failure
	// cannot turn into a tight reconnect loop.
	half := delay / 2
	return half + time.Duration(rand.Int64N(int64(delay-half)+1))
}
