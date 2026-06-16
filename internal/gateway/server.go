package gateway

import (
	"net/http"
	"strings"
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

	readTimeout  time.Duration
	writeTimeout time.Duration
}

// NewWebSocketServer creates a new WebSocket server.
// allowedOrigins restricts the Origin header that may initiate a WebSocket
// handshake. If empty, all origins are allowed (convenient for development but
// unsafe for production).
func NewWebSocketServer(handler *Handler, logger log.Logger, readTimeout time.Duration, writeTimeout time.Duration, allowedOrigins []string) *WebSocketServer {
	if readTimeout <= 0 {
		readTimeout = 60 * time.Second
	}
	if writeTimeout <= 0 {
		writeTimeout = 10 * time.Second
	}

	s := &WebSocketServer{
		handler:      handler,
		log:          log.NewHelper(logger),
		readTimeout:  readTimeout,
		writeTimeout: writeTimeout,
	}

	s.upgrader = websocket.Upgrader{
		CheckOrigin:     s.buildCheckOrigin(allowedOrigins),
		ReadBufferSize:  8192,
		WriteBufferSize: 8192,
	}

	if len(allowedOrigins) == 0 {
		s.log.Warn("no allowed_origins configured for WebSocket; accepting all origins (unsafe for production)")
	}

	return s
}

// buildCheckOrigin returns an Origin validation function.
// An empty allowedOrigins list permits every origin. Otherwise only the listed
// origins (matched case-insensitively by suffix) are accepted.
func (s *WebSocketServer) buildCheckOrigin(allowedOrigins []string) func(r *http.Request) bool {
	if len(allowedOrigins) == 0 {
		return func(r *http.Request) bool {
			return true
		}
	}

	// Normalize to lower case for case-insensitive comparison.
	allowed := make([]string, len(allowedOrigins))
	for i, o := range allowedOrigins {
		allowed[i] = strings.ToLower(strings.TrimSpace(o))
	}

	return func(r *http.Request) bool {
		origin := strings.ToLower(r.Header.Get("Origin"))
		if origin == "" {
			// No Origin header provided; reject unless explicitly allowed.
			return false
		}
		for _, a := range allowed {
			if a == origin || strings.HasSuffix(origin, a) {
				return true
			}
		}
		s.log.Warnf("websocket origin rejected: %s", origin)
		return false
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
	conn := NewConnection(wsConn, connID, s.readTimeout, s.writeTimeout, s.onConnectionClose)

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
