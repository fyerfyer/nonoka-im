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

	botv1 "nonoka-im/api/bot/v1"
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
	botv1.UnimplementedBotServiceServer

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
