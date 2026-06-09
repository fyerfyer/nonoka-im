package sdk

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	v1 "nonoka-im/api/im/v1"
	"google.golang.org/protobuf/proto"
)

// ReadReceiptHandler is called when a read receipt is received.
type ReadReceiptHandler func(topic string, upToSeq uint64, readerID int64)

// DeliveryReceiptHandler is called when a delivery receipt is received.
type DeliveryReceiptHandler func(topic string, topicSeq uint64, msgID int64)

// RealtimeOptions configures the realtime (WebSocket) client.
type RealtimeOptions struct {
	GatewayURL           string
	Token                string
	DeviceID             string
	HeartbeatInterval    time.Duration
	RequestTimeout       time.Duration
	ReconnectInterval    time.Duration
	AutoReconnect        bool
	MaxReconnectAttempts int
	AutoAck              bool
	OnMessage            MessageHandler
	OnDisconnect         DisconnectHandler
	OnConnect            ConnectHandler
	OnReadReceipt        ReadReceiptHandler
	OnDeliveryReceipt    DeliveryReceiptHandler
}

// RealtimeClient manages WebSocket connection, heartbeat, reconnection, and message handling.
// It can be used independently without the higher-level Application layer.
type RealtimeClient struct {
	opts RealtimeOptions

	// WebSocket
	wsConn *websocket.Conn
	connID string
	state  atomic.Int32 // 0=disconnected, 1=connecting, 2=connected, 3=authed
	userID atomic.Int64

	// Write protection for wsConn
	writeMu sync.Mutex

	// Request-response correlation
	seqGen    atomic.Uint64
	pending   map[uint64]chan *v1.Packet
	pendingMu sync.RWMutex

	// Background goroutine management
	stopCh    chan struct{}
	wg        sync.WaitGroup
	closeOnce sync.Once

	// Message deduplication for push notifications
	seenSeqs   map[string]uint64 // topic -> last seen seq
	seenSeqsMu sync.RWMutex

	// Sending messages tracking (clientMsgID -> send result channel)
	sendingMsgs map[string]chan *SendResult
	sendingMu   sync.RWMutex

	// Reconnect callback (for Application layer to pull offline messages)
	onReconnect func()
}

const (
	rtStateDisconnected  int32 = 0
	rtStateConnecting    int32 = 1
	rtStateConnected     int32 = 2
	rtStateAuthed        int32 = 3
	rtStateReconnecting  int32 = 4
)

// ConnectionState represents the current connection state as a string.
type ConnectionState string

const (
	ConnectionStateDisconnected ConnectionState = "disconnected"
	ConnectionStateConnecting   ConnectionState = "connecting"
	ConnectionStateConnected    ConnectionState = "connected"
	ConnectionStateAuthed       ConnectionState = "authed"
	ConnectionStateReconnecting ConnectionState = "reconnecting"
)

// State returns the current connection state.
func (rt *RealtimeClient) State() ConnectionState {
	switch rt.state.Load() {
	case rtStateConnecting:
		return ConnectionStateConnecting
	case rtStateConnected:
		return ConnectionStateConnected
	case rtStateAuthed:
		return ConnectionStateAuthed
	case rtStateReconnecting:
		return ConnectionStateReconnecting
	default:
		return ConnectionStateDisconnected
	}
}

// IsReconnecting returns true if the client is in the reconnecting state.
func (rt *RealtimeClient) IsReconnecting() bool {
	return rt.state.Load() == rtStateReconnecting
}

// NewRealtimeClient creates a new RealtimeClient with the given options.
func NewRealtimeClient(opts RealtimeOptions) *RealtimeClient {
	if opts.GatewayURL == "" {
		opts.GatewayURL = "ws://localhost:8000/ws"
	}
	if opts.DeviceID == "" {
		opts.DeviceID = "sdk-default"
	}
	if opts.HeartbeatInterval <= 0 {
		opts.HeartbeatInterval = 30 * time.Second
	}
	if opts.RequestTimeout <= 0 {
		opts.RequestTimeout = 10 * time.Second
	}
	if opts.ReconnectInterval <= 0 {
		opts.ReconnectInterval = 5 * time.Second
	}

	return &RealtimeClient{
		opts:        opts,
		pending:     make(map[uint64]chan *v1.Packet),
		seenSeqs:    make(map[string]uint64),
		sendingMsgs: make(map[string]chan *SendResult),
		stopCh:      make(chan struct{}),
	}
}

// Connect establishes a WebSocket connection to the gateway and authenticates.
func (rt *RealtimeClient) Connect(ctx context.Context) error {
	if rt.state.Load() >= rtStateConnected {
		return ErrAlreadyConnected
	}

	if err := rt.connectAndAuth(ctx); err != nil {
		return err
	}

	// Start background workers after successful connection + auth
	rt.wg.Add(2)
	go rt.heartbeatLoop()
	go rt.readLoop()

	// Start reconnection monitor if auto-reconnect is enabled
	if rt.opts.AutoReconnect {
		rt.wg.Add(1)
		go rt.reconnectMonitor()
	}

	return nil
}

// connectAndAuth establishes the WebSocket connection and authenticates.
func (rt *RealtimeClient) connectAndAuth(ctx context.Context) error {
	// Determine if this is a reconnection attempt
	wasAuthed := rt.state.Load() == rtStateAuthed
	targetState := rtStateConnecting
	if wasAuthed {
		targetState = rtStateReconnecting
	}

	if !rt.state.CompareAndSwap(rtStateDisconnected, targetState) {
		current := rt.state.Load()
		if current == rtStateConnecting || current == rtStateReconnecting {
			return ErrAlreadyConnected
		}
		// If already connected/authed, close connection first then reconnect.
		// Use closeConnection instead of Close to avoid wg.Wait deadlock
		// when called from within a goroutine (e.g. reconnectMonitor).
		rt.closeConnection()
		if !rt.state.CompareAndSwap(rtStateDisconnected, targetState) {
			return ErrAlreadyConnected
		}
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	wsConn, _, err := dialer.DialContext(ctx, rt.opts.GatewayURL, nil)
	if err != nil {
		rt.state.Store(rtStateDisconnected)
		return fmt.Errorf("dial gateway failed: %w", err)
	}

	rt.wsConn = wsConn
	rt.connID = uuid.New().String()
	rt.state.Store(rtStateConnected)

	// Authenticate if token is provided
	if rt.opts.Token != "" {
		if err := rt.authenticate(ctx); err != nil {
			rt.wsConn.Close()
			rt.state.Store(rtStateDisconnected)
			return err
		}
	}

	return nil
}

// authenticate sends an auth request and reads the response directly from WebSocket.
func (rt *RealtimeClient) authenticate(ctx context.Context) error {
	seq := rt.nextSeq()
	req := &v1.Packet{
		Cmd: v1.Command_CMD_AUTH,
		Seq: seq,
		Payload: &v1.Packet_AuthReq{
			AuthReq: &v1.AuthRequest{
				Token:    rt.opts.Token,
				DeviceId: rt.opts.DeviceID,
			},
		},
	}

	if err := rt.sendPacket(req); err != nil {
		return err
	}

	// Read auth response directly (readLoop is not running yet)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		rt.wsConn.SetReadDeadline(time.Now().Add(rt.opts.RequestTimeout))
		_, data, err := rt.wsConn.ReadMessage()
		if err != nil {
			return fmt.Errorf("%w: read auth response failed: %v", ErrAuthFailed, err)
		}

		var packet v1.Packet
		if err := proto.Unmarshal(data, &packet); err != nil {
			continue // skip invalid packets
		}

		if packet.Cmd == v1.Command_CMD_HEARTBEAT {
			continue
		}

		if packet.Seq != seq {
			continue
		}

		if err := rt.checkError(&packet); err != nil {
			return fmt.Errorf("%w: %v", ErrAuthFailed, err)
		}

		authResp := packet.GetAuthResp()
		if authResp == nil || !authResp.Success {
			return ErrAuthFailed
		}

		rt.userID.Store(authResp.UserId)
		rt.state.Store(rtStateAuthed)
		if rt.opts.OnConnect != nil {
			go rt.opts.OnConnect()
		}
		return nil
	}
}

// SendMessage sends a message to the specified topic via WebSocket.
// It returns the server ACK with the assigned message metadata.
func (rt *RealtimeClient) SendMessage(ctx context.Context, topic string, msgType v1.MsgType, content []byte, clientMsgID string) (*SendResult, error) {
	if rt.state.Load() != rtStateAuthed {
		return nil, ErrNotConnected
	}

	seq := rt.nextSeq()
	req := &v1.Packet{
		Cmd: v1.Command_CMD_PUBLISH,
		Seq: seq,
		Payload: &v1.Packet_SendReq{
			SendReq: &v1.SendMessageRequest{
				Topic:       topic,
				MsgType:     msgType,
				Content:     content,
				ClientMsgId: clientMsgID,
			},
		},
	}

	respCh := rt.registerPending(seq)
	defer rt.unregisterPending(seq)

	if err := rt.sendPacket(req); err != nil {
		return nil, err
	}

	select {
	case resp := <-respCh:
		if err := rt.checkError(resp); err != nil {
			return nil, err
		}
		reply := resp.GetSendReply()
		if reply == nil {
			return nil, ErrServerError
		}
		return &SendResult{
			ClientMsgID: reply.ClientMsgId,
			MsgID:       reply.MsgId,
			Timestamp:   reply.Timestamp,
			TopicSeq:    reply.TopicSeq,
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(rt.opts.RequestTimeout):
		return nil, ErrRequestTimeout
	}
}

// PullMessages pulls offline messages for a topic starting from lastSeq.
func (rt *RealtimeClient) PullMessages(ctx context.Context, topic string, lastSeq uint64, limit int32) (*PullResult, error) {
	if rt.state.Load() != rtStateAuthed {
		return nil, ErrNotConnected
	}

	seq := rt.nextSeq()
	req := &v1.Packet{
		Cmd: v1.Command_CMD_PULL,
		Seq: seq,
		Payload: &v1.Packet_PullReq{
			PullReq: &v1.PullRequest{
				Topic:   topic,
				LastSeq: lastSeq,
				Limit:   limit,
			},
		},
	}

	respCh := rt.registerPending(seq)
	defer rt.unregisterPending(seq)

	if err := rt.sendPacket(req); err != nil {
		return nil, err
	}

	select {
	case resp := <-respCh:
		if err := rt.checkError(resp); err != nil {
			return nil, err
		}
		reply := resp.GetPullReply()
		if reply == nil {
			return nil, ErrServerError
		}
		return &PullResult{
			Messages: toSDKMessages(reply.Messages),
			HasMore:  reply.HasMore,
			NextSeq:  reply.NextSeq,
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(rt.opts.RequestTimeout):
		return nil, ErrRequestTimeout
	}
}

// Acknowledge sends an ACK for a received message.
func (rt *RealtimeClient) Acknowledge(msgID int64, topic string, topicSeq uint64) error {
	if rt.state.Load() != rtStateAuthed {
		return ErrNotConnected
	}

	seq := rt.nextSeq()
	req := &v1.Packet{
		Cmd: v1.Command_CMD_ACK,
		Seq: seq,
		Payload: &v1.Packet_AckReq{
			AckReq: &v1.AckRequest{
				MsgId:    msgID,
				Topic:    topic,
				TopicSeq: topicSeq,
			},
		},
	}

	return rt.sendPacket(req)
}

// SendReadReceipt sends a read receipt to the server for messages up to the given seq.
func (rt *RealtimeClient) SendReadReceipt(topic string, upToSeq uint64) error {
	if rt.state.Load() != rtStateAuthed {
		return ErrNotConnected
	}

	seq := rt.nextSeq()
	req := &v1.Packet{
		Cmd: v1.Command_CMD_READ_RECEIPT,
		Seq: seq,
		Payload: &v1.Packet_ReadReceipt{
			ReadReceipt: &v1.ReadReceipt{
				Topic:    topic,
				UpToSeq:  upToSeq,
				ReaderId: rt.UserID(),
			},
		},
	}

	return rt.sendPacket(req)
}

// UserID returns the authenticated user ID (0 if not authenticated).
func (rt *RealtimeClient) UserID() int64 {
	return rt.userID.Load()
}

// IsConnected returns true if the client is connected (but not necessarily authed).
func (rt *RealtimeClient) IsConnected() bool {
	return rt.state.Load() >= rtStateConnected
}

// IsAuthed returns true if the client is authenticated.
func (rt *RealtimeClient) IsAuthed() bool {
	return rt.state.Load() == rtStateAuthed
}

// Close closes the client connection and stops all background goroutines.
func (rt *RealtimeClient) Close() error {
	rt.closeOnce.Do(func() {
		close(rt.stopCh)
		rt.closeConnection()
	})
	rt.wg.Wait()
	return nil
}

// closeConnection closes the WebSocket connection without stopping background workers.
func (rt *RealtimeClient) closeConnection() {
	if rt.wsConn != nil {
		rt.wsConn.Close()
	}
	rt.state.Store(rtStateDisconnected)
}

// sendPacket marshals and sends a packet over WebSocket.
// It holds the writeMu for the entire check-write sequence to avoid races with Close.
func (rt *RealtimeClient) sendPacket(packet *v1.Packet) error {
	data, err := proto.Marshal(packet)
	if err != nil {
		return fmt.Errorf("marshal packet: %w", err)
	}

	rt.writeMu.Lock()
	defer rt.writeMu.Unlock()

	conn := rt.wsConn
	if conn == nil {
		return ErrNotConnected
	}

	if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		return fmt.Errorf("write websocket: %w", err)
	}
	return nil
}

// nextSeq generates the next sequence number for request-response correlation.
func (rt *RealtimeClient) nextSeq() uint64 {
	return rt.seqGen.Add(1)
}

// registerPending registers a response channel for the given seq.
func (rt *RealtimeClient) registerPending(seq uint64) chan *v1.Packet {
	ch := make(chan *v1.Packet, 1)
	rt.pendingMu.Lock()
	rt.pending[seq] = ch
	rt.pendingMu.Unlock()
	return ch
}

// unregisterPending removes the pending response channel.
func (rt *RealtimeClient) unregisterPending(seq uint64) {
	rt.pendingMu.Lock()
	delete(rt.pending, seq)
	rt.pendingMu.Unlock()
}

// dispatchResponse routes a response packet to its pending request channel.
func (rt *RealtimeClient) dispatchResponse(packet *v1.Packet) {
	rt.pendingMu.RLock()
	ch, ok := rt.pending[packet.Seq]
	rt.pendingMu.RUnlock()
	if ok {
		select {
		case ch <- packet:
		default:
		}
	}
}

// checkError checks if a packet contains an error response.
func (rt *RealtimeClient) checkError(packet *v1.Packet) error {
	errResp := packet.GetError()
	if errResp == nil {
		return nil
	}
	return &ServerError{
		Code:       errResp.Code,
		Message:    errResp.Message,
		Retryable:  errResp.Retryable,
		RetryAfter: time.Duration(errResp.RetryAfterMs) * time.Millisecond,
	}
}

// heartbeatLoop sends periodic heartbeats to keep the connection alive.
func (rt *RealtimeClient) heartbeatLoop() {
	defer rt.wg.Done()

	ticker := time.NewTicker(rt.opts.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if rt.state.Load() < rtStateConnected {
				continue
			}
			seq := rt.nextSeq()
			req := &v1.Packet{
				Cmd: v1.Command_CMD_HEARTBEAT,
				Seq: seq,
			}
			if err := rt.sendPacket(req); err != nil {
				_ = err
			}
		case <-rt.stopCh:
			return
		}
	}
}

// readLoop reads packets from the WebSocket connection.
func (rt *RealtimeClient) readLoop() {
	defer rt.wg.Done()

	for {
		select {
		case <-rt.stopCh:
			return
		default:
		}

		if rt.wsConn == nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		_, data, err := rt.wsConn.ReadMessage()
		if err != nil {
			if rt.state.Load() >= rtStateConnected {
				rt.state.Store(rtStateDisconnected)
				if rt.opts.OnDisconnect != nil {
					go rt.opts.OnDisconnect(fmt.Errorf("websocket read error: %w", err))
				}
			}
			time.Sleep(rt.opts.ReconnectInterval)
			continue
		}

		var packet v1.Packet
		if err := proto.Unmarshal(data, &packet); err != nil {
			continue
		}

		rt.handlePacket(&packet)
	}
}

// handlePacket processes incoming packets.
func (rt *RealtimeClient) handlePacket(packet *v1.Packet) {
	switch packet.Cmd {
	case v1.Command_CMD_HEARTBEAT:
		// Heartbeat echo from server, no action needed

	case v1.Command_CMD_AUTH:
		rt.dispatchResponse(packet)

	case v1.Command_CMD_PUBLISH:
		rt.dispatchResponse(packet)

	case v1.Command_CMD_PULL:
		rt.dispatchResponse(packet)

	case v1.Command_CMD_NOTIFY:
		rt.handlePush(packet)

	case v1.Command_CMD_ACK:
		rt.dispatchResponse(packet)

	case v1.Command_CMD_READ_RECEIPT:
		rt.handleReadReceipt(packet)

	case v1.Command_CMD_DELIVERY_RECEIPT:
		rt.handleDeliveryReceipt(packet)

	default:
		rt.dispatchResponse(packet)
	}
}

// handleReadReceipt processes server-pushed read receipts.
func (rt *RealtimeClient) handleReadReceipt(packet *v1.Packet) {
	receipt := packet.GetReadReceipt()
	if receipt == nil {
		return
	}

	if rt.opts.OnReadReceipt != nil {
		go rt.opts.OnReadReceipt(receipt.Topic, receipt.UpToSeq, receipt.ReaderId)
	}
}

// handleDeliveryReceipt processes server-pushed delivery receipts.
func (rt *RealtimeClient) handleDeliveryReceipt(packet *v1.Packet) {
	receipt := packet.GetDeliveryReceipt()
	if receipt == nil {
		return
	}

	if rt.opts.OnDeliveryReceipt != nil {
		go rt.opts.OnDeliveryReceipt(receipt.Topic, receipt.TopicSeq, receipt.MsgId)
	}
}

// handlePush handles server push notifications.
func (rt *RealtimeClient) handlePush(packet *v1.Packet) {
	notify := packet.GetNotify()
	if notify == nil {
		// Defensive: the packet cmd says NOTIFY but payload is not a MessagePush.
		// This should not happen with well-formed server packets.
		return
	}
	_ = notify.Topic // silence unused warning if any

	// Deduplication: skip if we've already seen this seq for this topic
	rt.seenSeqsMu.RLock()
	lastSeen, ok := rt.seenSeqs[notify.Topic]
	rt.seenSeqsMu.RUnlock()

	if ok && notify.TopicSeq <= lastSeen {
		return
	}

	rt.seenSeqsMu.Lock()
	rt.seenSeqs[notify.Topic] = notify.TopicSeq
	rt.seenSeqsMu.Unlock()

	// Send ACK if enabled
	if rt.opts.AutoAck && rt.state.Load() == rtStateAuthed {
		_ = rt.Acknowledge(notify.MsgId, notify.Topic, notify.TopicSeq)
	}

	// Notify user handler
	if rt.opts.OnMessage != nil {
		go rt.opts.OnMessage(toSDKMessage(notify))
	}
}

// reconnectMonitor monitors the connection and reconnects if needed.
func (rt *RealtimeClient) reconnectMonitor() {
	defer rt.wg.Done()

	reconnectAttempts := 0
	for {
		select {
		case <-rt.stopCh:
			return
		default:
		}

		if rt.state.Load() == rtStateAuthed {
			time.Sleep(rt.opts.ReconnectInterval)
			continue
		}

		if rt.opts.MaxReconnectAttempts > 0 && reconnectAttempts >= rt.opts.MaxReconnectAttempts {
			if rt.opts.OnDisconnect != nil {
				go rt.opts.OnDisconnect(fmt.Errorf("max reconnection attempts exceeded"))
			}
			return
		}

		// Only attempt reconnect if we are in disconnected state.
		// Use CAS to atomically transition to reconnecting.
		if !rt.state.CompareAndSwap(rtStateDisconnected, rtStateReconnecting) {
			// Another goroutine is already handling connection or reconnection.
			time.Sleep(rt.opts.ReconnectInterval)
			continue
		}

		reconnectAttempts++
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		err := rt.connectAndAuth(ctx)
		cancel()

		if err == nil {
			reconnectAttempts = 0
			// Notify Application layer to pull offline messages
			if rt.onReconnect != nil {
				go rt.onReconnect()
			}
		} else {
			time.Sleep(rt.opts.ReconnectInterval)
		}
	}
}

// setOnReconnect sets the callback invoked after successful reconnection.
func (rt *RealtimeClient) setOnReconnect(fn func()) {
	rt.onReconnect = fn
}
