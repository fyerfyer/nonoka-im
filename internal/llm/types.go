package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

var (
	ErrGatewayUnavailable = errors.New("llm gateway unavailable")
	ErrGatewayRejected    = errors.New("llm gateway rejected the request")
)

// Auth configures Bifrost management API authentication. SetupToken is only
// used for the one-time bootstrap of the first admin account; runtime
// management should use AdminToken or admin username/password.
type Auth struct {
	AdminToken string
	SetupToken string
	Username   string
	Password   string
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
	Architecture    struct {
		InputModalities  []string `json:"input_modalities"`
		OutputModalities []string `json:"output_modalities"`
	} `json:"architecture"`
	SupportedParameters []string `json:"supported_parameters"`
	Pricing             struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
		CacheRead  string `json:"input_cache_read"`
		CacheWrite string `json:"input_cache_write"`
	} `json:"pricing"`
}

// BudgetSpec is one Bifrost budget window.
type BudgetSpec struct {
	MaxLimit      float64
	ResetDuration string
}

// RateLimitSpec is Bifrost's request/token rate limit for one owner.
type RateLimitSpec struct {
	TokenMaxLimit        int
	TokenResetDuration   string
	RequestMaxLimit      int
	RequestResetDuration string
}

// VirtualKeySpec describes the gateway-side governance entity for one agent or
// the shared playground sandbox.
// Hash returns a stable fingerprint of the key's governance settings. The
// backfill loop uses it to avoid rewriting unchanged keys on every tick.
func (s VirtualKeySpec) Hash() (string, error) {
	payload, err := json.Marshal(struct {
		Name           string
		Description    string
		ProviderModels map[string][]string
		AllowAll       bool
		DenyAll        bool
		CustomerID     string
		Metadata       map[string]string
		Budgets        []BudgetSpec
		ClearBudgets   bool
		RateLimit      *RateLimitSpec
		ClearRateLimit bool
	}{
		s.Name, s.Description, s.ProviderModels, s.AllowAll, s.DenyAll, s.CustomerID,
		s.Metadata, s.Budgets, s.ClearBudgets, s.RateLimit, s.ClearRateLimit,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

type VirtualKeySpec struct {
	Name           string
	Description    string
	ProviderModels map[string][]string
	AllowAll       bool
	DenyAll        bool
	CustomerID     string
	Metadata       map[string]string
	Budgets        []BudgetSpec
	ClearBudgets   bool
	RateLimit      *RateLimitSpec
	ClearRateLimit bool
}

// VirtualKey is the one-time creation response. Value is never returned by the
// gateway again.
type VirtualKey struct {
	ID    string
	Value string
}

// VirtualKeyInfo is the non-secret view used for diagnostics.
type VirtualKeyInfo struct {
	ID       string
	Name     string
	IsActive bool
}

// CustomerSpec defines the deployment-level Bifrost customer budget.
type CustomerSpec struct {
	Name     string
	Budgets  []BudgetSpec
	Metadata map[string]string
}

type gatewayError struct {
	Type    string
	Status  int
	Message string
}

var _ error = *new(gatewayError)

func (e gatewayError) Error() string {
	if e.Type != "" {
		return fmt.Sprintf("bifrost: %s (status %d): %s", e.Type, e.Status, e.Message)
	}
	return fmt.Sprintf("bifrost: status %d: %s", e.Status, e.Message)
}
