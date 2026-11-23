package llm

import (
	"errors"
	"fmt"
)

var (
	ErrGatewayUnavailable = errors.New("llm gateway unavailable")
	ErrGatewayRejected    = errors.New("llm gateway rejected the request")
)

// Auth configures Bifrost management API authentication. SetupToken is the OSS
// bootstrap header used while dashboard auth is not configured; AdminToken or
// admin username/password is the production path.
type Auth struct {
	AdminToken string
	SetupToken string
	Username   string
	Password   string
}

// ProviderModel is one model returned by the Bifrost management catalog.
type ProviderModel struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
}

// GatewayModel is one model returned by the OpenAI-compatible /v1/models
// endpoint. Prices are per token in the gateway payload.
type GatewayModel struct {
	ID              string `json:"id"`
	NormalizedName  string `json:"normalized_name"`
	ContextLength   int    `json:"context_length"`
	MaxInputTokens  int    `json:"max_input_tokens"`
	MaxOutputTokens int    `json:"max_output_tokens"`
	OwnedBy         string `json:"owned_by"`
	Pricing         struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
	} `json:"pricing"`
}

// VirtualKeySpec describes the gateway-side governance entity for one platform
// ModelService. An empty ProviderModels map with AllowAll true grants every
// configured provider; otherwise the key is deny-by-default for providers it
// does not list.
type VirtualKeySpec struct {
	Name           string
	Description    string
	ProviderModels map[string][]string
	AllowAll       bool
}

// VirtualKey is the one-time creation response. Value is never returned by the
// gateway again.
type VirtualKey struct {
	ID    string
	Value string
}

type gatewayError struct {
	Status  int
	Message string
}

var _ error = *new(gatewayError)

func (e gatewayError) Error() string {
	return fmt.Sprintf("bifrost: status %d: %s", e.Status, e.Message)
}
