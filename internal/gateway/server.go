package gateway

import (
	"net/http"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	v1 "nonoka-im/api/im/v1"
)

// WebSocketServer is the gateway WebSocket server.
type WebSocketServer struct {
	handler *Handler
	log     *log.Helper

	upgrader websocket.Upgrader

	readTimeout time.Duration
}

// NewWebSocketServer creates a new WebSocket server.
func NewWebSocketServer(handler *Handler, logger log.Logger, readTimeout time.Duration) *WebSocketServer {
	if readTimeout <= 0 {
		readTimeout = 60 * time.Second
	}
	return &WebSocketServer{
		handler: handler,
		log:     log.NewHelper(logger),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// Allow all origins for now; restrict in production
				return true
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		readTimeout: readTimeout,
	}
}

// ServeHTTP implements http.Handler for WebSocket upgrade.
func (s *WebSocketServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	wsConn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.log.Warnf("websocket upgrade failed: %v", err)
		return
	}

	connID := uuid.New().String()
	conn := NewConnection(wsConn, connID, s.readTimeout, s.onConnectionClose)

	// Start handling packets
	conn.Start(func(packet *v1.Packet) {
		s.handler.HandlePacket(conn, packet)
	})

	s.log.Infof("websocket connected: conn_id=%s, remote=%s", connID, wsConn.RemoteAddr().String())
}

// onConnectionClose is called when a connection is closed.
func (s *WebSocketServer) onConnectionClose(c *Connection) {
	s.handler.OnConnectionClose(c)
}

// StartIdleChecker starts a background goroutine to close idle connections.
func (s *WebSocketServer) StartIdleChecker(idleTimeout time.Duration, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			s.handler.manager.Range(func(c *Connection) bool {
				if c.IsIdle(idleTimeout) {
					s.log.Infof("closing idle connection: conn_id=%s, user_id=%d", c.ConnID(), c.UserID())
					c.Close()
				}
				return true
			})
		}
	}()
}
