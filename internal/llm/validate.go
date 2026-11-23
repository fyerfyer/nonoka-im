package llm

import (
	"fmt"
	"strings"

	"nonoka-im/internal/agent/domain"
)

func normalizeSpec(spec *domain.ModelServiceSpec) error {
	spec.Name = strings.TrimSpace(spec.Name)
	spec.DisplayName = strings.TrimSpace(spec.DisplayName)
	spec.GatewayType = strings.TrimSpace(spec.GatewayType)
	spec.BaseURL = strings.TrimRight(strings.TrimSpace(spec.BaseURL), "/")
	if spec.GatewayType == "" {
		spec.GatewayType = "bifrost"
	}
	if spec.DisplayName == "" {
		spec.DisplayName = spec.Name
	}
	if spec.Name == "" || len(spec.Name) > 100 {
		return fmt.Errorf("%w: name is required (max 100 chars)", ErrInvalidInput)
	}
	if len(spec.DisplayName) > 200 {
		return fmt.Errorf("%w: display name is too long", ErrInvalidInput)
	}
	if spec.BaseURL != "" && !strings.HasPrefix(spec.BaseURL, "http://") && !strings.HasPrefix(spec.BaseURL, "https://") {
		return fmt.Errorf("%w: base_url must be http(s)", ErrInvalidInput)
	}
	if len(spec.APIKey) > 4096 {
		return fmt.Errorf("%w: api_key is too long", ErrInvalidInput)
	}
	return nil
}

func normalizeModelSpec(spec *domain.ModelSpec) error {
	spec.ModelKey = strings.TrimSpace(spec.ModelKey)
	spec.DisplayName = strings.TrimSpace(spec.DisplayName)
	if spec.DisplayName == "" {
		spec.DisplayName = spec.ModelKey
	}
	if spec.ServiceID == 0 {
		return fmt.Errorf("%w: service_id is required", ErrInvalidInput)
	}
	if spec.ModelKey == "" || len(spec.ModelKey) > 200 {
		return fmt.Errorf("%w: model_key is required (max 200 chars)", ErrInvalidInput)
	}
	if len(spec.DisplayName) > 200 {
		return fmt.Errorf("%w: display name is too long", ErrInvalidInput)
	}
	if spec.MaxOutputTokens <= 0 {
		spec.MaxOutputTokens = 4096
	}
	if spec.MaxOutputTokens > 1_000_000 {
		return fmt.Errorf("%w: max_output_tokens is too large", ErrInvalidInput)
	}
	if spec.ContextWindow < 0 {
		return fmt.Errorf("%w: context_window must be positive", ErrInvalidInput)
	}
	return nil
}
