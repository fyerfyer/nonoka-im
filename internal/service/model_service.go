package service

import (
	"context"
	"net/http"
	"strconv"

	"nonoka-im/internal/agent/domain"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

type ModelView struct {
	ID                         uint64  `json:"id"`
	ModelKey                   string  `json:"modelKey"`
	Provider                   string  `json:"provider"`
	GatewayModelKey            string  `json:"gatewayModelKey"`
	DisplayName                string  `json:"displayName"`
	Icon                       string  `json:"icon,omitempty"`
	ContextWindow              int     `json:"contextWindow"`
	MaxOutputTokens            int     `json:"maxOutputTokens"`
	SupportsTools              bool    `json:"supportsTools"`
	SupportsVision             bool    `json:"supportsVision"`
	SupportsReasoning          bool    `json:"supportsReasoning"`
	InputPricePerMtok          float64 `json:"inputPricePerMtok"`
	OutputPricePerMtok         float64 `json:"outputPricePerMtok"`
	InputCacheReadPricePerMtok float64 `json:"inputCacheReadPricePerMtok"`
	Enabled                    bool    `json:"enabled"`
	Curated                    bool    `json:"curated"`
	Recommended                bool    `json:"recommended"`
	SortOrder                  int     `json:"sortOrder"`
}

type modelCurationRequest struct {
	DisplayName       string `json:"displayName"`
	Icon              string `json:"icon"`
	SupportsTools     *bool  `json:"supportsTools"`
	SupportsVision    *bool  `json:"supportsVision"`
	SupportsReasoning *bool  `json:"supportsReasoning"`
	Enabled           *bool  `json:"enabled"`
	Recommended       *bool  `json:"recommended"`
	SortOrder         *int   `json:"sortOrder"`
}

func (s *AgentHTTP) ListModels(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		models, admin, err := s.models.Catalog(ctx, extractUserIDFromContext(ctx))
		if err != nil {
			return err
		}
		out := make([]ModelView, 0, len(models))
		for i := range models {
			out = append(out, modelView(models[i]))
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{"models": out, "canManage": admin})
		return nil
	})
}

func (s *AgentHTTP) SyncModels(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		n, err := s.models.SyncCatalog(ctx, extractUserIDFromContext(ctx))
		if err != nil {
			return err
		}
		models, _, err := s.models.Catalog(ctx, extractUserIDFromContext(ctx))
		if err != nil {
			return err
		}
		out := make([]ModelView, 0, len(models))
		for i := range models {
			out = append(out, modelView(models[i]))
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{"synced": n, "models": out})
		return nil
	})
}

func (s *AgentHTTP) UpdateModel(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		id, err := strconv.ParseUint(c.Vars().Get("id"), 10, 64)
		if err != nil || id == 0 {
			return invalidAgentInput("invalid model id")
		}
		var input modelCurationRequest
		if err := decodeJSONBody(r, &input); err != nil {
			return err
		}
		model, err := s.models.UpdateCuration(ctx, extractUserIDFromContext(ctx), id, domain.ModelCuration{
			DisplayName: input.DisplayName, Icon: input.Icon,
			SupportsTools: input.SupportsTools, SupportsVision: input.SupportsVision,
			SupportsReasoning: input.SupportsReasoning, Enabled: input.Enabled,
			Recommended: input.Recommended, SortOrder: input.SortOrder,
		})
		if err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, modelView(*model))
		return nil
	})
}

func modelView(m domain.Model) ModelView {
	return ModelView{
		ID: m.ID, ModelKey: m.ModelKey, Provider: m.Provider, GatewayModelKey: m.GatewayModelKey,
		DisplayName: m.DisplayName, Icon: m.Icon, ContextWindow: m.ContextWindow,
		MaxOutputTokens: m.MaxOutputTokens, SupportsTools: m.SupportsTools,
		SupportsVision: m.SupportsVision, SupportsReasoning: m.SupportsReasoning,
		InputPricePerMtok: m.InputPricePerMTok, OutputPricePerMtok: m.OutputPricePerMTok,
		InputCacheReadPricePerMtok: m.InputCacheReadPricePerMTok,
		Enabled:                    m.Enabled, Curated: m.Curated, Recommended: m.Recommended, SortOrder: m.SortOrder,
	}
}
