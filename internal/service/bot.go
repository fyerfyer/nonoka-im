package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nonoka-im/internal/audit"
	botapp "nonoka-im/internal/bot/app"
	"nonoka-im/internal/bot/domain"
	"nonoka-im/internal/httpapi"

	"github.com/go-kratos/kratos/v2/log"
	jwtMiddleware "github.com/go-kratos/kratos/v2/middleware/auth/jwt"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	jwt5 "github.com/golang-jwt/jwt/v5"
)

// BotHTTP is the transport adapter for external bot management and runtime
// APIs. Business rules live in internal/bot/app.
type BotHTTP struct {
	bots     *botapp.Service
	auth     *botapp.AuthService
	delivery *botapp.DeliveryService
	streams  *botapp.StreamService
	reader   botapp.MessageReader
	sender   botapp.MessageSender
	limiter  botapp.RateLimiter
	log      *log.Helper
}

func NewBotHTTP(bots *botapp.Service, auth *botapp.AuthService, delivery *botapp.DeliveryService, streams *botapp.StreamService, reader botapp.MessageReader, sender botapp.MessageSender, limiter botapp.RateLimiter, logger log.Logger) *BotHTTP {
	return &BotHTTP{
		bots: bots, auth: auth, delivery: delivery, streams: streams,
		reader: reader, sender: sender, limiter: limiter, log: log.NewHelper(logger),
	}
}

const (
	botSendRateLimit   = 120
	botStreamRateLimit = 30
)

type createBotRequest struct {
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	Username           string   `json:"username"`
	EventSubscriptions []string `json:"eventSubscriptions"`
}

type updateBotRequest struct {
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	Status             string   `json:"status"`
	EventSubscriptions []string `json:"eventSubscriptions"`
}

type createTokenRequest struct {
	Name          string `json:"name"`
	ExpiresInDays int    `json:"expiresInDays"`
}

type webhookRequest struct {
	URL     string   `json:"url"`
	Events  []string `json:"events"`
	Enabled *bool    `json:"enabled"`
	Rotate  bool     `json:"rotate"`
}

type sendMessageRequest struct {
	Topic            string    `json:"topic"`
	ClientMsgID      string    `json:"client_msg_id"`
	Content          string    `json:"content"`
	MentionedUserIDs stringIDs `json:"mentioned_user_ids"`
}

type streamCreateRequest struct {
	Topic       string `json:"topic"`
	ClientMsgID string `json:"client_msg_id"`
}

type streamChunkRequest struct {
	Seq   uint64 `json:"seq"`
	Delta string `json:"delta"`
}

type streamFinalizeRequest struct {
	Content string `json:"content"`
}

// stringIDs accepts both JSON strings and numbers for 64-bit identifiers.
type stringIDs []string

func (s *stringIDs) UnmarshalJSON(data []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		var str string
		if err := json.Unmarshal(item, &str); err == nil {
			out = append(out, str)
			continue
		}
		var num json.Number
		if err := json.Unmarshal(item, &num); err != nil {
			return fmt.Errorf("invalid user id")
		}
		out = append(out, num.String())
	}
	*s = out
	return nil
}

func (s stringIDs) int64s() ([]int64, error) {
	out := make([]int64, 0, len(s))
	for _, raw := range s {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("%w: invalid user id", botapp.ErrInvalidInput)
		}
		out = append(out, id)
	}
	return out, nil
}

func (s *BotHTTP) List(c khttp.Context) error {
	return s.withUser(c, func(ctx context.Context, userID int64) error {
		limit := httpapi.Limit(c.Request(), 50, 200)
		offset, err := httpapi.Offset(c.Request().URL.Query().Get("cursor"))
		if err != nil {
			return httpapi.BadRequest("INVALID_CURSOR", "invalid cursor")
		}
		all := strings.EqualFold(c.Request().URL.Query().Get("all"), "true")
		bots, total, err := s.bots.List(ctx, userID, all, limit, offset)
		if err != nil {
			return botHTTPError(err)
		}
		writeAgentJSON(c.Response(), http.StatusOK, listResponse{
			Items: botDTOs(bots), Total: total, HasMore: int64(offset+len(bots)) < total,
			NextCursor: nextCursor(offset, len(bots), int64(total)),
		})
		return nil
	})
}

func (s *BotHTTP) Create(c khttp.Context) error {
	return s.withUser(c, func(ctx context.Context, userID int64) error {
		var req createBotRequest
		if err := decodeBotJSON(c.Request(), &req); err != nil {
			return err
		}
		bot, err := s.bots.Create(ctx, userID, botapp.CreateInput{
			Name: req.Name, Description: req.Description, Username: req.Username,
			EventSubscriptions: req.EventSubscriptions,
		})
		if err != nil {
			return botHTTPError(err)
		}
		writeAgentJSON(c.Response(), http.StatusCreated, botDTOFrom(bot))
		return nil
	})
}

func (s *BotHTTP) Directory(c khttp.Context) error {
	return s.withUser(c, func(ctx context.Context, userID int64) error {
		limit := httpapi.Limit(c.Request(), 100, 200)
		offset, err := httpapi.Offset(c.Request().URL.Query().Get("cursor"))
		if err != nil {
			return httpapi.BadRequest("INVALID_CURSOR", "invalid cursor")
		}
		bots, err := s.bots.Directory(ctx, userID, limit, offset)
		if err != nil {
			return botHTTPError(err)
		}
		writeAgentJSON(c.Response(), http.StatusOK, listResponse{Items: botDTOs(bots), Total: int64(len(bots))})
		return nil
	})
}

func (s *BotHTTP) Detail(c khttp.Context) error {
	return s.withUser(c, func(ctx context.Context, userID int64) error {
		id, err := parseBotID(c, "id")
		if err != nil {
			return err
		}
		bot, err := s.bots.Detail(ctx, userID, id)
		if err != nil {
			return botHTTPError(err)
		}
		writeAgentJSON(c.Response(), http.StatusOK, botDTOFrom(bot))
		return nil
	})
}

func (s *BotHTTP) Update(c khttp.Context) error {
	return s.withUser(c, func(ctx context.Context, userID int64) error {
		id, err := parseBotID(c, "id")
		if err != nil {
			return err
		}
		var req updateBotRequest
		if err := decodeBotJSON(c.Request(), &req); err != nil {
			return err
		}
		bot, err := s.bots.Update(ctx, userID, id, botapp.UpdateInput{
			Name: req.Name, Description: req.Description, Status: req.Status,
			EventSubscriptions: req.EventSubscriptions,
		})
		if err != nil {
			return botHTTPError(err)
		}
		writeAgentJSON(c.Response(), http.StatusOK, botDTOFrom(bot))
		return nil
	})
}

func (s *BotHTTP) Delete(c khttp.Context) error {
	return s.withUser(c, func(ctx context.Context, userID int64) error {
		id, err := parseBotID(c, "id")
		if err != nil {
			return err
		}
		if err := s.bots.Disable(ctx, userID, id); err != nil {
			return botHTTPError(err)
		}
		writeAgentJSON(c.Response(), http.StatusOK, map[string]string{"status": "disabled"})
		return nil
	})
}

func (s *BotHTTP) CreateToken(c khttp.Context) error {
	return s.withUser(c, func(ctx context.Context, userID int64) error {
		id, err := parseBotID(c, "id")
		if err != nil {
			return err
		}
		var req createTokenRequest
		if err := decodeBotJSON(c.Request(), &req); err != nil {
			return err
		}
		if req.ExpiresInDays < 0 || req.ExpiresInDays > 3650 {
			return httpapi.BadRequest("INVALID_REQUEST", "expiresInDays must be between 0 and 3650")
		}
		token, plain, err := s.bots.CreateToken(ctx, userID, id, req.Name, time.Duration(req.ExpiresInDays)*24*time.Hour)
		if err != nil {
			return botHTTPError(err)
		}
		writeAgentJSON(c.Response(), http.StatusCreated, map[string]any{
			"token": tokenDTOFrom(token), "secret": plain,
		})
		return nil
	})
}

func (s *BotHTTP) ListTokens(c khttp.Context) error {
	return s.withUser(c, func(ctx context.Context, userID int64) error {
		id, err := parseBotID(c, "id")
		if err != nil {
			return err
		}
		tokens, err := s.bots.ListTokens(ctx, userID, id)
		if err != nil {
			return botHTTPError(err)
		}
		out := make([]tokenDTO, 0, len(tokens))
		for i := range tokens {
			out = append(out, tokenDTOFrom(&tokens[i]))
		}
		writeAgentJSON(c.Response(), http.StatusOK, map[string]any{"tokens": out})
		return nil
	})
}

func (s *BotHTTP) RevokeToken(c khttp.Context) error {
	return s.withUser(c, func(ctx context.Context, userID int64) error {
		id, err := parseBotID(c, "id")
		if err != nil {
			return err
		}
		tokenID := strings.TrimSpace(c.Vars().Get("tokenId"))
		if tokenID == "" {
			return httpapi.BadRequest("INVALID_REQUEST", "token id is required")
		}
		if err := s.bots.RevokeToken(ctx, userID, id, tokenID); err != nil {
			return botHTTPError(err)
		}
		writeAgentJSON(c.Response(), http.StatusOK, map[string]string{"status": "revoked"})
		return nil
	})
}

func (s *BotHTTP) GetWebhook(c khttp.Context) error {
	return s.withUser(c, func(ctx context.Context, userID int64) error {
		id, err := parseBotID(c, "id")
		if err != nil {
			return err
		}
		webhook, err := s.bots.GetWebhook(ctx, userID, id)
		if err != nil {
			return botHTTPError(err)
		}
		writeAgentJSON(c.Response(), http.StatusOK, webhookDTOFrom(webhook))
		return nil
	})
}

func (s *BotHTTP) PutWebhook(c khttp.Context) error {
	return s.withUser(c, func(ctx context.Context, userID int64) error {
		id, err := parseBotID(c, "id")
		if err != nil {
			return err
		}
		var req webhookRequest
		if err := decodeBotJSON(c.Request(), &req); err != nil {
			return err
		}
		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		webhook, secret, err := s.bots.SetWebhook(ctx, userID, id, botapp.WebhookInput{
			URL: req.URL, Events: req.Events, Enabled: enabled, Rotate: req.Rotate,
		})
		if err != nil {
			return botHTTPError(err)
		}
		response := map[string]any{"webhook": webhookDTOFrom(webhook)}
		if secret != "" {
			response["secret"] = secret
		}
		writeAgentJSON(c.Response(), http.StatusOK, response)
		return nil
	})
}

func (s *BotHTTP) RotateWebhookSecret(c khttp.Context) error {
	return s.withUser(c, func(ctx context.Context, userID int64) error {
		id, err := parseBotID(c, "id")
		if err != nil {
			return err
		}
		webhook, secret, err := s.bots.RotateWebhookSecret(ctx, userID, id)
		if err != nil {
			return botHTTPError(err)
		}
		writeAgentJSON(c.Response(), http.StatusOK, map[string]any{
			"webhook": webhookDTOFrom(webhook), "secret": secret,
		})
		return nil
	})
}

func (s *BotHTTP) DeleteWebhook(c khttp.Context) error {
	return s.withUser(c, func(ctx context.Context, userID int64) error {
		id, err := parseBotID(c, "id")
		if err != nil {
			return err
		}
		if err := s.bots.DeleteWebhook(ctx, userID, id); err != nil {
			return botHTTPError(err)
		}
		writeAgentJSON(c.Response(), http.StatusOK, map[string]string{"status": "deleted"})
		return nil
	})
}

func (s *BotHTTP) TestWebhook(c khttp.Context) error {
	return s.withUser(c, func(ctx context.Context, userID int64) error {
		id, err := parseBotID(c, "id")
		if err != nil {
			return err
		}
		bot, err := s.bots.Detail(ctx, userID, id)
		if err != nil {
			return botHTTPError(err)
		}
		webhook, secret, err := s.bots.WebhookSecret(ctx, id)
		if err != nil {
			return botHTTPError(err)
		}
		testCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		resp, sendErr := s.delivery.SendTest(testCtx, bot, webhook, secret)
		out := map[string]any{
			"ok":         sendErr == nil && resp.StatusCode >= 200 && resp.StatusCode < 300,
			"statusCode": resp.StatusCode, "snippet": resp.Snippet,
		}
		if sendErr != nil {
			out["error"] = sendErr.Error()
		}
		writeAgentJSON(c.Response(), http.StatusOK, out)
		return nil
	})
}

func (s *BotHTTP) ListDeliveries(c khttp.Context) error {
	return s.withUser(c, func(ctx context.Context, userID int64) error {
		id, err := parseBotID(c, "id")
		if err != nil {
			return err
		}
		limit := httpapi.Limit(c.Request(), 50, 200)
		offset, err := httpapi.Offset(c.Request().URL.Query().Get("cursor"))
		if err != nil {
			return httpapi.BadRequest("INVALID_CURSOR", "invalid cursor")
		}
		status := strings.TrimSpace(c.Request().URL.Query().Get("status"))
		deliveries, total, err := s.bots.ListDeliveries(ctx, userID, id, status, limit, offset)
		if err != nil {
			return botHTTPError(err)
		}
		out := make([]deliveryDTO, 0, len(deliveries))
		for i := range deliveries {
			out = append(out, deliveryDTOFrom(&deliveries[i]))
		}
		writeAgentJSON(c.Response(), http.StatusOK, listResponse{
			Items: out, Total: total, HasMore: int64(offset+len(out)) < total,
			NextCursor: nextCursor(offset, len(out), total),
		})
		return nil
	})
}

func (s *BotHTTP) ReplayDelivery(c khttp.Context) error {
	return s.withUser(c, func(ctx context.Context, userID int64) error {
		botID, err := parseBotID(c, "id")
		if err != nil {
			return err
		}
		deliveryID, err := strconv.ParseInt(c.Vars().Get("deliveryId"), 10, 64)
		if err != nil || deliveryID <= 0 {
			return httpapi.BadRequest("INVALID_REQUEST", "invalid delivery id")
		}
		if err := s.bots.ReplayDelivery(ctx, userID, botID, deliveryID); err != nil {
			return botHTTPError(err)
		}
		writeAgentJSON(c.Response(), http.StatusOK, map[string]string{"status": "queued"})
		return nil
	})
}

func (s *BotHTTP) Usage(c khttp.Context) error {
	return s.withUser(c, func(ctx context.Context, userID int64) error {
		id, err := parseBotID(c, "id")
		if err != nil {
			return err
		}
		days, _ := strconv.Atoi(c.Request().URL.Query().Get("days"))
		stats, err := s.bots.Usage(ctx, userID, id, days)
		if err != nil {
			return botHTTPError(err)
		}
		writeAgentJSON(c.Response(), http.StatusOK, map[string]any{"days": days, "deliveries": stats})
		return nil
	})
}

func (s *BotHTTP) Me(c khttp.Context) error {
	return s.withBot(c, func(ctx context.Context, principal *botapp.Principal) error {
		writeAgentJSON(c.Response(), http.StatusOK, map[string]any{"bot": botDTOFrom(principal.Bot)})
		return nil
	})
}

func (s *BotHTTP) SendMessage(c khttp.Context) error {
	return s.withBot(c, func(ctx context.Context, principal *botapp.Principal) error {
		var req sendMessageRequest
		if err := decodeBotJSON(c.Request(), &req); err != nil {
			return err
		}
		if strings.TrimSpace(req.Topic) == "" || strings.TrimSpace(req.Content) == "" {
			return httpapi.BadRequest("INVALID_REQUEST", "topic and content are required")
		}
		mentions, err := req.MentionedUserIDs.int64s()
		if err != nil {
			return httpapi.BadRequest("INVALID_REQUEST", err.Error())
		}
		clientMsgID := strings.TrimSpace(req.ClientMsgID)
		if clientMsgID == "" {
			clientMsgID = randomClientMsgID()
		}
		if len(clientMsgID) > 128 {
			return httpapi.BadRequest("INVALID_REQUEST", "client_msg_id is too long")
		}
		if err := s.allow(ctx, principal, "send", botSendRateLimit); err != nil {
			return err
		}
		result, err := s.sender.Send(ctx, botapp.SendInput{
			ActorID: principal.Bot.BotUserID, Topic: req.Topic,
			Content: []byte(req.Content), ClientMsgID: clientMsgID, MentionedIDs: mentions,
		})
		if err != nil {
			return &httpapi.Error{
				Status: http.StatusServiceUnavailable, Code: "DELIVERY_UNAVAILABLE",
				Message: "message delivery unavailable", Retryable: true, Internal: err,
			}
		}
		writeAgentJSON(c.Response(), http.StatusAccepted, map[string]any{
			"client_msg_id": result.ClientMsgID,
			"accepted_at":   time.Unix(result.Timestamp, 0).UTC().Format(time.RFC3339Nano),
		})
		return nil
	})
}

func (s *BotHTTP) MessageByClientID(c khttp.Context) error {
	return s.withBot(c, func(ctx context.Context, principal *botapp.Principal) error {
		topic := c.Request().URL.Query().Get("topic")
		clientMsgID := c.Vars().Get("clientMsgId")
		if topic == "" || clientMsgID == "" {
			return httpapi.BadRequest("INVALID_REQUEST", "topic and client message id are required")
		}
		wait := time.Duration(0)
		if raw := c.Request().URL.Query().Get("wait"); raw != "" {
			parsed, err := time.ParseDuration(raw)
			if err != nil || parsed < 0 || parsed > 10*time.Second {
				return httpapi.BadRequest("INVALID_REQUEST", "wait must be a duration up to 10s")
			}
			wait = parsed
		}
		// HTTP servers enforce a short request timeout. Treat wait as a hint:
		// return 202 early and let the SDK poll instead of holding the request
		// open longer than the server budget.
		if wait > 500*time.Millisecond {
			wait = 500 * time.Millisecond
		}
		deadline := time.Now().Add(wait)
		for {
			msg, err := s.reader.MessageByClientID(ctx, principal.Bot.BotUserID, topic, clientMsgID)
			if err == nil {
				writeAgentJSON(c.Response(), http.StatusOK, messageDTOFrom(msg))
				return nil
			}
			if !errors.Is(err, botapp.ErrNotFound) || time.Now().After(deadline) {
				if errors.Is(err, botapp.ErrNotFound) {
					writeAgentJSON(c.Response(), http.StatusAccepted, map[string]any{
						"client_msg_id": clientMsgID, "status": "pending",
					})
					return nil
				}
				return botHTTPError(err)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
	})
}

func (s *BotHTTP) History(c khttp.Context) error {
	return s.withBot(c, func(ctx context.Context, principal *botapp.Principal) error {
		topic := c.Request().URL.Query().Get("topic")
		if topic == "" {
			return httpapi.BadRequest("INVALID_REQUEST", "topic is required")
		}
		limit := httpapi.Limit(c.Request(), 50, 100)
		beforeSeq, _ := strconv.ParseUint(c.Request().URL.Query().Get("before_seq"), 10, 64)
		messages, err := s.reader.RecentMessages(ctx, principal.Bot.BotUserID, topic, limit, beforeSeq)
		if err != nil {
			return botHTTPError(err)
		}
		out := make([]messageDTO, 0, len(messages))
		for i := range messages {
			out = append(out, messageDTOFrom(messages[i]))
		}
		writeAgentJSON(c.Response(), http.StatusOK, map[string]any{"messages": out})
		return nil
	})
}

func (s *BotHTTP) CreateStream(c khttp.Context) error {
	return s.withBot(c, func(ctx context.Context, principal *botapp.Principal) error {
		var req streamCreateRequest
		if err := decodeBotJSON(c.Request(), &req); err != nil {
			return err
		}
		if err := s.allow(ctx, principal, "stream", botStreamRateLimit); err != nil {
			return err
		}
		state, err := s.streams.Create(ctx, principal.Bot, req.Topic, req.ClientMsgID)
		if err != nil {
			return botHTTPError(err)
		}
		writeAgentJSON(c.Response(), http.StatusCreated, map[string]any{
			"stream_id": state.ID, "client_msg_id": state.ClientMsgID,
			"expires_at": state.CreatedAt.Add(10 * time.Minute).UTC().Format(time.RFC3339Nano),
		})
		return nil
	})
}

func (s *BotHTTP) AppendStream(c khttp.Context) error {
	return s.withBot(c, func(ctx context.Context, principal *botapp.Principal) error {
		streamID := c.Vars().Get("streamId")
		var req streamChunkRequest
		if err := decodeBotJSON(c.Request(), &req); err != nil {
			return err
		}
		state, err := s.streams.Append(ctx, principal.Bot, streamID, req.Seq, req.Delta)
		if err != nil {
			return botHTTPError(err)
		}
		writeAgentJSON(c.Response(), http.StatusAccepted, map[string]any{
			"stream_id": state.ID, "seq": state.Seq,
		})
		return nil
	})
}

func (s *BotHTTP) FinalizeStream(c khttp.Context) error {
	return s.withBot(c, func(ctx context.Context, principal *botapp.Principal) error {
		streamID := c.Vars().Get("streamId")
		var req streamFinalizeRequest
		if err := decodeBotJSON(c.Request(), &req); err != nil {
			return err
		}
		result, err := s.streams.Finalize(ctx, principal.Bot, streamID, req.Content)
		if err != nil {
			return botHTTPError(err)
		}
		writeAgentJSON(c.Response(), http.StatusAccepted, map[string]any{
			"client_msg_id": result.ClientMsgID,
			"accepted_at":   time.Unix(result.Timestamp, 0).UTC().Format(time.RFC3339Nano),
		})
		return nil
	})
}

func (s *BotHTTP) AbortStream(c khttp.Context) error {
	return s.withBot(c, func(ctx context.Context, principal *botapp.Principal) error {
		streamID := c.Vars().Get("streamId")
		if err := s.streams.Abort(ctx, principal.Bot, streamID); err != nil {
			return botHTTPError(err)
		}
		c.Response().WriteHeader(http.StatusNoContent)
		return nil
	})
}

func (s *BotHTTP) allow(ctx context.Context, principal *botapp.Principal, scope string, limit int) error {
	if s.limiter == nil || principal == nil {
		return nil
	}
	key := fmt.Sprintf("bot:rl:%s:%d", scope, principal.Bot.ID)
	allowed, err := s.limiter.Allow(ctx, key, limit, time.Minute)
	if err != nil {
		s.log.Warnf("bot rate limiter error (allowing request): %v", err)
		return nil
	}
	if !allowed {
		return &httpapi.Error{
			Status: http.StatusTooManyRequests, Code: "RATE_LIMITED",
			Message: "too many requests, please try again later", Retryable: true,
			RetryAfter: time.Second,
		}
	}
	return nil
}

func (s *BotHTTP) withUser(c khttp.Context, fn func(context.Context, int64) error) error {
	ctx := c.Middleware(func(ctx context.Context, _ interface{}) (interface{}, error) {
		userID := extractUserIDFromContext(ctx)
		if userID <= 0 {
			httpapi.Write(c.Response(), ctx, &httpapi.Error{
				Status: http.StatusUnauthorized, Code: "UNAUTHORIZED", Message: "authentication required",
			})
			return nil, nil
		}
		ctx = audit.WithRequestMeta(ctx, requestMeta(ctx, c.Request()))
		if err := fn(ctx, userID); err != nil {
			httpapi.Write(c.Response(), ctx, botHTTPError(err))
		}
		return nil, nil
	})
	_, err := ctx(c, nil)
	return err
}

func (s *BotHTTP) withBot(c khttp.Context, fn func(context.Context, *botapp.Principal) error) error {
	ctx := c.Middleware(func(ctx context.Context, _ interface{}) (interface{}, error) {
		token := bearerToken(c.Request())
		if s.auth == nil {
			httpapi.Write(c.Response(), ctx, httpapi.AsError(ctx, domain.ErrTokenNotFound))
			return nil, nil
		}
		principal, err := s.auth.Authenticate(ctx, token)
		if err != nil {
			httpapi.Write(c.Response(), ctx, botHTTPError(err))
			return nil, nil
		}
		ctx = jwtMiddleware.NewContext(ctx, jwt5.MapClaims{
			"user_id": float64(principal.Bot.BotUserID),
			"bot_id":  float64(principal.Bot.ID),
			"scope":   "bot",
		})
		if err := fn(ctx, principal); err != nil {
			httpapi.Write(c.Response(), ctx, botHTTPError(err))
		}
		return nil, nil
	})
	_, err := ctx(c, nil)
	return err
}

var errUnauthorized = errors.New("authentication required")

func bearerToken(r *http.Request) string {
	raw := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(raw), "bearer ") {
		return strings.TrimSpace(raw[7:])
	}
	return ""
}

func decodeBotJSON(r *http.Request, dst any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return httpapi.BadRequest("INVALID_REQUEST", "invalid request body")
	}
	return nil
}

func parseBotID(c khttp.Context, name string) (int64, error) {
	id, err := strconv.ParseInt(c.Vars().Get(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, httpapi.BadRequest("INVALID_REQUEST", "invalid bot id")
	}
	return id, nil
}

func randomClientMsgID() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return "bot-" + hex.EncodeToString(buf)
}

func botHTTPError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *httpapi.Error
	if errors.As(err, &apiErr) {
		return apiErr
	}
	switch {
	case errors.Is(err, domain.ErrTokenNotFound) || errors.Is(err, errUnauthorized):
		return &httpapi.Error{Status: http.StatusUnauthorized, Code: "INVALID_BOT_TOKEN", Message: "invalid bot token"}
	case errors.Is(err, domain.ErrDisabled):
		return &httpapi.Error{Status: http.StatusForbidden, Code: "BOT_DISABLED", Message: "bot is disabled"}
	case errors.Is(err, domain.ErrStreamExpired):
		return &httpapi.Error{Status: http.StatusGone, Code: "STREAM_EXPIRED", Message: "stream expired"}
	case errors.Is(err, botapp.ErrRateLimited):
		return &httpapi.Error{Status: http.StatusTooManyRequests, Code: "RATE_LIMITED", Message: "too many requests", Retryable: true, RetryAfter: time.Second}
	case errors.Is(err, botapp.ErrInvalidInput), errors.Is(err, domain.ErrInvalid):
		return &httpapi.Error{Status: http.StatusBadRequest, Code: "INVALID_REQUEST", Message: err.Error()}
	case errors.Is(err, botapp.ErrForbidden), errors.Is(err, domain.ErrForbidden):
		return &httpapi.Error{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "access denied"}
	case errors.Is(err, botapp.ErrNotFound), errors.Is(err, domain.ErrWebhookAbsent):
		return &httpapi.Error{Status: http.StatusNotFound, Code: "NOT_FOUND", Message: "not found"}
	case errors.Is(err, botapp.ErrConflict), errors.Is(err, domain.ErrConflict):
		return &httpapi.Error{Status: http.StatusConflict, Code: "CONFLICT", Message: err.Error()}
	default:
		return &httpapi.Error{Status: http.StatusInternalServerError, Code: "INTERNAL", Message: "internal error", Internal: err}
	}
}

type botDTO struct {
	ID                 string   `json:"id"`
	BotUserID          string   `json:"botUserId"`
	OwnerUserID        string   `json:"ownerUserId"`
	Username           string   `json:"username"`
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	Status             string   `json:"status"`
	EventSubscriptions []string `json:"eventSubscriptions"`
	CreatedAt          string   `json:"createdAt"`
	UpdatedAt          string   `json:"updatedAt"`
}

func botDTOFrom(bot *domain.Bot) botDTO {
	if bot == nil {
		return botDTO{}
	}
	return botDTO{
		ID: fmt.Sprintf("%d", bot.ID), BotUserID: fmt.Sprintf("%d", bot.BotUserID),
		OwnerUserID: fmt.Sprintf("%d", bot.OwnerUserID), Username: bot.Username,
		Name: bot.Name, Description: bot.Description, Status: bot.Status,
		EventSubscriptions: bot.EventSubscriptions,
		CreatedAt:          bot.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:          bot.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func botDTOs(bots []domain.Bot) []botDTO {
	out := make([]botDTO, 0, len(bots))
	for i := range bots {
		out = append(out, botDTOFrom(&bots[i]))
	}
	return out
}

type tokenDTO struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Prefix     string   `json:"prefix"`
	Scopes     []string `json:"scopes"`
	ExpiresAt  string   `json:"expiresAt,omitempty"`
	RevokedAt  string   `json:"revokedAt,omitempty"`
	LastUsedAt string   `json:"lastUsedAt,omitempty"`
	CreatedAt  string   `json:"createdAt"`
}

func tokenDTOFrom(token *domain.Token) tokenDTO {
	if token == nil {
		return tokenDTO{}
	}
	out := tokenDTO{
		ID: token.ID, Name: token.Name, Prefix: token.Prefix, Scopes: token.Scopes,
		CreatedAt: token.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	if token.ExpiresAt != nil {
		out.ExpiresAt = token.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	if token.RevokedAt != nil {
		out.RevokedAt = token.RevokedAt.UTC().Format(time.RFC3339Nano)
	}
	if token.LastUsedAt != nil {
		out.LastUsedAt = token.LastUsedAt.UTC().Format(time.RFC3339Nano)
	}
	return out
}

type webhookDTO struct {
	ID                  string   `json:"id"`
	URL                 string   `json:"url"`
	SecretPrefix        string   `json:"secretPrefix"`
	Events              []string `json:"events"`
	Enabled             bool     `json:"enabled"`
	VerifiedAt          string   `json:"verifiedAt,omitempty"`
	LastSuccessAt       string   `json:"lastSuccessAt,omitempty"`
	ConsecutiveFailures int      `json:"consecutiveFailures"`
	DisabledAt          string   `json:"disabledAt,omitempty"`
	CreatedAt           string   `json:"createdAt"`
}

func webhookDTOFrom(webhook *domain.Webhook) webhookDTO {
	if webhook == nil {
		return webhookDTO{}
	}
	out := webhookDTO{
		ID: fmt.Sprintf("%d", webhook.ID), URL: webhook.URL, SecretPrefix: webhook.SecretPrefix,
		Events: webhook.Events, Enabled: webhook.Enabled,
		ConsecutiveFailures: webhook.ConsecutiveFailures,
		CreatedAt:           webhook.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	if webhook.VerifiedAt != nil {
		out.VerifiedAt = webhook.VerifiedAt.UTC().Format(time.RFC3339Nano)
	}
	if webhook.LastSuccessAt != nil {
		out.LastSuccessAt = webhook.LastSuccessAt.UTC().Format(time.RFC3339Nano)
	}
	if webhook.DisabledAt != nil {
		out.DisabledAt = webhook.DisabledAt.UTC().Format(time.RFC3339Nano)
	}
	return out
}

type deliveryDTO struct {
	ID              string `json:"id"`
	EventID         string `json:"eventId"`
	EventType       string `json:"eventType"`
	Topic           string `json:"topic"`
	MsgID           string `json:"msgId"`
	Status          string `json:"status"`
	Attempts        int    `json:"attempts"`
	NextAttemptAt   string `json:"nextAttemptAt"`
	LastStatusCode  int    `json:"lastStatusCode,omitempty"`
	LastError       string `json:"lastError,omitempty"`
	ResponseSnippet string `json:"responseSnippet,omitempty"`
	CreatedAt       string `json:"createdAt"`
	DeliveredAt     string `json:"deliveredAt,omitempty"`
}

func deliveryDTOFrom(delivery *domain.Delivery) deliveryDTO {
	if delivery == nil {
		return deliveryDTO{}
	}
	out := deliveryDTO{
		ID: fmt.Sprintf("%d", delivery.ID), EventID: delivery.EventID,
		EventType: delivery.EventType, Topic: delivery.Topic, MsgID: fmt.Sprintf("%d", delivery.MsgID),
		Status: delivery.Status, Attempts: delivery.Attempts,
		NextAttemptAt:  delivery.NextAttemptAt.UTC().Format(time.RFC3339Nano),
		LastStatusCode: delivery.LastStatusCode, LastError: delivery.LastError,
		ResponseSnippet: delivery.ResponseSnippet,
		CreatedAt:       delivery.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	if delivery.DeliveredAt != nil {
		out.DeliveredAt = delivery.DeliveredAt.UTC().Format(time.RFC3339Nano)
	}
	return out
}

type messageDTO struct {
	MessageID string `json:"messageId"`
	Topic     string `json:"topic"`
	TopicSeq  string `json:"topicSeq"`
	SenderID  string `json:"senderId"`
	Type      string `json:"type"`
	Content   string `json:"content"`
	CreatedAt string `json:"createdAt"`
}

func messageDTOFrom(msg botapp.Message) messageDTO {
	return messageDTO{
		MessageID: fmt.Sprintf("%d", msg.ID), Topic: msg.Topic, TopicSeq: fmt.Sprintf("%d", msg.TopicSeq),
		SenderID: fmt.Sprintf("%d", msg.SenderID), Type: messageTypeName(msg.Type),
		Content: string(msg.Content), CreatedAt: time.Unix(msg.Timestamp, 0).UTC().Format(time.RFC3339Nano),
	}
}

func messageTypeName(value int32) string {
	switch value {
	case 1:
		return "text"
	case 2:
		return "image"
	case 3:
		return "file"
	case 4:
		return "voice"
	case 5:
		return "agent_card"
	default:
		return "unknown"
	}
}

type listResponse struct {
	Items      any    `json:"items"`
	Total      int64  `json:"total"`
	NextCursor string `json:"nextCursor,omitempty"`
	HasMore    bool   `json:"hasMore"`
}

func nextCursor(offset, count int, total int64) string {
	if int64(offset+count) >= total {
		return ""
	}
	return httpapi.EncodeCursor(offset + count)
}
