package gateway

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	v1 "nonoka-im/api/im/v1"
	"google.golang.org/protobuf/proto"
)

// ConnState represents the current state of a connection.
type ConnState int32

const (
	ConnStateConnected ConnState = iota // connected but not authenticated
	ConnStateAuthed                     // authenticated
	ConnStateClosed                     // closed
)

// Connection wraps a WebSocket connection.
// Each Connection corresponds to one device of a user.
type Connection struct {
	connID   string // unique connection ID
	userID   int64  // filled after authentication
	deviceID string // device identifier

	wsConn *websocket.Conn

	state atomic.Int32

	sendCh chan *v1.Packet

	closeCh   chan struct{}
	closeOnce sync.Once

	lastActive time.Time

	onClose func(c *Connection)

	readTimeout time.Duration
}

// NewConnection creates a new connection wrapper.
func NewConnection(wsConn *websocket.Conn, connID string, readTimeout time.Duration, onClose func(c *Connection)) *Connection {
	c := &Connection{
		wsConn:      wsConn,
		connID:      connID,
		sendCh:      make(chan *v1.Packet, 128),
		closeCh:     make(chan struct{}),
		lastActive:  time.Now(),
		onClose:     onClose,
		readTimeout: readTimeout,
	}
	c.state.Store(int32(ConnStateConnected))
	return c
}

// Start starts the read and write goroutines.
func (c *Connection) Start(readHandler func(*v1.Packet)) {
	go c.readLoop(readHandler)
	go c.writeLoop()
}

// readLoop reads and parses Protobuf packets.
func (c *Connection) readLoop(handler func(*v1.Packet)) {
	defer c.Close()

	for {
		if c.readTimeout > 0 {
			c.wsConn.SetReadDeadline(time.Now().Add(c.readTimeout))
		}

		_, data, err := c.wsConn.ReadMessage()
		if err != nil {
			// Normal close or timeout, no need to log
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				// log handled by caller
			}
			return
		}

		c.lastActive = time.Now()

		var packet v1.Packet
		if err := proto.Unmarshal(data, &packet); err != nil {
			// Protocol parse error, keep reading to allow client retry
			continue
		}

		if handler != nil {
			handler(&packet)
		}
	}
}

// writeLoop sends packets to the client.
func (c *Connection) writeLoop() {
	defer c.Close()

	for {
		select {
		case packet := <-c.sendCh:
			data, err := proto.Marshal(packet)
			if err != nil {
				continue
			}
			if err := c.wsConn.WriteMessage(websocket.BinaryMessage, data); err != nil {
				return
			}

		case <-c.closeCh:
			return
		}
	}
}

// Send sends a packet to the client asynchronously (non-blocking).
// Returns ErrSendChannelFull if the send buffer is full.
func (c *Connection) Send(packet *v1.Packet) error {
	if c.State() == ConnStateClosed {
		return ErrConnectionClosed
	}

	select {
	case c.sendCh <- packet:
		return nil
	case <-c.closeCh:
		return ErrConnectionClosed
	default:
		return ErrSendChannelFull
	}
}

// SendWithTimeout sends a packet with a timeout.
// Use this for critical messages (e.g., ACKs, auth responses) where delivery is important.
func (c *Connection) SendWithTimeout(packet *v1.Packet, timeout time.Duration) error {
	if c.State() == ConnStateClosed {
		return ErrConnectionClosed
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case c.sendCh <- packet:
		return nil
	case <-c.closeCh:
		return ErrConnectionClosed
	case <-timer.C:
		return ErrSendChannelFull
	}
}

// Close closes the connection and cleans up resources.
// Safe to call multiple times; only the first call takes effect.
func (c *Connection) Close() {
	c.closeOnce.Do(func() {
		c.state.Store(int32(ConnStateClosed))
		c.wsConn.Close()
		close(c.closeCh)
		if c.onClose != nil {
			c.onClose(c)
		}
	})
}

// State returns the current connection state.
func (c *Connection) State() ConnState {
	return ConnState(c.state.Load())
}

// SetAuthed marks the connection as authenticated.
func (c *Connection) SetAuthed(userID int64, deviceID string) {
	c.userID = userID
	c.deviceID = deviceID
	c.state.Store(int32(ConnStateAuthed))
}

// UserID returns the associated user ID (0 if not authenticated).
func (c *Connection) UserID() int64 {
	return c.userID
}

// DeviceID returns the device identifier.
func (c *Connection) DeviceID() string {
	return c.deviceID
}

// ConnID returns the unique connection ID.
func (c *Connection) ConnID() string {
	return c.connID
}

// IsIdle checks if the connection has been inactive for the specified duration.
func (c *Connection) IsIdle(timeout time.Duration) bool {
	return time.Since(c.lastActive) > timeout
}

// RefreshActivity refreshes the last active time.
func (c *Connection) RefreshActivity() {
	c.lastActive = time.Now()
}
