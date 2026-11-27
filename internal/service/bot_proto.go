package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	botv1 "nonoka-im/api/bot/v1"
	botapp "nonoka-im/internal/bot/app"
	"nonoka-im/internal/bot/domain"
	"nonoka-im/internal/httpapi"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"google.golang.org/protobuf/types/known/emptypb"
)

// botRPCUser returns the authenticated user for generated proto handlers.
func (s *BotHTTP) botRPCUser(ctx context.Context) (int64, error) {
	uid := extractUserIDFromContext(ctx)
	if uid <= 0 {
		return 0, kerrors.Unauthorized("UNAUTHORIZED", "authentication required")
	}
	return uid, nil
}

// botRPCError adapts the shared HTTP error envelope to Kratos errors so
// generated HTTP handlers return the same status/code/retryability contract.
func botRPCError(err error) error {
	if err == nil {
		return nil
	}
	adapted := botHTTPError(err)
	var e *httpapi.Error
	if errors.As(adapted, &e) {
		return kerrors.New(e.Status, e.Code, e.Message)
	}
	return kerrors.InternalServer("INTERNAL", "internal error")
}

func botRPCParseID(raw, field string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0, kerrors.BadRequest("INVALID_ARGUMENT", "invalid "+field)
	}
	return id, nil
}

func botRPCNextCursor(offset, count int, total int64) string {
	if int64(offset+count) >= total {
		return ""
	}
	return httpapi.EncodeCursor(offset + count)
}

func botProtoFrom(b *domain.Bot) *botv1.Bot {
	if b == nil {
		return nil
	}
	return &botv1.Bot{
		Id: strconv.FormatInt(b.ID, 10), BotUserId: strconv.FormatInt(b.BotUserID, 10),
		OwnerUserId: strconv.FormatInt(b.OwnerUserID, 10), Username: b.Username,
		Name: b.Name, Description: b.Description, Status: b.Status,
		EventSubscriptions: b.EventSubscriptions,
		CreatedAt:          b.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:          b.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func botTokenProtoFrom(token *domain.Token) *botv1.BotToken {
	if token == nil {
		return nil
	}
	out := &botv1.BotToken{
		Id: token.ID, Name: token.Name, Prefix: token.Prefix, Scopes: token.Scopes,
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

func botWebhookProtoFrom(webhook *domain.Webhook) *botv1.BotWebhook {
	if webhook == nil {
		return nil
	}
	out := &botv1.BotWebhook{
		Id: strconv.FormatInt(webhook.ID, 10), Url: webhook.URL, SecretPrefix: webhook.SecretPrefix,
		Events: webhook.Events, Enabled: webhook.Enabled,
		ConsecutiveFailures: int32(webhook.ConsecutiveFailures),
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

func botDeliveryProtoFrom(delivery *domain.Delivery) *botv1.BotDelivery {
	if delivery == nil {
		return nil
	}
	out := &botv1.BotDelivery{
		Id: strconv.FormatInt(delivery.ID, 10), EventId: delivery.EventID,
		EventType: delivery.EventType, Topic: delivery.Topic,
		MsgId: strconv.FormatInt(delivery.MsgID, 10), Status: delivery.Status,
		Attempts:       int32(delivery.Attempts),
		NextAttemptAt:  delivery.NextAttemptAt.UTC().Format(time.RFC3339Nano),
		LastStatusCode: int32(delivery.LastStatusCode), LastError: delivery.LastError,
		ResponseSnippet: delivery.ResponseSnippet,
		CreatedAt:       delivery.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	if delivery.DeliveredAt != nil {
		out.DeliveredAt = delivery.DeliveredAt.UTC().Format(time.RFC3339Nano)
	}
	return out
}

func (s *BotHTTP) ListBots(ctx context.Context, req *botv1.ListBotsRequest) (*botv1.ListBotsReply, error) {
	uid, err := s.botRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	limit := int(req.GetLimit())
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset, err := httpapi.Offset(req.GetCursor())
	if err != nil {
		return nil, kerrors.BadRequest("INVALID_CURSOR", "invalid cursor")
	}
	bots, total, err := s.bots.List(ctx, uid, req.GetAll(), limit, offset)
	if err != nil {
		return nil, botRPCError(err)
	}
	items := make([]*botv1.Bot, 0, len(bots))
	for i := range bots {
		items = append(items, botProtoFrom(&bots[i]))
	}
	return &botv1.ListBotsReply{
		Items: items, Total: int32(total),
		HasMore:    int64(offset+len(bots)) < total,
		NextCursor: botRPCNextCursor(offset, len(bots), total),
	}, nil
}

func (s *BotHTTP) GetBotDirectory(ctx context.Context, req *botv1.ListBotsRequest) (*botv1.ListBotsReply, error) {
	uid, err := s.botRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	limit := int(req.GetLimit())
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	offset, err := httpapi.Offset(req.GetCursor())
	if err != nil {
		return nil, kerrors.BadRequest("INVALID_CURSOR", "invalid cursor")
	}
	bots, err := s.bots.Directory(ctx, uid, limit, offset)
	if err != nil {
		return nil, botRPCError(err)
	}
	items := make([]*botv1.Bot, 0, len(bots))
	for i := range bots {
		items = append(items, botProtoFrom(&bots[i]))
	}
	return &botv1.ListBotsReply{Items: items, Total: int32(len(items))}, nil
}

func (s *BotHTTP) CreateBot(ctx context.Context, req *botv1.CreateBotRequest) (*botv1.Bot, error) {
	uid, err := s.botRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	bot, err := s.bots.Create(ctx, uid, botapp.CreateInput{
		Name: req.GetName(), Description: req.GetDescription(), Username: req.GetUsername(),
		EventSubscriptions: req.GetEventSubscriptions(),
	})
	if err != nil {
		return nil, botRPCError(err)
	}
	return botProtoFrom(bot), nil
}

func (s *BotHTTP) GetBot(ctx context.Context, req *botv1.GetBotRequest) (*botv1.Bot, error) {
	uid, err := s.botRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	id, err := botRPCParseID(req.GetId(), "bot id")
	if err != nil {
		return nil, err
	}
	bot, err := s.bots.Detail(ctx, uid, id)
	if err != nil {
		return nil, botRPCError(err)
	}
	return botProtoFrom(bot), nil
}

func (s *BotHTTP) UpdateBot(ctx context.Context, req *botv1.UpdateBotRequest) (*botv1.Bot, error) {
	uid, err := s.botRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	id, err := botRPCParseID(req.GetId(), "bot id")
	if err != nil {
		return nil, err
	}
	bot, err := s.bots.Update(ctx, uid, id, botapp.UpdateInput{
		Name: req.GetName(), Description: req.GetDescription(), Status: req.GetStatus(),
		EventSubscriptions: req.GetEventSubscriptions(),
	})
	if err != nil {
		return nil, botRPCError(err)
	}
	return botProtoFrom(bot), nil
}

func (s *BotHTTP) DeleteBot(ctx context.Context, req *botv1.GetBotRequest) (*emptypb.Empty, error) {
	uid, err := s.botRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	id, err := botRPCParseID(req.GetId(), "bot id")
	if err != nil {
		return nil, err
	}
	if err := s.bots.Disable(ctx, uid, id); err != nil {
		return nil, botRPCError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *BotHTTP) ListBotTokens(ctx context.Context, req *botv1.GetBotRequest) (*botv1.ListBotTokensReply, error) {
	uid, err := s.botRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	id, err := botRPCParseID(req.GetId(), "bot id")
	if err != nil {
		return nil, err
	}
	tokens, err := s.bots.ListTokens(ctx, uid, id)
	if err != nil {
		return nil, botRPCError(err)
	}
	out := make([]*botv1.BotToken, 0, len(tokens))
	for i := range tokens {
		out = append(out, botTokenProtoFrom(&tokens[i]))
	}
	return &botv1.ListBotTokensReply{Tokens: out}, nil
}

func (s *BotHTTP) CreateBotToken(ctx context.Context, req *botv1.CreateBotTokenRequest) (*botv1.CreateBotTokenReply, error) {
	uid, err := s.botRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	id, err := botRPCParseID(req.GetId(), "bot id")
	if err != nil {
		return nil, err
	}
	if req.GetExpiresInDays() < 0 || req.GetExpiresInDays() > 3650 {
		return nil, kerrors.BadRequest("INVALID_ARGUMENT", "expiresInDays must be between 0 and 3650")
	}
	token, secret, err := s.bots.CreateToken(ctx, uid, id, req.GetName(), time.Duration(req.GetExpiresInDays())*24*time.Hour)
	if err != nil {
		return nil, botRPCError(err)
	}
	return &botv1.CreateBotTokenReply{Token: botTokenProtoFrom(token), Secret: secret}, nil
}

func (s *BotHTTP) RevokeBotToken(ctx context.Context, req *botv1.RevokeBotTokenRequest) (*emptypb.Empty, error) {
	uid, err := s.botRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	id, err := botRPCParseID(req.GetId(), "bot id")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.GetTokenId()) == "" {
		return nil, kerrors.BadRequest("INVALID_ARGUMENT", "token id is required")
	}
	if err := s.bots.RevokeToken(ctx, uid, id, req.GetTokenId()); err != nil {
		return nil, botRPCError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *BotHTTP) GetBotWebhook(ctx context.Context, req *botv1.GetBotRequest) (*botv1.BotWebhook, error) {
	uid, err := s.botRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	id, err := botRPCParseID(req.GetId(), "bot id")
	if err != nil {
		return nil, err
	}
	webhook, err := s.bots.GetWebhook(ctx, uid, id)
	if err != nil {
		return nil, botRPCError(err)
	}
	return botWebhookProtoFrom(webhook), nil
}

func (s *BotHTTP) PutBotWebhook(ctx context.Context, req *botv1.PutBotWebhookRequest) (*botv1.PutBotWebhookReply, error) {
	uid, err := s.botRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	id, err := botRPCParseID(req.GetId(), "bot id")
	if err != nil {
		return nil, err
	}
	enabled := true
	if req.Enabled != nil {
		enabled = req.GetEnabled()
	}
	webhook, secret, err := s.bots.SetWebhook(ctx, uid, id, botapp.WebhookInput{
		URL: req.GetUrl(), Events: req.GetEvents(), Enabled: enabled, Rotate: req.GetRotate(),
	})
	if err != nil {
		return nil, botRPCError(err)
	}
	return &botv1.PutBotWebhookReply{Webhook: botWebhookProtoFrom(webhook), Secret: secret}, nil
}

func (s *BotHTTP) DeleteBotWebhook(ctx context.Context, req *botv1.GetBotRequest) (*emptypb.Empty, error) {
	uid, err := s.botRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	id, err := botRPCParseID(req.GetId(), "bot id")
	if err != nil {
		return nil, err
	}
	if err := s.bots.DeleteWebhook(ctx, uid, id); err != nil {
		return nil, botRPCError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *BotHTTP) RotateBotWebhookSecret(ctx context.Context, req *botv1.GetBotRequest) (*botv1.PutBotWebhookReply, error) {
	uid, err := s.botRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	id, err := botRPCParseID(req.GetId(), "bot id")
	if err != nil {
		return nil, err
	}
	webhook, secret, err := s.bots.RotateWebhookSecret(ctx, uid, id)
	if err != nil {
		return nil, botRPCError(err)
	}
	return &botv1.PutBotWebhookReply{Webhook: botWebhookProtoFrom(webhook), Secret: secret}, nil
}

func (s *BotHTTP) TestBotWebhook(ctx context.Context, req *botv1.GetBotRequest) (*botv1.TestBotWebhookReply, error) {
	uid, err := s.botRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	id, err := botRPCParseID(req.GetId(), "bot id")
	if err != nil {
		return nil, err
	}
	bot, err := s.bots.Detail(ctx, uid, id)
	if err != nil {
		return nil, botRPCError(err)
	}
	webhook, secret, err := s.bots.WebhookSecret(ctx, id)
	if err != nil {
		return nil, botRPCError(err)
	}
	testCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	resp, sendErr := s.delivery.SendTest(testCtx, bot, webhook, secret)
	out := &botv1.TestBotWebhookReply{
		StatusCode: int32(resp.StatusCode), Snippet: resp.Snippet,
		Ok: sendErr == nil && resp.StatusCode >= 200 && resp.StatusCode < 300,
	}
	if sendErr != nil {
		out.Error = sendErr.Error()
	}
	return out, nil
}

func (s *BotHTTP) ListBotDeliveries(ctx context.Context, req *botv1.ListBotDeliveriesRequest) (*botv1.ListBotDeliveriesReply, error) {
	uid, err := s.botRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	id, err := botRPCParseID(req.GetId(), "bot id")
	if err != nil {
		return nil, err
	}
	limit := int(req.GetLimit())
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset, err := httpapi.Offset(req.GetCursor())
	if err != nil {
		return nil, kerrors.BadRequest("INVALID_CURSOR", "invalid cursor")
	}
	deliveries, total, err := s.bots.ListDeliveries(ctx, uid, id, req.GetStatus(), limit, offset)
	if err != nil {
		return nil, botRPCError(err)
	}
	items := make([]*botv1.BotDelivery, 0, len(deliveries))
	for i := range deliveries {
		items = append(items, botDeliveryProtoFrom(&deliveries[i]))
	}
	return &botv1.ListBotDeliveriesReply{
		Items: items, Total: int32(total),
		HasMore:    int64(offset+len(items)) < total,
		NextCursor: botRPCNextCursor(offset, len(items), total),
	}, nil
}

func (s *BotHTTP) ReplayBotDelivery(ctx context.Context, req *botv1.ReplayBotDeliveryRequest) (*botv1.ReplayBotDeliveryReply, error) {
	uid, err := s.botRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	botID, err := botRPCParseID(req.GetId(), "bot id")
	if err != nil {
		return nil, err
	}
	deliveryID, err := strconv.ParseInt(req.GetDeliveryId(), 10, 64)
	if err != nil || deliveryID <= 0 {
		return nil, kerrors.BadRequest("INVALID_ARGUMENT", "invalid delivery id")
	}
	if err := s.bots.ReplayDelivery(ctx, uid, botID, deliveryID); err != nil {
		return nil, botRPCError(err)
	}
	return &botv1.ReplayBotDeliveryReply{Status: "queued"}, nil
}

func (s *BotHTTP) GetBotUsage(ctx context.Context, req *botv1.GetBotUsageRequest) (*botv1.GetBotUsageReply, error) {
	uid, err := s.botRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	id, err := botRPCParseID(req.GetId(), "bot id")
	if err != nil {
		return nil, err
	}
	days := int(req.GetDays())
	if days <= 0 {
		days = 7
	}
	stats, err := s.bots.Usage(ctx, uid, id, days)
	if err != nil {
		return nil, botRPCError(err)
	}
	out := make(map[string]int32, len(stats))
	for key, value := range stats {
		out[key] = int32(value)
	}
	return &botv1.GetBotUsageReply{Days: int32(days), Deliveries: out}, nil
}
