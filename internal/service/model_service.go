package service

import (
	"context"
	"net/http"
	"strconv"

	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/llm"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

type ModelServiceView struct {
	ID           uint64      `json:"id"`
	Name         string      `json:"name"`
	DisplayName  string      `json:"displayName"`
	GatewayType  string      `json:"gatewayType"`
	BaseURL      string      `json:"baseUrl,omitempty"`
	GatewayKeyID string      `json:"gatewayKeyId,omitempty"`
	Enabled      bool        `json:"enabled"`
	HasAPIKey    bool        `json:"hasApiKey"`
	Models       []ModelView `json:"models"`
}

type ModelView struct {
	ID                 uint64  `json:"id"`
	ServiceID          uint64  `json:"serviceId"`
	ModelKey           string  `json:"modelKey"`
	DisplayName        string  `json:"displayName"`
	ContextWindow      int     `json:"contextWindow"`
	MaxOutputTokens    int     `json:"maxOutputTokens"`
	SupportsTools      bool    `json:"supportsTools"`
	SupportsVision     bool    `json:"supportsVision"`
	SupportsReasoning  bool    `json:"supportsReasoning"`
	InputPricePerMTok  float64 `json:"inputPricePerMtok"`
	OutputPricePerMTok float64 `json:"outputPricePerMtok"`
	Enabled            bool    `json:"enabled"`
}

type modelServiceRequest struct {
	Name           string              `json:"name"`
	DisplayName    string              `json:"displayName"`
	GatewayType    string              `json:"gatewayType"`
	BaseURL        string              `json:"baseUrl"`
	APIKey         string              `json:"apiKey"`
	Enabled        *bool               `json:"enabled"`
	AllowAll       bool                `json:"allowAll"`
	ProviderModels map[string][]string `json:"providerModels"`
	UpdateAPIKey   bool                `json:"updateApiKey"`
}

type modelRequest struct {
	ServiceID          uint64  `json:"serviceId"`
	ModelKey           string  `json:"modelKey"`
	DisplayName        string  `json:"displayName"`
	ContextWindow      int     `json:"contextWindow"`
	MaxOutputTokens    int     `json:"maxOutputTokens"`
	SupportsTools      *bool   `json:"supportsTools"`
	SupportsVision     bool    `json:"supportsVision"`
	SupportsReasoning  bool    `json:"supportsReasoning"`
	InputPricePerMTok  float64 `json:"inputPricePerMtok"`
	OutputPricePerMTok float64 `json:"outputPricePerMtok"`
	Enabled            *bool   `json:"enabled"`
}

type providerModelView struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
}

func (s *AgentHTTP) ListModelServices(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		catalog, admin, err := s.models.Catalog(ctx, extractUserIDFromContext(ctx))
		if err != nil {
			return err
		}
		out := make([]ModelServiceView, 0, len(catalog))
		for _, item := range catalog {
			out = append(out, modelServiceView(item))
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{"services": out, "canManage": admin})
		return nil
	})
}

func (s *AgentHTTP) ListGatewayModels(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		rows, err := s.models.AvailableProviderModels(ctx, extractUserIDFromContext(ctx))
		if err != nil {
			return err
		}
		out := make([]providerModelView, 0, len(rows))
		for _, row := range rows {
			out = append(out, providerModelView{Name: row.Name, Provider: row.Provider})
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{"models": out})
		return nil
	})
}

func (s *AgentHTTP) CreateModelService(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		var input modelServiceRequest
		if err := decodeJSONBody(r, &input); err != nil {
			return err
		}
		row, err := s.models.Create(ctx, extractUserIDFromContext(ctx), input.spec())
		if err != nil {
			return err
		}
		models, _, err := s.models.ListModels(ctx, extractUserIDFromContext(ctx), row.ID)
		if err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusCreated, modelServiceView(llm.CatalogService{Service: *row, Models: models}))
		return nil
	})
}

func (s *AgentHTTP) UpdateModelService(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		id, err := parseModelID(c)
		if err != nil {
			return err
		}
		var input modelServiceRequest
		if err := decodeJSONBody(r, &input); err != nil {
			return err
		}
		row, err := s.models.Update(ctx, extractUserIDFromContext(ctx), id, input.spec())
		if err != nil {
			return err
		}
		models, _, err := s.models.ListModels(ctx, extractUserIDFromContext(ctx), row.ID)
		if err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, modelServiceView(llm.CatalogService{Service: *row, Models: models}))
		return nil
	})
}

func (s *AgentHTTP) DeleteModelService(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		id, err := parseModelID(c)
		if err != nil {
			return err
		}
		if err := s.models.Delete(ctx, extractUserIDFromContext(ctx), id); err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		return nil
	})
}

func (s *AgentHTTP) SyncModelService(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		id, err := parseModelID(c)
		if err != nil {
			return err
		}
		if err := s.models.SyncModels(ctx, extractUserIDFromContext(ctx), id); err != nil {
			return err
		}
		models, _, err := s.models.ListModels(ctx, extractUserIDFromContext(ctx), id)
		if err != nil {
			return err
		}
		out := make([]ModelView, 0, len(models))
		for i := range models {
			out = append(out, modelView(&models[i]))
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{"models": out})
		return nil
	})
}

func (s *AgentHTTP) ListModels(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		id, err := parseModelID(c)
		if err != nil {
			return err
		}
		rows, _, err := s.models.ListModels(ctx, extractUserIDFromContext(ctx), id)
		if err != nil {
			return err
		}
		out := make([]ModelView, 0, len(rows))
		for i := range rows {
			out = append(out, modelView(&rows[i]))
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{"models": out})
		return nil
	})
}

func (s *AgentHTTP) CreateModel(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		serviceID, err := parseModelID(c)
		if err != nil {
			return err
		}
		var input modelRequest
		if err := decodeJSONBody(r, &input); err != nil {
			return err
		}
		input.ServiceID = serviceID
		row, err := s.models.CreateModel(ctx, extractUserIDFromContext(ctx), input.spec())
		if err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusCreated, modelView(row))
		return nil
	})
}

func (s *AgentHTTP) UpdateModel(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		id, err := parseModelModelID(c)
		if err != nil {
			return err
		}
		var input modelRequest
		if err := decodeJSONBody(r, &input); err != nil {
			return err
		}
		row, err := s.models.UpdateModel(ctx, extractUserIDFromContext(ctx), id, input.spec())
		if err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, modelView(row))
		return nil
	})
}

func (s *AgentHTTP) DeleteModel(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		id, err := parseModelModelID(c)
		if err != nil {
			return err
		}
		if err := s.models.DeleteModel(ctx, extractUserIDFromContext(ctx), id); err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		return nil
	})
}

func parseModelID(c khttp.Context) (uint64, error) {
	id, err := strconv.ParseUint(c.Vars().Get("id"), 10, 64)
	if err != nil || id == 0 {
		return 0, invalidAgentInput("invalid model service id")
	}
	return id, nil
}

func parseModelModelID(c khttp.Context) (uint64, error) {
	id, err := strconv.ParseUint(c.Vars().Get("id"), 10, 64)
	if err != nil || id == 0 {
		return 0, invalidAgentInput("invalid model id")
	}
	return id, nil
}

func (r modelServiceRequest) spec() domain.ModelServiceSpec {
	return domain.ModelServiceSpec{
		Name: r.Name, DisplayName: r.DisplayName, GatewayType: r.GatewayType,
		BaseURL: r.BaseURL, APIKey: r.APIKey, UpdateAPIKey: r.UpdateAPIKey,
		Enabled: r.Enabled, ProviderModels: r.ProviderModels, AllowAll: r.AllowAll,
	}
}

func (r modelRequest) spec() domain.ModelSpec {
	supportsTools := true
	if r.SupportsTools != nil {
		supportsTools = *r.SupportsTools
	}
	enabled := true
	if r.Enabled != nil {
		enabled = *r.Enabled
	}
	return domain.ModelSpec{
		ServiceID: r.ServiceID, ModelKey: r.ModelKey, DisplayName: r.DisplayName,
		ContextWindow: r.ContextWindow, MaxOutputTokens: r.MaxOutputTokens,
		SupportsTools: supportsTools, SupportsVision: r.SupportsVision, SupportsReasoning: r.SupportsReasoning,
		InputPricePerMTok: r.InputPricePerMTok, OutputPricePerMTok: r.OutputPricePerMTok,
		Enabled: enabled,
	}
}

func modelServiceView(item llm.CatalogService) ModelServiceView {
	models := make([]ModelView, 0, len(item.Models))
	for i := range item.Models {
		models = append(models, modelView(&item.Models[i]))
	}
	return ModelServiceView{
		ID: item.Service.ID, Name: item.Service.Name, DisplayName: item.Service.DisplayName,
		GatewayType: item.Service.GatewayType, BaseURL: item.Service.BaseURL,
		GatewayKeyID: item.Service.GatewayKeyID, Enabled: item.Service.Enabled,
		HasAPIKey: item.Service.APIKeyEnc != "", Models: models,
	}
}

func modelView(row *domain.Model) ModelView {
	return ModelView{
		ID: row.ID, ServiceID: row.ServiceID, ModelKey: row.ModelKey, DisplayName: row.DisplayName,
		ContextWindow: row.ContextWindow, MaxOutputTokens: row.MaxOutputTokens,
		SupportsTools: row.SupportsTools, SupportsVision: row.SupportsVision,
		SupportsReasoning: row.SupportsReasoning, InputPricePerMTok: row.InputPricePerMTok,
		OutputPricePerMTok: row.OutputPricePerMTok, Enabled: row.Enabled,
	}
}
