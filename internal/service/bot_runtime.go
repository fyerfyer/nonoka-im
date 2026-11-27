package service

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	botapp "nonoka-im/internal/bot/app"
	"nonoka-im/internal/httpapi"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

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
