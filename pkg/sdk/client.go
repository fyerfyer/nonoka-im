package sdk

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	v1 "nonoka-im/api/im/v1"

	"github.com/google/uuid"
)

// Client is the main entry point of the SDK.
// It composes Layer 1 (Service), Layer 2 (Realtime), and Layer 3 (Application).
type Client struct {
	opts Options

	// Layer 1: HTTP services (can be used independently)
	Auth     *AuthService
	Message  *MessageService
	Dispatch *DispatchService

	// Layer 2: Realtime
	Realtime *RealtimeClient

	// Layer 3: Application
	Conversations *ConversationManager

	svc *serviceClient

	// Send deduplication cache: clientMsgID -> result
	sendCache   map[string]*sendCacheEntry
	sendCacheMu sync.RWMutex

	// In-flight messages for state machine tracking
	sendingMsgs map[string]*Message // clientMsgID -> Message
	sendingMu   sync.RWMutex

	// User-level message handler (called for every message, including those routed to Conversations)
	onMessage MessageHandler

	// Background cleanup
	stopCh chan struct{}

	// closeOnce ensures Close is idempotent.
	closeOnce sync.Once
}

type sendCacheEntry struct {
	result *SendResult
	err    error
	time   time.Time
}

const (
	sendCacheTTL             = 30 * time.Second
	maxSendRetries           = 3
	sendCacheMaxSize         = 10000
	sendCacheCleanupInterval = 60 * time.Second
)

// NewClient creates a new IM SDK client with the given options.
// If BaseURL is provided, the HTTP service layer (Auth, Message, Dispatch)
// is initialized immediately and can be used before Connect().
func NewClient(opts Options) *Client {
	opts = opts.withDefaults()

	client := &Client{
		opts:        opts,
		sendCache:   make(map[string]*sendCacheEntry),
		sendingMsgs: make(map[string]*Message),
		stopCh:      make(chan struct{}),
	}

	// Start background sendCache cleanup goroutine.
	go client.sendCacheCleanupLoop()

	// Initialize HTTP service layer immediately if BaseURL is available.
	// This allows standalone use of Auth.Register/Login before Connect().
	if opts.BaseURL != "" {
		if svc, err := newServiceClient(opts.BaseURL, opts.RequestTimeout, opts.Token); err == nil {
			client.svc = svc
			client.Auth = newAuthService(svc)
			client.Message = newMessageService(svc)
			client.Dispatch = newDispatchService(svc)
		}
	}

	client.Conversations = newConversationManager(client)

	return client
}

// Connect establishes connection to the gateway.
// It initializes the service layer (if not already done in NewClient), resolves
// the gateway URL if needed, and starts the realtime client.
func (c *Client) Connect(ctx context.Context) error {
	if c.Realtime != nil && c.Realtime.IsConnected() {
		return ErrAlreadyConnected
	}

	// Derive BaseURL from GatewayURL if not explicitly provided.
	// e.g. ws://host:port/ws -> http://host:port
	baseURL := c.opts.BaseURL
	if baseURL == "" && c.opts.GatewayURL != "" {
		baseURL = deriveBaseURLFromGateway(c.opts.GatewayURL)
	}

	// Initialize HTTP service layer if not already initialized in NewClient.
	if c.svc == nil && baseURL != "" {
		svc, err := newServiceClient(baseURL, c.opts.RequestTimeout, c.opts.Token)
		if err != nil {
			return fmt.Errorf("init service layer: %w", err)
		}
		c.svc = svc
		c.Auth = newAuthService(svc)
		c.Message = newMessageService(svc)
		c.Dispatch = newDispatchService(svc)
	}

	// Resolve gateway URL automatically if not provided
	gatewayURL := c.opts.GatewayURL
	if gatewayURL == "" && c.opts.Token != "" {
		userID, err := extractUserIDFromToken(c.opts.Token)
		if err == nil && userID > 0 {
			url, dispatchErr := c.Dispatch.GetGateway(ctx, userID)
			if dispatchErr == nil && url != "" {
				gatewayURL = url
			}
		}
	}
	if gatewayURL == "" {
		gatewayURL = c.opts.GatewayURL
	}

	// Wire up user-level message handler from options.
	if c.opts.OnMessage != nil {
		c.onMessage = c.opts.OnMessage
	}

	// Initialize realtime layer
	rtOpts := RealtimeOptions{
		GatewayURL:           gatewayURL,
		Token:                c.opts.Token,
		DeviceID:             c.opts.DeviceID,
		HeartbeatInterval:    c.opts.HeartbeatInterval,
		HeartbeatTimeout:     c.opts.HeartbeatTimeout,
		RequestTimeout:       c.opts.RequestTimeout,
		ReconnectInterval:    c.opts.ReconnectInterval,
		AutoReconnect:        c.opts.AutoReconnect,
		MaxReconnectAttempts: c.opts.MaxReconnectAttempts,
		AutoAck:              c.opts.AutoAck,
		OnMessage:            c.handleIncomingMessage,
		OnDisconnect:         c.opts.OnDisconnect,
		OnConnect:            c.opts.OnConnect,
		OnReadReceipt:        c.handleReadReceipt,
		OnDeliveryReceipt:    c.handleDeliveryReceipt,
	}
	c.Realtime = NewRealtimeClient(rtOpts)
	c.Realtime.setOnReconnect(func() {
		c.pullOfflineForConversations()
	})

	if err := c.Realtime.Connect(ctx); err != nil {
		if c.svc != nil {
			_ = c.svc.close()
			c.svc = nil
		}
		return err
	}

	return nil
}

// MsgTypeText is the message type for text messages.
const (
	MsgTypeText  = v1.MsgType_MSG_TYPE_TEXT
	MsgTypeImage = v1.MsgType_MSG_TYPE_IMAGE
	MsgTypeFile  = v1.MsgType_MSG_TYPE_FILE
	MsgTypeVoice = v1.MsgType_MSG_TYPE_VOICE
)

// SendText sends a text message to the specified topic.
func (c *Client) SendText(ctx context.Context, topic string, text string) (*SendResult, error) {
	return c.SendMessage(ctx, topic, MsgTypeText, []byte(text))
}

// SendImage sends an image message to the specified topic.
func (c *Client) SendImage(ctx context.Context, topic string, imageURL string) (*SendResult, error) {
	return c.SendMessage(ctx, topic, MsgTypeImage, []byte(imageURL))
}

// SendFile sends a file message to the specified topic.
func (c *Client) SendFile(ctx context.Context, topic string, fileURL string) (*SendResult, error) {
	return c.SendMessage(ctx, topic, MsgTypeFile, []byte(fileURL))
}

// SendMessage sends a message to the specified topic.
// It tries WebSocket first; if not connected, it falls back to HTTP.
// It automatically generates client_msg_id, deduplicates recent sends, and retries on transient errors.
func (c *Client) SendMessage(ctx context.Context, topic string, msgType v1.MsgType, content []byte) (*SendResult, error) {
	clientMsgID := uuid.New().String()

	// 1. Client-side deduplication: if the same client_msg_id was recently sent, return cached result.
	// Note: clientMsgID is freshly generated above, so this check primarily benefits explicit retries.
	if cached := c.getSendCache(clientMsgID); cached != nil {
		return cached.result, cached.err
	}

	// 2. Track in-flight message with Sending status
	msg := &Message{
		Topic:       topic,
		SenderID:    c.UserID(),
		MsgType:     msgType,
		Content:     content,
		Timestamp:   time.Now().Unix(),
		Status:      MessageStatusSending,
		ClientMsgID: clientMsgID,
	}
	c.trackSendingMsg(msg)

	// 3. Try send with retry
	var lastErr error
	for attempt := 0; attempt <= maxSendRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<(attempt-1)) * 500 * time.Millisecond
			if backoff > 5*time.Second {
				backoff = 5 * time.Second
			}
			select {
			case <-ctx.Done():
				c.updateMsgStatus(clientMsgID, MessageStatusFailed)
				c.setSendCache(clientMsgID, nil, ctx.Err())
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		result, err := c.trySendOnce(ctx, topic, msgType, content, clientMsgID)
		if err == nil {
			c.updateMsgStatus(clientMsgID, MessageStatusSent)
			c.setSendCache(clientMsgID, result, nil)
			c.untrackSendingMsg(clientMsgID)
			return result, nil
		}

		lastErr = err

		// Don't retry on context cancellation or auth failures
		if ctx.Err() != nil {
			break
		}
		if _, ok := IsServerError(err); ok {
			break
		}
	}

	c.updateMsgStatus(clientMsgID, MessageStatusFailed)
	c.setSendCache(clientMsgID, nil, lastErr)
	c.untrackSendingMsg(clientMsgID)
	return nil, lastErr
}

// SendMessageWithMentions sends a message with @mentions.
func (c *Client) SendMessageWithMentions(ctx context.Context, topic string, msgType v1.MsgType, content []byte, mentionedUserIDs []int64) (*SendResult, error) {
	clientMsgID := uuid.New().String()

	msg := &Message{
		Topic:       topic,
		SenderID:    c.UserID(),
		MsgType:     msgType,
		Content:     content,
		Timestamp:   time.Now().Unix(),
		Status:      MessageStatusSending,
		ClientMsgID: clientMsgID,
	}
	c.trackSendingMsg(msg)

	var lastErr error
	for attempt := 0; attempt <= maxSendRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<(attempt-1)) * 500 * time.Millisecond
			if backoff > 5*time.Second {
				backoff = 5 * time.Second
			}
			select {
			case <-ctx.Done():
				c.updateMsgStatus(clientMsgID, MessageStatusFailed)
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		var result *SendResult
		var err error
		if c.Realtime != nil && c.Realtime.IsAuthed() {
			result, err = c.Realtime.SendMessage(ctx, topic, msgType, content, clientMsgID)
		} else if c.Message != nil {
			result, err = c.Message.SendMessage(ctx, &SendMessageRequest{
				Topic:            topic,
				MsgType:          msgType,
				Content:          content,
				ClientMsgID:      clientMsgID,
				MentionedUserIDs: mentionedUserIDs,
			})
		} else {
			err = ErrNotConnected
		}

		if err == nil {
			c.updateMsgStatus(clientMsgID, MessageStatusSent)
			c.untrackSendingMsg(clientMsgID)
			return result, nil
		}

		lastErr = err
		if ctx.Err() != nil {
			break
		}
		if _, ok := IsServerError(err); ok {
			break
		}
	}

	c.updateMsgStatus(clientMsgID, MessageStatusFailed)
	c.untrackSendingMsg(clientMsgID)
	return nil, lastErr
}

// trySendOnce attempts one send via WebSocket or HTTP fallback.
func (c *Client) trySendOnce(ctx context.Context, topic string, msgType v1.MsgType, content []byte, clientMsgID string) (*SendResult, error) {
	// Prefer WebSocket when connected and authed
	if c.Realtime != nil && c.Realtime.IsAuthed() {
		return c.Realtime.SendMessage(ctx, topic, msgType, content, clientMsgID)
	}

	// Fallback to HTTP
	if c.Message != nil {
		return c.Message.SendMessage(ctx, &SendMessageRequest{
			Topic:       topic,
			MsgType:     msgType,
			Content:     content,
			ClientMsgID: clientMsgID,
		})
	}

	return nil, ErrNotConnected
}

// PullMessages pulls offline messages for a topic starting from lastSeq.
// This is the public API; internal routing uses pullMessagesInternal.
func (c *Client) PullMessages(ctx context.Context, topic string, lastSeq uint64, limit int32) (*PullResult, error) {
	return c.pullMessagesInternal(ctx, topic, lastSeq, limit)
}

// pullMessagesInternal pulls messages via Realtime or HTTP fallback.
func (c *Client) pullMessagesInternal(ctx context.Context, topic string, lastSeq uint64, limit int32) (*PullResult, error) {
	if c.Realtime != nil && c.Realtime.IsAuthed() {
		return c.Realtime.PullMessages(ctx, topic, lastSeq, limit)
	}
	// HTTP fallback: use MessageService.PullMessages when WebSocket is unavailable.
	if c.Message != nil {
		return c.Message.PullMessages(ctx, &PullMessagesRequest{
			Topic:   topic,
			LastSeq: lastSeq,
			Limit:   limit,
		})
	}
	return nil, ErrNotConnected
}

// OnMessage registers a global handler for all incoming messages.
func (c *Client) OnMessage(handler MessageHandler) {
	c.onMessage = handler
}

// UserID returns the authenticated user ID (0 if not authenticated).
func (c *Client) UserID() int64 {
	if c.Realtime != nil {
		return c.Realtime.UserID()
	}
	return 0
}

// IsConnected returns true if the realtime client is connected.
func (c *Client) IsConnected() bool {
	if c.Realtime != nil {
		return c.Realtime.IsConnected()
	}
	return false
}

// IsAuthed returns true if the realtime client is authenticated.
func (c *Client) IsAuthed() bool {
	if c.Realtime != nil {
		return c.Realtime.IsAuthed()
	}
	return false
}

// UpdateToken updates the authentication token and triggers reconnection
// if already connected. This avoids the need to recreate the entire client
// when the token changes (e.g., after login).
func (c *Client) UpdateToken(token string) {
	c.opts.Token = token
	if c.Realtime != nil {
		c.Realtime.UpdateToken(token)
	}
}

// Close closes all client resources. It is safe to call multiple times.
func (c *Client) Close() error {
	var errs []error
	// Signal cleanup goroutine to stop (idempotent via closeOnce)
	c.closeOnce.Do(func() {
		if c.stopCh != nil {
			close(c.stopCh)
		}
	})
	if c.Realtime != nil {
		if err := c.Realtime.Close(); err != nil {
			errs = append(errs, err)
		}
		c.Realtime = nil
	}
	if c.svc != nil {
		if err := c.svc.close(); err != nil {
			errs = append(errs, err)
		}
		c.svc = nil
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// sendCacheCleanupLoop periodically cleans up expired entries from sendCache.
func (c *Client) sendCacheCleanupLoop() {
	ticker := time.NewTicker(sendCacheCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.cleanupSendCache()
		case <-c.stopCh:
			return
		}
	}
}

// cleanupSendCache removes expired entries and enforces max size limit.
func (c *Client) cleanupSendCache() {
	c.sendCacheMu.Lock()
	defer c.sendCacheMu.Unlock()

	now := time.Now()
	for id, entry := range c.sendCache {
		if now.Sub(entry.time) > sendCacheTTL {
			delete(c.sendCache, id)
		}
	}

	// If still over max size, remove oldest entries
	if len(c.sendCache) > sendCacheMaxSize {
		// Collect entries with their IDs
		type item struct {
			id   string
			time time.Time
		}
		items := make([]item, 0, len(c.sendCache))
		for id, entry := range c.sendCache {
			items = append(items, item{id: id, time: entry.time})
		}
		// Sort by time ascending (oldest first)
		sort.Slice(items, func(i, j int) bool {
			return items[i].time.Before(items[j].time)
		})
		// Remove oldest until under limit
		toRemove := len(c.sendCache) - sendCacheMaxSize
		for i := 0; i < toRemove && i < len(items); i++ {
			delete(c.sendCache, items[i].id)
		}
	}
}

// handleIncomingMessage processes a server push message.
func (c *Client) handleIncomingMessage(msg *Message) {
	// Route to conversation manager
	if c.Conversations != nil {
		c.Conversations.onMessage(msg)
	}
	// Also invoke user-level handler
	if c.onMessage != nil {
		c.onMessage(msg)
	}
}

// handleReadReceipt processes a server-pushed read receipt.
// It updates the status of local messages to Read and invokes user callbacks.
func (c *Client) handleReadReceipt(topic string, upToSeq uint64, readerID int64) {
	// Update in-flight messages: collect matches under lock, then update outside lock.
	var toUpdate []*Message
	c.sendingMu.Lock()
	for _, msg := range c.sendingMsgs {
		if msg.Topic == topic && msg.TopicSeq > 0 && msg.TopicSeq <= upToSeq {
			toUpdate = append(toUpdate, msg)
		}
	}
	c.sendingMu.Unlock()
	for _, msg := range toUpdate {
		msg.Status = MessageStatusRead
	}

	// Update conversation messages
	if c.Conversations != nil {
		conv := c.Conversations.Get(topic)
		if conv != nil {
			conv.updateMessageStatus(upToSeq, MessageStatusRead)
			if conv.OnReadReceipt != nil {
				conv.OnReadReceipt(upToSeq)
			}
		}
	}

	// Invoke user-level handler
	if c.opts.OnReadReceipt != nil {
		go c.opts.OnReadReceipt(topic, upToSeq, readerID)
	}
}

// handleDeliveryReceipt processes a server-pushed delivery receipt.
// It updates the status of local messages to Delivered and invokes user callbacks.
func (c *Client) handleDeliveryReceipt(topic string, topicSeq uint64, msgID int64) {
	// Update in-flight messages: collect matches under lock, then update outside lock.
	var toUpdate []*Message
	c.sendingMu.Lock()
	for _, msg := range c.sendingMsgs {
		if msg.Topic == topic && msg.TopicSeq == topicSeq {
			toUpdate = append(toUpdate, msg)
		}
	}
	c.sendingMu.Unlock()
	for _, msg := range toUpdate {
		msg.Status = MessageStatusDelivered
	}

	// Update conversation messages
	if c.Conversations != nil {
		conv := c.Conversations.Get(topic)
		if conv != nil {
			conv.updateSingleMessageStatus(topicSeq, MessageStatusDelivered)
		}
	}

	// Invoke user-level handler
	if c.opts.OnDeliveryReceipt != nil {
		go c.opts.OnDeliveryReceipt(topic, topicSeq, msgID)
	}
}

// pullOfflineForConversations is invoked after successful reconnection.
// It logs any errors from pulling offline messages so the application
// can be aware of failures (#9).
func (c *Client) pullOfflineForConversations() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if c.Conversations == nil {
		return
	}
	errs := c.Conversations.pullOfflineForAll(ctx)
	if len(errs) > 0 {
		for topic, err := range errs {
			// Log the error; application layer can monitor via OnDisconnect or logs.
			_ = fmt.Errorf("offline pull failed: topic=%s: %v", topic, err)
		}
		if c.opts.OnDisconnect != nil {
			c.opts.OnDisconnect(fmt.Errorf("offline message pull failed for %d topics", len(errs)))
		}
	}
}

// trackSendingMsg records an in-flight message.
func (c *Client) trackSendingMsg(msg *Message) {
	c.sendingMu.Lock()
	c.sendingMsgs[msg.ClientMsgID] = msg
	c.sendingMu.Unlock()
}

// untrackSendingMsg removes an in-flight message.
func (c *Client) untrackSendingMsg(clientMsgID string) {
	c.sendingMu.Lock()
	delete(c.sendingMsgs, clientMsgID)
	c.sendingMu.Unlock()
}

// updateMsgStatus updates the status of an in-flight message.
func (c *Client) updateMsgStatus(clientMsgID string, status MessageStatus) {
	c.sendingMu.Lock()
	defer c.sendingMu.Unlock()
	if msg, ok := c.sendingMsgs[clientMsgID]; ok {
		msg.Status = status
	}
}

// getSendCache returns a cached send result if still valid.
func (c *Client) getSendCache(clientMsgID string) *sendCacheEntry {
	c.sendCacheMu.RLock()
	entry, ok := c.sendCache[clientMsgID]
	c.sendCacheMu.RUnlock()
	if !ok {
		return nil
	}
	if time.Since(entry.time) > sendCacheTTL {
		c.sendCacheMu.Lock()
		delete(c.sendCache, clientMsgID)
		c.sendCacheMu.Unlock()
		return nil
	}
	return entry
}

// setSendCache stores a send result with TTL.
func (c *Client) setSendCache(clientMsgID string, result *SendResult, err error) {
	c.sendCacheMu.Lock()
	c.sendCache[clientMsgID] = &sendCacheEntry{
		result: result,
		err:    err,
		time:   time.Now(),
	}
	c.sendCacheMu.Unlock()
}

// deriveBaseURLFromGateway converts a WebSocket gateway URL to an HTTP base URL.
// e.g. ws://127.0.0.1:18000/ws -> http://127.0.0.1:18000
// e.g. wss://example.com/ws -> https://example.com
func deriveBaseURLFromGateway(gatewayURL string) string {
	// Replace ws:// with http:// and wss:// with https://
	base := gatewayURL
	if strings.HasPrefix(base, "wss://") {
		base = "https://" + base[len("wss://"):]
	} else if strings.HasPrefix(base, "ws://") {
		base = "http://" + base[len("ws://"):]
	}
	// Remove path (e.g. /ws)
	if idx := strings.Index(base, "/"); idx > len("https://") {
		base = base[:idx]
	}
	return base
}

// extractUserIDFromToken is a best-effort helper to extract user_id from a JWT token
// without validation. It allows automatic gateway dispatch when GatewayURL is empty.
func extractUserIDFromToken(token string) (int64, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid token format")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, fmt.Errorf("decode payload: %w", err)
	}

	var claims struct {
		UserID float64 `json:"user_id"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return 0, fmt.Errorf("unmarshal claims: %w", err)
	}

	if claims.UserID <= 0 {
		return 0, fmt.Errorf("invalid or missing user_id")
	}

	return int64(claims.UserID), nil
}
