package app

import (
	"context"

	"nonoka-im/internal/audit"
	"nonoka-im/internal/bot/domain"

	"github.com/go-kratos/kratos/v2/log"
)

const (
	defaultMaxTokensPerBot = 10
	webhookSecretBytes     = 32
	tokenSecretBytes       = 32
	maxBotNameBytes        = 100
	maxBotDescription      = 1000
	maxTokenNameBytes      = 100
)

type Config struct {
	MaxTokensPerBot int
}

type Service struct {
	Store     Store
	Authz     Authorizer
	Sealer    Sealer
	Validator URLValidator
	Audit     AuditRecorder
	Log       *log.Helper
	Config    Config
}

func NewService(store Store, authz Authorizer, sealer Sealer, validator URLValidator, auditor AuditRecorder, logger log.Logger) *Service {
	return &Service{
		Store: store, Authz: authz, Sealer: sealer, Validator: validator, Audit: auditor,
		Log: log.NewHelper(logger),
	}
}

func (s *Service) log() *log.Helper {
	if s.Log != nil {
		return s.Log
	}
	return log.NewHelper(log.DefaultLogger)
}

func (s *Service) maxTokens() int {
	if s.Config.MaxTokensPerBot > 0 {
		return s.Config.MaxTokensPerBot
	}
	return defaultMaxTokensPerBot
}

func (s *Service) requireOwner(ctx context.Context, userID int64, bot *domain.Bot) error {
	if userID > 0 && userID == bot.OwnerUserID {
		return nil
	}
	return ErrForbidden
}

func (s *Service) requireOwnerOrAdmin(ctx context.Context, userID int64, bot *domain.Bot) error {
	if userID > 0 && userID == bot.OwnerUserID {
		return nil
	}
	if s.Authz == nil || s.Authz.CanManagePlatform(ctx, userID) != nil {
		return ErrForbidden
	}
	return nil
}

func (s *Service) requireAdmin(ctx context.Context, userID int64) error {
	if s.Authz == nil || s.Authz.CanManagePlatform(ctx, userID) != nil {
		return ErrForbidden
	}
	return nil
}

func (s *Service) record(ctx context.Context, actor int64, action, resourceType, resourceID string, before, after map[string]any) {
	if s.Audit == nil {
		return
	}
	if err := s.Audit.Record(ctx, audit.Event{
		ActorUserID: actor, Action: action, ResourceType: resourceType,
		ResourceID: resourceID, Before: before, After: after,
	}); err != nil {
		s.log().Warnf("record bot audit failed: action=%s err=%v", action, err)
	}
}
