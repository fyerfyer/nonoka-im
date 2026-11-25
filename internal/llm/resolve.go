package llm

import (
	"context"
	"fmt"
	"strings"

	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/agent/port"
)

// ResolveForRun returns the runtime endpoint for a published agent run or a
// playground test run. It fails closed when the model is not curated/enabled or
// when the required virtual key is unavailable.
func (s *Service) ResolveForRun(ctx context.Context, agentID, modelID int64, test bool) (port.ModelEndpoint, error) {
	model, err := s.Lookup(ctx, modelID)
	if err != nil {
		return port.ModelEndpoint{}, err
	}
	if !model.Enabled {
		return port.ModelEndpoint{}, fmt.Errorf("%w: model %q is disabled", domain.ErrModelDisabled, model.ModelKey)
	}
	if !model.Curated {
		return port.ModelEndpoint{}, fmt.Errorf("%w: model %q is not curated", domain.ErrModelNotCurated, model.ModelKey)
	}
	var key string
	if test {
		key, err = s.playgroundKeyValue(ctx)
	} else {
		key, err = s.agentKeyValue(ctx, agentID)
	}
	if err != nil {
		return port.ModelEndpoint{}, err
	}
	baseURL := strings.TrimRight(s.Config.BaseURL, "/")
	if baseURL == "" {
		return port.ModelEndpoint{}, fmt.Errorf("%w: Bifrost URL is not configured", ErrInvalidInput)
	}
	if !strings.HasSuffix(baseURL, "/v1") {
		baseURL += "/v1"
	}
	return port.ModelEndpoint{
		BaseURL: baseURL, VirtualKey: key, Model: model.ModelKey, ModelKey: model.ModelKey,
		ModelID: model.ID, Provider: model.Provider, MaxTokens: model.MaxOutputTokens, ContextWindow: model.ContextWindow,
		SupportsVision: model.SupportsVision, InputPricePerMTok: model.InputPricePerMTok,
		OutputPricePerMTok: model.OutputPricePerMTok,
	}, nil
}
